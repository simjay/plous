package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
)

const (
	maxConcurrentMCPCalls = 16
	mcpWriteTimeout       = 15 * time.Second
)

// ServeStdio adapts newline-delimited MCP to the same protocol handler as the
// private native bridge. On disconnect it stops host services before waiting
// for pending tools to save their evidence.
func ServeStdio(ctx context.Context, input io.ReadCloser, output io.Writer, tools agent.Tools, id, version, instructions string, onDisconnect func()) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)
	toolCtx, stopTools := context.WithCancelCause(ctx)
	defer stopTools(context.Canceled)
	b := &Bridge{ctx: toolCtx, cancel: stopTools, tools: tools, id: id, seen: map[string]bool{}, Version: version, Instructions: instructions}
	stop := context.AfterFunc(ctx, func() {
		_ = input.Close()
		if closer, ok := output.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	defer stop()
	var writes sync.Mutex
	var workers sync.WaitGroup
	slots := make(chan struct{}, maxConcurrentMCPCalls)
	var writeErr error
	write := func(body []byte) {
		writes.Lock()
		defer writes.Unlock()
		if ctx.Err() != nil {
			return
		}
		var err error
		if stream, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
			err = stream.SetWriteDeadline(time.Now().Add(mcpWriteTimeout))
			if errors.Is(err, os.ErrNoDeadline) {
				err = nil
			}
		}
		if err == nil {
			frame := append(bytes.TrimSpace(body), '\n')
			var written int
			written, err = output.Write(frame)
			if err == nil && written != len(frame) {
				err = io.ErrShortWrite
			}
		}
		if err != nil && ctx.Err() == nil {
			writeErr = fmt.Errorf("write MCP response: %w", err)
			cancel(writeErr)
		}
	}
	replyError := func(id json.RawMessage, code int, message string) {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
		write(body)
	}
	prompts := &stdioPrompts{send: write}
	process := func(raw []byte) {
		response := &mcpCapture{header: make(http.Header)}
		requestContext := context.WithValue(ctx, confirmationKey{}, prompts)
		r, _ := http.NewRequestWithContext(requestContext, http.MethodPost, "http://127.0.0.1/mcp", bytes.NewReader(raw))
		r.Header.Set("Authorization", b.Authorization())
		b.serveRPC(response, r, true)
		if response.status == http.StatusAccepted {
			return
		}
		body := response.body.Bytes()
		if !json.Valid(body) {
			replyError(nil, -32600, string(body))
			return
		}
		write(body)
	}
	dispatch := func(raw []byte) {
		var request stdioPacket
		if json.Unmarshal(raw, &request) == nil && prompts.receive(request) {
			return
		}
		if request.Method != "tools/call" {
			process(raw)
			return
		}
		select {
		case slots <- struct{}{}:
			frame := bytes.Clone(raw)
			workers.Add(1)
			go func() { defer workers.Done(); defer func() { <-slots }(); process(frame) }()
		default:
			// Keep reading so cancellations can reach calls occupying the slots.
			if canonicalID(request.ID) != "" {
				replyError(request.ID, -32001, "Plous has too many in-flight tool requests. This call was not dispatched.")
			} else {
				process(raw)
			}
		}
	}
	reader := bufio.NewReaderSize(input, maxMCPRequestBytes+2)
	var readErr error
	for ctx.Err() == nil {
		raw, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			for errors.Is(err, bufio.ErrBufferFull) && ctx.Err() == nil {
				_, err = reader.ReadSlice('\n')
			}
			replyError(nil, -32600, "MCP request exceeds the size limit.")
		} else if len(raw) > 0 {
			raw = bytes.TrimSuffix(bytes.TrimSuffix(raw, []byte("\n")), []byte("\r"))
			if len(raw) > maxMCPRequestBytes {
				replyError(nil, -32600, "MCP request exceeds the size limit.")
			} else {
				dispatch(raw)
			}
		}
		if err != nil {
			readErr = err
			break
		}
	}
	// EOF means the client has disconnected. Stop remote work, but still wait
	// for its independent cleanup and evidence commits before closing storage.
	cancel(context.Canceled)
	if onDisconnect != nil {
		onDisconnect()
	}
	workers.Wait()
	if writeErr != nil {
		return writeErr
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrClosedPipe) && !errors.Is(readErr, context.Canceled) && !errors.Is(readErr, os.ErrClosed) {
		return fmt.Errorf("read MCP request: %w", readErr)
	}
	return nil
}

type mcpCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *mcpCapture) Header() http.Header    { return r.header }
func (r *mcpCapture) WriteHeader(status int) { r.status = status }
func (r *mcpCapture) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/simjay/plous/internal/ai/agent"
)

// ServeStdio adapts newline-delimited MCP to the same protocol handler as the
// private native bridge. No listener or bearer credential is created.
func ServeStdio(ctx context.Context, input io.ReadCloser, output io.Writer, tools agent.Tools, id, version, instructions string) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)
	b := &Bridge{ctx: ctx, cancel: cancel, tools: tools, id: id, seen: map[string]bool{}, Version: version, Instructions: instructions}
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	var writes sync.Mutex
	var workers sync.WaitGroup
	slots := make(chan struct{}, 16)
	var writeErr error
	process := func(raw []byte) {
		response := &mcpCapture{header: make(http.Header)}
		r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1/mcp", bytes.NewReader(raw))
		r.Header.Set("Authorization", b.Authorization())
		b.ServeHTTP(response, r)
		if response.status == http.StatusAccepted {
			return
		}
		body := response.body.Bytes()
		if !json.Valid(body) {
			body, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32600, "message": string(body)}})
		}
		writes.Lock()
		defer writes.Unlock()
		if writeErr == nil {
			_, writeErr = output.Write(append(bytes.TrimSpace(body), '\n'))
			if writeErr != nil {
				cancel(writeErr)
			}
		}
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(raw, &request)
		if request.Method != "tools/call" {
			process(raw)
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
		workers.Add(1)
		go func() { defer workers.Done(); defer func() { <-slots }(); process(raw) }()
	}
	// EOF means the client has disconnected. Stop remote work, but still wait
	// for its independent cleanup and evidence commits before closing storage.
	cancel(context.Canceled)
	workers.Wait()
	if writeErr != nil {
		return writeErr
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		return err
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

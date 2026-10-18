package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
)

// Bridge serves one already-authorized tool session over loopback MCP.
// The caller owns its lifetime; this does not create tool authority.
type Bridge struct {
	URL, token string
	server     *http.Server
	ctx        context.Context
	tools      agent.Tools
	id         string
	cancel     context.CancelCauseFunc
	mu         sync.Mutex
	seen       map[string]bool
	calls      int
}

// OpenBridge starts the same session-bound transport used by native terminals.
func OpenBridge(ctx context.Context, tools agent.Tools, id string) (*Bridge, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local Plous tools: %w", err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		listener.Close()
		return nil, err
	}
	// Losing tool authority must not close the CLI or discard its native draft.
	ctx, cancel := context.WithCancelCause(ctx)
	b := &Bridge{URL: "http://" + listener.Addr().String() + "/mcp", token: hex.EncodeToString(secret), ctx: ctx, tools: tools, id: id, cancel: cancel, seen: map[string]bool{}}
	b.server = &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
	go func() { _ = b.server.Serve(listener) }()
	return b, nil
}
func (b *Bridge) Authorization() string { return "Bearer " + b.token }

func (b *Bridge) Close() { b.cancel(context.Canceled); _ = b.server.Close() }
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.token)) != 1 || r.Header.Get("Origin") != "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if b.ctx.Err() != nil {
		http.Error(w, "session closed", http.StatusGone)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if d.Decode(&req) != nil || req.JSONRPC != "2.0" {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "expected one request", http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 {
		if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
			w.WriteHeader(http.StatusAccepted)
		} else {
			http.Error(w, "request identity required", http.StatusBadRequest)
		}
		return
	}
	var identity any
	idDecoder := json.NewDecoder(bytes.NewReader(req.ID))
	idDecoder.UseNumber()
	_ = idDecoder.Decode(&identity)
	var callID string
	switch id := identity.(type) {
	case string:
		callID = "string:" + id
	case json.Number:
		n, err := strconv.ParseInt(string(id), 10, 64)
		if err == nil {
			callID = "number:" + strconv.FormatInt(n, 10)
		}
	}
	if callID == "" {
		http.Error(w, "request identity must be a string or integer", http.StatusBadRequest)
		return
	}
	var result any
	var rpcErr any
	switch req.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "plous", "version": "1"}}
	case "ping":
		result = map[string]any{}
	case "tools/list":
		list := []any{}
		for _, t := range b.tools.Definitions() {
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Parameters, "annotations": map[string]bool{"readOnlyHint": true, "destructiveHint": false}})
		}
		result = map[string]any{"tools": list}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil || p.Name == "" {
			rpcErr = map[string]any{"code": -32602, "message": "invalid tool call"}
			break
		}
		b.mu.Lock()
		if b.seen[callID] {
			b.mu.Unlock()
			rpcErr = map[string]any{"code": -32600, "message": "tool request identity already used"}
			break
		}
		b.seen[callID] = true
		b.calls++
		n := b.calls
		b.mu.Unlock()
		ctx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(b.ctx, cancel)
		body, err := b.tools.Call(ctx, agent.ToolCall{ID: fmt.Sprintf("%s/cli-tool-%d", b.id, n), Name: p.Name, Args: p.Arguments}, 65536)
		stop()
		cancel()
		if err == nil {
			err = b.ctx.Err()
		}
		if err != nil {
			b.cancel(err)
			rpcErr = map[string]any{"code": -32000, "message": "Plous tool access stopped after a tool failure. The terminal is still running. Close and reopen the native session to reconnect tools."}
			break
		}
		result = map[string]any{"content": []any{map[string]string{"type": "text", "text": string(body)}}, "isError": false}
	default:
		rpcErr = map[string]any{"code": -32601, "message": "method not available"}
	}
	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rpcErr != nil {
		response["error"] = rpcErr
	} else {
		response["result"] = result
	}
	_ = json.NewEncoder(w).Encode(response)
}

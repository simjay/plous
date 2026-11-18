package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
)

type cancellableTools struct{ started chan struct{} }

func (f *cancellableTools) Definitions() []agent.Tool {
	return []agent.Tool{{Name: "run", Mutating: true, Parameters: json.RawMessage(`{"type":"object"}`)}}
}
func (f *cancellableTools) Call(ctx context.Context, call agent.ToolCall, _ int) (json.RawMessage, error) {
	close(f.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestStdioSharesDiscoveryAndCancelsInFlightCalls(t *testing.T) {
	input, send := io.Pipe()
	receive, output := io.Pipe()
	tools := &cancellableTools{started: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- ServeStdio(t.Context(), input, output, tools, "session", "0.0.1", "captured staging", nil)
		output.Close()
	}()
	responses := make(chan map[string]any, 4)
	go func() {
		scanner := bufio.NewScanner(receive)
		for scanner.Scan() {
			var response map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &response)
			responses <- response
		}
		close(responses)
	}()
	write := func(s string) {
		t.Helper()
		if _, err := io.WriteString(send, s+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	next := func() map[string]any {
		t.Helper()
		select {
		case r := <-responses:
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("no MCP response")
			return nil
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	init := next()["result"].(map[string]any)
	if init["instructions"] != "captured staging" || init["serverInfo"].(map[string]any)["version"] != "0.0.1" {
		t.Fatal(init)
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	write(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	list := next()["result"].(map[string]any)["tools"].([]any)
	annotations := list[0].(map[string]any)["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != true {
		t.Fatal("active tool advertised as read-only", annotations)
	}
	write(`{"jsonrpc":"2.0","id":"active","method":"tools/call","params":{"name":"run","arguments":{}}}`)
	select {
	case <-tools.started:
	case <-time.After(5 * time.Second):
		t.Fatal("call did not start")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"active"}}`)
	if r := next(); r["id"] != "active" || r["error"] == nil {
		t.Fatal("cancelled call succeeded", r)
	}
	write(`{"jsonrpc":"2.0","id":"later","method":"tools/call","params":{"name":"run","arguments":{}}}`)
	if r := next(); r["id"] != "later" || r["error"] == nil || r["error"].(map[string]any)["message"] != toolAccessStopped {
		t.Fatal("revoked tool did not return a correlated error", r)
	}
	write(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`)
	if r := next(); r["id"] != "ping" || r["result"] == nil {
		t.Fatal("tool revocation closed the transport", r)
	}
	send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after the client disconnected")
	}
	receive.Close()
}

type blockingTools struct{ started chan struct{} }

func (f blockingTools) Definitions() []agent.Tool { return nil }

func (f blockingTools) Call(ctx context.Context, _ agent.ToolCall, _ int) (json.RawMessage, error) {
	f.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestStdioSaturationStillReadsCancellation(t *testing.T) {
	input, send := io.Pipe()
	output, receive := io.Pipe()
	t.Cleanup(func() { send.Close(); output.Close() })
	tools := blockingTools{started: make(chan struct{}, maxConcurrentMCPCalls)}
	done := make(chan error, 1)
	go func() {
		done <- ServeStdio(t.Context(), input, receive, tools, "session", "0.0.1", "", nil)
	}()
	for id := range maxConcurrentMCPCalls + 1 {
		if _, err := fmt.Fprintf(send, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"run","arguments":{}}}`+"\n", id); err != nil {
			t.Fatal(err)
		}
	}
	responses := make(chan map[string]any, maxConcurrentMCPCalls+2)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			var response map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &response)
			responses <- response
		}
	}()
	select {
	case r := <-responses:
		rpcErr, _ := r["error"].(map[string]any)
		if r["id"] != float64(maxConcurrentMCPCalls) || rpcErr["code"] != float64(-32001) {
			t.Fatal("saturated server did not reject the excess call", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("saturated server blocked its request reader")
	}
	for range maxConcurrentMCPCalls {
		select {
		case <-tools.started:
		case <-time.After(5 * time.Second):
			t.Fatal("tool request did not start")
		}
	}
	if _, err := io.WriteString(send, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":0}}`+"\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-responses:
		if response["error"] == nil {
			t.Fatal("cancelled request succeeded", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not reach a saturated server")
	}
	send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}

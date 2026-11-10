package cli

import (
	"bufio"
	"context"
	"encoding/json"
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
		done <- ServeStdio(t.Context(), input, output, tools, "session", "0.0.1", "captured staging")
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
	if r := next(); r["error"] == nil {
		t.Fatal("cancelled call succeeded", r)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after revoking tools")
	}
	send.Close()
	receive.Close()
}

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	agent "github.com/simjay/plous/internal/ai"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeProviderWireDirectRead(t *testing.T) {
	for _, provider := range aimodel.Providers() {
		t.Run(provider, func(t *testing.T) {
			o, _ := workspaceFixture(t)
			nativeOptions(t, &o)
			scope, reads := scopedKubernetesFixture(t)
			o.Connections = []agent.Connection{*scope}
			o.Config.Provider, o.Config.Model = provider, aimodel.Models(provider)[0].ID
			o.Config.KeyEnv = ""
			o.Key = "wire-provider-key"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if provider == "openai" && r.Header.Get("Authorization") != "Bearer wire-provider-key" || provider == "anthropic" && r.Header.Get("X-Api-Key") != "wire-provider-key" {
					t.Error("captured credential missing")
				}
				if request["model"] != o.Config.Model {
					t.Error("selected model changed")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch calls.Add(1) {
				case 1:
					raw, _ := json.Marshal(request["tools"])
					for _, name := range []string{"history_read", "kubernetes_resources_read", "purpose", "connection_id", "resource"} {
						if !strings.Contains(string(raw), name) {
							t.Error("missing full main tool schema", name)
						}
					}
					if strings.Contains(string(raw), `"name":"recall"`) || strings.Contains(string(raw), `"name":"investigate"`) || strings.Contains(string(raw), `"bash"`) {
						t.Error("retired investigator exposed")
					}
					writeNamedWireToolCall(w, provider, "kubernetes_resources_read", "read-1", `{"purpose":"Read checkout phase","connection_id":"captured","operation":"get","resource":"pods","name":"checkout","namespace":"payments"}`)
				case 2:
					raw, _ := json.Marshal(request["messages"])
					if !strings.Contains(string(raw), "Running") {
						t.Error("direct result missing")
					}
					for _, secret := range []string{"fixture-kube-token", "private-payload", "wire-provider-key"} {
						if strings.Contains(string(raw), secret) {
							t.Error("secret in prompt")
						}
					}
					writeWireAnswer(w, provider, "Checkout is running.")
				default:
					t.Error("unexpected model call")
					writeWireAnswer(w, provider, "Stop.")
				}
			}))
			defer server.Close()
			o.Config.Endpoint = server.URL
			r, err := agent.Run(context.Background(), o)
			if err != nil || r.Text != "Checkout is running." || calls.Load() != 2 || reads.Load() != 2 {
				t.Fatalf("%+v %v calls=%d reads=%d", r, err, calls.Load(), reads.Load())
			}
		})
	}
}
func writeWireEvent(w http.ResponseWriter, event string, value any) {
	data, _ := json.Marshal(value)
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	w.(http.Flusher).Flush()
}

func writeOpenAIWireChunk(w http.ResponseWriter, delta any, finish any) {
	writeWireEvent(w, "", map[string]any{"id": "reply", "object": "chat.completion.chunk", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
}

func startAnthropicWireMessage(w http.ResponseWriter) {
	writeWireEvent(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg", "type": "message", "role": "assistant", "model": "fixture", "content": []any{}, "usage": map[string]any{"input_tokens": 4, "output_tokens": 0}}})
}

func finishAnthropicWireMessage(w http.ResponseWriter, reason string) {
	writeWireEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	writeWireEvent(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason}, "usage": map[string]any{"output_tokens": 3}})
	writeWireEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}

func writeNamedWireToolCall(w http.ResponseWriter, provider, name, id, arguments string) {
	parts := []string{arguments[:len(arguments)/2], arguments[len(arguments)/2:]}
	if provider == "openai" {
		writeOpenAIWireChunk(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": parts[0]}}}}, nil)
		writeOpenAIWireChunk(w, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": parts[1]}}}}, nil)
		writeOpenAIWireChunk(w, map[string]any{}, "tool_calls")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	startAnthropicWireMessage(w)
	writeWireEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
	for _, part := range parts {
		writeWireEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": part}})
	}
	finishAnthropicWireMessage(w, "tool_use")
}

func writeWireAnswer(w http.ResponseWriter, provider, text string) {
	if provider == "openai" {
		writeOpenAIWireChunk(w, map[string]any{"role": "assistant", "content": text}, nil)
		writeOpenAIWireChunk(w, map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	startAnthropicWireMessage(w)
	writeWireEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	writeWireEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
	finishAnthropicWireMessage(w, "end_turn")
}

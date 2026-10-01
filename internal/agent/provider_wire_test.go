package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/memory"
)

// Exercise the actual provider adapters and Eino tool loop rather than replacing
// the model interface. Splitting JSON arguments also verifies stream assembly.
func TestAgentProviderWireKubernetesReadAndDeniedCommands(t *testing.T) {
	for _, provider := range ai.Providers() {
		t.Run(provider, func(t *testing.T) {
			o, store := fixture(t)
			scope, kubeRequests := scopedKubernetesFixture(t)
			o.Kubernetes = scope
			o.Config.Provider, o.Config.Model = provider, ai.Models(provider)[0].ID
			o.Config.KeyEnv = ""
			o.Key = "wire-provider-key"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				index := int(calls.Add(1)) - 1
				if r.Method != http.MethodPost || provider == "openai" && !strings.HasSuffix(r.URL.Path, "/chat/completions") || provider == "anthropic" && !strings.HasSuffix(r.URL.Path, "/messages") {
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL)
				}
				if provider == "openai" && r.Header.Get("Authorization") != "Bearer wire-provider-key" || provider == "anthropic" && r.Header.Get("X-Api-Key") != "wire-provider-key" {
					t.Error("provider did not use the captured credential")
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, "invalid JSON", http.StatusBadRequest)
					return
				}
				if request["model"] != o.Config.Model || request["stream"] != true {
					t.Errorf("model or stream changed between calls: %v", request)
				}
				if index == 0 {
					assertReadOnlyWireTool(t, provider, request)
				} else {
					body := wireToolResult(t, provider, request)
					if strings.Contains(body, "fixture-kube-token") || strings.Contains(body, "private-payload") || strings.Contains(body, "wire-provider-key") {
						t.Error("credentials reached provider tool context")
					}
					switch index {
					case 1:
						if !strings.Contains(body, `"namespace":"payments"`) || !strings.Contains(body, `"phase":"Running"`) || !strings.Contains(body, `"evidence":`) {
							t.Errorf("approved read did not return scoped evidence: %s", body)
						}
					case 2:
						if !strings.Contains(body, "only read operations") {
							t.Errorf("write was not denied by policy: %s", body)
						}
					case 3:
						if !strings.Contains(body, "unknown or malformed") {
							t.Errorf("unknown field was not denied by the strict decoder: %s", body)
						}
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				commands := []string{
					`{"operation":"get","resource":"pods","name":"checkout"}`,
					`{"operation":"apply","resource":"pods","name":"checkout"}`,
					`{"operation":"get","resource":"pods","raw":"/unscoped"}`,
				}
				if index < len(commands) {
					writeWireToolCall(w, provider, fmt.Sprintf("call-%d", index), commands[index])
				} else if index == len(commands) {
					writeWireAnswer(w, provider, "Read confirmed and writes denied.")
				} else {
					t.Error("unexpected extra model call")
					writeWireAnswer(w, provider, "Stop.")
				}
			}))
			defer server.Close()
			o.Config.Endpoint = server.URL
			var emitted string
			o.Emit = func(text string) error { emitted = text; return nil }
			result, err := Run(context.Background(), o)
			if err != nil || result.Text != "Read confirmed and writes denied." || emitted != result.Text {
				t.Fatalf("provider loop result=%+v err=%v emitted=%q", result, err, emitted)
			}
			if calls.Load() != 4 || kubeRequests.Load() != 2 {
				t.Fatalf("unexpected calls: provider=%d Kubernetes=%d", calls.Load(), kubeRequests.Load())
			}
			mem, err := memory.New(store, "", o.ConversationID)
			if err != nil {
				t.Fatal(err)
			}
			page, err := mem.Search(context.Background(), memory.Query{Source: "reasoning", Text: `"status":"denied"`, Limit: 20})
			if err != nil || len(page.Records) != 2 {
				t.Fatalf("denials were not saved exactly once: %+v %v", page, err)
			}
		})
	}
}

func assertReadOnlyWireTool(t *testing.T, provider string, request map[string]any) {
	t.Helper()
	tools, ok := request["tools"].([]any)
	if !ok {
		t.Error("provider request omitted tool declarations")
		return
	}
	found := false
	for _, raw := range tools {
		entry := raw.(map[string]any)
		schemaKey := "input_schema"
		if provider == "openai" {
			entry = entry["function"].(map[string]any)
			schemaKey = "parameters"
		}
		if entry["name"] != "kubernetes_read" {
			continue
		}
		found = true
		definition := entry[schemaKey].(map[string]any)
		properties := definition["properties"].(map[string]any)
		operation := properties["operation"].(map[string]any)
		encoded, _ := json.Marshal(operation["enum"])
		if string(encoded) != `["get","describe","logs"]` {
			t.Errorf("provider exposed non-read operations: %s", encoded)
		}
	}
	if !found {
		t.Error("provider did not receive the Kubernetes tool")
	}
}

func wireToolResult(t *testing.T, provider string, request map[string]any) string {
	t.Helper()
	messages, ok := request["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Error("provider omitted message history")
		return ""
	}
	last := messages[len(messages)-1].(map[string]any)
	if provider == "openai" {
		if last["role"] != "tool" || last["tool_call_id"] == "" {
			t.Errorf("OpenAI tool result has no matching call: %v", last)
		}
		text, _ := last["content"].(string)
		return text
	}
	if last["role"] != "user" {
		t.Errorf("Anthropic tool result has wrong role: %v", last)
	}
	blocks, _ := last["content"].([]any)
	for _, raw := range blocks {
		block := raw.(map[string]any)
		if block["type"] != "tool_result" {
			continue
		}
		if block["tool_use_id"] == "" {
			t.Error("Anthropic tool result has no matching call")
		}
		if text, ok := block["content"].(string); ok {
			return text
		}
		if content, ok := block["content"].([]any); ok {
			var result strings.Builder
			for _, chunk := range content {
				if text, ok := chunk.(map[string]any)["text"].(string); ok {
					result.WriteString(text)
				}
			}
			return result.String()
		}
	}
	t.Error("Anthropic request omitted tool result")
	return ""
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

func writeWireToolCall(w http.ResponseWriter, provider, id, arguments string) {
	parts := []string{arguments[:len(arguments)/2], arguments[len(arguments)/2:]}
	if provider == "openai" {
		writeOpenAIWireChunk(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "kubernetes_read", "arguments": parts[0]}}}}, nil)
		writeOpenAIWireChunk(w, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": parts[1]}}}}, nil)
		writeOpenAIWireChunk(w, map[string]any{}, "tool_calls")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	startAnthropicWireMessage(w)
	writeWireEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": id, "name": "kubernetes_read", "input": map[string]any{}}})
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

func TestAgentProviderWireCancellationStopsKubernetesAndModelLoop(t *testing.T) {
	for _, provider := range ai.Providers() {
		t.Run(provider, func(t *testing.T) {
			o, _ := fixture(t)
			started := make(chan struct{})
			kubeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1" {
					fmt.Fprint(w, `{"resources":[{"name":"pods","kind":"Pod","namespaced":true,"verbs":["get"]}]}`)
					return
				}
				if r.URL.Path != "/api/v1/namespaces/payments/pods/checkout" {
					t.Errorf("read escaped scope: %s", r.URL)
				}
				fmt.Fprint(w, `{`)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer kubeServer.Close()
			path := filepath.Join(t.TempDir(), "kubeconfig")
			config := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: captured
contexts:
- name: captured
  context: {cluster: fixture, namespace: payments}
clusters:
- name: fixture
  cluster: {server: %q}
`, kubeServer.URL)
			if err := os.WriteFile(path, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			o.Kubernetes = &KubernetesScope{Path: path, Context: "captured", Namespace: "payments", ConnectionID: "fixture"}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) != 1 {
					t.Error("cancellation made another model request")
					http.Error(w, "unexpected request", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				writeWireToolCall(w, provider, "cancel-read", `{"operation":"get","resource":"pods","name":"checkout"}`)
			}))
			defer server.Close()
			o.Config.Provider, o.Config.Model, o.Config.Endpoint, o.Config.KeyEnv = provider, ai.Models(provider)[0].ID, server.URL, ""
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, err := Run(ctx, o)
				finished <- err
			}()
			select {
			case <-started:
			case err := <-finished:
				t.Fatalf("agent finished before Kubernetes read: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("agent did not start its Kubernetes read")
			}
			cancel()
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
					t.Fatalf("canceled loop continued: err=%v calls=%d", err, calls.Load())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation did not terminate the agent and Kubernetes read")
			}
		})
	}
}

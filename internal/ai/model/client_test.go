package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/eino/schema"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func chunk(w http.ResponseWriter, delta any, finish any) {
	b, _ := json.Marshal(map[string]any{"id": "reply", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
	fmt.Fprintf(w, "data: %s\n\n", b)
	w.(http.Flusher).Flush()
}
func TestProvidersStreamAndUseOnlyExplicitCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "UNRELATED_SECRET")
	t.Setenv("ANTHROPIC_API_KEY", "UNRELATED_SECRET")
	t.Setenv("PLOUS_TEST_KEY", "configured-secret")
	for _, provider := range Providers() {
		for _, auth := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/auth%v", provider, auth), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					key := r.Header.Get("Authorization") + r.Header.Get("X-Api-Key")
					if strings.Contains(key, "UNRELATED") || (auth && !strings.Contains(key, "configured-secret")) || (!auth && key != "") {
						t.Errorf("incorrect auth header: %q", key)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["model"] != Models(provider)[0].ID || body["stream"] != true {
						t.Errorf("incorrect request %v", body)
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if provider == "anthropic" && !auth {
						// The SDK accepts streaming proxies that omit the SSE content type.
						w.Header().Set("Content-Type", "application/octet-stream")
					}
					if provider == "anthropic" {
						fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test-model\",\"content\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello world\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					} else {
						chunk(w, map[string]any{"role": "assistant", "content": "Hello "}, nil)
						chunk(w, map[string]any{"content": "world"}, nil)
						chunk(w, map[string]any{}, "stop")
						fmt.Fprint(w, "data: [DONE]\n\n")
					}
				}))
				defer server.Close()
				c := DefaultConfig()
				c.Provider = provider
				c.Model = Models(c.Provider)[0].ID
				c.Endpoint = server.URL
				c.KeyEnv = ""
				if auth {
					c.KeyEnv = "PLOUS_TEST_KEY"
				}
				var snapshots []string
				result, err := streamText(context.Background(), c, os.Getenv(c.KeyEnv), []Message{{Role: "user", Text: "Hi"}}, func(s string) error { snapshots = append(snapshots, s); return nil })
				if err != nil {
					t.Fatal(err)
				}
				if result.Text != "Hello world" || len(snapshots) == 0 {
					t.Fatalf("missing streamed response: %+v", result)
				}
			})
		}
	}
}
func TestFailureAndCancellation(t *testing.T) {
	for _, kind := range []string{"http", "access", "truncated", "cancel", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "http" {
					http.Error(w, `{"error":{"message":"SECRET echo response"}}`, http.StatusUnauthorized)
					return
				}
				if kind == "access" {
					http.Error(w, `{"error":{"message":"SECRET model unavailable"}}`, http.StatusForbidden)
					return
				}
				if kind == "redirect" {
					http.Redirect(w, r, "https://example.com", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				chunk(w, map[string]any{"content": "partial"}, nil)
				if kind == "cancel" {
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			c := DefaultConfig()
			c.Endpoint = server.URL
			c.Model = Models(c.Provider)[0].ID
			c.KeyEnv = ""
			result, err := streamText(ctx, c, os.Getenv(c.KeyEnv), []Message{{Role: "user", Text: "Hi"}}, func(string) error {
				if kind == "cancel" {
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("failure reported as complete")
			}
			if kind == "access" && !strings.Contains(err.Error(), "account access") {
				t.Fatalf("missing model access guidance: %v", err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("provider response leaked")
			}
			if (kind == "cancel" || kind == "truncated") && result.Text != "partial" {
				t.Fatal("partial output lost")
			}
		})
	}
}
func TestConfigValidation(t *testing.T) {
	for _, url := range []string{"https://user:secret@example.com", "http://example.com", "https://example.com/?key=secret", "file:///tmp/key"} {
		c := DefaultConfig()
		c.Endpoint = url
		if c.Validate() == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	c := DefaultConfig()
	c.KeyEnv = "sk-secret-key"
	if c.Validate() == nil {
		t.Fatal("accepted a key value")
	}
}

func TestProvidersDoNotRetryOrExposeErrorBodies(t *testing.T) {
	for _, provider := range Providers() {
		for _, status := range []int{429, 503} {
			t.Run(fmt.Sprintf("%s/%d", provider, status), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Should-Retry", "true")
					w.WriteHeader(status)
					fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"PRIVATE_PROVIDER_BODY"}}`)
				}))
				defer server.Close()
				c := DefaultConfig()
				c.Provider, c.Model, c.Endpoint, c.KeyEnv = provider, Models(provider)[0].ID, server.URL, ""
				_, err := streamText(context.Background(), c, "", []Message{{Role: "user", Text: "Hello"}}, func(string) error { return nil })
				if err == nil || strings.Contains(err.Error(), "PRIVATE_PROVIDER_BODY") || !strings.Contains(err.Error(), fmt.Sprint(status)) {
					t.Fatalf("unsafe or missing provider status: %v", err)
				}
				if calls.Load() != 1 {
					t.Fatalf("SDK retried: %d requests", calls.Load())
				}
			})
		}
	}
}

func TestProvidersSanitizeStreamingErrors(t *testing.T) {
	for _, provider := range Providers() {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if provider == "anthropic" {
					fmt.Fprint(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"PRIVATE_STREAM_ERROR\"}}\n\n")
				} else {
					fmt.Fprint(w, "data: {\"error\":{\"message\":\"PRIVATE_STREAM_ERROR\",\"type\":\"server_error\"}}\n\n")
				}
			}))
			defer server.Close()
			c := DefaultConfig()
			c.Provider, c.Model, c.Endpoint, c.KeyEnv = provider, Models(provider)[0].ID, server.URL, ""
			_, err := streamText(context.Background(), c, "", []Message{{Role: "user", Text: "Hello"}}, func(string) error { return nil })
			if err == nil || strings.Contains(err.Error(), "PRIVATE_STREAM_ERROR") {
				t.Fatalf("stream failure leaked or reported success: %v", err)
			}
		})
	}
}

func TestAnthropicCancellationClosesStreamingRequest(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"cancel-test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test\",\"content\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial answer\"}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := DefaultConfig()
	c.Provider, c.Model, c.Endpoint, c.KeyEnv = "anthropic", Models("anthropic")[0].ID, server.URL, ""
	result, err := streamText(ctx, c, "fixture-key", []Message{{Role: "user", Text: "Wait for stop"}}, func(string) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || result.Text != "partial answer" {
		t.Fatalf("cancelled stream lost partial text or status: %+v %v", result, err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not close the provider request")
	}
}

func streamText(ctx context.Context, c Config, key string, messages []Message, emit func(string) error) (result Result, err error) {
	defer func() {
		if err != nil {
			err = SafeError(ctx, err)
		}
	}()
	lm, err := NewChatModel(ctx, c, key)
	if err != nil {
		return result, err
	}
	input, err := schemaMessages(messages)
	if err != nil {
		return result, err
	}
	stream, err := lm.Stream(ctx, input)
	if err != nil {
		return result, err
	}
	defer stream.Close()
	for {
		msg, e := stream.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return result, e
		}
		if len(msg.ToolCalls) > 0 {
			return result, UserError("unexpected tool call outside the agent")
		}
		result.Text += msg.Content
		if len(result.Text) > MaxResponseBytes {
			return result, UserError("response exceeds the local size limit")
		}
		if msg.Content != "" {
			if err = emit(result.Text); err != nil {
				return result, err
			}
		}
		if msg.ResponseMeta != nil {
			result.Finish = msg.ResponseMeta.FinishReason
			if u := msg.ResponseMeta.Usage; u != nil {
				result.Usage = Usage{Input: max(result.Usage.Input, int64(u.PromptTokens)), Output: max(result.Usage.Output, int64(u.CompletionTokens)), Total: max(result.Usage.Total, int64(u.TotalTokens))}
			}
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if !CompletedFinish(result.Finish) {
		return result, UserError("response did not complete; partial output was saved")
	}
	if strings.TrimSpace(result.Text) == "" {
		return result, UserError("provider returned an empty answer")
	}
	return result, nil
}

func schemaMessages(messages []Message) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system", "user", "assistant":
		default:
			return nil, errors.New("unsupported message role")
		}
		out = append(out, &schema.Message{Role: schema.RoleType(m.Role), Content: m.Text})
	}
	return out, nil
}

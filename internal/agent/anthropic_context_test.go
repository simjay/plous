package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/memory"
	"github.com/simjay/plous/internal/workspace"
)

func TestAnthropicCompactedAndReopenedContextReachesProvider(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad fixture request", http.StatusBadRequest)
			return
		}
		if !body.Stream || len(body.Messages) == 0 || body.Messages[0].Role != "user" {
			t.Errorf("compacted Anthropic context must start with a user message: %+v", body)
			http.Error(w, "invalid first message", http.StatusBadRequest)
			return
		}
		data, _ := json.Marshal(body.Messages)
		if !strings.Contains(string(data), "Release decision: blue") || strings.Contains(string(data), "original-marker-facts") {
			t.Errorf("provider did not receive the compacted context: %s", data)
		}
		writeAnthropicContextAnswer(w)
	}))
	defer server.Close()

	root := t.TempDir()
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	conversation, err := store.NewConversation("", "Compaction")
	if err != nil {
		t.Fatal(err)
	}
	config := ai.DefaultConfig()
	config.Provider, config.Model, config.Endpoint, config.KeyEnv = "anthropic", ai.Models("anthropic")[0].ID, server.URL, ""
	for range 6 {
		turn, err := store.StartTurn("", conversation.ID, workspace.Request{Agent: config, Prompt: strings.Repeat("original-marker-facts ", 400)})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveResponse("", conversation.ID, turn.Request.ID, workspace.Response{Status: "completed", Text: strings.Repeat("old answer ", 400)}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.Turns("", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.StartTurn("", conversation.ID, workspace.Request{Agent: config, Prompt: "Which release?"})
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Store: store, ConversationID: conversation.ID, TurnID: turn.Request.ID, Config: config, System: Instructions, Prompt: turn.Request.Prompt, History: history, contextWindow: 20000}
	summarizations := 0
	if _, err := prepareContext(context.Background(), o, func(context.Context, string, string) (string, error) {
		summarizations++
		return "Release decision: blue. Original observations remain in the conversation records.", nil
	}); err != nil || summarizations == 0 {
		t.Fatal("fixture did not compact", err)
	}

	for _, phase := range []string{"compacted", "reopened"} {
		t.Run(phase, func(t *testing.T) {
			if phase == "reopened" {
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = workspace.Open(root)
				if err != nil {
					t.Fatal(err)
				}
				o.Store = store
				o.History, err = store.Turns("", conversation.ID)
				if err != nil {
					t.Fatal(err)
				}
				turn, err = store.StartTurn("", conversation.ID, workspace.Request{Agent: config, Prompt: "Confirm the saved release."})
				if err != nil {
					t.Fatal(err)
				}
				o.TurnID, o.Prompt = turn.Request.ID, turn.Request.Prompt
			}
			result, err := Run(context.Background(), o)
			if err != nil || result.Text != "The saved release is blue." {
				t.Fatalf("actual Anthropic adapter failed for %s context: %+v %v", phase, result, err)
			}
			if err := store.SaveResponse("", conversation.ID, o.TurnID, workspace.Response{Status: "completed", Text: result.Text}); err != nil {
				t.Fatal(err)
			}
			summary, err := store.LatestCompaction("", conversation.ID)
			if err != nil || summary == nil || !strings.Contains(summary.Content, "Release decision: blue") {
				t.Fatal("compaction was not restored", summary, err)
			}
			mem, err := memory.New(store, "", conversation.ID)
			if err != nil {
				t.Fatal(err)
			}
			page, err := mem.Search(context.Background(), memory.Query{Source: "conversation", Text: "original-marker-facts", Limit: 1})
			if err != nil || len(page.Records) != 1 {
				t.Fatal("original evidence was lost after compaction/reopen", page, err)
			}
			chunk, err := mem.Read(context.Background(), page.Records[0].Ref, 0, memory.MaxReadBytes)
			if err != nil || !strings.Contains(chunk.Text, "original-marker-facts") {
				t.Fatal("original evidence reference could not be read", chunk, err)
			}
		})
	}
	if requests.Load() != 2 {
		t.Fatalf("expected one provider request per phase, got %d", requests.Load())
	}
}

func writeAnthropicContextAnswer(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range []struct {
		name string
		data string
	}{
		{"message_start", `{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture","content":[],"usage":{"input_tokens":42,"output_tokens":0}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The saved release is blue."}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`},
		{"message_stop", `{"type":"message_stop"}`},
	} {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.name, event.data)
	}
}

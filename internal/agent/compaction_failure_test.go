package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

func compactionHistory(t *testing.T) (Options, string, workspace.ReasoningEntry) {
	t.Helper()
	root := t.TempDir()
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	conversation, err := store.NewConversation("", "Compaction recovery")
	if err != nil {
		t.Fatal(err)
	}
	config := ai.DefaultConfig()
	config.Model = ai.Models("openai")[0].ID
	for i := range 9 {
		turn, err := store.StartTurn("", conversation.ID, workspace.Request{Agent: config, Prompt: fmt.Sprintf("source-%02d ", i) + strings.Repeat("input ", 700)})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveResponse("", conversation.ID, turn.Request.ID, workspace.Response{Status: "completed", Text: fmt.Sprintf("answer-%02d ", i) + strings.Repeat("record ", 120)}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.Turns("", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := store.AppendReasoning("", conversation.ID, workspace.ReasoningEntry{Kind: "compaction", Status: "completed", CoveredTurnID: history[0].Request.ID, Content: "Previously saved checkpoint."})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.StartTurn("", conversation.ID, workspace.Request{Agent: config, Prompt: "Which evidence remains?"})
	if err != nil {
		t.Fatal(err)
	}
	return Options{Store: store, ConversationID: conversation.ID, TurnID: turn.Request.ID, Config: config, System: Instructions, Prompt: turn.Request.Prompt, History: history, contextWindow: 20000}, root, prior
}

func TestAgentCompactionRecoveryAfterPartialWork(t *testing.T) {
	for _, failure := range []string{"provider failure", "empty summary", "cancellation", "storage failure"} {
		t.Run(failure, func(t *testing.T) {
			o, root, prior := compactionHistory(t)
			before, err := o.Store.RecordBoundaries("", o.ConversationID)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			providerFailure := errors.New("synthetic provider failure")
			calls := 0
			_, err = prepareContext(ctx, o, func(_ context.Context, system, prompt string) (string, error) {
				calls++
				if Estimate([]ai.Message{{Role: "system", Text: system}, {Role: "user", Text: prompt}}) > InputBudget(o.contextWindow) {
					t.Fatal("compaction sent an oversized batch")
				}
				if failure == "cancellation" {
					cancel()
					return "Late summary after cancellation.", nil
				}
				if failure == "storage failure" && calls == 1 {
					if err := o.Store.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if calls == 2 {
					switch failure {
					case "provider failure":
						return "", providerFailure
					case "empty summary":
						return " \n\t", nil
					}
				}
				return "Intermediate summary must not replace the saved checkpoint on failure.", nil
			})
			if err == nil {
				t.Fatal("incomplete compaction was accepted")
			}
			switch failure {
			case "provider failure":
				if calls != 2 || !errors.Is(err, providerFailure) {
					t.Fatalf("callback failure not exercised: calls=%d error=%v", calls, err)
				}
			case "empty summary":
				if calls != 2 || !strings.Contains(err.Error(), "empty summary") {
					t.Fatalf("empty callback not exercised: calls=%d error=%v", calls, err)
				}
			case "cancellation":
				if calls != 1 || !errors.Is(err, context.Canceled) {
					t.Errorf("canceled compaction kept invoking the model: calls=%d error=%v", calls, err)
				}
			case "storage failure":
				if calls < 2 || !errors.Is(err, workspace.ErrClosed) {
					t.Fatalf("storage failure not exercised: calls=%d error=%v", calls, err)
				}
			}
			_ = o.Store.Close()
			reopened, reopenErr := workspace.Open(root)
			if reopenErr != nil {
				t.Fatal(reopenErr)
			}
			defer reopened.Close()
			summary, readErr := reopened.LatestCompaction("", o.ConversationID)
			if readErr != nil || summary == nil || summary.ID != prior.ID || summary.CoveredTurnID != prior.CoveredTurnID || summary.Content != prior.Content {
				t.Fatalf("checkpoint changed after reopen: %+v %v", summary, readErr)
			}
			after, readErr := reopened.RecordBoundaries("", o.ConversationID)
			if readErr != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("failed compaction changed durable records: before=%+v after=%+v error=%v", before, after, readErr)
			}
			turns, readErr := reopened.Turns("", o.ConversationID)
			if readErr != nil || len(turns) != len(o.History)+1 {
				t.Fatalf("original conversation was lost: %d %v", len(turns), readErr)
			}
			if !reflect.DeepEqual(Messages(o.System, o.History, &prior, o.Prompt), Messages(o.System, turns, summary, o.Prompt)) {
				t.Fatal("active context changed after reopening a failed compaction")
			}
		})
	}
}

func TestAgentCompactionExcludesUnfinishedTurns(t *testing.T) {
	o, _, _ := compactionHistory(t)
	unfinished, err := o.Store.StartTurn("", o.ConversationID, workspace.Request{Agent: o.Config, Prompt: "unfinished-question-marker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Store.SaveResponse("", o.ConversationID, unfinished.Request.ID, workspace.Response{Status: "cancelled", Text: "unfinished-answer-marker"}); err != nil {
		t.Fatal(err)
	}
	o.History, err = o.Store.Turns("", o.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	messages, err := prepareContext(context.Background(), o, func(_ context.Context, _, prompt string) (string, error) {
		calls++
		if strings.Contains(prompt, "unfinished-") {
			t.Fatal("unfinished exchange reached the compactor")
		}
		return "Saved facts and evidence only.", nil
	})
	if err != nil || calls < 2 {
		t.Fatalf("multi-batch compaction not exercised: calls=%d error=%v", calls, err)
	}
	for _, message := range messages {
		if strings.Contains(message.Text, "unfinished-") {
			t.Fatal("unfinished exchange entered active context")
		}
	}
	summary, err := o.Store.LatestCompaction("", o.ConversationID)
	if err != nil || summary == nil || summary.CoveredTurnID == unfinished.Request.ID {
		t.Fatalf("incorrect coverage: %+v %v", summary, err)
	}
}

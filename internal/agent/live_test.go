package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/memory"
	"github.com/simjay/plous/internal/workspace"
)

// Explicit opt-in: real provider requests incur usage on the selected connection.
func TestSavedProviderLive(t *testing.T) {
	profile := os.Getenv("PLOUS_LIVE_AI_PROFILE")
	if profile == "" {
		t.Skip("set PLOUS_LIVE_AI_PROFILE to explicitly test a saved AI connection")
	}
	config, key := liveSavedConnection(t, profile)
	t.Logf("Selected provider=%s model=%s; all evidence and Kubernetes APIs are synthetic", config.Provider, config.Model)

	if !t.Run("streaming_reply", func(t *testing.T) {
		o, _ := liveOptions(t, config, key, "Reply with exactly: plous-live-ok")
		result := liveRun(t, &o)
		if !strings.Contains(result.Text, "plous-live-ok") {
			t.Fatal("provider did not return the requested reply")
		}
		for _, entry := range liveRecords(t, o) {
			if entry.Kind == "tool" {
				t.Fatal("ordinary reply unexpectedly used a tool")
			}
		}
	}) {
		return
	}

	t.Run("memory_search_and_read", func(t *testing.T) {
		o, _ := liveOptions(t, config, key, "Search the saved reasoning records for 'Archive lookup sample', read the matching record, and tell me its exact reference_code. It is absent from this prompt.")
		code := "sample-" + rand.Text()
		liveEvidence(t, o, "Archive lookup sample. "+strings.Repeat("Synthetic diagnostic padding. ", 100)+" reference_code: "+code)
		result := liveRun(t, &o)
		if !strings.Contains(result.Text, code) {
			t.Fatal("answer did not recover the value stored only in evidence")
		}
		actions := map[string]bool{}
		for _, entry := range liveRecords(t, o) {
			if entry.Tool == "memory" && entry.Status == "started" {
				var command memoryCommand
				if err := json.Unmarshal(entry.Input, &command); err != nil {
					t.Fatal(err)
				}
				actions[command.Action] = true
			}
		}
		if !actions["search"] || !actions["read"] {
			t.Fatal("provider did not exercise both memory search and read")
		}
	})

	t.Run("guarded_kubernetes_read", func(t *testing.T) {
		o, _ := liveOptions(t, config, key, "Use kubernetes_read to get the pod named checkout in the captured payments namespace. Report its status.phase and cite the observation.")
		scope, requests := scopedKubernetesFixture(t)
		scope.Namespace = "payments"
		o.Kubernetes = scope
		result := liveRun(t, &o)
		if requests.Load() == 0 || !strings.Contains(result.Text, "Running") {
			t.Fatal("provider did not obtain the pod phase from the synthetic API")
		}
		for _, entry := range liveRecords(t, o) {
			if entry.Tool == "kubernetes_read" && entry.Status == "completed" {
				return
			}
		}
		t.Fatal("successful guarded read was not recorded")
	})

	t.Run("recursive_investigation", func(t *testing.T) {
		o, _ := liveOptions(t, config, key, "Use investigate for this task. Its investigator should search and read the saved reasoning record containing 'Recursive sample', then delegate the arithmetic check to one child using ask with the selected evidence. Return the sum of left and right and the exact reference_code from the saved record. Neither value is in this prompt.")
		code := "recursive-" + rand.Text()
		liveEvidence(t, o, "Recursive sample. left=17; right=25; reference_code="+code)
		result := liveRun(t, &o)
		if !strings.Contains(result.Text, code) || !strings.Contains(result.Text, "42") {
			t.Logf("synthetic answer: %s", result.Text)
			liveTrace(t, o)
			t.Fatal("recursive answer did not recover and analyze the saved evidence")
		}
		child, recursiveRead := false, false
		for _, entry := range liveRecords(t, o) {
			child = child || entry.Kind == "investigation" && entry.Tool == "investigation" && entry.Status == "completed" && entry.ParentSource != ""
			recursiveRead = recursiveRead || entry.Tool == "memory" && entry.Status == "started" && entry.ParentSource != ""
		}
		if !child || !recursiveRead {
			liveTrace(t, o)
			t.Fatal("trace did not confirm a child investigation and a guarded recursive memory read")
		}
	})

	t.Run("compaction_and_reopen", func(t *testing.T) {
		o, root := liveOptions(t, config, key, "Which release code did we choose? Answer from the earlier conversation without using tools.")
		code := "release-" + rand.Text()
		for i := range 8 {
			question := "Synthetic conversation details. " + strings.Repeat("Repeated observation with no additional decision. ", 30)
			answer := strings.Repeat("No additional decisions. ", 15)
			if i == 0 {
				question = "What is our chosen release? " + question
				answer = "Our chosen release is " + code + ". " + answer
			}
			turn, err := o.Store.StartTurn("", o.ConversationID, workspace.Request{Agent: config, Prompt: question})
			if err != nil {
				t.Fatal(err)
			}
			if err = o.Store.SaveResponse("", o.ConversationID, turn.Request.ID, workspace.Response{Status: "completed", Text: answer}); err != nil {
				t.Fatal(err)
			}
		}
		var err error
		o.History, err = o.Store.Turns("", o.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		// Force the production compactor with a small test budget instead of paying
		// to fill the selected model's entire context window.
		o.contextWindow = 17000
		if Estimate(Messages(o.System, o.History, nil, o.Prompt)) <= InputBudget(o.contextWindow) {
			t.Fatal("fixture does not require compaction")
		}
		result := liveRun(t, &o)
		if !strings.Contains(result.Text, code) {
			t.Fatal("compacted context lost the chosen release")
		}
		checkpoint, err := o.Store.LatestCompaction("", o.ConversationID)
		if err != nil || checkpoint == nil || !strings.Contains(checkpoint.Content, code) {
			t.Fatal("live compaction did not persist the chosen release")
		}
		if err := o.Store.Close(); err != nil {
			t.Fatal(err)
		}
		o.Store, err = workspace.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { o.Store.Close() })
		restored, err := o.Store.LatestCompaction("", o.ConversationID)
		if err != nil || restored == nil || restored.ID != checkpoint.ID {
			t.Fatal("compaction checkpoint did not survive reopen")
		}
		archive, err := memory.New(o.Store, "", o.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		page, err := archive.Search(t.Context(), memory.Query{Source: "conversation", Text: "What is our chosen release?", Limit: 1})
		if err != nil || len(page.Records) != 1 {
			t.Fatal("original Q&A was lost after compaction")
		}
		o.History, err = o.Store.Turns("", o.ConversationID)
		if err != nil || len(o.History) != 9 {
			t.Fatal("original conversation changed after compaction")
		}
		turn, err := o.Store.StartTurn("", o.ConversationID, workspace.Request{Agent: config, Prompt: "Repeat only the chosen release code from our earlier conversation. Do not use tools."})
		if err != nil {
			t.Fatal(err)
		}
		o.TurnID, o.Prompt = turn.Request.ID, turn.Request.Prompt
		if result := liveRun(t, &o); !strings.Contains(result.Text, code) {
			t.Fatal("reopened context lost the chosen release")
		}
	})
}

func liveSavedConnection(t *testing.T, profile string) (ai.Config, string) {
	t.Helper()
	// Store.Open and Credential can repair permissions. Only open a temporary
	// settings copy, and read the original credential without the writable store.
	copyRoot := t.TempDir()
	if err := os.CopyFS(filepath.Join(copyRoot, "settings"), os.DirFS(filepath.Join(profile, "settings"))); err != nil {
		t.Fatal("could not copy the saved settings for a read-only test")
	}
	store, err := workspace.Open(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	settings, err := store.ReadSettings()
	if err != nil {
		t.Fatal("saved settings failed validation")
	}
	config := settings.Agent
	key := os.Getenv(config.KeyEnv)
	if config.CredentialRef != "" {
		path := filepath.Join(profile, "credentials.json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatal("saved credentials must be a regular file under 1 MiB")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("could not read the saved credential")
		}
		var credentials map[string]string
		if json.Unmarshal(data, &credentials) != nil {
			t.Fatal("could not decode saved credentials")
		}
		key = credentials[config.CredentialRef]
	}
	if err := config.Ready(key); err != nil {
		t.Fatal(err)
	}
	return config, key
}

func liveOptions(t *testing.T, config ai.Config, key, prompt string) (Options, string) {
	t.Helper()
	root := t.TempDir()
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	conversation, err := store.NewConversation("", "Synthetic live-provider acceptance")
	if err != nil {
		t.Fatal(err)
	}
	return Options{Store: store, ConversationID: conversation.ID, Config: config, Key: key, System: Instructions + "\n" + config.Instructions, Prompt: prompt}, root
}

func liveEvidence(t *testing.T, o Options, content string) {
	t.Helper()
	if _, err := o.Store.AppendReasoning("", o.ConversationID, workspace.ReasoningEntry{Kind: "tool", Tool: "synthetic_fixture", Status: "completed", Content: content}); err != nil {
		t.Fatal(err)
	}
}

func liveRun(t *testing.T, o *Options) ai.Result {
	t.Helper()
	if o.TurnID == "" {
		turn, err := o.Store.StartTurn("", o.ConversationID, workspace.Request{Agent: o.Config, Prompt: o.Prompt})
		if err != nil {
			t.Fatal(err)
		}
		o.TurnID = turn.Request.ID
	}
	updates, last := 0, ""
	o.Emit = func(text string) error { updates++; last = text; return nil }
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	result, err := Run(ctx, *o)
	t.Logf("provider usage: input=%d output=%d total=%d; stream updates=%d", result.Usage.Input, result.Usage.Output, result.Usage.Total, updates)
	if err != nil {
		message := err.Error()
		if o.Key != "" {
			message = strings.ReplaceAll(message, o.Key, "[redacted]")
		}
		t.Fatalf("live agent: %s", message)
	}
	if updates == 0 || last != result.Text || !ai.CompletedFinish(result.Finish) {
		t.Fatal("provider did not complete the streamed reply")
	}
	if o.Key != "" && strings.Contains(result.Text, o.Key) {
		t.Fatal("credential appeared in the response")
	}
	if err := o.Store.SaveResponse("", o.ConversationID, o.TurnID, workspace.Response{Status: "completed", Text: result.Text, Usage: result.Usage, Finish: result.Finish}); err != nil {
		t.Fatal(err)
	}
	return result
}

func liveRecords(t *testing.T, o Options) []workspace.ReasoningEntry {
	t.Helper()
	boundaries, err := o.Store.RecordBoundaries("", o.ConversationID)
	if err != nil || len(boundaries) != 1 {
		t.Fatal("could not locate live-test records")
	}
	var entries []workspace.ReasoningEntry
	for offset, boundary := int64(0), boundaries[0].Reasoning; offset < boundary; {
		lines, next, err := o.Store.ReadRecords(t.Context(), "", o.ConversationID, "reasoning", offset, boundary, 100, 4*workspace.MaxRecordBytes)
		if err != nil || next <= offset {
			t.Fatal("could not read live-test records")
		}
		for _, line := range lines {
			if o.Key != "" && strings.Contains(string(line.Data), o.Key) {
				t.Fatal("credential appeared in the execution records")
			}
			var entry workspace.ReasoningEntry
			if err := json.Unmarshal(line.Data, &entry); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, entry)
		}
		offset = next
	}
	return entries
}

func liveTrace(t *testing.T, o Options) {
	t.Helper()
	for _, entry := range liveRecords(t, o) {
		if entry.Kind == "request" {
			continue
		}
		t.Logf("synthetic trace %s/%s %s: input=%s content=%s", entry.Kind, entry.Tool, entry.Status, entry.Input, entry.Content)
	}
}

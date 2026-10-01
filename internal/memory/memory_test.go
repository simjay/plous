package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/workspace"
)

func fixture(t *testing.T) (*workspace.Store, workspace.Workspace, workspace.Conversation) {
	t.Helper()
	store, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	w, err := store.Save(workspace.Workspace{Name: "One", Connections: []workspace.Connection{{Tool: "argo", Name: "App", Settings: map[string]string{"repo": "https://username:private-secret@example.test/repo?token=private-secret", "token": "private-secret", "revision": "main"}}}}, "user")
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation(w.ID, "Question")
	if err != nil {
		t.Fatal(err)
	}
	return store, w, c
}

func question(t *testing.T, store *workspace.Store, c workspace.Conversation, text string) workspace.Turn {
	t.Helper()
	turn, err := store.StartTurn(c.WorkspaceID, c.ID, workspace.Request{Prompt: text})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResponse(c.WorkspaceID, c.ID, turn.Request.ID, workspace.Response{Status: "completed", Text: "answer " + text}); err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestMemoryScopeSnapshotAndSafeWorkspace(t *testing.T) {
	store, w, old := fixture(t)
	question(t, store, old, "earlier evidence")
	current, err := store.NewConversation(w.ID, "Current")
	if err != nil {
		t.Fatal(err)
	}
	question(t, store, current, "current evidence")
	m, err := New(store, w.ID, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	question(t, store, current, "appended after snapshot")
	other, _ := store.Save(workspace.Workspace{Name: "Other"}, "user")
	otherChat, _ := store.NewConversation(other.ID, "Other")
	question(t, store, otherChat, "outside evidence")
	general, _ := store.NewConversation("", "General")
	question(t, store, general, "outside evidence")
	later, _ := store.NewConversation(w.ID, "Future")
	question(t, store, later, "outside evidence")

	page, err := m.Search(context.Background(), Query{Source: "conversation", Text: "evidence", Limit: 20})
	if err != nil || len(page.Records) != 4 {
		t.Fatal(page, err)
	}
	for _, record := range page.Records {
		if strings.Contains(record.Text, "outside") || strings.Contains(record.Text, "appended") {
			t.Fatal("snapshot included unavailable evidence", record)
		}
		chunk, err := m.Read(context.Background(), record.Ref, 0, 0)
		if err != nil || !strings.Contains(chunk.Text, "evidence") {
			t.Fatal(chunk, err)
		}
	}
	for _, foreign := range []workspace.Conversation{otherChat, general, later} {
		if _, err := m.Search(context.Background(), Query{ConversationID: foreign.ID}); err == nil {
			t.Fatal("query crossed captured conversation scope")
		}
		if _, err := m.Read(context.Background(), "conversation:"+foreign.ID+":0", 0, 0); err == nil {
			t.Fatal("read crossed captured conversation scope")
		}
	}
	for _, source := range []string{"workspace", "activity"} {
		page, err := m.Search(context.Background(), Query{Source: source})
		if err != nil || len(page.Records) == 0 {
			t.Fatal(page, err)
		}
		for _, record := range page.Records {
			chunk, err := m.Read(context.Background(), record.Ref, 0, 0)
			if err != nil || strings.Contains(chunk.Text, "private-secret") || strings.Contains(chunk.Text, "username") || !strings.Contains(chunk.Text, "example.test/repo") {
				t.Fatal("unsafe or missing workspace reference", chunk, err)
			}
		}
	}
}

func TestMemoryPaginationAndBoundedChunks(t *testing.T) {
	store, w, c := fixture(t)
	for i := range 80 {
		if _, err := store.AppendReasoning(w.ID, c.ID, workspace.ReasoningEntry{Kind: "tool", Status: "completed", Content: fmt.Sprintf("entry-%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	large := strings.Repeat("観測", 20000)
	if _, err := store.AppendReasoning(w.ID, c.ID, workspace.ReasoningEntry{Kind: "tool", Status: "completed", Content: large}); err != nil {
		t.Fatal(err)
	}
	m, err := New(store, w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	query := Query{Source: "reasoning", Text: "entry", Limit: 7}
	refs := map[string]bool{}
	for {
		page, err := m.Search(context.Background(), query)
		if err != nil || len(page.Records) > 7 || page.Scanned > ScanRecords {
			t.Fatal(page, err)
		}
		for _, record := range page.Records {
			if refs[record.Ref] {
				t.Fatal("page repeated a record")
			}
			refs[record.Ref] = true
		}
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	if len(refs) != 80 {
		t.Fatalf("retrieved %d records, expected 80", len(refs))
	}
	query = Query{Source: "reasoning", Text: "not present"}
	page, err := m.Search(context.Background(), query)
	if err != nil || len(page.Records) != 0 || page.Scanned != ScanRecords || page.NextCursor == "" {
		t.Fatal("search must yield even when no record matches", page, err)
	}
	query = Query{Source: "reasoning", Text: "観測"}
	var ref string
	for {
		page, err := m.Search(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Records) > 0 {
			ref = page.Records[0].Ref
			if !page.Records[0].Truncated || len(page.Records[0].Text) > 1024 {
				t.Fatal("large search result was not bounded")
			}
			break
		}
		if page.NextCursor == "" {
			t.Fatal("large record was not found")
		}
		query.Cursor = page.NextCursor
	}
	var result strings.Builder
	for offset := 0; ; {
		chunk, err := m.Read(context.Background(), ref, offset, 1000)
		if err != nil || len(chunk.Text) > 1000 || chunk.NextOffset <= offset {
			t.Fatal(chunk, err)
		}
		result.WriteString(chunk.Text)
		if !chunk.Truncated {
			break
		}
		offset = chunk.NextOffset
	}
	var entry workspace.ReasoningEntry
	if err := json.Unmarshal([]byte(result.String()), &entry); err != nil || entry.Content != large {
		t.Fatal("chunking changed the original result", err)
	}
}

func TestMemoryRejectsPathsLimitsAndCancelledReads(t *testing.T) {
	store, w, c := fixture(t)
	question(t, store, c, "readable")
	m, err := New(store, w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []Query{{Source: "../../credentials.json"}, {Limit: 1000}, {Cursor: "bad cursor"}, {Text: strings.Repeat("x", 4097)}, {Text: string([]byte{0xff})}} {
		if _, err := m.Search(context.Background(), query); err == nil {
			t.Fatal("invalid query accepted", query)
		}
	}
	for _, ref := range []string{"../../credentials.json", "conversation:" + c.ID + ":1", "reasoning:" + c.ID + ":9999999"} {
		if _, err := m.Read(context.Background(), ref, 0, 0); err == nil {
			t.Fatal("invalid reference accepted", ref)
		}
	}
	if _, err := m.Read(context.Background(), "workspace", 0, MaxReadBytes+1); err == nil {
		t.Fatal("unbounded read accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Search(ctx, Query{}); err == nil {
		t.Fatal("cancelled search succeeded")
	}
	if _, err := m.Read(ctx, "workspace", 0, 0); err == nil {
		t.Fatal("cancelled read succeeded")
	}
}

func TestGeneralMemoryOnlyIncludesItsConversation(t *testing.T) {
	store, _, _ := fixture(t)
	old, _ := store.NewConversation("", "Old general")
	question(t, store, old, "old question")
	current, _ := store.NewConversation("", "Current general")
	question(t, store, current, "current question")
	m, err := New(store, "", current.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := m.Search(context.Background(), Query{Source: "conversation"})
	if err != nil || len(page.Records) != 2 {
		t.Fatal(page, err)
	}
	if _, err := m.Read(context.Background(), "workspace", 0, 0); err == nil {
		t.Fatal("general chat had workspace metadata")
	}
	if _, err := m.Search(context.Background(), Query{ConversationID: old.ID}); err == nil {
		t.Fatal("general chat read another general conversation")
	}
}

func TestMemorySearchUnicodeCaseFoldingUsesOriginalOffsets(t *testing.T) {
	store, w, c := fixture(t)
	if _, err := store.AppendReasoning(w.ID, c.ID, workspace.ReasoningEntry{Kind: "tool", Status: "completed", Content: strings.Repeat("Ⱥ", 2000) + "NEEDLE"}); err != nil {
		t.Fatal(err)
	}
	m, err := New(store, w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := m.Search(context.Background(), Query{Source: "reasoning", Text: "needle"})
	if err != nil || len(page.Records) != 1 || !strings.Contains(page.Records[0].Text, "NEEDLE") {
		t.Fatal(page, err)
	}
}

func TestWorkspaceEvidenceReferencesSurviveLaterChanges(t *testing.T) {
	store, w, c := fixture(t)
	m, err := New(store, w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := m.Read(context.Background(), "workspace", 0, 0)
	if err != nil || !strings.HasPrefix(original.Ref, "workspace:") {
		t.Fatal("workspace read did not identify its immutable revision", original, err)
	}
	w.Name = "Changed later"
	if _, err := store.Save(w, "user"); err != nil {
		t.Fatal(err)
	}
	newMemory, err := New(store, w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := newMemory.Read(context.Background(), "workspace", 0, 0)
	if err != nil || current.Ref == original.Ref || !strings.Contains(current.Text, "Changed later") {
		t.Fatal("new scope did not capture changed workspace", current, err)
	}
	historical, err := newMemory.Read(context.Background(), original.Ref, 0, 0)
	if err != nil || historical != original {
		t.Fatal("historical workspace citation changed", historical, err)
	}
	if _, err := m.Read(context.Background(), current.Ref, 0, 0); err == nil {
		t.Fatal("old snapshot could access a later workspace revision")
	}
}

func TestWorkspaceReferencesNeverExposeEmbeddedCredentials(t *testing.T) {
	for _, value := range []string{
		"https://user:private-secret@example.test/repo",
		"//user:private-secret@example.test/repo",
		"https://user:private-secret@example.test/%zz",
		"user:private-secret@example.test:repository",
	} {
		w := safeWorkspace(workspace.Workspace{Connections: []workspace.Connection{{Tool: "argo", Name: "repo", Settings: map[string]string{"repo": value, "server": value}}}})
		data, err := json.Marshal(w)
		if err != nil || strings.Contains(string(data), "private-secret") {
			t.Fatal("embedded credential reached memory", string(data), err)
		}
	}
}

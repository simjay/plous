package history

import (
	ai "github.com/simjay/plous/internal/ai/model"
	"os"
	"path/filepath"
	"testing"
)

func TestChatChronologyCheckpointAndImmutableRequest(t *testing.T) {
	root := t.TempDir()
	s, err := openTestHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := addWorkspaceFixture(s)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.NewConversation(w.ID, "A question")
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Agent: ai.DefaultConfig(), Prompt: "hello", Context: "metadata"}
	turn, err := s.StartTurn(w.ID, c.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(root, "workspaces", w.ID, "conversations", c.ID, "reasoning", "000001.jsonl")
	before, _ := os.ReadFile(requestPath)
	turn.Response.Text = "partial"
	if err = s.SaveResponse(w.ID, c.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	turns, err := s.Turns(w.ID, c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Text != "partial" {
		t.Fatal(turns, err)
	}
	turn.Response.Status = "completed"
	turn.Response.Text = "complete"
	if err = s.SaveResponse(w.ID, c.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	turn.Response.Text = "rewritten"
	if err = s.SaveResponse(w.ID, c.ID, turn.Request.ID, turn.Response); err == nil {
		t.Fatal("rewrote terminal response")
	}
	after, _ := os.ReadFile(requestPath)
	if string(after) != string(before) {
		t.Fatal("request mutated")
	}
	list, err := s.Conversations(w.ID)
	if err != nil || len(list) != 1 || list[0].ID != c.ID {
		t.Fatal(list, err)
	}
	if _, err = s.Turns(w.ID, "../escape"); err == nil {
		t.Fatal("accepted traversal")
	}
	if err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Errorf("permissions on %s: %v", path, info.Mode())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestGeneralChatStorageAndWorkspaceIsolation(t *testing.T) {
	s, err := openTestHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.NewConversation("", "General question")
	if err != nil || c.WorkspaceID != "" {
		t.Fatal("general conversation needs no workspace", err)
	}
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Response.Status, turn.Response.Text = "completed", "Answer"
	if err := s.SaveResponse("", c.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	turns, err := s.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Text != "Answer" {
		t.Fatal("general response did not persist", err)
	}
	w, err := addWorkspaceFixture(s)
	if err != nil {
		t.Fatal(err)
	}
	choices, err := s.Conversations(w.ID)
	if err != nil || len(choices) != 0 {
		t.Fatal("general chat leaked into workspace history", err)
	}
	if _, err := s.Turns(w.ID, c.ID); err == nil {
		t.Fatal("workspace opened a general chat")
	}
	choices, err = s.Conversations("")
	if err != nil || len(choices) != 1 || choices[0].ID != c.ID {
		t.Fatal("general history lost its conversation", err)
	}
}

func TestConversationStorageRejectsSymlinks(t *testing.T) {
	store, err := openTestHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workspace, err := addWorkspaceFixture(store)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(store.root, "workspaces", workspace.ID, "conversations")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Conversations(workspace.ID); err == nil {
		t.Fatal("followed conversation symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("modified symlink target", entries, err)
	}
}

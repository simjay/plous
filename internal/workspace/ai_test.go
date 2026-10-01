package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai"
)

func TestSettingsRequireExplicitSavedConnections(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	config := ai.DefaultConfig()
	config.Name = "Work"
	if got := (Settings{Agent: config}).Configs(); len(got) != 0 {
		t.Fatal("selected model became an implicit saved connection")
	}
	if _, err := s.SaveSettings(Settings{Agent: config}); err == nil {
		t.Fatal("saved a selection without its connection")
	}
	saved, err := s.SaveSettings(Settings{Agent: config, Providers: []ai.Config{config.Connection()}})
	if err != nil || len(saved.Configs()) != 1 {
		t.Fatal(saved, err)
	}
}

func TestSettingsAuditAndConflict(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial, err := s.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	next := initial
	next.Agent.Model = "test-model"
	next.Agent.KeyEnv = "MODEL_KEY"
	next.Providers = []ai.Config{next.Agent.Connection()}
	saved, err := s.SaveSettings(next)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 {
		t.Fatal(saved)
	}
	if _, err = s.SaveSettings(initial); err != ErrConflict {
		t.Fatalf("stale save: %v", err)
	}
	saved.Agent.Name = "Reviewer"
	saved.Providers = []ai.Config{saved.Agent.Connection()}
	if _, err = s.SaveSettings(saved); err != nil {
		t.Fatal(err)
	}
	entries, err := filepath.Glob(filepath.Join(root, "settings", "*", "change.json"))
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	var second SettingsChange
	if err = readJSON(entries[1], &second); err != nil {
		t.Fatal(err)
	}
	if second.Before.Agent.Name != "Assistant" || second.After.Agent.Name != "Reviewer" {
		t.Fatal("missing edit history")
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reloaded, err := s.ReadSettings()
	if err != nil || reloaded.Revision != 2 || reloaded.Agent.KeyEnv != "MODEL_KEY" {
		t.Fatal(reloaded, err)
	}
	if err = os.WriteFile(entries[0], []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadSettings(); err == nil {
		t.Fatal("corrupt settings accepted")
	}
	if _, err = s.SaveSettings(reloaded); err == nil {
		t.Fatal("corrupt settings overwritten")
	}
}
func TestChatChronologyCheckpointAndImmutableRequest(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.Save(Workspace{Name: "Test"}, "user")
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
	requestPath := filepath.Join(root, "workspaces", w.ID, "conversations", c.ID, "reasoning.jsonl")
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
func TestAIStorageRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(root, "settings")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadSettings(); err == nil {
		t.Fatal("followed settings symlink")
	}
	w, err := s.Save(Workspace{Name: "Test"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(root, "workspaces", w.ID, "conversations")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Conversations(w.ID); err == nil {
		t.Fatal("followed chats symlink")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("modified symlink target")
	}
}

func TestGeneralChatStorageAndWorkspaceIsolation(t *testing.T) {
	s, err := Open(t.TempDir())
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
	w, err := s.Save(Workspace{Name: "Separate"}, "user")
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

func TestVendorConnectionAndChatSelectionAreStoredSeparately(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	connection := ai.DefaultConfig()
	connection.Name, connection.CredentialRef, connection.KeyEnv = "Work", strings.Repeat("a", 32), ""
	selected := connection
	selected.Model = "gpt-5.4"
	saved, err := s.SaveSettings(Settings{Agent: selected, Providers: []ai.Config{connection}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.ReadSettings()
	if err != nil || loaded.Agent != selected || loaded.Configs()[0] != connection {
		t.Fatal("selection or vendor connection lost", err)
	}
	wrong := saved
	wrong.Agent.CredentialRef = strings.Repeat("b", 32)
	if _, err := s.SaveSettings(wrong); err == nil {
		t.Fatal("chat selection detached from its connection's key")
	}
	paths, _ := filepath.Glob(filepath.Join(s.root, "settings", "*", "change.json"))
	data, _ := os.ReadFile(paths[0])
	var change map[string]json.RawMessage
	json.Unmarshal(data, &change)
	var after struct {
		Providers []map[string]any `json:"providers"`
	}
	json.Unmarshal(change["after"], &after)
	if _, exists := after.Providers[0]["model"]; exists {
		t.Fatal("vendor connection contains a chat model")
	}
}

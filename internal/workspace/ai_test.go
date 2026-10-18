package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/simjay/plous/internal/ai/model"
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
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("modified symlink target")
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

func TestMousePreferenceSurvivesSettingsRevisions(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial, err := s.ReadSettings()
	if err != nil || initial.MouseDisabled {
		t.Fatal("missing preference must enable mouse", err)
	}
	data, err := json.Marshal(initial)
	if err != nil || strings.Contains(string(data), "mouse_disabled") {
		t.Fatal("default changed legacy record encoding", err)
	}
	initial.MouseDisabled = true
	saved, err := s.SaveSettings(initial)
	if err != nil {
		t.Fatal(err)
	}
	saved.Agent.Model = "test-model"
	saved.Agent.KeyEnv = "TEST_MOUSE_KEY"
	saved.Providers = []ai.Config{saved.Agent.Connection()}
	if _, err = s.SaveSettings(saved); err != nil {
		t.Fatal(err)
	}
	restored, err := s.ReadSettings()
	if err != nil || !restored.MouseDisabled || restored.Revision != 2 {
		t.Fatal("AI update lost mouse preference", err)
	}
}

func TestMainAgentPreferenceSurvivesRestartWithoutProviders(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.MainAgent.Effective() != "plous" {
		t.Fatal(settings)
	}
	settings.MainAgent = "codex"
	saved, err := s.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.ReadSettings()
	if err != nil || restored.MainAgent != "codex" || restored.Revision != saved.Revision || len(restored.Providers) != 0 {
		t.Fatal(restored, err)
	}
	restored.MainAgent = "unknown"
	if _, err = s.SaveSettings(restored); err == nil {
		t.Fatal("unknown backend accepted")
	}
}

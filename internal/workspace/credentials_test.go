package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/simjay/plous/internal/ai/model"
)

func TestCredentialStorageRotationAndRemoval(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ref, err := s.SaveCredential("first-secret")
	if err != nil {
		t.Fatal(err)
	}
	config := ai.DefaultConfig()
	config.Model, config.CredentialRef, config.KeyEnv = "test-model", ref, ""
	settings, err := s.SaveSettings(Settings{Agent: config, Providers: []ai.Config{config}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.Credential(config)
	if err != nil || key != "first-secret" {
		t.Fatal("saved credential cannot be resolved", err)
	}
	info, err := os.Stat(filepath.Join(root, "credentials.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credentials must be private", err)
	}
	ref2, err := s.SaveCredential("replacement-secret")
	if err != nil {
		t.Fatal(err)
	}
	settings.Agent.CredentialRef = ref2
	settings.Providers = []ai.Config{settings.Agent}
	settings, err = s.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PruneCredentials(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Credential(config); err == nil {
		t.Fatal("rotation kept the old secret")
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	key, err = s.Credential(settings.Agent)
	if err != nil || key != "replacement-secret" {
		t.Fatal("key did not survive restart", err)
	}
	paths, _ := filepath.Glob(filepath.Join(root, "settings", "*", "change.json"))
	for _, path := range paths {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "secret") {
			t.Fatal("secret leaked into settings history")
		}
	}
	settings.Agent, settings.Providers = ai.DefaultConfig(), nil
	if _, err = s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err = s.PruneCredentials(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "credentials.json"))
	keys := map[string]string{}
	if json.Unmarshal(data, &keys) != nil || len(keys) != 0 {
		t.Fatal("removed connection kept its key")
	}
}

func TestCredentialCorruptionAndSymlinksAreNotOverwritten(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		s, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		path := filepath.Join(s.root, "credentials.json")
		original := "SECRET-DO-NOT-PRINT"
		if symlink {
			target := filepath.Join(t.TempDir(), "target")
			os.WriteFile(target, []byte(original), 0600)
			os.Symlink(target, path)
		} else {
			os.WriteFile(path, []byte(original), 0600)
		}
		if _, err = s.SaveCredential("new-secret"); err == nil || strings.Contains(err.Error(), original) {
			t.Fatal("invalid storage accepted or error exposed key")
		}
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Fatal("invalid credential storage was replaced")
		}
	}
}

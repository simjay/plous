package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalSourceIgnoreUsesProfileAndPreservesExistingFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom-profile")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, ".plousignore")
	data, err := os.ReadFile(name)
	if err != nil || string(data) != defaultSourceIgnore {
		t.Fatalf("missing defaults: %q %v", data, err)
	}
	info, err := os.Stat(name)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("policy permissions", info, err)
	}
	for _, rules := range []string{"generated/\n!generated/keep.yaml\n", ""} {
		if err := os.WriteFile(name, []byte(rules), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(root)
		if err != nil {
			t.Fatal(err)
		}
		data, err = s.SourceIgnore()
		if err != nil || string(data) != rules {
			t.Fatalf("policy overwritten: %q %v", data, err)
		}
	}
	s.Close()
}

func TestGlobalSourceIgnoreDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "rules")
	if err := os.WriteFile(outside, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".plousignore")); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(root); err == nil {
		s.Close()
		t.Fatal("followed ignore symlink")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "keep me" {
		t.Fatal("changed external file")
	}
}

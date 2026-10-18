package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/workspace"
)

func sourceFixture(t *testing.T) (*operation, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	op := &operation{o: Options{Store: store, WorkspaceID: "workspace", Connections: []Connection{{ConnectionID: "src", Name: "source", Tool: "source", Path: root}}}}
	return op, root
}

func TestRegisteredSourceReadPolicy(t *testing.T) {
	op, root := sourceFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("private outside"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"main.go": "package main\n// target\n", "credentials.json": "private token", ".env": "private env", "key.pem": "private key"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.txt", "/etc/passwd", "credentials.json", ".env", "key.pem", "linked.go", "linked-dir/main.go"} {
		input := string(marshal(map[string]any{"connection_id": "src", "operation": "read", "path": path}))
		if _, err := op.externalRead(context.Background(), "source_files_read", input); err == nil {
			t.Fatalf("allowed %s", path)
		}
	}
	for _, input := range []string{`{"connection_id":"forged","operation":"read","path":"main.go"}`, `{"connection_id":"src","operation":"write","path":"main.go"}`, `{"connection_id":"src","operation":"read","path":"main.go","command":"cat"}`, `{"connection_id":"src","operation":"read","path":"main.go","path":"credentials.json"}`} {
		if _, err := op.externalRead(context.Background(), "source_files_read", input); err == nil {
			t.Fatal("allowed", input)
		}
	}
	result, err := op.externalRead(context.Background(), "source_files_read", `{"connection_id":"src","operation":"list"}`)
	if err != nil {
		t.Fatal(err)
	}
	b := string(marshal(result))
	if !strings.Contains(b, "main.go") || strings.Contains(b, "credentials.json") || strings.Contains(b, "linked.go") {
		t.Fatal(b)
	}
	result, err = op.externalRead(context.Background(), "source_files_read", `{"connection_id":"src","operation":"search","query":"target"}`)
	if err != nil || !strings.Contains(string(marshal(result)), "target") {
		t.Fatal(result, err)
	}
}

func TestSourceToolsUseGlobalProfileIgnore(t *testing.T) {
	op, root := sourceFixture(t)
	profile, err := op.o.Store.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, ".plousignore"), []byte("hidden.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hidden.txt"), []byte("unique-hidden-content"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := op.externalRead(t.Context(), "source_files_read", `{"connection_id":"src","operation":"search","query":"unique-hidden-content"}`)
	if err != nil || strings.Contains(string(marshal(result)), "hidden.txt") {
		t.Fatal("search did not use profile rules", result, err)
	}
	result, err = op.externalRead(t.Context(), "source_files_read", `{"connection_id":"src","operation":"read","path":"hidden.txt"}`)
	if err != nil || !strings.Contains(string(marshal(result)), "unique-hidden-content") {
		t.Fatal("exact read used scanning exclusions", result, err)
	}
}

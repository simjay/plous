package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHelmOperationOutcomesAreDurableAndDoNotChangeRevisions(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Save(Workspace{Name: "Shop", Connections: []Connection{{Name: "staging", Tool: "helm"}}}, "user")
	if err != nil {
		t.Fatal(err)
	}
	started, err := s.RecordOperation(Operation{WorkspaceID: w.ID, ConnectionID: w.Connections[0].ID, Tool: "helm", Context: "staging", Namespace: "apps", Release: "shop", Kind: "install", Status: "submitted"})
	if err != nil {
		t.Fatal(err)
	}
	done := started
	done.Status = "completed"
	done.ObservedAt = started.ObservedAt.Add(time.Second)
	if _, err = s.RecordOperation(done); err != nil {
		t.Fatal(err)
	}
	history, err := s.History(w.ID)
	if err != nil || len(history) != 1 {
		t.Fatal("operation altered configuration history", err)
	}
	_ = s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ops, err := s.Operations(w.ID)
	if err != nil || len(ops) != 1 || ops[0].Status != "completed" {
		t.Fatal(ops, err)
	}
	dir := filepath.Join(root, "workspaces", w.ID, "operations")
	if err = os.WriteFile(filepath.Join(dir, ".pending-interrupted"), []byte(`{"incomplete":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Operations(w.ID); err != nil {
		t.Fatal("incomplete unpublished record was read", err)
	}
}

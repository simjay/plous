package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionScopeReaderExcludesSiblingAndLegacyEvidence(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.Save(Workspace{Name: "Shared", Connections: []Connection{
		{Name: "Selected", Tool: "kubernetes"}, {Name: "Sibling", Tool: "kubernetes"},
	}}, "user")
	if err != nil {
		t.Fatal(err)
	}
	var operations []Operation
	for _, c := range w.Connections {
		op, err := s.RecordOperation(Operation{WorkspaceID: w.ID, ConnectionID: c.ID, Tool: "helm", Release: c.Name, Status: "observed"})
		if err != nil {
			t.Fatal(err)
		}
		operations = append(operations, op)
	}
	c := w.Connections[0]
	c.Settings = map[string]string{"token": "private-secret", "path": "https://user:private-secret@example.invalid/repo?token=private-secret"}
	id := ConnectionScopeID(w.ID, c.ID)
	if err = s.EnsureScope(Scope{ID: id, Kind: "connection", WorkspaceID: w.ID, Connection: c}); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(s, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := r.Traverse(context.Background(), Query{Source: "activity", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 {
		t.Fatalf("unexpected scope: %+v", page)
	}
	config, err := r.Read(context.Background(), "workspace", 0, 0)
	if err != nil || strings.Contains(config.Text, "private-secret") || strings.Contains(config.Text, "Sibling") {
		t.Fatal("unsafe metadata", err)
	}
	for _, record := range page.Records {
		if strings.Contains(record.Text, "Sibling") || strings.Contains(record.Text, "private-secret") {
			t.Fatal("foreign or secret evidence", record.Ref)
		}
	}
	if _, err = r.Read(context.Background(), "activity:operation-"+operations[1].ID, 0, 0); err == nil {
		t.Fatal("direct reference bypassed scope")
	}
	refs, err := s.ActivityReferences(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Read(context.Background(), "workspace:"+refs[0], 0, 0); err == nil {
		t.Fatal("legacy workspace evidence entered connection scope")
	}
}

func TestCurrentScopePersistsWithoutWorkspaceAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := CurrentScopeID(strings.Repeat("a", 64))
	c := Connection{ID: "kube-" + strings.Repeat("a", 64), Name: "Temporary", Tool: "kubernetes"}
	if err = s.EnsureScope(Scope{ID: id, Kind: "current_context", Connection: c}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordOperation(Operation{WorkspaceID: id, ConnectionID: c.ID, Tool: "helm", Status: "observed"}); err != nil {
		t.Fatal(err)
	}
	if ws, err := s.List(); err != nil || len(ws) != 0 {
		t.Fatal("scope created workspace", err)
	}
	dir, err := s.ConversationScopeDir(id)
	if err != nil || dir != filepath.Join(root, "contexts", id) {
		t.Fatal(dir, err)
	}
	if err = os.Rename(dir, dir+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(dir+"-old", dir); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConversationScopeDir(id); err == nil {
		t.Fatal("symlink escaped scope validation")
	}
}

func TestCurrentDirectoryScopeUsesValidatedHistoryInfrastructure(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	directory := CurrentDirectoryScope(filepath.Join(t.TempDir(), "project"))
	other := CurrentDirectoryScope(filepath.Join(t.TempDir(), "project"))
	if directory.ID == other.ID || directory.Connection.ID == other.Connection.ID {
		t.Fatal("same-named directories shared an identity")
	}
	for _, scope := range []Scope{directory, other} {
		if err = s.EnsureScope(scope); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := s.ConversationScopeDir(directory.ID)
	if err != nil || dir != filepath.Join(root, "directories", directory.ID) {
		t.Fatal("directory history bypassed validated routing", dir, err)
	}
	r, err := NewReader(s, directory.ID)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := r.Read(t.Context(), "workspace", 0, 0)
	if err != nil || !strings.Contains(metadata.Text, directory.Connection.Settings["path"]) || strings.Contains(metadata.Text, other.Connection.Settings["path"]) {
		t.Fatal("metadata crossed source scopes", metadata.Text, err)
	}
	if scopes, err := s.Scopes(); err != nil || len(scopes) != 2 {
		t.Fatal("directory histories not discoverable", scopes, err)
	}
	if saved, err := s.List(); err != nil || len(saved) != 0 {
		t.Fatal("transient directory created a saved workspace", saved, err)
	}
	tampered := directory
	tampered.Connection = other.Connection
	if err = s.EnsureScope(tampered); err == nil {
		t.Fatal("directory identity accepted a different path")
	}
	if err = os.Rename(dir, dir+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(dir+"-old", dir); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConversationScopeDir(directory.ID); err == nil {
		t.Fatal("directory scope symlink bypassed validation")
	}
}

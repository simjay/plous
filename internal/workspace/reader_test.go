package workspace

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func workspaceReaderFixture(t *testing.T) (*Store, Workspace) {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	w, err := store.Save(Workspace{Name: "One", Connections: []Connection{{Tool: "argo", Name: "App", Settings: map[string]string{"repo": "https://username:private-secret@example.test/repo?token=private-secret", "token": "private-secret", "revision": "main"}}}}, "user")
	if err != nil {
		t.Fatal(err)
	}
	return store, w
}

func TestWorkspaceEvidenceReferencesSurviveLaterChanges(t *testing.T) {
	store, w := workspaceReaderFixture(t)
	m, err := NewReader(store, w.ID)
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
	newMemory, err := NewReader(store, w.ID)
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
		w := safeWorkspace(Workspace{Connections: []Connection{{Tool: "argo", Name: "repo", Settings: map[string]string{"repo": value, "server": value}}}})
		data, err := json.Marshal(w)
		if err != nil || strings.Contains(string(data), "private-secret") {
			t.Fatal("embedded credential reached memory", string(data), err)
		}
	}
}

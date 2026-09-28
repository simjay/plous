package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "plous")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, root
}

func TestSaveAuditAndRestart(t *testing.T) {
	s, root := openTestStore(t)
	input := Workspace{Name: "Payments", Connections: []Connection{
		{Tool: "kubernetes", Name: "Development", Settings: map[string]string{"context": "dev", "kubeconfig": "/existing/config"}},
		{Tool: "kubernetes", Name: "Production", Settings: map[string]string{"context": "prod"}},
	}}
	first, err := s.Save(input, "user")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Revision != 1 || first.Connections[0].ID == "" || first.Connections[0].ID == first.Connections[1].ID {
		t.Fatalf("missing stable unique IDs/revision: %+v", first)
	}
	if input.ID != "" || input.Connections[0].ID != "" {
		t.Fatal("Save mutated its input")
	}
	edited := clone(first)
	edited.Name = "Payments renamed"
	edited.Connections[0].Name = "Staging"
	edited.Connections[0].Settings["context"] = "staging"
	edited.Connections = edited.Connections[:1]
	second, err := s.Save(edited, "user")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Connections[0].ID != first.Connections[0].ID || second.Revision != 2 {
		t.Fatal("rename/reconfiguration changed identity")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	list, err := reopened.List()
	if err != nil || len(list) != 1 || !reflect.DeepEqual(list[0], second) {
		t.Fatalf("restart lost current state: %+v, %v", list, err)
	}
	history, err := reopened.History(first.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	if history[0].Before != nil || !reflect.DeepEqual(history[0].After, first) ||
		!reflect.DeepEqual(history[1].Before, &first) || !reflect.DeepEqual(history[1].After, second) {
		t.Fatal("audit did not retain exact before/after configuration")
	}
	if history[0].Actor != "user" || history[0].Time.IsZero() || history[0].ID == history[1].ID {
		t.Fatal("audit metadata missing")
	}
}

func TestInterruptedSaveDoesNotBecomeState(t *testing.T) {
	s, root := openTestStore(t)
	w, err := s.Save(Workspace{Name: "Stable"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(root, "workspaces", w.ID, "revisions", ".pending-interrupted")
	if err := os.Mkdir(pending, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "change.json"), []byte(`{"after":`), 0600); err != nil {
		t.Fatal(err)
	}
	// A crash on a first save can leave empty directories too.
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "unfinished", "revisions"), 0700); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.List()
	if err != nil || len(list) != 1 || !reflect.DeepEqual(list[0], w) {
		t.Fatalf("interrupted write affected committed state: %+v, %v", list, err)
	}
	w.Name = "Next successful save"
	w, err = s.Save(w, "user")
	if err != nil || w.Revision != 2 {
		t.Fatalf("could not save after interruption: %+v, %v", w, err)
	}
}

func TestRevisionConflictAndConcurrentSaves(t *testing.T) {
	s, _ := openTestStore(t)
	w, err := s.Save(Workspace{Name: "Original"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errorsOut := make(chan error, 2)
	for _, name := range []string{"First editor", "Second editor"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			copy := w
			copy.Name = name
			_, err := s.Save(copy, "user")
			errorsOut <- err
		}(name)
	}
	wg.Wait()
	close(errorsOut)
	successes, conflicts := 0, 0
	for err := range errorsOut {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d, conflicts=%d", successes, conflicts)
	}
	history, err := s.History(w.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("conflicting save wrote audit state: %+v, %v", history, err)
	}
}

func TestFailedSavePreservesCurrentRevision(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	s, root := openTestStore(t)
	w, err := s.Save(Workspace{Name: "Committed"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	// Make the containing storage tree unwritable, as on a read-only mount.
	// A failed first save must leave existing workspace data intact.
	parent := filepath.Join(root, "workspaces")
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0700) })
	if _, err := s.Save(Workspace{Name: "Cannot publish"}, "user"); err == nil {
		t.Fatal("save unexpectedly succeeded without write permission")
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || !reflect.DeepEqual(list[0], w) {
		t.Fatalf("write failure changed committed state: %+v, %v", list, err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	w.Name = "Storage writable again"
	w, err = s.Save(w, "user")
	if err != nil || w.Revision != 2 {
		t.Fatalf("save after recovery = %+v, %v", w, err)
	}
}

func TestProcessLock(t *testing.T) {
	s, root := openTestStore(t)
	second, err := Open(root)
	if second != nil {
		second.Close()
		t.Fatal("second open acquired existing lock")
	}
	if err == nil || !strings.Contains(err.Error(), "another Plous process") {
		t.Fatalf("unhelpful lock error: %v", err)
	}
	s.Close()
	second, err = Open(root)
	if err != nil {
		t.Fatalf("lock remained after close: %v", err)
	}
	second.Close()
	if _, err := s.List(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed store usable: %v", err)
	}
}

func TestCorruptCommittedRevisionIsPreserved(t *testing.T) {
	s, root := openTestStore(t)
	w, err := s.Save(Workspace{Name: "Original"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.History(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "workspaces", w.ID, "revisions", revisionName(history[0]), "change.json")
	broken := []byte(`{"after":`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	w.Name = "Must not replace corruption"
	if _, err := s.Save(w, "user"); err == nil {
		t.Fatal("Save ignored corrupt history")
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, broken) {
		t.Fatal("corrupt data was overwritten")
	}
	s.Close()
	if reopened, err := Open(root); err == nil {
		reopened.Close()
		t.Fatal("Open ignored corrupt history")
	}
	// A failed Open must also release its lock, allowing explicit recovery.
	good, _ := json.Marshal(history[0])
	if err := os.WriteFile(path, good, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Close()
}

func TestChangedHistoricalBeforeIsRejected(t *testing.T) {
	s, root := openTestStore(t)
	w, err := s.Save(Workspace{Name: "First"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	w.Name = "Second"
	if _, err := s.Save(w, "user"); err != nil {
		t.Fatal(err)
	}
	history, err := s.History(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	history[1].Before.Name = "Rewritten past"
	data, _ := json.Marshal(history[1])
	path := filepath.Join(root, "workspaces", w.ID, "revisions", revisionName(history[1]), "change.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("accepted inconsistent audit chain")
	}
}

func TestSessionAtomicPersistenceAndCorruption(t *testing.T) {
	s, root := openTestStore(t)
	if data, err := s.ReadSession(); err != nil || data != nil {
		t.Fatalf("missing session = %q, %v", data, err)
	}
	draft := []byte(`{"selected":"payments","drafts":{"payments":":1 /etc/kubernetes !"}}`)
	if err := s.WriteSession(draft); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSession([]byte(`invalid`)); err == nil {
		t.Fatal("accepted invalid session JSON")
	}
	// An unpublished temporary file must not replace the last good session.
	if err := os.WriteFile(filepath.Join(root, ".session-interrupted"), []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if actual, err := s.ReadSession(); err != nil || !bytes.Equal(actual, draft) {
		t.Fatalf("session changed: %q, %v", actual, err)
	}
	broken := []byte(`{`)
	path := filepath.Join(root, "session.json")
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSession(draft); err == nil {
		t.Fatal("overwrote corrupt session")
	}
	if actual, _ := os.ReadFile(path); !bytes.Equal(actual, broken) {
		t.Fatal("lost corrupt session data")
	}
}

func TestPrivatePermissionsAndPathValidation(t *testing.T) {
	s, root := openTestStore(t)
	w, err := s.Save(Workspace{Name: "Private"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSession([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s permissions = %o; want %o", path, info.Mode().Perm(), want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../escape", "/absolute", ".hidden", "", "a/b"} {
		if _, err := s.History(id); err == nil {
			t.Errorf("accepted invalid ID %q", id)
		}
	}
	w.ID = "../escape"
	if _, err := s.Save(w, "user"); err == nil {
		t.Fatal("accepted escaping workspace ID")
	}
	if _, err := s.Save(Workspace{Name: "bad", Connections: []Connection{{Name: "bad", Tool: "unsupported"}}}, "user"); err == nil {
		t.Fatal("accepted unsupported tool")
	}
}

func TestStorageSymlinksAreRejected(t *testing.T) {
	s, root := openTestStore(t)
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"untouched":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "session.json")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSession([]byte(`{}`)); err == nil {
		t.Fatal("followed session symlink")
	}
	if data, err := os.ReadFile(external); err != nil || string(data) != `{"untouched":true}` {
		t.Fatal("changed external data")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "workspaces", "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("followed workspace directory symlink")
	}
}

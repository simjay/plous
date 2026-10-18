package history

import (
	"os"
	"path/filepath"
)

type fixtureProfile string

func (p fixtureProfile) DataRoot() (string, error)    { return string(p), nil }
func (p fixtureProfile) AddCloser(func() error) error { return nil }
func openTestHistory(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	return Open(fixtureProfile(root))
}

type fixtureWorkspace struct{ ID string }

func addWorkspaceFixture(s *Store) (fixtureWorkspace, error) {
	w := fixtureWorkspace{ID: newID()}
	return w, os.MkdirAll(filepath.Join(s.root, "workspaces", w.ID), 0700)
}

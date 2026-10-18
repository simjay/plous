package app

import (
	"crypto/sha256"
	"fmt"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
	"testing"
)

func sourceTextFixture(t *testing.T, s *source.Snapshot, name string) string {
	t.Helper()
	data, err := s.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func captureSourceFixture(t *testing.T, s *source.Snapshot) {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range s.Paths() {
		files[name] = []byte(sourceTextFixture(t, s, name))
	}
	s.Files = files
}
func changedSourceFixture(t *testing.T, s *source.Snapshot, name string, data []byte) *source.Snapshot {
	t.Helper()
	next := *s
	captureSourceFixture(t, &next)
	next.Files[name] = data
	next.Identity = fmt.Sprintf("%s:%x", s.Identity, sha256.Sum256(data))
	return &next
}

func applyCodeResult(t *testing.T, m *model, result appcode.CodeResult) {
	t.Helper()
	for {
		next := m.updateCodeResult(result)
		if next == nil {
			return
		}
		var ok bool
		result, ok = next().(appcode.CodeResult)
		if !ok {
			t.Fatal("unexpected Code continuation")
		}
	}
}

func (m *model) codeHas(workspace, id, kind string) bool {
	if s := m.savedCodeState(workspace, id); s != nil && s.Inventory != nil {
		if kind == "terraform" {
			return len(s.Inventory.Terraform) > 0
		}
		return len(s.Inventory.Entries) > 0
	}
	return false
}

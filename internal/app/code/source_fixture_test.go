package code

import (
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

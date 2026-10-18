package source

import (
	"testing"
)

func sourceTextFixture(t *testing.T, s *Snapshot, name string) string {
	t.Helper()
	data, err := s.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func captureSourceFixture(t *testing.T, s *Snapshot) {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range s.Paths() {
		files[name] = []byte(sourceTextFixture(t, s, name))
	}
	s.Files = files
}

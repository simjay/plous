package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalDirectoryPaths(t *testing.T) {
	base := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for input, want := range map[string]string{".": base, "./my apps": filepath.Join(base, "my apps"), "'./my apps'": filepath.Join(base, "my apps"), "~": home, "~/apps": filepath.Join(home, "apps"), base: base} {
		got, err := ResolveDirectory(input, base)
		if err != nil || got != want {
			t.Fatalf("%q resolved to %q, want %q: %v", input, got, want, err)
		}
	}
	for _, input := range []string{"", "~somebody/apps", "path\nnext"} {
		if _, err := ResolveDirectory(input, base); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	file := filepath.Join(base, "kustomization.yaml")
	os.WriteFile(file, []byte("resources: []"), 0600)
	if CheckDirectory(file) == nil || CheckDirectory(filepath.Join(base, "missing")) == nil || CheckDirectory(base) != nil {
		t.Fatal("directory validation accepted a file or missing path")
	}
}

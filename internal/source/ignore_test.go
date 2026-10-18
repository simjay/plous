package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLocalScanRespectsScopedIgnoreRulesWithoutGit(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		".gitignore":                "# local rules\r\n.*/\r\n!/.github/\r\n*.yaml\r\n!keep.yaml\r\n/root-only.txt\r\n",
		".cache/generated.yaml":     "ignored",
		".github/workflows/ci.yml":  "name: CI",
		".settings/private.txt":     "ignored",
		"keep.yaml":                 "kept",
		"ignored.yaml":              "ignored",
		"root-only.txt":             "ignored",
		"nested/.gitignore":         "!included.yaml\nkeep.yaml\n",
		"nested/included.yaml":      "kept",
		"nested/keep.yaml":          "ignored",
		"nested/root-only.txt":      "kept",
		"sibling/included.yaml":     "ignored",
		"sibling/keep.yaml":         "kept",
		"README.md":                 "kept",
		"node_modules/package/file": "ignored",
	})
	captured, err := Load(context.Background(), Config{Path: root, ScanRules: func() ([]byte, error) { return []byte("node_modules/\n"), nil }})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".github/workflows/ci.yml", ".gitignore", "README.md", "keep.yaml", "nested/.gitignore", "nested/included.yaml", "nested/root-only.txt", "sibling/keep.yaml"}
	if !slices.Equal(captured.Paths(), want) || len(captured.Gaps) != 0 {
		t.Fatalf("wrong scan contents: %v; gaps: %v", captured.Paths(), captured.Gaps)
	}
}

func TestIgnoredCacheDoesNotExhaustLocalScan(t *testing.T) {
	root := sourceFiles(t, map[string]string{".gitignore": ".cache/\n", "README.md": "source", ".cache/first": "cache"})
	for n := range maxFiles + 1 {
		if err := os.WriteFile(filepath.Join(root, ".cache", fmt.Sprint(n)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	captured, err := Load(context.Background(), Config{Path: root})
	if err != nil || len(captured.Paths()) != 2 {
		t.Fatalf("ignored cache affected scan: %v, %v", captured, err)
	}
}

func TestLocalScanDoesNotFollowIgnoreSymlink(t *testing.T) {
	outside := sourceFiles(t, map[string]string{"rules": "*.yaml\n"})
	root := sourceFiles(t, map[string]string{"resource.yaml": "apiVersion: v1\nkind: ConfigMap\n"})
	if err := os.Symlink(filepath.Join(outside, "rules"), filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	captured, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Paths()) != 1 || !captured.Has("resource.yaml") || len(captured.Gaps) != 1 {
		t.Fatal("ignore symlink changed the source boundary", captured.Paths(), captured.Gaps)
	}
}

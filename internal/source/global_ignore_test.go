package source

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGlobalIgnoreScanningAndExactDependenciesLocalAndGit(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint(remote), func(t *testing.T) {
			scope, root := readScope(t)
			files := map[string]string{
				"kustomization.yaml": "resources: [generated/app.yaml]\n",
				"generated/app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: ignored-input}\n",
				"global-only.txt":    "needle", "git-only.txt": "needle", "keep.txt": "needle",
				"sub/global-only.txt": "needle", "sub/keep.txt": "needle",
				"skip/.gitignore": strings.Repeat("x", maxFile+1),
			}
			for name, data := range files {
				target := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			config := Config{Path: root}
			if remote {
				repo, err := git.PlainInit(root, false)
				if err != nil {
					t.Fatal(err)
				}
				wt, err := repo.Worktree()
				if err != nil {
					t.Fatal(err)
				}
				if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("git-only.txt\n!global-only.txt\ngenerated/\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := wt.Add(".gitignore"); err != nil {
					t.Fatal(err)
				}
				hash, err := wt.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
				if err != nil {
					t.Fatal(err)
				}
				config = Config{Repository: root, Revision: hash.String()}
			} else if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("git-only.txt\n!global-only.txt\ngenerated/\n"), 0600); err != nil {
				t.Fatal(err)
			}
			policy := func() ([]byte, error) {
				return []byte("# global\nglobal-only.txt\n!git-only.txt\ngenerated/\nskip/\n*.tmp\n!keep.tmp\n"), nil
			}
			config.ScanRules = policy
			scope.ScanRules = policy
			snapshot, err := Load(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Has("generated/app.yaml") || snapshot.Has("global-only.txt") || snapshot.Has("git-only.txt") || snapshot.Has("sub/global-only.txt") {
				t.Fatal("ignored file in discovery", snapshot.Paths())
			}
			inv := Discover(snapshot)
			if remote {
				scope.View = CaptureReadView(inv, nil, nil, nil)
			}
			result, err := (Renderer{}).Render(t.Context(), inv, "kustomize:.", Inputs{})
			if err != nil || len(result.Resources) != 1 || result.Resources[0].Name != "ignored-input" {
				t.Fatalf("ignored dependency failed: %v %v", result, err)
			}
			page, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "search", Query: "needle"})
			if err != nil || len(page.Data.Matches) != 2 {
				t.Fatal("scan policy mismatch", page, err)
			}
			for _, name := range []string{"generated/app.yaml", "global-only.txt", "git-only.txt"} {
				page, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "read", Path: name})
				if err != nil || page.Data.Text == "" {
					t.Fatal("exact ignored read failed", name, page, err)
				}
				page, err = readSourcePage(t.Context(), scope, ReadRequest{Operation: "search", Path: name, Query: "needle"})
				if err != nil || len(page.Data.Matches) != 0 {
					t.Fatal("exact search bypassed exclusions", name, page, err)
				}
			}
			if _, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "read", Path: "credentials.json"}); err == nil {
				t.Fatal("scan policy bypassed access restrictions")
			}
		})
	}
}

func TestGlobalIgnoreReloadAndSearchContinuation(t *testing.T) {
	scope, root := readScope(t)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rules := "c.txt\n"
	loads := 0
	scope.ScanRules = func() ([]byte, error) { loads++; return []byte(rules), nil }
	req := ReadRequest{Operation: "search", Query: "needle", Limit: 1}
	page, err := readSourcePage(t.Context(), scope, req)
	if err != nil || len(page.Data.Matches) != 1 {
		t.Fatal(page, err)
	}
	req.Cursor = page.Continuation["cursor"].(string)
	rules = "b.txt\n"
	page, err = readSourcePage(t.Context(), scope, req)
	if err != nil || len(page.Data.Matches) != 1 || page.Data.Matches[0].Path != "b.txt" || loads != 1 {
		t.Fatal("continuation policy changed", page, err, loads)
	}
	page, err = readSourcePage(t.Context(), scope, ReadRequest{Operation: "search", Query: "needle"})
	if err != nil || len(page.Data.Matches) != 2 || page.Data.Matches[1].Path != "c.txt" || loads != 2 {
		t.Fatal("new scan ignored policy update", page, err, loads)
	}
}

func TestGlobalIgnoreNegationAndExcludedParents(t *testing.T) {
	root := sourceFiles(t, map[string]string{"a.tmp": "", "keep.tmp": "", "sub/keep.tmp": "", "hidden/keep.tmp": ""})
	config := Config{Path: root, ScanRules: func() ([]byte, error) { return []byte("*.tmp\n!keep.tmp\nhidden/\n!hidden/keep.tmp\n"), nil }}
	snapshot, err := Load(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snapshot.Paths(), []string{"keep.tmp", "sub/keep.tmp"}) {
		t.Fatal(snapshot.Paths())
	}
}

func TestExactInputsCannotReadGitInternals(t *testing.T) {
	root := sourceFiles(t, map[string]string{".git/config": "private", "app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\n"})
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.ReadFile(".git/config"); err == nil {
		t.Fatal("exact read exposed Git internals")
	}
	if _, err := snapshot.inputKind(".git/config"); err == nil {
		t.Fatal("render dependency exposed Git internals")
	}
}

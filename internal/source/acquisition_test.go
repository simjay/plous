package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
)

func sourceFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestDiscoveryAndDependencyIdentity(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"payments/base/kustomization.yaml":        "resources: [deployment.yaml]\n",
		"payments/base/deployment.yaml":           "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: payments}\n",
		"payments/overlays/dev/kustomization.yml": "resources: [../../base]\nnamespace: dev\n",
		"payments/overlays/prod/Kustomization":    "resources: [../../base]\nnamespace: prod\n",
		"storefront/kustomization.yaml":           "resources: []\n",
		".git/config":                             "private metadata",
	})
	captured, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	s, err := kustomize.Inspect(captured.Paths(), captured.ReadFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 4 || len(s.Paths()) != 5 {
		t.Fatalf("wrong discovery: %v, files %v", s.Entries, s.Paths())
	}
	refs := s.Entry("payments/overlays/dev").References
	if len(refs) != 1 || refs[0].Path != "payments/base" || !refs[0].Entry || refs[0].Error != "" {
		t.Fatal(refs)
	}
	if len(Discover(captured).Consumers("kustomize:payments/base")) != 2 {
		t.Fatal("shared base lost a consumer")
	}
}

func TestBuildUsesCapturedInputAndRejectsUnsafeDependencies(t *testing.T) {
	root := sourceFiles(t, map[string]string{"kustomization.yaml": "resources: [resource.yaml]\n", "resource.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: captured}\n"})
	captured, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(captured)
	renderer := Renderer{Kustomize: func(ctx context.Context, dir string) ([]byte, string, error) {
		// Input capture has completed before the integration receives its private directory.
		os.WriteFile(filepath.Join(root, "resource.yaml"), []byte("changed"), 0600)
		data, err := os.ReadFile(filepath.Join(dir, "resource.yaml"))
		return data, "fixture", err
	}}
	result, err := renderer.Render(t.Context(), inventory, "kustomize:.", Inputs{})
	if err != nil || result.Resources[0].Name != "captured" {
		t.Fatalf("snapshot was not used: %+v %v", result, err)
	}
	for _, config := range []string{"resources: [../outside]", "resources: [/tmp/outside]", "resources: ['https://example.invalid/repo']", "generators: [plugin.yaml]", "helmCharts: [{name: external}]", "resources: [missing.yaml]"} {
		t.Run(config, func(t *testing.T) {
			dir := sourceFiles(t, map[string]string{"kustomization.yaml": config})
			captured, err := Load(context.Background(), Config{Path: dir})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := Discover(captured)
			called := false
			_, err = (Renderer{Kustomize: func(context.Context, string) ([]byte, string, error) { called = true; return nil, "", nil }}).Render(t.Context(), snapshot, "kustomize:.", Inputs{})
			if err == nil || called {
				t.Fatalf("unsafe build dispatched: %v", err)
			}
		})
	}
}

func TestSymlinksCyclesAndInvalidConfigurationStayInspectable(t *testing.T) {
	outside := sourceFiles(t, map[string]string{"secret.yaml": "private"})
	root := sourceFiles(t, map[string]string{"kustomization.yaml": "resources: [link.yaml]\n", "cycle/kustomization.yaml": "resources: [../cycle]\n", "invalid/kustomization.yaml": "resources: [\n"})
	if err := os.Symlink(filepath.Join(outside, "secret.yaml"), filepath.Join(root, "link.yaml")); err != nil {
		t.Fatal(err)
	}
	captured, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	s, err := kustomize.Inspect(captured.Paths(), captured.ReadFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Gaps) != 1 || s.Entry("invalid").Error == "" {
		t.Fatal("missing source diagnostics")
	}
	for _, name := range []string{".", "cycle", "invalid"} {
		if _, err := (Renderer{}).Render(t.Context(), Discover(captured), "kustomize:"+name, Inputs{}); err == nil {
			t.Fatalf("accepted invalid entry %s", name)
		}
	}
	if _, ok := s.Files["link.yaml"]; ok {
		t.Fatal("read symlink target")
	}
}

func TestLocalGitSnapshotDoesNotCheckoutOrRunFilters(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	root := sourceFiles(t, map[string]string{"apps/payments/kustomization.yaml": "resources: []\n", ".gitattributes": "*.yaml filter=plous-test\n"})
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	hash, err := worktree.Commit("fixture", &git.CommitOptions{Author: &gitobject.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	commit := hash.String()
	config, err := repository.Config()
	if err != nil {
		t.Fatal(err)
	}
	config.Raw.Section("filter").Subsection("plous-test").SetOption("smudge", "exit 1")
	if err := repository.SetConfig(config); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "apps/payments/kustomization.yaml"), []byte("working: changes"), 0600)
	captured, err := Load(context.Background(), Config{Repository: root, Revision: commit, Subdirectory: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := kustomize.Inspect(captured.Paths(), captured.ReadFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Revision != commit || s.Entry("payments") == nil || sourceTextFixture(t, captured, "payments/kustomization.yaml") != "resources: []\n" {
		t.Fatal("wrong Git snapshot")
	}
	data, _ := os.ReadFile(filepath.Join(root, "apps/payments/kustomization.yaml"))
	head, err := repository.Head()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "working: changes" || head.Hash().String() != commit {
		t.Fatal("Git source was changed")
	}
}

func TestCancellationAndURLCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Load(ctx, Config{Path: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, repo := range []string{"https://token@example.invalid/repo", "https://user:password@example.invalid/repo", "https://example.invalid/repo?token=hidden", "ext::command", "-option"} {
		if (Config{Repository: repo}).Validate() == nil {
			t.Fatalf("accepted %s", repo)
		}
	}
}

package code

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	"github.com/simjay/plous/internal/integrations/manifest"
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
func TestConfigurationPreviewMasksInlineValues(t *testing.T) {
	masked := kustomizecommon.MaskConfiguration([]byte("secretGenerator:\n- name: secret\n  literals: [arbitrary=hidden-value]\npatches:\n- patch: sensitive-patch\n"))
	if strings.Contains(masked, "hidden-value") || strings.Contains(masked, "sensitive-patch") {
		t.Fatal("inline values leaked")
	}
}
func TestNativeBuildWithSyntheticOverlay(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := sourceFiles(t, map[string]string{
		"base/kustomization.yaml":          "resources: [deployment.yaml]\n",
		"base/deployment.yaml":             "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: demo}\nspec:\n  replicas: 1\n  selector: {matchLabels: {app: demo}}\n  template:\n    metadata: {labels: {app: demo}}\n    spec:\n      containers: [{name: app, image: example.invalid/demo:old}]\n",
		"overlays/prod/kustomization.yaml": "resources: [../../base]\nnamespace: production\nreplicas: [{name: demo, count: 3}]\nimages: [{name: example.invalid/demo, newTag: new}]\n",
	})
	s := fixtureInventory(t, root)
	result, err := Build(context.Background(), s, "overlays/prod", nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered := manifest.YAML(result.Resources[0].Object, false)
	for _, text := range []string{"namespace: production", "replicas: 3", "image: example.invalid/demo:new"} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("missing %s: %s", text, rendered)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "unused-kubeconfig")); !os.IsNotExist(err) {
		t.Fatal("build wrote into the source")
	}
}
func TestSourcePreviewRetainsAllYAMLDocuments(t *testing.T) {
	input := "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: first}\ndata: {arbitrary: hidden-one}\n---\napiVersion: v1\nkind: Secret\nmetadata: {name: second}\nstringData: {arbitrary: hidden-two}\n"
	preview := kustomizecommon.MaskConfiguration([]byte(input))
	if !strings.Contains(preview, "name: first") || !strings.Contains(preview, "name: second") || strings.Contains(preview, "hidden-") {
		t.Fatal("preview lost a document or exposed values", preview)
	}
}

// fixtureInventory reads trusted test inputs without invoking source acquisition.
func fixtureInventory(t *testing.T, directory string) *Inventory {
	t.Helper()
	files := map[string][]byte{}
	tree := os.DirFS(directory)
	err := fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		files[name], err = fs.ReadFile(tree, name)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return Discover(files)
}

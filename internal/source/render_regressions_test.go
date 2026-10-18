package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
)

func TestTransformerDependencyCapture(t *testing.T) {
	files := map[string]string{
		"kustomization.yaml": "resources: [app.yaml]\ntransformers: [transformer.yaml]\n",
		"app.yaml":           "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {mode: old}\n",
		"transformer.yaml":   "apiVersion: builtin\nkind: PatchTransformer\nmetadata: {name: update}\npath: patch.yaml\ntarget:\n  kind: ConfigMap\n  name: app\n",
		"patch.yaml":         "- op: replace\n  path: /data/mode\n  value: updated\n",
	}
	captured := map[string][]byte{}
	for name, data := range files {
		captured[name] = []byte(data)
	}
	direct, err := kustomize.Build(t.Context(), kustomize.Discover(captured), ".", nil)
	if err != nil {
		t.Fatalf("native fixture failed: %v", err)
	}
	t.Logf("native integration rendered %d resource(s)", len(direct.Resources))
	root := sourceFiles(t, files)
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{}); err != nil {
		t.Fatalf("same valid transformer failed after selected-input capture: %v", err)
	}
}

func TestSelectedRenderIgnoresUnrelatedDirectory(t *testing.T) {
	root := sourceFiles(t, map[string]string{"app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\n"})
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	if err := os.Mkdir(filepath.Join(root, "unrelated"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated", ".gitignore"), []byte(strings.Repeat("x", maxFile+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(t.Context(), inventory, "plain:app.yaml", Inputs{}); err != nil {
		t.Fatalf("selected one-file render inspected an unrelated directory: %v", err)
	}
}

func TestDiscoveryBudgetCountsRetainedMetadata(t *testing.T) {
	files := map[string]string{}
	body := "resources: []\n#" + strings.Repeat("x", maxFile-64) + "\n"
	for n := range 17 {
		files[fmt.Sprintf("env-%02d/kustomization.yaml", n)] = body
	}
	root := sourceFiles(t, files)
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := DiscoverContext(t.Context(), snapshot)
	if err != nil {
		t.Fatalf("17 tiny configuration maps exceeded metadata budget because of discarded comments: %v (entries=%d)", err, len(inventory.Kustomize.Entries))
	}
}

func TestRenderIncludesDependenciesAddedAfterDiscovery(t *testing.T) {
	root := sourceFiles(t, map[string]string{"kustomization.yaml": "resources: []\n"})
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	for name, data := range map[string]string{
		"kustomization.yaml":     "resources: [new]\n",
		"new/kustomization.yaml": "resources: [app.yaml]\n",
		"new/app.yaml":           "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: added}\n",
	} {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (Renderer{}).Render(t.Context(), inventory, "kustomize:.", Inputs{})
	if err != nil || len(result.Resources) != 1 || result.Resources[0].Name != "added" {
		t.Fatal(result, err)
	}
	if len(result.CapturedSource().Source.Files) != 3 {
		t.Fatal("unexpected dependencies")
	}
	if err := os.WriteFile(filepath.Join(root, "new/Kustomization"), []byte("resources: []"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(t.Context(), inventory, "kustomize:.", Inputs{}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("accepted ambiguous dependency", err)
	}
}

func TestTransformerReferencesMatchNativeRendering(t *testing.T) {
	transformer := "apiVersion: builtin\nkind: PatchTransformer\nmetadata: {name: update}\npath: patch.yaml\ntarget: {kind: ConfigMap, name: app}\n"
	for _, mode := range []string{"nested", "inline", "multiple"} {
		t.Run(mode, func(t *testing.T) {
			files := map[string]string{
				"kustomization.yaml":      "resources: [app.yaml]\ntransformers: [nested/transformer.yaml]\n",
				"app.yaml":                "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {mode: old}\n",
				"nested/transformer.yaml": transformer,
				"patch.yaml":              "- op: replace\n  path: /data/mode\n  value: updated\n",
			}
			if mode == "inline" {
				files["kustomization.yaml"] = "resources: [app.yaml]\ntransformers:\n- |\n  " + strings.ReplaceAll(strings.TrimSpace(transformer), "\n", "\n  ") + "\n"
				delete(files, "nested/transformer.yaml")
			}
			if mode == "multiple" {
				files["nested/transformer.yaml"] = transformer + "---\n" + strings.Replace(transformer, "name: update", "name: update-again", 1)
			}
			raw := map[string][]byte{}
			for name, data := range files {
				raw[name] = []byte(data)
			}
			expected, err := kustomize.Build(t.Context(), kustomize.Discover(raw), ".", nil)
			if err != nil {
				t.Fatal("native fixture", err)
			}
			root := sourceFiles(t, files)
			snapshot, err := Load(t.Context(), Config{Path: root})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{})
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(expected.Resources[0].Object) != fmt.Sprint(actual.Resources[0].Object) {
				t.Fatal("selected inputs changed native result")
			}
			if err := os.Remove(filepath.Join(root, "patch.yaml")); err != nil {
				t.Fatal(err)
			}
			if _, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{}); err == nil || !strings.Contains(err.Error(), "patch.yaml") {
				t.Fatal("missing transformer input not identified", err)
			}
		})
	}
}

func TestDiscoveryStopsForRetainedConfigurationMetadata(t *testing.T) {
	files := map[string]string{}
	body := "commonAnnotations:\n  note: " + strings.Repeat("x", maxFile-128) + "\n"
	for n := range 18 {
		files[fmt.Sprintf("env-%02d/kustomization.yaml", n)] = body
	}
	root := sourceFiles(t, files)
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := DiscoverContext(t.Context(), snapshot)
	if err == nil || !strings.Contains(err.Error(), "metadata exceeds") || !inventory.Discovering || len(inventory.Kustomize.Entries) >= 18 {
		t.Fatal("retained metadata was not bounded incrementally", err, len(inventory.Kustomize.Entries))
	}
}

func TestHelmRenderReadsNewChartFilesAndIgnoredValues(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"chart/Chart.yaml":         "apiVersion: v2\nname: shop\nversion: 0.1.0\n",
		"chart/templates/app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: shop}\ndata:\n  mode: {{ .Values.mode | quote }}\n  content: {{ .Files.Get \"data.txt\" | quote }}\n",
		"chart/data.txt":           "old",
		"values.yaml":              "mode: updated\n",
	})
	snapshot, err := Load(t.Context(), Config{Path: root, ScanRules: func() ([]byte, error) { return []byte("values.yaml\n*.txt\n"), nil }})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	if err := os.WriteFile(filepath.Join(root, "chart/data.txt"), []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chart/templates/new.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: added}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := (Renderer{}).Render(t.Context(), inventory, "helm:chart", Inputs{Values: []string{"values.yaml"}})
	if err != nil || len(result.Resources) != 2 {
		t.Fatal(result, err)
	}
	captured := result.CapturedSource().Source.Files
	if string(captured["chart/data.txt"]) != "current" || captured["chart/templates/new.yaml"] == nil || captured["values.yaml"] == nil {
		t.Fatal("missing current chart inputs")
	}
}

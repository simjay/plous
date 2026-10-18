package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	helm "github.com/simjay/plous/internal/integrations/helm/common"
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
	"github.com/simjay/plous/internal/integrations/manifest"
)

func TestIgnoredUnreadableChild(t *testing.T) {
	for _, mode := range []string{"global", "repository"} {
		t.Run(mode, func(t *testing.T) {
			scope, root := readScope(t)
			blocked := filepath.Join(root, "archives")
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "app.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(blocked, 0000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(blocked, 0700) })
			if f, err := os.Open(blocked); err == nil {
				f.Close()
				t.Skip("cannot reproduce access denial with this OS identity")
			}
			policy := func() ([]byte, error) { return []byte("archives/\n"), nil }
			if mode == "repository" {
				if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("archives/\n"), 0600); err != nil {
					t.Fatal(err)
				}
				policy = nil
			}
			scope.ScanRules = policy
			if _, err := Load(t.Context(), Config{Path: root, ScanRules: policy}); err != nil {
				t.Errorf("discovery opened excluded directory: %v", err)
			}
			if _, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "list"}); err != nil {
				t.Errorf("list opened excluded directory: %v", err)
			}
			if _, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "search", Query: "app"}); err != nil {
				t.Errorf("search opened excluded directory: %v", err)
			}
		})
	}
}

func TestHelmIgnoredOversizedFile(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"chart/Chart.yaml":         "apiVersion: v2\nname: sample\nversion: 0.1.0\n",
		"chart/templates/app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\n",
		"chart/.helmignore":        "archive.bin\n",
		"chart/archive.bin":        strings.Repeat("x", maxFile+1),
	})
	native, err := helm.Render(t.Context(), helm.TemplateRequest{ChartPath: filepath.Join(root, "chart"), Release: "sample"})
	if err != nil || !strings.Contains(string(native), "name: app") {
		t.Fatalf("native control failed: %v", err)
	}
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "helm:chart", Inputs{}); err != nil {
		t.Fatalf("valid chart rejected before Helm could exclude input: %v", err)
	}
}

func TestBuiltinTransformerFilesMatchNativeRendering(t *testing.T) {
	replacement := "source: {kind: ConfigMap, name: app, fieldPath: data.source}\ntargets:\n- select: {kind: ConfigMap, name: app}\n  fieldPaths: [data.target]\n"
	for _, fixture := range []struct{ kind, config, dependency string }{
		{"PatchTransformer", "path: dependency.yaml\ntarget: {kind: ConfigMap, name: app}\n", "- op: replace\n  path: /data/target\n  value: updated\n"},
		{"PatchJson6902Transformer", "path: dependency.yaml\ntarget: {version: v1, kind: ConfigMap, name: app}\n", "- op: replace\n  path: /data/target\n  value: updated\n"},
		{"PatchStrategicMergeTransformer", "paths: [dependency.yaml]\n", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {target: updated}\n"},
		{"ReplacementTransformer", "replacements: [{path: dependency.yaml}]\n", replacement},
		{"ReplacementTransformer", "replacements: [{path: dependency.yaml}]\n", "- " + strings.ReplaceAll(strings.TrimSpace(replacement), "\n", "\n  ") + "\n"},
		{"ValueAddTransformer", "value: updated\ntargetFilePath: dependency.yaml\n", "targets:\n- selector: {kind: ConfigMap, name: app}\n  fieldPath: data.target\n"},
	} {
		for _, mode := range []string{"file", "inline", "multiple"} {
			t.Run(fmt.Sprintf("%s/%s", fixture.kind, mode), func(t *testing.T) {
				transformer := "apiVersion: builtin\nkind: " + fixture.kind + "\nmetadata: {name: update}\n" + fixture.config
				files := map[string]string{
					"kustomization.yaml":      "resources: [app.yaml]\ntransformers: [nested/transformer.yaml]\n",
					"app.yaml":                "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {source: updated, target: old}\n",
					"nested/transformer.yaml": transformer,
					"dependency.yaml":         fixture.dependency,
				}
				if mode == "inline" {
					files["kustomization.yaml"] = "resources: [app.yaml]\ntransformers:\n- |\n  " + strings.ReplaceAll(strings.TrimSpace(transformer), "\n", "\n  ") + "\n"
					delete(files, "nested/transformer.yaml")
				}
				if mode == "multiple" {
					files["nested/transformer.yaml"] += "---\n" + strings.Replace(transformer, "name: update", "name: second", 1)
				}
				raw := map[string][]byte{}
				for name, body := range files {
					raw[name] = []byte(body)
				}
				native, err := kustomize.Build(t.Context(), kustomize.Discover(raw), ".", nil)
				if err != nil || len(native.Resources) != 1 {
					t.Fatalf("native control failed: %v", err)
				}
				root := sourceFiles(t, files)
				snapshot, err := Load(t.Context(), Config{Path: root})
				if err != nil {
					t.Fatal(err)
				}
				actual, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{})
				if err != nil {
					t.Fatalf("supported transformer input not collected: %v", err)
				}
				if len(actual.Resources) != 1 || !reflect.DeepEqual(native.Resources[0].Object, actual.Resources[0].Object) {
					t.Fatal("selected inputs changed native output")
				}
				if actual.CapturedSource().Source.Files["dependency.yaml"] == nil {
					t.Fatal("missing dependency")
				}
				if err := os.Remove(filepath.Join(root, "dependency.yaml")); err != nil {
					t.Fatal(err)
				}
				if _, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{}); err == nil || !strings.Contains(err.Error(), "dependency.yaml") {
					t.Fatal("missing input not identified", err)
				}
			})
		}
	}
}

func TestHelmCollectionUsesChartRules(t *testing.T) {
	for _, chartRoot := range []string{".", "chart"} {
		t.Run(chartRoot, func(t *testing.T) {
			files := map[string]string{
				"Chart.yaml":                      "apiVersion: v2\nname: sample\nversion: 0.1.0\n",
				".helmignore":                     "archive/\n*.bin\n",
				"archive/huge.txt":                strings.Repeat("x", maxFile+1),
				"payload.bin":                     strings.Repeat("x", maxFile+1),
				"templates/.hidden":               strings.Repeat("x", maxFile+1),
				"data.txt":                        "included",
				"templates/app.yaml":              "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: sample}\ndata:\n  value: {{ .Files.Get \"data.txt\" | quote }}\n",
				"charts/child/Chart.yaml":         "apiVersion: v2\nname: child\nversion: 0.1.0\n",
				"charts/child/.helmignore":        "templates/\n",
				"charts/child/templates/app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: child}\n",
			}
			selected := map[string]string{}
			for name, body := range files {
				selected[filepath.ToSlash(filepath.Join(chartRoot, name))] = body
			}
			root := sourceFiles(t, selected)
			native, err := helm.Render(t.Context(), helm.TemplateRequest{ChartPath: filepath.Join(root, chartRoot), Release: "sample"})
			if err != nil {
				t.Fatal("native control", err)
			}
			snapshot, err := Load(t.Context(), Config{Path: root})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "helm:"+chartRoot, Inputs{})
			if err != nil {
				t.Fatal(err)
			}
			expected, err := manifest.Resources(string(native))
			if err != nil || len(actual.Resources) != len(expected) {
				t.Fatal("different rendered resources", err)
			}
			for i := range expected {
				if !reflect.DeepEqual(expected[i].Object, actual.Resources[i].Object) {
					t.Fatal("selected chart differs from native rendering")
				}
			}
			for _, name := range []string{"archive/huge.txt", "payload.bin", "templates/.hidden"} {
				if _, ok := actual.CapturedSource().Source.Files[filepath.ToSlash(filepath.Join(chartRoot, name))]; ok {
					t.Fatalf("collected Helm-excluded input %s", name)
				}
			}
		})
	}
}

func TestStrategicTransformerInlinePaths(t *testing.T) {
	for _, patch := range []string{
		"{apiVersion: v1, kind: ConfigMap, metadata: {name: app}, data: {target: updated}}",
		"apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {target: updated}\n",
	} {
		config, err := json.Marshal(map[string]any{"apiVersion": "builtin", "kind": "PatchStrategicMergeTransformer", "metadata": map[string]string{"name": "update"}, "paths": []string{patch}})
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"kustomization.yaml": "resources: [app.yaml]\ntransformers: [transformer.yaml]\n",
			"transformer.yaml":   string(config),
			"app.yaml":           "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\ndata: {target: old}\n",
		}
		snapshot, err := Load(t.Context(), Config{Path: sourceFiles(t, files)})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "kustomize:.", Inputs{})
		if err != nil {
			t.Fatal(err)
		}
		if actual.Resources[0].Object["data"].(map[string]any)["target"] != "updated" {
			t.Fatal("inline patch not applied")
		}
	}
}

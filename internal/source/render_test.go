package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func mixed(t *testing.T) *Inventory {
	t.Helper()
	s, err := Load(context.Background(), Config{Path: "../../tests/fixtures/code/mixed"})
	if err != nil {
		t.Fatal(err)
	}
	return Discover(s)
}
func TestMixedDiscoveryNeverChoosesOneRepositoryType(t *testing.T) {
	i := mixed(t)
	for _, id := range []string{"plain:plain/service.yaml", "helm:chart", "kustomize:base", "kustomize:prod", "kustomize:composed"} {
		if i.Entry(id) == nil {
			t.Fatal("undetected", id)
		}
	}
	if i.Entry("kustomize:composed").Kind != "kustomize+helm" || len(i.Terraform) != 1 {
		t.Fatal("lost combined or Terraform detection")
	}
	if i.Entry("plain:chart/templates/deployment.yaml") != nil {
		t.Fatal("template treated as plain YAML")
	}
	if len(i.Consumers("kustomize:base")) != 1 || len(i.Affected("plain:base/deployment.yaml")) != 2 {
		t.Fatal("incorrect dependency graph", i.Description("kustomize:base"))
	}
}

func TestAffectedConsumersCountSharedTargetsOnce(t *testing.T) {
	i := Discover(&Snapshot{Files: map[string][]byte{
		"base/kustomization.yaml":  []byte("resources: []\n"),
		"left/kustomization.yaml":  []byte("resources: [../base]\n"),
		"right/kustomization.yaml": []byte("resources: [../base]\n"),
		"top/kustomization.yaml":   []byte("resources: [../left, ../right]\n"),
	}})
	if affected := i.Affected("kustomize:base"); strings.Join(affected, ",") != "kustomize:left,kustomize:right,kustomize:top" {
		t.Fatal("shared consumers must appear once", affected)
	}
}
func TestPlainTraceUsesActualLinesAndDoesNotLeakSecretValues(t *testing.T) {
	i := mixed(t)
	r, err := (Renderer{}).Render(context.Background(), i, "plain:plain/service.yaml", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range Trace(i, r, r.Resources[0]) {
		if f.Pointer == "/metadata/name" {
			found = true
			if f.File != "plain/service.yaml" || f.Line != 4 || f.Evidence != "Exact source field" {
				t.Fatal(f)
			}
		}
	}
	if !found {
		t.Fatal("field missing")
	}
	r.SourceIdentity = "old"
	for _, f := range Trace(i, r, r.Resources[0]) {
		if f.File != "plain/service.yaml" || f.Line == 0 {
			t.Fatal("captured output lost its original source", f)
		}
	}
}

func TestImageFindingsInspectContainersAndValidateDigests(t *testing.T) {
	data := "apiVersion: v1\nkind: Pod\nmetadata: {name: api}\nspec:\n  containers:\n    - {name: tagged, image: 'demo:1.2'}\n    - {name: malformed, image: 'demo@sha256:bad'}\n    - {name: pinned, image: 'demo@sha256:" + strings.Repeat("a", 64) + "'}\n  initContainers:\n    - {name: pinned, image: 'init@sha512:" + strings.Repeat("b", 128) + "'}\n---\napiVersion: v1\nkind: Secret\nmetadata: {name: unrelated}\nstringData: {image: private-value}\n"
	i := Discover(&Snapshot{Files: map[string][]byte{"pods.yaml": []byte(data)}})
	r, err := (Renderer{}).Render(context.Background(), i, "plain:pods.yaml", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 2 {
		t.Fatalf("expected findings only for tagged and malformed container references: %v", r.Findings)
	}
	for _, finding := range r.Findings {
		if strings.Contains(finding.Message, "private-value") || strings.Contains(finding.Message, "init@") {
			t.Fatal("reported a non-container field or valid content digest", finding)
		}
	}
}
func TestHelmOriginResolvesOnlyInsideItsCapturedChart(t *testing.T) {
	i := mixed(t)
	e := i.Entry("helm:chart")
	captureSourceFixture(t, i.Source)
	i.Source.Files["demo/templates/missing.yaml"] = []byte("unrelated file")
	for origin, want := range map[string]string{
		"demo/templates/deployment.yaml":  "chart/templates/deployment.yaml",
		"demo/templates/missing.yaml":     "",
		"other/templates/deployment.yaml": "",
		"demo/../plain/service.yaml":      "",
	} {
		if got := capturedHelmOrigin(i, e, origin); got != want {
			t.Fatalf("%s resolved to %q, want %q", origin, got, want)
		}
	}
}

func TestNativeHelmAndComposedOutputAndSourcePreservation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	i := mixed(t)
	identity := i.Source.Identity
	renderer := Renderer{}
	cases := []struct {
		id       string
		inputs   Inputs
		name     string
		replicas int
	}{
		{"helm:chart", Inputs{Release: "custom", Values: []string{"inputs/one.yaml", "inputs/two.yaml"}, Overrides: []string{"replicas=5"}}, "custom", 5},
		{"kustomize:prod", Inputs{}, "prod-api", 6},
		{"kustomize:composed", Inputs{}, "composed-demo", 6},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			r, err := renderer.Render(context.Background(), i, tc.id, tc.inputs)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Resources) != 1 || r.Resources[0].Name != tc.name {
				t.Fatal(r.Resources)
			}
			spec := r.Resources[0].Object["spec"].(map[string]any)
			if spec["replicas"] != float64(tc.replicas) {
				t.Fatal(spec)
			}
			if r.Engine == "" || r.OperationID == "" {
				t.Fatal("missing engine/input identity")
			}
			if tc.id == "helm:chart" && r.Origins[r.Resources[0].Key()] != "chart/templates/deployment.yaml" {
				t.Fatal("chart origin does not resolve to the captured template", r.Origins)
			}
			if tc.id == "kustomize:prod" {
				native, err := kustomize.Build(context.Background(), kustomize.Discover(r.CapturedSource().Source.Files), "prod", nil)
				if err != nil {
					t.Fatal(err)
				}
				if manifest.JSON(native.Resources[0].Object) != manifest.JSON(r.Resources[0].Object) {
					t.Fatal("trace changed authoritative output")
				}
				for _, f := range Trace(i, r, r.Resources[0]) {
					if f.Line != 0 || strings.Contains(f.Evidence, "Exact") {
						t.Fatal("invented field writer", f)
					}
				}
			}
		})
	}
	after, err := Load(context.Background(), i.Source.Config)
	if err != nil || after.Identity != identity {
		t.Fatal("renderer mutated source")
	}
}
func TestMissingReferencesCyclesAndPartialSources(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"a/kustomization.yaml": "resources: [../b, missing.yaml]\n", "b/kustomization.yaml": "resources: [../a]\n"} {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(data), 0600)
	}
	s, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	i := Discover(s)
	rules := map[string]bool{}
	for _, f := range i.Findings {
		rules[f.Rule] = true
	}
	if !rules["cycle"] || !rules["reference"] {
		t.Fatal(i.Findings)
	}
	if _, err = (Renderer{}).Render(context.Background(), i, "kustomize:a", Inputs{}); err == nil {
		t.Fatal("rendered broken dependency graph")
	}
}

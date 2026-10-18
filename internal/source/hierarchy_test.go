package source

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func hierarchyFixture(t *testing.T, files map[string]string) *Inventory {
	t.Helper()
	directory := t.TempDir()
	for name, text := range files {
		file := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := Load(context.Background(), Config{Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	return Discover(snapshot)
}

func TestHierarchyUsesDeclaredEdgesAndCapturedCharts(t *testing.T) {
	i := hierarchyFixture(t, map[string]string{
		"base/kustomization.yaml":      "resources: [../qa, ../qa]\ncomponents: [../shared]\npatches: [{path: ../unrelated}]\n",
		"staging/kustomization.yaml":   "bases: [../qa]\ncomponents: [../shared]\n",
		"qa/kustomization.yaml":        "helmGlobals: {chartHome: vendor}\nhelmCharts:\n- name: demo\n  version: 9.0.0\n  repo: https://charts.example.invalid\n",
		"qa/vendor/demo/Chart.yaml":    "apiVersion: v2\nname: captured-demo\nversion: 1.2.3\n",
		"shared/kustomization.yaml":    "apiVersion: kustomize.config.k8s.io/v1alpha1\nkind: Component\nhelmCharts:\n- name: remote\n  version: 4.5.6\n  repo: https://user:password@charts.example.invalid/private?token=secret#fragment\n",
		"unrelated/kustomization.yaml": "resources: []\n",
		"chart/Chart.yaml":             "apiVersion: v2\nname: standalone\nversion: 7.8.9\n",
	})
	t.Setenv("PATH", t.TempDir())
	before, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		id                             string
		dependencies, consumers, roles []string
	}{
		{"kustomize:base", []string{"kustomize:qa", "kustomize:shared"}, nil, []string{"overlay"}},
		{"kustomize:qa", []string{"helm:qa/vendor/demo"}, []string{"kustomize:base", "kustomize:staging"}, []string{"base", "shared"}},
		{"kustomize:shared", nil, []string{"kustomize:base", "kustomize:staging"}, []string{"component", "shared"}},
		{"helm:qa/vendor/demo", nil, []string{"kustomize:qa"}, []string{"entry"}},
		{"kustomize:unrelated", nil, nil, []string{"entry"}},
		{"helm:chart", nil, nil, []string{"entry"}},
		{"absent", nil, nil, nil},
	}
	for _, check := range checks {
		if got := i.EntryDependencies(check.id); !slices.Equal(got, check.dependencies) {
			t.Errorf("%s dependencies = %v, want %v", check.id, got, check.dependencies)
		}
		if got := i.EntryConsumers(check.id); !slices.Equal(got, check.consumers) {
			t.Errorf("%s consumers = %v, want %v", check.id, got, check.consumers)
		}
		if got := i.EntryRoles(check.id); !slices.Equal(got, check.roles) {
			t.Errorf("%s roles = %v, want %v", check.id, got, check.roles)
		}
	}
	charts := i.EntryHelmCharts("kustomize:base")
	if len(charts) != 2 {
		t.Fatalf("expected local and remote inherited charts: %+v", charts)
	}
	local, remote := charts[0], charts[1]
	if local.Name != "captured-demo" || local.Version != "1.2.3" || local.DeclaredName != "demo" || local.DeclaredVersion != "9.0.0" || local.Path != "qa/vendor/demo" || local.EntryID != "helm:qa/vendor/demo" || local.Status != "captured" || local.DeclaredBy != "kustomize:qa" || !local.Inherited {
		t.Fatalf("wrong captured metadata: %+v", local)
	}
	if remote.Name != "remote" || remote.Version != "4.5.6" || remote.Repository != "https://charts.example.invalid/private" || remote.EntryID != "" || remote.Status != "remote" || remote.DeclaredBy != "kustomize:shared" || !remote.Inherited {
		t.Fatalf("wrong declaration metadata: %+v", remote)
	}
	if direct := i.EntryHelmCharts("kustomize:qa"); len(direct) != 1 || direct[0].Inherited {
		t.Fatalf("own declaration marked inherited: %+v", direct)
	}
	standalone := i.EntryHelmCharts("helm:chart")
	if len(standalone) != 1 || standalone[0].Name != "standalone" || standalone[0].Version != "7.8.9" || standalone[0].Path != "chart" || standalone[0].DeclaredBy != "helm:chart" || standalone[0].Status != "captured" || standalone[0].Inherited {
		t.Fatalf("wrong standalone chart metadata: %+v", standalone)
	}
	if i.Entry("helm:shared/charts/remote") != nil {
		t.Fatal("remote declaration became a standalone entry")
	}
	after, err := json.Marshal(i)
	if err != nil || string(before) != string(after) {
		t.Fatalf("hierarchy mutated captured inventory: %v", err)
	}
}

func TestHierarchySurvivesCyclesAndMissingInputs(t *testing.T) {
	i := hierarchyFixture(t, map[string]string{
		"a/kustomization.yaml":             "resources: [../b, ../c, ../missing, ../../outside, 'https://example.invalid/base']\nhelmCharts: [{name: absent}]\n",
		"b/kustomization.yaml":             "resources: [../a, ../c]\n",
		"c/kustomization.yaml":             "helmCharts: [{name: remote, version: 1.0.0, repo: 'https://example.invalid'}]\n",
		"outside-home/kustomization.yaml":  "helmGlobals: {chartHome: ../elsewhere}\nhelmCharts: [{name: demo}]\n",
		"bad-name/kustomization.yaml":      "helmCharts: [{name: ../demo}, malformed]\n",
		"bad-local/kustomization.yaml":     "helmCharts: [{name: demo}]\n",
		"bad-local/charts/demo/Chart.yaml": "name: [not, a, name]\n",
	})
	if got := i.EntryDependencies("kustomize:a"); !slices.Equal(got, []string{"kustomize:b", "kustomize:c"}) {
		t.Fatal("missing or external references became captured edges", got)
	}
	charts := i.EntryHelmCharts("kustomize:a")
	if len(charts) != 2 || charts[0].Status != "missing" || charts[0].Path != "a/charts/absent" || charts[0].Inherited || charts[1].Status != "remote" || !charts[1].Inherited {
		t.Fatalf("cyclic/shared traversal lost or duplicated chart declarations: %+v", charts)
	}
	for id, status := range map[string]string{
		"kustomize:outside-home":     "out-of-scope",
		"kustomize:bad-name":         "invalid",
		"kustomize:bad-local":        "invalid",
		"helm:bad-local/charts/demo": "invalid",
	} {
		for _, chart := range i.EntryHelmCharts(id) {
			if chart.Status != status || chart.Error == "" {
				t.Errorf("%s: expected %s and reason, got %+v", id, status, chart)
			}
		}
	}
	if got := i.EntryHelmCharts("missing"); got != nil {
		t.Fatal("missing entry produced chart metadata", got)
	}
}

func TestHierarchyRepositoryLabelsDoNotExposeCredentials(t *testing.T) {
	for input, want := range map[string]string{
		"https://secret%xx@example.invalid/charts":                       "[invalid repository URL]",
		"https://secret@example.invalid/charts":                          "https://example.invalid/charts",
		"oci://name:password@example.invalid/charts?token=secret#secret": "oci://example.invalid/charts",
		"https://example.invalid/charts":                                 "https://example.invalid/charts",
	} {
		if got := chartRepositoryLabel(input); got != want || strings.Contains(got, "secret") || strings.Contains(got, "password") {
			t.Errorf("repository label = %q, want %q", got, want)
		}
	}
}

func TestSourceDisplayTextMasksEmbeddedRepositoryCredentials(t *testing.T) {
	for input, want := range map[string]string{
		"resources: \"https://user:pass'word@example.invalid/base?token=secret\"":                        "resources: \"https://example.invalid/base\"",
		"resources: ['https://user:password@example.invalid/base?token=secret']":                         "resources: ['https://example.invalid/base']",
		"resources: https://username-token@example.invalid/base?token=query-token#fragment: unavailable": "resources: https://example.invalid/base: unavailable",
		"Missing (https://name:password@example.invalid/base).":                                          "Missing (https://example.invalid/base).",
		"https://username-token%xx@example.invalid/base: unavailable":                                    "[invalid repository URL]: unavailable",
		"file:///tmp/source.yaml: missing":                                                               "file:///tmp/source.yaml: missing",
		"base/config.yaml: missing":                                                                      "base/config.yaml: missing",
	} {
		if got := DisplayText(input); got != want {
			t.Errorf("source display text = %q, want %q", got, want)
		}
	}
}

func TestHierarchyDoesNotTreatDirectoryNamesAsRoles(t *testing.T) {
	i := hierarchyFixture(t, map[string]string{
		"base/kustomization.yaml":     "resources: []\n",
		"overlays/kustomization.yaml": "resources: []\n",
		"shared/kustomization.yaml":   "resources: []\n",
	})
	for _, id := range []string{"kustomize:base", "kustomize:overlays", "kustomize:shared"} {
		if got := i.EntryRoles(id); !reflect.DeepEqual(got, []string{"entry"}) {
			t.Errorf("%s inferred roles from its path: %v", id, got)
		}
	}
}

func TestHierarchyIntermediateNodeIsBothOverlayAndBase(t *testing.T) {
	i := hierarchyFixture(t, map[string]string{
		"z/kustomization.yaml": "resources: []\n",
		"x/kustomization.yaml": "resources: [../z]\n",
		"y/kustomization.yaml": "resources: [../x]\n",
		"w/kustomization.yaml": "resources: [../x]\n",
	})
	if got := i.EntryRoles("kustomize:x"); !slices.Equal(got, []string{"overlay", "base", "shared"}) {
		t.Fatalf("intermediate node lost its role for consumers: %v", got)
	}
}

func TestHierarchyExplainsUncapturedChartRenderRequirements(t *testing.T) {
	for _, version := range []string{"", "^1.0.0"} {
		t.Run(version, func(t *testing.T) {
			i := hierarchyFixture(t, map[string]string{
				"kustomization.yaml": "helmCharts:\n- name: remote\n  repo: https://example.invalid/charts\n  version: '" + version + "'\n",
			})
			charts := i.EntryHelmCharts("kustomize:.")
			if len(charts) != 1 || charts[0].Status != "remote" || !strings.Contains(charts[0].Error, "exact version") || charts[0].EntryID != "" {
				t.Fatalf("missing uncaptured chart limitation: %+v", charts)
			}
			_, err := (Renderer{}).Render(context.Background(), i, "kustomize:.", Inputs{})
			if err == nil || !strings.Contains(err.Error(), "exact chart version") {
				t.Fatalf("metadata disagrees with renderer: %v", err)
			}
		})
	}
}

func TestHierarchyReadsScalarChartVersionsLikeHelm(t *testing.T) {
	i := hierarchyFixture(t, map[string]string{
		"Chart.yaml":               "apiVersion: v2\nname: scalar-version\nversion: 1.2\nx-custom: metadata\n",
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: test}\ndata:\n  version: {{ .Chart.Version | quote }}\n",
	})
	charts := i.EntryHelmCharts("helm:.")
	if len(charts) != 1 || charts[0].Version != "1.2" || charts[0].Status != "captured" {
		t.Fatalf("scalar version was lost: %+v", charts)
	}
	out, err := (Renderer{}).Render(context.Background(), i, "helm:.", Inputs{})
	if err != nil || len(out.Resources) != 1 {
		t.Fatalf("metadata disagrees with Helm output: %v, %+v", err, out)
	}
	data, _ := out.Resources[0].Object["data"].(map[string]any)
	if data["version"] != charts[0].Version {
		t.Fatalf("rendered chart version %v differs from metadata %s", data["version"], charts[0].Version)
	}
}

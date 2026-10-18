package source

import (
	"slices"
	"strings"
	"testing"
)

func catalogFixture(files map[string]string) *Inventory {
	s := &Snapshot{Files: map[string][]byte{}}
	for name, data := range files {
		s.Files[name] = []byte(data)
	}
	return Discover(s)
}
func TestCatalogMixedDocumentsAndTechnologyTags(t *testing.T) {
	i := catalogFixture(map[string]string{
		".fluxschema.yml":                  "apiVersion: schema.plugin.fluxcd.io/v1beta1\nkind: Config\nvalidate: {}\n",
		"apps/base/kustomization.yaml":     "resources: [release.yaml]\n",
		"apps/base/release.yaml":           "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata: {name: api, namespace: team}\nspec:\n  chart:\n    spec:\n      chart: api\n      version: 1.2.3\n      sourceRef: {kind: HelmRepository, name: charts}\n---\napiVersion: source.toolkit.fluxcd.io/v1\nkind: HelmRepository\nmetadata: {name: charts, namespace: team}\nspec: {url: 'https://user:secret@example.test/charts?token=hidden'}\n",
		"apps/prod/kustomization.yaml":     "resources: [../base]\n",
		"clusters/prod/kustomization.yaml": "apiVersion: kustomize.toolkit.fluxcd.io/v1\nkind: Kustomization\nmetadata: {name: apps}\nspec: {path: ./apps/prod, sourceRef: {kind: GitRepository, name: repo}}\n",
		"argo/apps.yaml":                   "apiVersion: argoproj.io/v1alpha1\nkind: Application\nmetadata: {name: api}\nspec:\n  sources:\n  - repoURL: https://example.test/charts\n    chart: api\n    targetRevision: 1.0.0\n  - repoURL: https://example.test/git\n    path: apps/prod\n",
	})
	if i.Entry("plain:.fluxschema.yml") != nil {
		t.Fatal("tool config became a render target")
	}
	if i.Entry("kustomize:clusters/prod") != nil {
		t.Fatal("Flux object became a native Kustomize build")
	}
	if i.DirectoryTags["apps"] != "H K F" || i.DirectoryTags["argo"] != "H A" || i.DirectoryTags["clusters"] != "F" {
		t.Fatalf("tags: %#v", i.DirectoryTags)
	}
	counts := map[string]int{}
	for _, item := range i.Catalog {
		counts[item.Technology]++
		if strings.Contains(item.Location, "secret") || strings.Contains(item.Location, "hidden") {
			t.Fatal("credentials in catalog")
		}
		if item.Technology == "kustomize" && item.Group != "apps" {
			t.Fatalf("related configurations split: %#v", item)
		}
	}
	if counts["helm"] != 2 || counts["kustomize"] != 2 || counts["flux"] != 3 || counts["argo"] != 1 {
		t.Fatalf("counts: %#v", counts)
	}
}
func TestCatalogRenderTargetsUseOwnershipAndActualChartReferences(t *testing.T) {
	i := catalogFixture(map[string]string{
		"app/kustomization.yaml":                   "helmCharts:\n- name: api\n  version: 1.0.0\nresources: [../shared/service.yaml]\n",
		"app/charts/api/Chart.yaml":                "apiVersion: v2\nname: api\nversion: 1.0.0\n",
		"app/charts/api/templates/deployment.yaml": "kind: Deployment\n",
		"app/unrelated/Chart.yaml":                 "apiVersion: v2\nname: other\nversion: 1.0.0\n",
		"shared/service.yaml":                      "apiVersion: v1\nkind: Service\nmetadata: {name: api}\n",
		"prod/kustomization.yaml":                  "resources: [../app]\n",
	})
	ids := func(file string, dir bool) []string {
		var out []string
		for _, e := range i.RenderTargets(file, dir) {
			out = append(out, e.ID)
		}
		return out
	}
	if got := ids("app/charts/api/templates/deployment.yaml", false); !slices.Equal(got, []string{"helm:app/charts/api", "kustomize:app"}) {
		t.Fatalf("chart choice: %v", got)
	}
	if got := ids("app/unrelated", true); !slices.Equal(got, []string{"helm:app/unrelated"}) {
		t.Fatalf("nested unrelated chart promoted: %v", got)
	}
	if got := ids("shared/service.yaml", false); !slices.Equal(got, []string{"kustomize:app"}) {
		t.Fatalf("shared file owner: %v", got)
	}
	if got := ids("app", true); !slices.Equal(got, []string{"kustomize:app"}) {
		t.Fatalf("base promoted to overlay: %v", got)
	}
	if got := ids(".", true); len(got) != 4 {
		t.Fatalf("root targets: %v", got)
	}
}

func TestCatalogPatchIsTaggedWithoutAnotherControllerCard(t *testing.T) {
	i := catalogFixture(map[string]string{
		"prod/kustomization.yaml": "resources: []\npatches: [{path: values.yaml}]\n",
		"prod/values.yaml":        "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata: {name: api}\nspec: {values: {replicas: 2}}\n",
	})
	if i.DirectoryTags["prod"] != "K F" {
		t.Fatal(i.DirectoryTags)
	}
	if len(i.Catalog) != 1 || i.Catalog[0].Technology != "kustomize" {
		t.Fatal("patch became a second controller", i.Catalog)
	}
}

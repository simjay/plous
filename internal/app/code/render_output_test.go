package code

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestRenderedResourceTableIdentifiesKindsNamespacesAndScope(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{"kustomization.yaml": "resources: []"}, "kustomize:.")
	s.Preference.Section = 1
	resources := []manifest.Resource{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "east"},
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "west"},
		{APIVersion: "v1", Kind: "Service", Name: "api"},
		{APIVersion: "v1", Kind: "Namespace", Name: "east"},
		{APIVersion: "example.invalid/v1", Kind: "Extension", Name: "unknown"},
	}
	s.Builds[s.Preference.Selected] = &source.Result{ID: s.Preference.Selected, BuildResult: &source.BuildResult{Resources: resources}}
	for _, width := range []int{30, 62} {
		view := s.ResourceNavigation(width, 20)
		plain := ansi.Strip(view)
		for _, want := range []string{"Kind", "Name", "Namespace", "east", "west", "Cluster-scoped", "Not specified"} {
			if !strings.Contains(plain, want) {
				t.Fatal("missing resource identity", width, want, plain)
			}
		}
		_, spans := textview.MouseLayout(view)
		for _, span := range spans {
			if strings.HasPrefix(span.Target, "sort/") {
				t.Fatal("table advertised unsupported sorting")
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("resource table overflow", width)
			}
		}
	}
	for index, r := range s.Resources() {
		s.Cursor = index
		s.RememberSelection()
		s.Cursor = 0
		s.RestoreCursor()
		selected, ok := s.CurrentResource()
		if !ok || selected.Key() != r.Key() {
			t.Fatal("same-name resources lost exact selection", selected, r)
		}
	}
	if resourceNamespaceLabel(resources[2]) != "Not specified" || resourceNamespaceLabel(resources[3]) != "Cluster-scoped" || resourceNamespaceLabel(resources[4]) != "Not specified" {
		t.Fatal("missing namespace was treated as a discovered cluster scope")
	}
}

func TestUnrenderedKustomizeOutputDoesNotClaimItIsLoading(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{"kustomization.yaml": "resources: []"}, "kustomize:.")
	s.View, s.ReturnView, s.Preference.Section = "manifests", "code-kustomize", 1
	view := ansi.Strip(s.RenderInspector(100, 20))
	if !strings.Contains(view, "Not rendered yet") || !strings.Contains(view, "Use r") || strings.Contains(view, "Preparing preview") || strings.Contains(view, "No generated resources") {
		t.Fatal("unrendered output has a misleading loading or empty state", view)
	}
}

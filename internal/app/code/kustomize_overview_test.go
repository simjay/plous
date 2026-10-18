package code

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKustomizeOverviewKeepsFailureReadableWithoutRepeatedMetadata(t *testing.T) {
	s := kustomizeGraphFixture(t)
	id := "kustomize:clusters/prod-ap-south"
	s.OpenKustomizeOverview(id)
	s.Failures = map[string]string{id: `no resource matches strategic merge patch "Deployment.v1.apps/api-v1.[noNs]": no matches for Id Deployment.v1.apps/api-v1.[noNs]`}
	s.MarkPreviewAttempt(id)
	item, _ := s.SelectedCatalogItem()
	for _, width := range []int{28, 48, 80, 110, 160} {
		view := s.KustomizeOverview(item, width)
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("overview overflows", width, ansi.Strip(line))
			}
		}
		plain := ansi.Strip(view)
		for _, unwanted := range []string{"Last render:", "Configuration:", "[noNs]"} {
			if strings.Contains(plain, unwanted) {
				t.Fatal("redundant or raw error detail", unwanted, plain)
			}
		}
		for _, wanted := range []string{"Includes", "Changes here", "Technical details", "api-v1"} {
			if !strings.Contains(plain, wanted) {
				t.Fatal("missing overview information", wanted, plain)
			}
		}
		if width >= 80 && strings.Count(plain, "kustomization.yaml") != 1 {
			t.Fatal("source location must appear once", plain)
		}
	}
	actions := s.KustomizeActions()[0].Actions
	for _, action := range actions {
		if action.Mouse == "code-browser/graph" && action.Key != "G" || action.Mouse == "code-browser/render" && action.Key != "r" {
			t.Fatal("action does not advertise its single key", action)
		}
	}
}

func TestKustomizeComponentOverviewDoesNotClaimIndependentOutput(t *testing.T) {
	s := kustomizeGraphFixture(t)
	s.OpenKustomizeOverview("kustomize:components/tls")
	item, _ := s.SelectedCatalogItem()
	view := ansi.Strip(s.KustomizeOverview(item, 85))
	if !strings.Contains(view, "Used by") || !strings.Contains(view, "consuming configuration") || strings.Contains(view, "Not rendered yet") {
		t.Fatal("component render guidance is misleading", view)
	}
}

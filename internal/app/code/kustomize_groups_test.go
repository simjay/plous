package code

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestKustomizeDirectorySummariesSearchAndFrames(t *testing.T) {
	s := kustomizeGraphFixture(t)
	s.ShowKustomizeGroups()
	groups := s.KustomizeGroupItems()
	if len(groups) != 2 || groups[0].Name != "Source root" || groups[0].Summary != "6 overlays · 3 bases · 1 component" {
		t.Fatalf("unclear directory metadata: %#v", groups)
	}
	if _, ok := s.SelectedCatalogItem(); ok || len(s.RenderCandidates()) != 0 {
		t.Fatal("directory exposes a hidden selected node")
	}
	s.Filter = "source root"
	s.RestoreCursor()
	if len(s.KustomizeGroupItems()) != 1 {
		t.Fatal("displayed group name cannot be searched")
	}
	s.Filter = "missing"
	if body := ansi.Strip(s.CatalogView(60, 18)); !strings.Contains(body, "No matching groups") {
		t.Fatal("missing no-match state", body)
	}
	s.Filter = ""
	for _, size := range [][2]int{{18, 3}, {32, 12}, {48, 18}, {80, 24}, {160, 42}} {
		for s.Cursor = 0; s.Cursor < len(groups); s.Cursor++ {
			frame := s.CatalogView(size[0], size[1])
			assertHierarchyFrame(t, frame, size[0], size[1])
			if !strings.Contains(ansi.Strip(frame), "› "+groups[s.Cursor].Name) {
				t.Fatalf("selection hidden at %v: %s", size, ansi.Strip(frame))
			}
			_, spans := textview.MouseLayout(textview.MouseScope(frame, "main"))
			if !slices.ContainsFunc(spans, func(span textview.MouseSpan) bool {
				return span.Target == "code-kustomize-group/"+groups[s.Cursor].Path
			}) {
				t.Fatal("selected group has no mouse target")
			}
		}
	}
}

func TestKustomizeDirectorySeparatesGroupAndNodeNavigation(t *testing.T) {
	s := kustomizeGraphFixture(t)
	node := s.Preference.BrowserItem
	s.ShowKustomizeGroups()
	s.Filter = "root"
	s.RestoreCursor()
	if !s.OpenKustomizeGroup(".") || s.KustomizeDirectory() || s.Filter != "" || s.Preference.BrowserItem != node {
		t.Fatal("group open lost selected node or applied group filter to graph")
	}
	s.MoveCatalog("end")
	if item, ok := s.SelectedCatalogItem(); !ok || item.Group != "." {
		t.Fatal("node navigation escaped the selected group")
	}
	selected := s.Preference.BrowserItem
	if !s.OpenKustomizeOverview(selected) {
		t.Fatal("node overview did not open")
	}
	s.OpenKustomizeGraph()
	if !s.CloseDetail() || !s.BrowserDetail || !s.CloseDetail() || s.BrowserDetail || s.KustomizeDirectory() {
		t.Fatal("node Back did not preserve graph then overview order")
	}
	if !s.CloseDetail() || !s.KustomizeDirectory() || s.Filter != "root" || s.Preference.BrowserItem != selected {
		t.Fatal("directory Back lost filter or last selected node")
	}
	s.OpenKustomizeGroup(".")
	if s.Preference.BrowserItem != selected || s.Pending != "" || len(s.Builds) != 0 {
		t.Fatal("reopening group lost selection or rendered")
	}
}

func TestKustomizeDirectorySingleGroupLongPathsAndRefresh(t *testing.T) {
	group := "projects/a-long-environment-name/infrastructure/service"
	s := hierarchyViewFixture(map[string]string{group + "/kustomization.yaml": "resources: []\n"}, "")
	s.View = "code-kustomize"
	s.SyncListScope()
	if !s.KustomizeDirectory() || len(s.KustomizeGroupItems()) != 1 {
		t.Fatal("single group skipped directory")
	}
	for _, size := range [][2]int{{24, 9}, {48, 16}, {160, 32}} {
		frame := s.CatalogView(size[0], size[1])
		assertHierarchyFrame(t, frame, size[0], size[1])
		if !strings.Contains(ansi.Strip(frame), "› service") {
			t.Fatal("long path hid short name", ansi.Strip(frame))
		}
	}
	s.RememberSelection()
	delete(s.Inventory.Source.Files, group+"/kustomization.yaml")
	s.Inventory = source.Discover(s.Inventory.Source)
	s.RestoreCursor()
	if len(s.KustomizeGroupItems()) != 0 || s.OpenKustomizeGroup(group) || !s.KustomizeDirectory() || s.Cursor != 0 {
		t.Fatal("refresh opened a missing group")
	}
	if !strings.Contains(ansi.Strip(s.CatalogView(60, 18)), "No Kustomize groups") {
		t.Fatal("removed configurations left a stale directory row")
	}
}

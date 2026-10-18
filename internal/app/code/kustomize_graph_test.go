package code

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
)

func kustomizeGraphFixture(t *testing.T) *CodeState {
	t.Helper()
	snapshot, err := source.Load(t.Context(), source.Config{Path: "../../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	captureSourceFixture(t, snapshot)
	snapshot.Files["unrelated/kustomization.yaml"] = []byte("resources: []\n")
	s := &CodeState{View: "code-kustomize", Inventory: source.Discover(snapshot), Builds: map[string]*source.Result{}}
	if !s.SelectKustomizeNode("kustomize:clusters/prod-us-east") {
		t.Fatal("missing target")
	}
	return s
}

func TestKustomizeGraphKeepsGroupAndSharedNodes(t *testing.T) {
	s := kustomizeGraphFixture(t)
	item, _ := s.SelectedCatalogItem()
	g := s.kustomizeGraph(item)
	if len(g.entries) != 10 {
		t.Fatalf("group includes %d nodes, want 10", len(g.entries))
	}
	for _, entry := range g.entries {
		if entry.Path == "unrelated" {
			t.Fatal("unrelated group leaked into graph")
		}
	}
	chain := g.chain(s)
	for _, id := range []string{"clusters/prod-us-east", "envs/prod", "apps/api/base", "apps/cache/base", "apps/worker/base", "components/tls"} {
		if !chain["kustomize:"+id] {
			t.Fatal("missing dependency highlight", id)
		}
	}
	if chain["kustomize:clusters/prod-eu-west"] || chain["kustomize:envs/dev"] {
		t.Fatal("overlay highlighted siblings")
	}
	s.SelectKustomizeNode("kustomize:apps/api/base")
	item, _ = s.SelectedCatalogItem()
	chain = s.kustomizeGraph(item).chain(s)
	for _, id := range []string{"envs/dev", "envs/prod", "clusters/dev-local", "clusters/prod-us-east", "clusters/prod-eu-west", "clusters/prod-ap-south"} {
		if !chain["kustomize:"+id] {
			t.Fatal("missing consumer highlight", id)
		}
	}
	if chain["kustomize:components/tls"] || chain["kustomize:apps/worker/base"] {
		t.Fatal("base highlighted sibling dependencies")
	}
	s.SelectKustomizeNode("kustomize:components/tls")
	item, _ = s.SelectedCatalogItem()
	chain = s.kustomizeGraph(item).chain(s)
	if chain["kustomize:clusters/dev-local"] || !chain["kustomize:clusters/prod-us-east"] {
		t.Fatal("component consumer scope is wrong")
	}
	if s.Pending != "" || s.Busy || len(s.Builds) != 0 {
		t.Fatal("graph inspection started rendering")
	}
}

func TestKustomizeGraphFramesAndStableMouseTargets(t *testing.T) {
	s := kustomizeGraphFixture(t)
	for _, size := range [][2]int{{32, 12}, {48, 18}, {80, 30}, {120, 36}} {
		frame := s.CatalogView(size[0], size[1])
		assertHierarchyFrame(t, frame, size[0], size[1])
		plain := ansi.Strip(frame)
		if !strings.Contains(plain, "prod-us-east") {
			t.Fatalf("selection hidden at %v\n%s", size, plain)
		}
		_, spans := textview.MouseLayout(textview.MouseScope(frame, "main"))
		counts := map[string]int{}
		for _, span := range spans {
			if span.X < 0 || span.X+span.Width > size[0] || span.Y >= size[1] {
				t.Fatal("mouse target escaped graph", span)
			}
			counts[span.Target]++
		}
		if size[0] >= 80 {
			for _, item := range s.Inventory.Catalog {
				if item.Technology == "kustomize" && item.Group != "unrelated" && counts["code-kustomize/"+item.EntryID] != 4 {
					t.Fatal("shared node duplicated or hidden", item.EntryID, counts)
				}
			}
			if !strings.Contains(plain, "▶") || !strings.Contains(plain, "⇢") {
				t.Fatal("graph lost reference arrows", plain)
			}
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for ext, text := range map[string]string{"txt": plain, "ansi": frame} {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("kustomize-graph-%d.%s", size[0], ext)), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	positions := func() map[string][2]int {
		_, spans := textview.MouseLayout(textview.MouseScope(s.CatalogView(120, 36), "main"))
		result := map[string][2]int{}
		for _, span := range spans {
			if _, ok := result[span.Target]; !ok {
				result[span.Target] = [2]int{span.X, span.Y}
			}
		}
		return result
	}
	before := positions()
	s.SelectKustomizeNode("kustomize:apps/api/base")
	after := positions()
	for id, position := range before {
		if after[id] != position {
			t.Fatal("selection moved graph node", id)
		}
	}
}

func TestKustomizeGraphSearchPreservesContextAndNavigation(t *testing.T) {
	s := kustomizeGraphFixture(t)
	s.Filter = "clusters/prod-us-east"
	s.RestoreCursor()
	if len(s.CatalogItems()) != 1 {
		t.Fatal("search no longer selects matching configurations")
	}
	frame := ansi.Strip(s.CatalogView(120, 36))
	for _, label := range []string{"api", "worker", "tls", "dev-local"} {
		if !strings.Contains(frame, label) {
			t.Fatal("filter removed graph context", label)
		}
	}
	s.MoveCatalog("right")
	item, _ := s.SelectedCatalogItem()
	if item.EntryID != "kustomize:envs/prod" || s.Filter != "" {
		t.Fatal("right did not follow a hidden dependency", item, s.Filter)
	}
	s.MoveCatalog("left")
	item, _ = s.SelectedCatalogItem()
	if !strings.HasPrefix(item.EntryID, "kustomize:clusters/prod-") {
		t.Fatal("left did not follow a consumer")
	}
	s.ChooseKustomizeGroup("unrelated")
	item, _ = s.SelectedCatalogItem()
	if item.EntryID != "kustomize:unrelated" || s.Preference.BrowserItem != item.ID {
		t.Fatal("group selection did not preserve identity")
	}
	if len(s.RenderCandidates()) != 1 {
		t.Fatal("standalone kustomization cannot render")
	}
	s.ChooseKustomizeGroup(".")
	item, _ = s.SelectedCatalogItem()
	if len(s.Inventory.EntryConsumers(item.EntryID)) != 0 || !slices.Contains(s.Inventory.EntryRoles(item.EntryID), "overlay") {
		t.Fatal("new group defaults to a base")
	}
}

func TestKustomizeGraphFallbackKeepsIncomingAndOutgoingEdges(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{
		"a/kustomization.yaml": "resources: [../b, ../c]\n",
		"b/kustomization.yaml": "resources: [../c]\n",
		"c/kustomization.yaml": "resources: [../a]\n",
	}, "")
	s.View = "code-kustomize"
	s.SelectKustomizeNode("kustomize:b")
	for _, width := range []int{32, 120} {
		frame := s.CatalogView(width, 24)
		assertHierarchyFrame(t, frame, width, 24)
		for _, edge := range []string{"› b", "→ c", "← a"} {
			if !strings.Contains(ansi.Strip(frame), edge) {
				t.Fatal("fallback lost a declared edge", edge, ansi.Strip(frame))
			}
		}
	}
}

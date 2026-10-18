package code

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
)

func hierarchyViewFixture(files map[string]string, selected string) *CodeState {
	snapshot := &source.Snapshot{Identity: "captured-fixture", Files: map[string][]byte{}}
	for name, text := range files {
		snapshot.Files[name] = []byte(text)
	}
	return &CodeState{View: "manifests", Inventory: source.Discover(snapshot), Preference: CodePreference{Selected: selected}, Builds: map[string]*source.Result{}, Inputs: map[string]source.Inputs{}}
}

func hierarchyMapFixture() *CodeState {
	return hierarchyViewFixture(map[string]string{
		"apps/api/root/kustomization.yaml":         "resources: []\n",
		"apps/worker/root/kustomization.yaml":      "resources: []\n",
		"apps/cache/root/kustomization.yaml":       "helmCharts: [{name: redis, version: 18.2.1, repo: 'https://charts.example.invalid'}]\n",
		"components/tls/kustomization.yaml":        "apiVersion: kustomize.config.k8s.io/v1alpha1\nkind: Component\n",
		"envs/dev/kustomization.yaml":              "resources: [../../apps/api/root]\n",
		"envs/prod/kustomization.yaml":             "resources: [../../apps/api/root, ../../apps/worker/root, ../../apps/cache/root]\ncomponents: [../../components/tls]\nnamePrefix: prod-\nnamespace: production\n",
		"clusters/dev-local/kustomization.yaml":    "resources: [../../envs/dev]\n",
		"clusters/prod-us-east/kustomization.yaml": "resources: [../../envs/prod]\n",
		"clusters/prod-eu-west/kustomization.yaml": "resources: [../../envs/prod]\n",
	}, "kustomize:envs/prod")
}

func assertHierarchyFrame(t *testing.T, frame string, width, height int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != height {
		t.Fatalf("height = %d, want %d\n%s", len(lines), height, ansi.Strip(frame))
	}
	for index, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("line %d width = %d, want %d\n%s", index, got, width, ansi.Strip(frame))
		}
	}
}

func TestHierarchyMapFramesSelectionAndCapturedFacts(t *testing.T) {
	s := hierarchyMapFixture()
	t.Setenv("PATH", t.TempDir())
	before, _ := json.Marshal(s.Inventory)
	for _, size := range [][2]int{{64, 22}, {100, 30}, {130, 34}, {180, 40}} {
		s.InspectorArea, s.DetailOffset = 0, 0
		frame := s.SourcesView(size[0], size[1])
		assertHierarchyFrame(t, frame, size[0], size[1])
		plain := ansi.Strip(frame)
		for _, want := range []string{"› ", "envs/prod"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%v lost %q\n%s", size, want, plain)
			}
		}
		if size[0] >= 100 {
			for _, want := range []string{"Reference graph", "HELM CHARTS", "Helm redis 18.2.1", "Overlay · Base · Shared", "◀"} {
				if !strings.Contains(plain, want) {
					t.Fatalf("%v lost %q\n%s", size, want, plain)
				}
			}
		}
		_, spans := textview.MouseLayout(textview.MouseScope(frame, "main"))
		targets := map[string]bool{}
		for _, span := range spans {
			if span.X < 0 || span.X+span.Width > size[0] || span.Y >= size[1] {
				t.Fatal("mouse target escaped map", span)
			}
			targets[span.Target] = true
		}
		for _, target := range []string{fmt.Sprintf("code-row/%d", s.SourceIndex(s.Preference.Selected))} {
			if !targets[target] {
				t.Fatal("selection or layout control lost mouse mapping", target)
			}
		}
		if size[0] == 100 {
			for index := range s.Entries() {
				if !targets[fmt.Sprintf("code-row/%d", index)] {
					t.Fatal("compact graph hid a node despite enough rows for adjacent cards", index)
				}
			}
		}
		if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			for extension, data := range map[string]string{"ansi": frame, "txt": plain} {
				if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("render-map-%d.%s", size[0], extension)), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	s.InspectorArea, s.DetailOffset = 1, 0
	compact := s.SourcesView(64, 22)
	assertHierarchyFrame(t, compact, 64, 22)
	if plain := ansi.Strip(compact); !strings.Contains(plain, "envs/prod/kustomization.yaml") || !strings.Contains(plain, "Inherited from apps/cache/root") || !strings.Contains(plain, "← Graph") {
		t.Fatal("compact details lost hierarchy or return route", plain)
	}
	if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
		if err := os.WriteFile(filepath.Join(directory, "render-map-details-64.txt"), []byte(ansi.Strip(compact)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := json.Marshal(s.Inventory)
	if string(before) != string(after) || s.Pending != "" || s.Busy || len(s.Builds) != 0 {
		t.Fatal("reading the map changed captured inputs or started rendering")
	}
}

func TestHierarchyKeepsUnicodePathTailsAndSelectedOverflowNode(t *testing.T) {
	files := map[string]string{}
	for n := range 12 {
		files[fmt.Sprintf("repositories/platform/東京/services/production-target-%02d/kustomization.yaml", n)] = "resources: []\n"
	}
	s := hierarchyViewFixture(files, "kustomize:repositories/platform/東京/services/production-target-11")
	graph := s.SourceGraph(54, 11)
	assertHierarchyFrame(t, graph, 54, 11)
	if plain := ansi.Strip(graph); !strings.Contains(plain, "› ") || !strings.Contains(plain, "production-target-11") || !strings.Contains(plain, "+11 nodes") {
		t.Fatal("selected node was hidden or lost its distinguishing path tail", plain)
	}
	tree := ansi.Strip(s.CompactGraph(36, 9))
	if !strings.Contains(tree, "production-target-11") || !strings.Contains(tree, "› ") {
		t.Fatal("compact tree lost selected path tail", tree)
	}
	for _, line := range strings.Split(tree, "\n") {
		if ansi.StringWidth(line) > 36 {
			t.Fatal("Unicode path exceeded the tree", line)
		}
	}
}

func TestHierarchyTreePreservesCyclesDeepAndCrossLayerEdges(t *testing.T) {
	for _, test := range []struct {
		name                   string
		files                  map[string]string
		selected, reason, want string
	}{
		{"cycle", map[string]string{"a/kustomization.yaml": "resources: [../b]\n", "b/kustomization.yaml": "resources: [../a]\n"}, "kustomize:a", "a → b", "b"},
		{"deep-narrow", map[string]string{"a/kustomization.yaml": "resources: [../b]\n", "b/kustomization.yaml": "resources: [../c]\n", "c/kustomization.yaml": "resources: [../d]\n", "d/kustomization.yaml": "resources: []\n"}, "kustomize:c", "c → d", "d"},
		{"cross-layer", map[string]string{"a/kustomization.yaml": "resources: [../b, ../c]\n", "b/kustomization.yaml": "resources: [../c]\n", "c/kustomization.yaml": "resources: []\n"}, "kustomize:a", "a → c", "c"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := hierarchyViewFixture(test.files, test.selected)
			view := ansi.Strip(s.SourceGraph(80, 22))
			for _, want := range []string{test.reason, test.want, "› "} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing %q\n%s", want, view)
				}
			}
		})
	}
}

func TestHierarchyInspectorShowsCapturedChartLocationAndMasksValues(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{
		"layer/kustomization.yaml":      "resources: ['https://user:secret@example.invalid/base?token=secret']\nhelmGlobals: {chartHome: vendor}\nhelmCharts: [{name: redis, version: 9.9.9, repo: 'https://user:secret@charts.example.invalid?token=secret'}]\nsecretGenerator: [{name: private, literals: ['password=hidden-value']}]\n",
		"layer/vendor/redis/Chart.yaml": "apiVersion: v2\nname: redis\nversion: 18.2.1\n",
		"consumer/kustomization.yaml":   "resources: [../layer]\n",
	}, "kustomize:layer")
	view := ansi.Strip(s.SourceInspector(60, 90))
	for _, want := range []string{"Base", "Helm redis 18.2.1", "Location: layer/vendor/redis", "Declared version: 9.9.9", "Repository: https://charts.example.invalid", "secretGenerator: 1", "→ consumer"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q\n%s", want, view)
		}
	}
	for _, secret := range []string{"user:secret", "token=secret", "hidden-value"} {
		if strings.Contains(view, secret) {
			t.Fatal("inspector exposed sensitive source content", secret)
		}
	}
}

func TestHierarchyTreeOrderAndGraphPositionsStayStable(t *testing.T) {
	s := hierarchyMapFixture()
	order := s.GraphOrder()
	if len(order) != len(s.Entries()) || order[0] != "kustomize:clusters/dev-local" || order[1] != "kustomize:envs/dev" || order[2] != "kustomize:apps/api/root" {
		t.Fatal("navigation is not stable target-to-dependency order", order)
	}
	seen := map[string]bool{}
	for _, id := range order {
		if seen[id] {
			t.Fatal("shared node repeated in keyboard traversal", id)
		}
		seen[id] = true
	}
	positions := func() map[string][2]int {
		_, spans := textview.MouseLayout(textview.MouseScope(s.SourceGraph(110, 38), "map"))
		result := map[string][2]int{}
		for _, span := range spans {
			if strings.HasPrefix(span.Target, "code-row/") {
				if _, ok := result[span.Target]; !ok {
					result[span.Target] = [2]int{span.X, span.Y}
				}
			}
		}
		return result
	}
	before := positions()
	s.Preference.Selected = "kustomize:clusters/dev-local"
	after := positions()
	for id, position := range before {
		if after[id] != position {
			t.Fatal("highlighting a source moved its mouse target", id, position, after[id])
		}
	}
}

func TestHierarchyInspectorKeepsTargetsOutsideMapFilter(t *testing.T) {
	s := hierarchyMapFixture()
	s.Filter, s.Preference.Selected = "apps/cache", "kustomize:apps/cache/root"
	view := ansi.Strip(s.SourceInspector(64, 100))
	for _, want := range []string{"TARGET CANDIDATES", "clusters/prod-us-east", "clusters/prod-eu-west"} {
		if !strings.Contains(view, want) {
			t.Fatal("filter erased captured hierarchy facts", want, view)
		}
	}
	s.Preference.Selected = "kustomize:envs/dev"
	if view := ansi.Strip(s.CompactGraph(50, 16)); !strings.Contains(view, "envs/dev") {
		t.Fatal("a hidden selected source erased visible map nodes", view)
	}
}

func TestHierarchyGraphDoesNotJoinUnrelatedCrossingEdges(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{
		"a/kustomization.yaml": "resources: []\n",
		"b/kustomization.yaml": "resources: []\n",
		"x/kustomization.yaml": "resources: [../b]\n",
		"y/kustomization.yaml": "resources: [../a]\n",
		"z/kustomization.yaml": "resources: []\n",
	}, "kustomize:z")
	frame := s.SourceGraph(84, 30)
	assertHierarchyFrame(t, frame, 84, 30)
	view := ansi.Strip(frame)
	for _, want := range []string{"╳ crossing, no join", "◀", "› z"} {
		if !strings.Contains(view, want) {
			t.Fatal("crossed edges became a false shared trunk", want, view)
		}
	}
	if strings.Count(view, "╳") < 2 || strings.Contains(view, "Tree ") {
		t.Fatal("independent routes did not keep their explicit crossing", view)
	}
}

func TestHierarchyGraphKeepsRealisticSharedBasesAsCards(t *testing.T) {
	snapshot, err := source.Load(t.Context(), source.Config{Path: "../../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	s := &CodeState{View: "manifests", Inventory: source.Discover(snapshot), Preference: CodePreference{Selected: "kustomize:envs/prod"}}
	frame := s.SourcesView(150, 46)
	assertHierarchyFrame(t, frame, 150, 46)
	plain := ansi.Strip(frame)
	for _, want := range []string{"╳ crossing, no join", "◀", "⇠", "› envs/prod", "apps/api/base", "apps/cache/base", "apps/worker/base", "components/tls", "clusters/prod-ap-south", "clusters/prod-eu-west", "clusters/prod-us-east"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("realistic shared-base graph lost %q\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Tree keeps") || strings.Contains(plain, "Tree preserves") || strings.Contains(plain, "Tree fits") {
		t.Fatal("ordinary shared bases unnecessarily fell back to a tree", plain)
	}
	_, spans := textview.MouseLayout(textview.MouseScope(frame, "map"))
	rows := map[string]int{}
	for _, span := range spans {
		if span.X < 0 || span.X+span.Width > 150 || span.Y < 0 || span.Y >= 46 {
			t.Fatal("mouse target escaped realistic map", span)
		}
		rows[span.Target]++
	}
	for index := range s.Entries() {
		if got := rows[fmt.Sprintf("code-row/%d", index)]; got != 6 {
			t.Fatalf("source %d has %d clickable card rows, want 6", index, got)
		}
	}
	if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		for extension, data := range map[string]string{"ansi": frame, "txt": plain} {
			if err := os.WriteFile(filepath.Join(directory, "render-map-realistic-150."+extension), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestHierarchyGraphIncludesCapturedHelmLevelWhenItFits(t *testing.T) {
	s := hierarchyViewFixture(map[string]string{
		"base/kustomization.yaml":      "helmCharts: [{name: redis, version: 18.2.1}]\n",
		"base/charts/redis/Chart.yaml": "apiVersion: v2\nname: redis\nversion: 18.2.1\n",
		"env/kustomization.yaml":       "resources: [../base]\n",
		"cluster/kustomization.yaml":   "resources: [../env]\n",
	}, "kustomize:cluster")
	frame := s.SourceGraph(125, 14)
	assertHierarchyFrame(t, frame, 125, 14)
	plain := ansi.Strip(frame)
	if strings.Contains(plain, "Tree ") || strings.Count(plain, "◀") != 3 || !strings.Contains(plain, "base/charts/redis") || !strings.Contains(plain, "Helm redis 18.2.1") {
		t.Fatal("captured chart forced an unnecessary graph fallback", plain)
	}
}

package app

import (
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

func treeIndexFor(t *testing.T, m *model, key string) int {
	t.Helper()
	for i, row := range m.treeRows() {
		if row.key == key {
			return i
		}
	}
	t.Fatalf("tree key %q is not visible", key)
	return 0
}

func TestCategoryExpansionMigrationPreservesFoldsAndCurrentKeys(t *testing.T) {
	m := exploreModel(t)
	legacyNetwork := m.connection + "/category/Services, Load Balancing, and Networking"
	legacyWorkloads := m.connection + "/category/Workloads"
	m.expanded[legacyNetwork] = true
	m.expanded[legacyWorkloads] = false
	m.syncExploreTree()
	if !m.expanded[categoryTreeKey(m.connection, "Network")] {
		t.Fatal("Network expansion was not migrated")
	}
	if open, exists := m.expanded[categoryTreeKey(m.connection, "Workloads")]; !exists || open {
		t.Fatal("initial discovery replaced a saved deliberate fold")
	}
	for _, key := range []string{legacyNetwork, legacyWorkloads} {
		if _, exists := m.expanded[key]; exists {
			t.Fatalf("obsolete category key retained: %s", key)
		}
	}
	m.expanded[legacyNetwork] = true
	m.expanded[categoryTreeKey(m.connection, "Network")] = false
	m.treeRows()
	if m.expanded[categoryTreeKey(m.connection, "Network")] {
		t.Fatal("legacy expansion overrode the newer fold")
	}
	if canonicalTreeKey(legacyNetwork) != categoryTreeKey(m.connection, "Network") {
		t.Fatal("persisted category cursor was not migrated")
	}
}

func TestDiscoveryRefreshPreservesFoldedPath(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	workloads := categoryTreeKey(m.connection, "Workloads")
	m.expanded[workloads] = false
	m.expanded[m.connection] = false
	m.syncExploreTree()
	if m.expanded[workloads] || m.expanded[m.connection] {
		t.Fatal("refresh reopened a deliberate fold")
	}
}

func TestConnectionCollapseKeepsCurrentResourceAndSurvivesRestart(t *testing.T) {
	for _, key := range []string{"enter", "left"} {
		t.Run(key, func(t *testing.T) {
			m := exploreModel(t)
			m.syncExploreTree()
			m.explore.preference.Filter = "checkout"
			m.openExploreSelection(1)
			m.draft().insert("Keep this draft", true)
			e := m.explore
			object, stream, preference := e.detail, e.stream, e.preference
			connection := m.connection
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, connection)
			press(t, m, key)
			if m.expanded[connection] || m.autoExpanded[connection] {
				t.Fatal("connection did not become deliberately collapsed")
			}
			for _, row := range m.treeRows() {
				if row.connection == connection && (row.kind == "category" || row.kind == "resource") {
					t.Fatal("collapsed connection still shows its children")
				}
			}
			row := m.treeRows()[m.treeCursor]
			if row.key != connection || treeDisclosure(row) != "▸ " {
				t.Fatal("collapse lost the connection cursor or disclosure state")
			}
			if m.connection != connection || m.focus != 0 || m.mode != 1 || e.detail != object || e.tab != 1 || e.stream != stream || e.preference != preference || m.draft().text != "Keep this draft" {
				t.Fatal("collapsing the sidebar changed the open resource, scope, or draft")
			}
			m.Update(exploreUpdate{stream: stream, snapshot: kubernetes.Snapshot{Status: "Live"}})
			m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
			if m.expanded[connection] {
				t.Fatal("background update or resize reopened the connection")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.explore = e
			restored.syncExploreTree()
			if open, saved := restored.expanded[connection]; open || !saved || restored.treeRows()[restored.treeCursor].key != connection {
				t.Fatal("restart or rediscovery lost the collapsed connection")
			}
			press(t, m, "right")
			if !m.expanded[connection] || m.focus != 0 || e.detail != object {
				t.Fatal("Right did not reopen the connection without losing the resource")
			}
			treeIndexFor(t, m, connection+"/resource/pods")
			press(t, m, "left")
			press(t, m, "left")
			if m.treeRows()[m.treeCursor].key != m.active+"/kubernetes" {
				t.Fatal("Left on a collapsed connection did not move to its parent")
			}
		})
	}
}

func TestBranchEnterTogglesWithoutChangingMainMode(t *testing.T) {
	for _, kind := range []string{"workspace", "tool", "connection", "category"} {
		for mode, name := range modes {
			t.Run(kind+"/"+name, func(t *testing.T) {
				m := exploreModel(t)
				m.syncExploreTree()
				keys := map[string]string{"workspace": m.active, "tool": m.active + "/kubernetes", "connection": m.connection, "category": categoryTreeKey(m.connection, "Workloads")}
				key := keys[kind]
				m.mode, m.focus = mode, 0
				m.treeCursor = treeIndexFor(t, m, key)
				row := m.treeRows()[m.treeCursor]
				press(t, m, "enter")
				if m.expanded[key] || m.autoExpanded[key] || m.mode != mode || m.focus != 0 || m.treeRows()[m.treeCursor].key != key {
					t.Fatal("Enter failed to fold the branch in the current mode")
				}
				for _, visible := range m.treeRows() {
					if visible.parent == key {
						t.Fatal("folded branch still shows children")
					}
				}
				if kind != "category" && (m.active != row.workspace || m.tool != row.tool || m.connection != row.connection) {
					t.Fatal("Enter did not select the workspace, tool, or connection")
				}
				press(t, m, "enter")
				if !m.expanded[key] || m.mode != mode || m.focus != 0 {
					t.Fatal("Enter failed to expand the branch in the current mode")
				}
			})
		}
	}
}

func TestBranchArrowsPreserveActiveResourceAndSavedFolds(t *testing.T) {
	for _, kind := range []string{"workspace", "tool", "connection", "category"} {
		t.Run(kind, func(t *testing.T) {
			m := exploreModel(t)
			m.syncExploreTree()
			keys := map[string]string{"workspace": m.active, "tool": m.active + "/kubernetes", "connection": m.connection, "category": categoryTreeKey(m.connection, "Workloads")}
			key := keys[kind]
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, key)
			connection, stream := m.connection, m.explore.stream
			press(t, m, "left")
			if m.expanded[key] || m.connection != connection || m.mode != 1 || m.explore.stream != stream || stream.ctx.Err() != nil {
				t.Fatal("Left did not fold without interrupting the resource view")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.explore = m.explore
			restored.syncExploreTree()
			if open, saved := restored.expanded[key]; open || !saved {
				t.Fatal("restart or discovery reopened a folded branch")
			}
			press(t, m, "right")
			if !m.expanded[key] || m.connection != connection || m.mode != 1 || m.explore.stream != stream {
				t.Fatal("Right changed the active resource")
			}
			m.mode = 0
			press(t, m, "right")
			if m.mode != 0 || m.connection != connection {
				t.Fatal("Right switched views instead of only expanding")
			}
		})
	}
}

func TestTreeSearchExpandsEveryBranchResult(t *testing.T) {
	for _, query := range []string{"Shop", "Kubernetes", "test-context", "Workloads"} {
		for _, open := range []bool{false, true} {
			m := exploreModel(t)
			m.syncExploreTree()
			keys := map[string]string{"Shop": m.active, "Kubernetes": m.active + "/kubernetes", "test-context": m.connection, "Workloads": categoryTreeKey(m.connection, "Workloads")}
			key := keys[query]
			connection := m.connection
			m.setTreeExpanded(key, open)
			m.focus = 0
			typeText(m, "/"+query)
			m.treeCursor = treeIndexFor(t, m, key)
			press(t, m, "enter")
			if !m.expanded[key] || m.searches[0] != nil || m.focus != 0 {
				t.Fatalf("opening search result %q folded its branch", query)
			}
			treeIndexFor(t, m, connection+"/resource/pods")
		}
	}
}

func TestRootEscFocusesNearestVisibleAncestorWithoutChangingExplore(t *testing.T) {
	for _, fold := range []string{"", "category", "connection", "tool", "workspace"} {
		t.Run(fold, func(t *testing.T) {
			m := exploreModel(t)
			m.syncExploreTree()
			want := m.connection + "/resource/pods"
			switch fold {
			case "category":
				want = categoryTreeKey(m.connection, "Workloads")
			case "connection":
				want = m.connection
			case "tool":
				want = m.active + "/kubernetes"
			case "workspace":
				want = m.active
			}
			if fold != "" {
				m.expanded[want] = false
			}
			m.explore.preference.Filter = "checkout"
			m.explore.preference.Selector = "app=checkout"
			m.explore.scroll = 9
			preference, selected := m.explore.preference, m.explore.selected
			expansion := maps.Clone(m.expanded)
			m.focusRootSidebar()
			if m.focus != 0 || m.treeRows()[m.treeCursor].key != want {
				t.Fatalf("root back-navigation selected %q with focus %d, want %q", m.treeRows()[m.treeCursor].key, m.focus, want)
			}
			if m.explore.preference != preference || m.explore.selected != selected || m.explore.scroll != 9 || !maps.Equal(expansion, m.expanded) {
				t.Fatal("root back-navigation changed filters, selection, scroll, or deliberate folds")
			}
		})
	}
}

func TestRootSidebarFocusGuardsVisibilityAndCurrentPanel(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	m.leftHidden = true
	m.focusRootSidebar()
	if m.focus != 1 || !m.leftHidden {
		t.Fatal("root back-navigation revealed a hidden sidebar")
	}
	m.leftHidden, m.focus = false, 2
	m.focusRootSidebar()
	if m.focus != 2 {
		t.Fatal("root back-navigation stole chat focus")
	}
	m.focus, m.width = 1, minWidth-1
	m.focusRootSidebar()
	if m.focus != 1 {
		t.Fatal("small-terminal fallback focused invisible controls")
	}
}

func TestTreeHierarchyUsesEqualDepthStepsAndKeepsLongResourceNames(t *testing.T) {
	m := exploreModel(t)
	m.explore.resources = append(m.explore.resources, kubernetes.Resource{Name: "horizontalpodautoscalers", Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"})
	m.syncExploreTree()
	rows := m.treeRows()
	parent := treeIndexFor(t, m, categoryTreeKey(m.connection, "Workloads"))
	child := treeIndexFor(t, m, m.connection+"/resource/pods")
	parentLine := ansi.Strip(m.renderTreeRow(rows, parent, 36)[0])
	childLine := ansi.Strip(m.renderTreeRow(rows, child, 36)[0])
	if got := ansi.StringWidth(childLine[:strings.Index(childLine, "Pods")]) - ansi.StringWidth(parentLine[:strings.Index(parentLine, "Workloads")]); got != 2 {
		t.Fatalf("resource indents %d cells from category, want 2", got)
	}
	if !strings.Contains(childLine, "─") {
		t.Fatal("parent/child guide missing")
	}
	long := treeIndexFor(t, m, m.connection+"/resource/horizontalpodautoscalers.autoscaling")
	for _, width := range []int{20, 28, 36} {
		var label strings.Builder
		for _, line := range m.renderTreeRow(rows, long, width) {
			line = ansi.Strip(line)
			if ansi.StringWidth(line) > width {
				t.Fatalf("resource row exceeds %d cells: %q", width, line)
			}
			label.WriteString(strings.TrimSpace(ansi.Cut(line, 11, width)))
		}
		if label.String() != "HorizontalPodAutoscalers" {
			t.Fatalf("resource name was lost at width %d: %q", width, label.String())
		}
	}
}

func TestTreeCursorAndOpenResourceHaveDifferentMarkers(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	m.focus = 0
	rows := m.treeRows()
	resource := treeIndexFor(t, m, m.connection+"/resource/pods")
	connection := treeIndexFor(t, m, m.connection)
	m.treeCursor = connection
	if !strings.HasPrefix(ansi.Strip(m.renderTreeRow(rows, connection, 36)[0]), "›") || !strings.HasPrefix(ansi.Strip(m.renderTreeRow(rows, resource, 36)[0]), "▎") {
		t.Fatal("keyboard cursor and open resource look identical")
	}
	if current, ancestor := m.treeRowState(rows[connection]); current || !ancestor {
		t.Fatal("active connection must be an ancestor, not a second open resource")
	}
}

func TestTreeCategoriesFormOneContinuousHierarchy(t *testing.T) {
	m := exploreModel(t)
	m.explore.resources = append(m.explore.resources,
		kubernetes.Resource{Name: "statefulsets", Group: "apps", Version: "v1", Kind: "StatefulSet"},
		kubernetes.Resource{Name: "services", Version: "v1", Kind: "Service"},
		kubernetes.Resource{Name: "persistentvolumes", Version: "v1", Kind: "PersistentVolume"},
		kubernetes.Resource{Name: "events", Version: "v1", Kind: "Event"},
	)
	m.syncExploreTree()
	for _, category := range []string{"Network", "Storage", "Admin"} {
		m.expanded[categoryTreeKey(m.connection, category)] = false
	}
	view := ansi.Strip(m.treeBody(48, 60))
	lines := strings.Split(view, "\n")
	newRow := -1
	for i, line := range lines {
		if strings.Contains(line, "+ New workspace") {
			newRow = i
			break
		}
	}
	if newRow < 1 || strings.TrimSpace(lines[newRow-1]) != "" {
		t.Fatal("workspace creation must remain separated from the resource hierarchy")
	}
	for i, line := range lines[:newRow-1] {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("blank row %d interrupted the hierarchy: %s", i, view)
		}
	}
	for _, label := range []string{"Workloads", "StatefulSets", "Network", "Storage", "Admin"} {
		if !strings.Contains(view, label) {
			t.Fatalf("resource hierarchy lost %s", label)
		}
	}
	rows := m.treeRows()
	for _, category := range []string{"Network", "Storage", "Admin"} {
		row := rows[treeIndexFor(t, m, categoryTreeKey(m.connection, category))]
		if row.open {
			t.Fatalf("rendering reopened the folded %s category", category)
		}
	}
}

func TestSidebarWidthStaysStableAcrossModes(t *testing.T) {
	m := exploreModel(t)
	for _, width := range []int{80, 120, 160, 200} {
		m.width = width
		m.mode = 0
		want := m.panelWidths()[0]
		for mode := 1; mode < len(modes); mode++ {
			m.mode = mode
			if got := m.panelWidths()[0]; got != want {
				t.Fatalf("sidebar width changed between modes: %d to %d", want, got)
			}
		}
	}
}

func TestEscUnwindsOneExploreLayerPerKey(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	m.explore.preference.Filter = "checkout"
	m.openExploreSelection(0)
	original := m.explore.detail.DeepCopy()
	m.openNamespacePicker()
	m.explore.namespaces.manual = true
	press(t, m, "esc")
	if m.explore.picker != "namespace" || m.explore.namespaces.manual || m.explore.detail == nil || m.focus != 1 {
		t.Fatal("manual namespace Esc did not return to the picker alone")
	}
	press(t, m, "esc")
	if m.explore.picker != "" || m.explore.detail == nil || m.focus != 1 {
		t.Fatal("namespace picker Esc did not return to the detail alone")
	}
	typeText(m, "b")
	press(t, m, "esc")
	if m.shortcutPrefix != "" || m.explore.detail == nil || m.focus != 1 {
		t.Fatal("Esc consumed more than the pending shortcut prefix")
	}
	m.explore.trail = []exploreLocation{{preference: m.explore.preference, selected: m.explore.selected, object: original, tab: 1}}
	press(t, m, "esc")
	if m.explore.detail == nil || m.explore.tab != 1 || len(m.explore.trail) != 0 || m.focus != 1 {
		t.Fatal("related detail Esc did not restore its parent detail and section")
	}
	press(t, m, "esc")
	if m.explore.detail != nil || m.explore.preference.Filter != "checkout" || m.focus != 1 {
		t.Fatal("top-level detail Esc did not restore the filtered list alone")
	}
	press(t, m, "esc")
	if m.focus != 0 || m.explore.preference.Filter != "checkout" {
		t.Fatal("list Esc did not preserve its committed filter and focus the sidebar")
	}
}

func TestEscAtRootModesAndActivityDetail(t *testing.T) {
	for mode := range modes {
		m := exploreModel(t)
		m.mode = mode
		press(t, m, "esc")
		if m.focus != 0 || m.mode != mode {
			t.Fatalf("root mode %d Esc changed mode or failed to focus sidebar", mode)
		}
	}
	m := exploreModel(t)
	m.mode, m.detail = 2, "revision"
	press(t, m, "esc")
	if m.detail != "" || m.focus != 1 {
		t.Fatal("Activity revision Esc skipped its list")
	}
	press(t, m, "esc")
	if m.focus != 0 {
		t.Fatal("Activity list Esc did not focus sidebar")
	}
}

func TestEscFromFilterCancelsDraftThenReturnsFocus(t *testing.T) {
	m := exploreModel(t)
	m.explore.preference.Filter = "checkout"
	m.openExploreInput("filter", "unsubmitted")
	press(t, m, "esc")
	if m.explore.input != nil || m.explore.preference.Filter != "checkout" || m.focus != 1 {
		t.Fatal("filter Esc failed to preserve the committed filter in the list")
	}
	m.leftHidden = true
	press(t, m, "esc")
	if m.focus != 1 || !m.leftHidden || m.explore.preference.Filter != "checkout" {
		t.Fatal("root Esc cleared a filter or revealed a hidden sidebar")
	}
}

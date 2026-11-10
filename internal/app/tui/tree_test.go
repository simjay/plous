package tui

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"github.com/simjay/plous/internal/workspace"
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

func TestConnectionExpansionKeepsHelmCollapsedUntilOpened(t *testing.T) {
	for _, action := range []string{"enter", "right", "label", "disclosure"} {
		t.Run(action, func(t *testing.T) {
			m := exploreModel(t)
			m.width, m.height = 200, 50
			m.clusterFor(m.active, *m.savedConnection()).HelmDriver = "secret"
			m.syncExploreTree()
			connection := m.connection
			helm := connection + "/view/helm"
			m.setTreeExpanded(connection+"/view/kubernetes", false)
			m.setTreeExpanded(connection, false)
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, connection)
			switch action {
			case "enter", "right":
				press(t, m, action)
			case "label":
				clickMouse(t, m, "tree", "tree/"+connection)
			case "disclosure":
				clickMouse(t, m, "tree", "fold/"+connection)
			}
			if !m.expanded[connection] || m.treeRows()[m.treeCursor].key != connection {
				t.Fatal("connection did not expand at the selected row")
			}
			for _, row := range m.treeRows() {
				if row.kind == "capability" && row.open || row.kind == "helmsection" {
					t.Fatal("opening the connection also opened a tool branch", row.label)
				}
			}
			clickMouse(t, m, "tree", "fold/"+helm)
			treeIndexFor(t, m, connection+"/helm/Releases")
			clickMouse(t, m, "tree", "fold/"+connection)
			clickMouse(t, m, "tree", "fold/"+connection)
			if m.treeRows()[treeIndexFor(t, m, helm)].open {
				t.Fatal("reopening the connection exposed the previously opened Helm sections")
			}
			clickMouse(t, m, "tree", "fold/"+helm)
			treeIndexFor(t, m, connection+"/helm/Releases")
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			if restored.expanded[helm] || restored.expanded[connection] {
				t.Fatal("restart restored an expanded connection or Helm branch")
			}
			clickMouse(t, m, "tree", "fold/"+helm)
			clickMouse(t, m, "tree", "fold/"+connection)
			clickMouse(t, m, "tree", "fold/"+connection)
			if m.treeRows()[treeIndexFor(t, m, helm)].open {
				t.Fatal("reopening the connection discarded the deliberate Helm collapse")
			}
		})
	}
}

func TestInitialKubernetesDiscoveryKeepsToolBranchesCollapsed(t *testing.T) {
	m := exploreModel(t)
	client, resources := m.explore.Client, m.explore.Resources
	m.explore.Key = ""
	m.setTreeExpanded(m.connection, false)
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, m.connection)
	press(t, m, "enter")
	if !m.explore.Connecting {
		t.Fatal("opening the connection did not start discovery")
	}
	m.Update(monitor.ExploreConnected{Generation: m.explore.Generation, Client: client, Resources: resources})
	for _, row := range m.treeRows() {
		if row.kind == "capability" && row.open || row.kind == "category" || row.kind == "resource" {
			t.Fatal("background discovery opened a tool branch", row.label)
		}
	}
	if m.explore.Preference.Resource != "pods" || m.explore.Resource.Key() != "pods" {
		t.Fatal("keeping the tree collapsed changed the main resource view")
	}
	m.executeExplore("pods")
	m.syncExploreTree()
	if m.expanded[m.connection+"/view/kubernetes"] {
		t.Fatal("resource navigation reopened a capability closed with its connection")
	}
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, m.connection+"/view/kubernetes")
	press(t, m, "right")
	treeIndexFor(t, m, m.connection+"/resource/pods")
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
			m.explore.Preference.Filter = "checkout"
			m.openExploreSelection(1)
			m.draft().Insert("Keep this draft", true)
			e := m.explore
			object, stream, preference := e.Detail, e.Stream, e.Preference
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
			if m.connection != connection || m.focus != 0 || m.mode != 1 || e.Detail != object || e.Tab != 1 || e.Stream != stream || e.Preference != preference || m.draft().Text != "Keep this draft" {
				t.Fatal("collapsing the sidebar changed the open resource, scope, or draft")
			}
			m.Update(monitor.ExploreUpdate{Stream: stream, Snapshot: kubernetes.Snapshot{Status: "Live"}})
			m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
			if m.expanded[connection] {
				t.Fatal("background update or resize reopened the connection")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.explore = e
			restored.syncExploreTree()
			if open, saved := restored.expanded[connection]; open || !saved || restored.treeRows()[restored.treeCursor].key != connection {
				t.Fatal("restart or rediscovery lost the collapsed connection")
			}
			press(t, m, "right")
			if !m.expanded[connection] || m.focus != 0 || e.Detail != object {
				t.Fatal("Right did not reopen the connection without losing the resource")
			}
			if m.treeRows()[treeIndexFor(t, m, connection+"/view/kubernetes")].open {
				t.Fatal("reopening the connection exposed its previous resource path")
			}
			press(t, m, "left")
			press(t, m, "left")
			if m.treeRows()[m.treeCursor].key != m.active+"/monitor" {
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
				keys := map[string]string{"workspace": m.active, "tool": m.active + "/monitor", "connection": m.connection, "category": categoryTreeKey(m.connection, "Workloads")}
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
			keys := map[string]string{"workspace": m.active, "tool": m.active + "/monitor", "connection": m.connection, "category": categoryTreeKey(m.connection, "Workloads")}
			key := keys[kind]
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, key)
			connection, stream := m.connection, m.explore.Stream
			press(t, m, "left")
			if m.expanded[key] || m.connection != connection || m.mode != 1 || m.explore.Stream != stream || stream.Ctx.Err() != nil {
				t.Fatal("Left did not fold without interrupting the resource view")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.explore = m.explore
			restored.syncExploreTree()
			if open, saved := restored.expanded[key]; open || !saved {
				t.Fatal("restart or discovery reopened a folded branch")
			}
			press(t, m, "right")
			if !m.expanded[key] || m.connection != connection || m.mode != 1 || m.explore.Stream != stream {
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
	for _, query := range []string{"Shop", "Monitor", "test-context", "Workloads"} {
		for _, open := range []bool{false, true} {
			m := exploreModel(t)
			m.syncExploreTree()
			keys := map[string]string{"Shop": m.active, "Monitor": m.active + "/monitor", "test-context": m.connection, "Workloads": categoryTreeKey(m.connection, "Workloads")}
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
			if query == "test-context" && !open {
				for _, row := range m.treeRows() {
					if row.kind == "capability" && row.open || row.kind == "resource" {
						t.Fatal("opening a connection search result exposed its previous tool children")
					}
				}
			} else {
				treeIndexFor(t, m, connection+"/resource/pods")
			}
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
				want = m.active + "/monitor"
			case "workspace":
				want = m.active
			}
			if fold != "" {
				m.expanded[want] = false
			}
			m.explore.Preference.Filter = "checkout"
			m.explore.Preference.Selector = "app=checkout"
			m.explore.Scroll = 9
			preference, selected := m.explore.Preference, m.explore.Selected
			expansion := maps.Clone(m.expanded)
			m.focusRootSidebar()
			if m.focus != 0 || m.treeRows()[m.treeCursor].key != want {
				t.Fatalf("root back-navigation selected %q with focus %d, want %q", m.treeRows()[m.treeCursor].key, m.focus, want)
			}
			if m.explore.Preference != preference || m.explore.Selected != selected || m.explore.Scroll != 9 || !maps.Equal(expansion, m.expanded) {
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
	m.explore.Resources = append(m.explore.Resources, kubecommon.Resource{Name: "horizontalpodautoscalers", Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"})
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
			label.WriteString(strings.TrimSpace(ansi.Cut(line, 3+2*rows[long].depth, width)))
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
	m.explore.Resources = append(m.explore.Resources,
		kubecommon.Resource{Name: "statefulsets", Group: "apps", Version: "v1", Kind: "StatefulSet"},
		kubecommon.Resource{Name: "services", Version: "v1", Kind: "Service"},
		kubecommon.Resource{Name: "persistentvolumes", Version: "v1", Kind: "PersistentVolume"},
		kubecommon.Resource{Name: "events", Version: "v1", Kind: "Event"},
	)
	m.syncExploreTree()
	for _, category := range []string{"Network", "Storage", "Admin"} {
		m.expanded[categoryTreeKey(m.connection, category)] = false
	}
	view := ansi.Strip(m.treeBody(48, 60))
	lines := strings.Split(view, "\n")
	newRow := -1
	for i, line := range lines {
		if strings.Contains(line, "+ New Workspace") {
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

func TestSidebarWidthStaysStableAcrossExploreAndActivity(t *testing.T) {
	m := exploreModel(t)
	for _, width := range []int{80, 120, 160, 200} {
		m.width = width
		m.mode = 1
		want := m.panelWidths()[0]
		for mode := 2; mode < len(modes); mode++ {
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
	m.explore.Preference.Filter = "checkout"
	m.openExploreSelection(0)
	original := m.explore.Detail.DeepCopy()
	m.openNamespacePicker()
	m.explore.Namespaces.Manual = true
	press(t, m, "esc")
	if m.explore.Picker != "namespace" || m.explore.Namespaces.Manual || m.explore.Detail == nil || m.focus != 1 {
		t.Fatal("manual namespace Esc did not return to the picker alone")
	}
	press(t, m, "esc")
	if m.explore.Picker != "" || m.explore.Detail == nil || m.focus != 1 {
		t.Fatal("namespace picker Esc did not return to the detail alone")
	}
	typeText(m, "b")
	press(t, m, "esc")
	if m.shortcutPrefix != "" || m.explore.Detail == nil || m.focus != 1 {
		t.Fatal("Esc consumed more than the pending shortcut prefix")
	}
	m.explore.Trail = []monitor.ExploreLocation{{Preference: m.explore.Preference, Selected: m.explore.Selected, Object: original, Tab: 1}}
	press(t, m, "esc")
	if m.explore.Detail == nil || m.explore.Tab != 1 || len(m.explore.Trail) != 0 || m.focus != 1 {
		t.Fatal("related detail Esc did not restore its parent detail and section")
	}
	press(t, m, "esc")
	if m.explore.Detail != nil || m.explore.Preference.Filter != "checkout" || m.focus != 1 {
		t.Fatal("top-level detail Esc did not restore the filtered list alone")
	}
	press(t, m, "esc")
	if m.focus != 0 || m.explore.Preference.Filter != "checkout" {
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
	m.explore.Preference.Filter = "checkout"
	m.openExploreInput("filter", "unsubmitted")
	press(t, m, "esc")
	if m.explore.Input != nil || m.explore.Preference.Filter != "checkout" || m.focus != 1 {
		t.Fatal("filter Esc failed to preserve the committed filter in the list")
	}
	m.leftHidden = true
	press(t, m, "esc")
	if m.focus != 0 || m.leftHidden || m.explore.Preference.Filter != "checkout" {
		t.Fatal("root Esc did not reveal the sidebar with its applied filter preserved")
	}
}

func TestTabTogglesOnlyTreeAndMain(t *testing.T) {
	for name, setup := range map[string]func(*testing.T) *model{
		"workspace": func(t *testing.T) *model { m, _ := testModel(t); return m },
		"overview":  func(t *testing.T) *model { m, _ := overviewModel(t); return m },
		"resources": exploreModel,
		"yaml":      yamlViewModel,
		"helm":      func(t *testing.T) *model { m, _ := helmModel(t); return m },
		"gitops":    func(t *testing.T) *model { m, _ := gitOpsModel(t); return m },
	} {
		t.Run(name, func(t *testing.T) {
			m := setup(t)
			m.focus, m.leftHidden = 1, true
			m.shortcutPrefix = "i"
			m.draft().Text = "agent draft"
			press(t, m, "tab")
			if m.focus != 0 || m.leftHidden || !m.panelVisible(0) || m.shortcutPrefix != "" {
				t.Fatal("Tab did not reveal and focus tree")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab, IsRepeat: true})
			press(t, m, "shift+tab")
			if m.focus != 0 {
				t.Fatal("repeat or Shift+Tab moved panel focus")
			}
			press(t, m, "tab")
			if m.focus != 1 || m.draft().Text != "agent draft" {
				t.Fatal("Tab did not return to main with the agent draft intact")
			}
			press(t, m, "ctrl+]")
			press(t, m, "tab")
			if m.focus != 2 {
				t.Fatal("Tab moved out of the agent")
			}
			press(t, m, "ctrl+]")
			if m.focus != 1 {
				t.Fatal("Ctrl+] lost its return panel")
			}
		})
	}
}

func TestTabRestoresMainFullscreen(t *testing.T) {
	m, _ := testModel(t)
	typeText(m, "ff")
	press(t, m, "tab")
	if m.viewExpansion != nil || m.focus != 0 || !m.panelVisible(0) {
		t.Fatal("Tab did not restore the layout and focus tree")
	}
	press(t, m, "tab")
	if m.focus != 1 {
		t.Fatal("Tab did not return to main")
	}
}

func TestTabStaysLocalInInputAndOverlays(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		before := d.Text()
		press(t, m, "tab")
		if m.focus != 1 || d.Mode != mode || mode == docedit.Insert && d.Text() == before {
			t.Fatal("Tab escaped the editable document or lost indentation")
		}
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	for _, action := range []bool{false, true} {
		if action {
			f.Index = len(f.Fields)
		}
		press(t, m, "tab")
		if m.focus != 1 || m.form != f || m.current().Name != "Saved" || f.Fields[0].Value.Text != "Saved unfinished" {
			t.Fatal("Tab left or submitted the form")
		}
	}
	m.openHelp()
	press(t, m, "tab")
	if !m.helpVisible() || m.focus != 1 {
		t.Fatal("Tab escaped Help")
	}

	m, _ = testModel(t)
	for _, panel := range []int{0, 1} {
		m.focus = panel
		typeText(m, "/needle")
		press(t, m, "tab")
		if m.focus != panel || m.activePanelSearch(panel) == nil {
			t.Fatal("Tab escaped search input")
		}
		press(t, m, "esc")
	}
	typeText(m, ":s")
	command := m.command
	press(t, m, "tab")
	if !m.commandFocused() || m.command != command || command.Text != "s" {
		t.Fatal("Tab escaped command input")
	}
}

func TestSidebarBranchesKeepSiblingConnectionsCollapsed(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm"} {
		t.Run(tool, func(t *testing.T) {
			m, _ := testModel(t)
			if !m.saveWorkspace(workspace.Workspace{Name: "Folders", Connections: []workspace.Connection{
				{Tool: tool, Name: "First", Settings: map[string]string{"context": "first", "path": t.TempDir()}},
				{Tool: tool, Name: "Second", Settings: map[string]string{"context": "second", "path": t.TempDir()}},
			}}) {
				t.Fatal(m.status)
			}
			first, second := m.current().Connections[0].ID, m.current().Connections[1].ID
			m.focus, m.mode = 0, 0
			m.setTreeExpanded(m.active, false)
			m.treeCursor = treeIndexFor(t, m, m.active)
			press(t, m, "right")
			for _, row := range m.treeRows() {
				if row.kind == "connection" && row.open || row.kind == "capability" {
					t.Fatal("opening a workspace exposed a connection's children", row.label)
				}
			}
			m.treeCursor = treeIndexFor(t, m, first)
			press(t, m, "right")
			area := m.active + "/" + "monitor"
			m.treeCursor = treeIndexFor(t, m, area)
			press(t, m, "left")
			press(t, m, "right")
			if !m.treeRows()[treeIndexFor(t, m, first)].open || m.treeRows()[treeIndexFor(t, m, second)].open {
				t.Fatal("reopening a section changed its connections' fold choices")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range restored.treeRows() {
				if row.kind == "connection" && row.open || row.kind == "capability" {
					t.Fatal("restart exposed a connection's children", row.label)
				}
			}
		})
	}
}

func TestSidebarExpansionBeforeClusterDiscoveryShowsResources(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "current"}[current], func(t *testing.T) {
			m, _ := testModel(t)
			if current {
				m.currentPaths = currentFixture(t)
				resolveCurrentTest(t, m, "alpha")
			} else {
				if !m.saveWorkspace(workspace.Workspace{Name: "Saved", Connections: []workspace.Connection{{Name: "alpha", Tool: "kubernetes", TargetCaptured: true}}}) {
					t.Fatal(m.status)
				}
				m.selectScope(m.active, "kubernetes", m.current().Connections[0].ID)
			}
			ws, c := m.active, *m.savedConnection()
			m.expanded[m.currentTreeKey()], m.expanded[ws], m.expanded[c.ID] = true, true, true
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, c.ID+"/view/kubernetes")
			m.updateTree("right")
			// Remain in Overview; opening Explore must not be required for tree discovery.
			m.mode = 0
			cluster := m.clusterFor(ws, c)
			cluster.Run++
			_, cluster.Cancel = context.WithCancel(t.Context())
			t.Cleanup(m.stopClusters)
			m.Update(clusterDiscoveryResult{Cluster: cluster, Run: cluster.Run, Target: c.Target, Catalog: []kubecommon.Resource{{Name: "pods", Kind: "Pod", Version: "v1"}}})
			category := categoryTreeKey(c.ID, "Workloads")
			m.treeCursor = treeIndexFor(t, m, category)
			m.updateTree("right")
			treeIndexFor(t, m, c.ID+"/resource/pods")
			if m.focus != 0 || m.mode != 0 || m.connection != c.ID {
				t.Fatal("background discovery changed the selected view")
			}
		})
	}
}

func TestStartupResolutionPreservesEarlyTreeInteraction(t *testing.T) {
	for _, selected := range []string{"root", "monitor", "saved"} {
		for _, open := range []bool{false, true} {
			t.Run(selected+"/"+map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) {
				m := launchDirectoryModel(t)
				m.currentPaths = currentFixture(t)
				if !m.saveWorkspace(workspace.Workspace{Name: "Saved"}) {
					t.Fatal(m.status)
				}
				saved := m.active
				m.selectScope("", "", "")
				m.currentRoot = true
				m.currentRequest = &currentRequest{Select: true, Startup: true}
				resolve := m.syncCurrent()
				oldRoot := m.currentTreeKey()
				keys := map[string]string{"root": oldRoot, "monitor": "current/monitor", "saved": saved}
				key := keys[selected]
				m.focus, m.treeCursor = 0, treeIndexFor(t, m, key)
				m.setTreeExpanded(key, open)
				m.applyCurrent(resolve().(currentResult))
				if m.currentCluster == nil {
					t.Fatal("early tree navigation discarded the startup context")
				}
				if selected == "root" {
					key = m.currentTreeKey()
				} else if selected == "monitor" {
					key = m.currentCluster.ID + "/monitor"
				}
				if row := m.treeRows()[m.treeCursor]; row.key != key || row.open != open {
					t.Fatalf("startup changed selected branch or fold: %+v, want %s open=%t", row, key, open)
				}
				if m.focus != 0 || m.active != "" || m.connection != "" {
					t.Fatal("startup interrupted tree navigation or changed its scope")
				}
			})
		}
	}
}

func TestStartupPublishesContextWithoutInterruptingActiveInput(t *testing.T) {
	for _, interaction := range []string{"chat", "command", "form", "help", "settings", "saved"} {
		t.Run(interaction, func(t *testing.T) {
			m := launchDirectoryModel(t)
			m.currentPaths = currentFixture(t)
			result := m.syncCurrent()().(currentResult)
			switch interaction {
			case "chat":
				m.focusChat(true)
				m.draft().Text = "Keep typing"
			case "command":
				m.beginCommand()
				m.command.Text = "set"
			case "form":
				m.beginWorkspace()
				m.form.Fields[0].Value.Text = "Unfinished workspace"
			case "help":
				m.openHelp()
			case "settings":
				m.openSettings()
			case "saved":
				if !m.saveWorkspace(workspace.Workspace{Name: "Saved"}) {
					t.Fatal(m.status)
				}
			}
			active, connection, tool, focus, mode := m.active, m.connection, m.tool, m.focus, m.mode
			form, command, help, settings, draft := m.form, m.command, m.help, m.settingsOverlay, m.draft()
			m.Update(result)
			if m.currentCluster == nil || m.active != active || m.connection != connection || m.tool != tool || m.focus != focus || m.mode != mode || m.form != form || m.command != command || m.help != help || m.settingsOverlay != settings || m.draft() != draft {
				t.Fatal("startup lost the available context or interrupted the active interaction")
			}
		})
	}
}

func TestStartupAvailableContextMessageSurvivesPolling(t *testing.T) {
	m := launchDirectoryModel(t)
	m.currentPaths = currentFixture(t)
	resolve := m.syncCurrent()
	m.focus = 0
	m.applyCurrent(resolve().(currentResult))
	m.requestCurrent("", false)
	m.applyCurrent(m.syncCurrent()().(currentResult))
	body := m.currentBody(90, 25)
	if m.currentError != "" || !strings.Contains(body, "available in Monitor") || !strings.Contains(body, "Open current context") || strings.Contains(body, "Reading the terminal") {
		t.Fatal("a resolved context still appears to be loading", body)
	}
}

func TestSidebarDiscoveryRefreshReplacesCachedResourcesAndKeepsFolds(t *testing.T) {
	m := exploreModel(t)
	ws, c := m.active, *m.savedConnection()
	m.syncExploreTree()
	m.mode = 0
	m.explore.Key = ""
	category := categoryTreeKey(c.ID, "Workloads")
	m.setTreeExpanded(category, false)
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, category)
	cluster := m.clusterFor(ws, c)
	cluster.Run++
	_, cluster.Cancel = context.WithCancel(t.Context())
	t.Cleanup(m.stopClusters)
	m.Update(clusterDiscoveryResult{Cluster: cluster, Run: cluster.Run, Target: c.Target, Catalog: []kubecommon.Resource{{Name: "deployments", Kind: "Deployment", Group: "apps", Version: "v1"}}})
	if m.treeRows()[m.treeCursor].key != category || m.expanded[category] {
		t.Fatal("catalog refresh moved the selection or reopened a folder")
	}
	m.updateTree("right")
	rows := m.treeRows()
	if !slices.ContainsFunc(rows, func(row treeRow) bool { return row.resource == "deployments.apps" }) || slices.ContainsFunc(rows, func(row treeRow) bool { return row.resource == "pods" }) {
		t.Fatal("tree used the previous Explore catalog after shared discovery refreshed")
	}
}

func TestStartupClosesSavedToolBranchesUnderCollapsedConnections(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm"} {
		t.Run(tool, func(t *testing.T) {
			m := exploreModel(t)
			w := *m.current()
			w.Connections[0].Tool = tool
			if !m.saveWorkspace(w) {
				t.Fatal(m.status)
			}
			connection := m.connection
			m.expanded[connection] = false
			m.expanded[connection+"/view/kubernetes"] = true
			m.expanded[connection+"/view/helm"] = true
			category := categoryTreeKey(connection, "Workloads")
			m.expanded[category] = true
			m.autoExpanded = nil
			m.draft().Text = "Keep this draft"
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, connection)
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.updateTree("right")
			for _, row := range restored.treeRows() {
				if row.kind == "capability" && row.open || row.kind == "helmsection" || row.kind == "category" {
					t.Fatal("restored connection exposed saved tool children", row.label)
				}
			}
			if !restored.expanded[category] || restored.connection != "" || restored.drafts[m.conversationScope()].Text != "Keep this draft" {
				t.Fatal("closing tool branches changed the deeper folds, scope, or draft")
			}
		})
	}
}

func TestStartupCollapsesEveryConnectionAndKeepsWorkspacePreferences(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	active, connection := m.active, m.connection
	m.explore.Preference.Filter = "checkout"
	m.rememberExplore()
	m.draft().Insert("Keep this draft", true)
	if !m.saveWorkspace(workspace.Workspace{Name: "Other", Connections: []workspace.Connection{
		{Tool: "code", Name: "Source", Settings: map[string]string{"path": t.TempDir()}},
		{Tool: "terraform", Name: "Terraform"},
	}}) {
		t.Fatal(m.status)
	}
	for _, w := range m.workspaces {
		for _, c := range w.Connections {
			m.expanded[c.ID], m.autoExpanded[c.ID] = true, true
		}
	}
	m.active, m.connection, m.tool = active, connection, "kubernetes"
	m.treeCursor = treeIndexFor(t, m, connection+"/resource/pods")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.explore = m.explore
	restored.syncExploreTree()
	for _, w := range restored.workspaces {
		for _, c := range w.Connections {
			if open, saved := restored.expanded[c.ID]; open || !saved || restored.autoExpanded[c.ID] {
				t.Fatal("startup or rediscovery reopened a connection", c.Name)
			}
		}
	}
	if restored.active != "" || restored.connection != "" || restored.tool != "" || restored.mode != 0 || restored.drafts[m.conversationScope()].Text != "Keep this draft" {
		t.Fatal("startup collapse changed the selected scope or draft")
	}
	if restored.treeRows()[restored.treeCursor].key != connection || !restored.expanded[categoryTreeKey(connection, "Workloads")] {
		t.Fatal("startup lost the visible cursor ancestor or nested fold preference")
	}
	if restored.explorePreferences[active+"/"+connection].Filter != "checkout" {
		t.Fatal("startup changed the selected resource filter")
	}
}

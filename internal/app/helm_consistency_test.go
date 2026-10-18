package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
)

func inspectorFixture() *helmtool.Snapshot {
	return &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 2,
		Chart:  helmcommon.Chart{Metadata: map[string]any{"name": "demo"}, Values: map[string]any{"replicas": 1}},
		Config: map[string]any{"replicas": 2}, Effective: map[string]any{"replicas": 2, "password": "private-value"},
		Manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec:\n  replicas: 2\n  template:\n    spec:\n      containers:\n      - name: web\n        image: example/web:v2\n        ports: [{containerPort: 8080}]\n        env:\n        - name: CONFIG\n          valueFrom: {configMapKeyRef: {name: settings, key: options}}\n---\napiVersion: v1\nkind: Service\nmetadata: {name: web}\nspec: {ports: [{port: 80, targetPort: 8080}]}\n",
		History:  []helmtool.Revision{{Revision: 1, Status: "superseded"}, {Revision: 2, Status: "deployed"}},
	}
}

func TestHelmTimelinePagingKeepsActionIdentityVisible(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Entries[2] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Added: 12, Changes: []helmtool.RevisionChange{{Group: "resources", Label: strings.Repeat("long-resource-name-", 5), Action: "changed"}}}}
	h.Timeline.Entries[1] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Added: 1}}
	for _, width := range []int{28, 50, 100} {
		for _, height := range []int{5, 9, 16} {
			for i := 0; i < 8; i++ {
				m.helmTimelineKey(h, "pgdown")
				view := ansi.Strip(monitor.HelmTimelineBody(h, width, height))
				if !strings.Contains(view, "› Revision 2") || lipgloss.Width(view) > width || lipgloss.Height(view) > height {
					t.Fatalf("lost target at %dx%d: %s", width, height, view)
				}
			}
			m.helmTimelineKey(h, "enter")
			if h.JobRevision != 2 || h.RequestedRelease.Revision != 2 {
				t.Fatal("inspect differs from visible selection")
			}
			h.Pending = ""
		}
	}
	m.helmTimelineKey(h, "down")
	m.helmTimelineKey(h, "pgdown")
	_ = monitor.HelmTimelineBody(h, 50, 8)
	m.rollbackHelmSelection(h)
	if h.JobRevision != 1 || h.Pending != "rollback" {
		t.Fatal("rollback differs from visible selection")
	}
}

func TestHelmRefreshPreservesSectionResourceAndDocumentPosition(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Subcursor = 1
	m.updateHelm("right")
	h.Subcursor = 9
	m.updateHelm("right")
	h.Subcursor = 4
	refreshed := inspectorFixture()
	refreshed.Manifest = "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: aaa}\n---\n" + refreshed.Manifest
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "detail", Snapshot: refreshed})
	if h.Tab != 1 || h.Subcursor != 4 {
		t.Fatal("refresh reset current document")
	}
	m.updateHelm("left")
	if h.Subcursor != 9 {
		t.Fatal("values scroll lost")
	}
	m.updateHelm("left")
	resources, _ := monitor.HelmResources(h.Detail)
	if resources[h.Subcursor].Kind != "Service" {
		t.Fatal("refresh followed old index instead of resource identity")
	}
	m.updateHelm("enter")
	if h.Document == nil || h.Menu != nil || !strings.Contains(h.Document.Text(), "targetPort: 8080") {
		t.Fatal("Enter did not open complete resource directly")
	}
	h.Document = nil
	m.updateHelm("right")
	m.updateHelm("enter")
	m.updateHelm("down")
	m.updateHelm("enter")
	if h.ValuesVariant != 1 || strings.Contains(monitor.HelmDetailText(h), "private-value") {
		t.Fatal("wrong values source or masking")
	}
	h.Subcursor = 2
	h.ChooseInspectorDocument("values", 0)
	if h.Subcursor != 9 {
		t.Fatal("variant reading position lost")
	}
}

func TestHelmInspectorPickerSearchIsLiteralAndMapsExactIdentity(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Releases = []helmtool.Release{{Name: "shop", Namespace: "apps", Chart: "demo"}, {Name: "worker", Namespace: "other", Chart: "demo", Revision: 8}}
	typeText(m, "ff")
	if !m.viewExpanded() || h.Menu != nil || h.Input != nil {
		t.Fatal("fullscreen opened the release picker")
	}
	mainCommand(t, m, "f")
	if m.viewExpanded() || h.Menu != nil {
		t.Fatal("fullscreen command did not restore the inspector")
	}
	mainCommand(t, m, "J")
	if !strings.Contains(strings.Join(h.Menu, "\n"), "apps/shop") || !strings.Contains(h.Menu[0], "✓") {
		t.Fatal("current release not marked")
	}
	typeText(m, "worker")
	rows, _ := h.MenuChoices()
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.RequestedRelease.Key() != "other/worker" || h.RequestedRelease.Revision != 0 {
		t.Fatal("filtered picker selected wrong release")
	}
	h.Pending = ""
	h.RequestedRelease = nil
	typeText(m, "F")
	m.Update(tea.PasteMsg{Content: "bb :quit"})
	if m.leftHidden || m.command != nil || h.Input.Text != "bb :quit" {
		t.Fatal("picker input executed actions")
	}
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.Input == nil || h.Pending != "" {
		t.Fatal("empty filtered results selected a revision")
	}
	press(t, m, "esc")
	typeText(m, "F")
	typeText(m, "superseded")
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.JobRevision != 1 {
		t.Fatal("filtered revision picker selected wrong revision")
	}
}

func TestHelmFindStaysInPlaceAndSearchesLoadedDiffs(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Entries[1] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Diff: "+ unique-setting: true\n"}}
	typeText(m, "/")
	typeText(m, "unique-setting")
	if h.Timeline.Selected() != 1 || h.Document != nil {
		t.Fatal("timeline find did not select the matching revision")
	}
	press(t, m, "enter")
	typeText(m, "n")
	if h.Namespaces != nil {
		t.Fatal("find-next opened namespaces")
	}
	if !strings.Contains(ansi.Strip(m.helmBody(100, 26)), "unique-setting") {
		t.Fatal("matching diff not visible")
	}
	press(t, m, "esc")
	if h.Find != nil || h.Timeline.Selected() != 1 {
		t.Fatal("clearing search lost current revision")
	}
	h.ApplyInspectorSnapshot(inspectorFixture())
	typeText(m, "/")
	typeText(m, "Service")
	resources, _ := monitor.HelmResources(h.Detail)
	if resources[h.Subcursor].Kind != "Service" || h.Document != nil {
		t.Fatal("resource search left the inventory")
	}
	press(t, m, "esc")
	press(t, m, "right")
	typeText(m, "/")
	typeText(m, "replicas")
	if h.Document != nil || h.Find == nil {
		t.Fatal("values find opened a separate document")
	}
	view := ansi.Strip(m.helmBody(80, 24))
	if !strings.Contains(view, "matches") || strings.Contains(view, "private-value") {
		t.Fatal("missing match or leaked value", view)
	}
}

func TestHelmCompactFactsAndActionsFit(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	resources, _ := monitor.HelmResources(h.Detail)
	facts := ansi.Strip(monitor.HelmResourceSummary(resources[0], "apps", 100))
	for _, want := range []string{"Images", "example/web:v2", "8080", "settings"} {
		if !strings.Contains(facts, want) {
			t.Fatal("missing fact", want, facts)
		}
	}
	if strings.Contains(facts, "valueFrom:") {
		t.Fatal("preview still dumps containers")
	}
	for _, tab := range []int{5, 2, 1} {
		h.Tab = tab
		m.helmActions(h)
		if len(h.Menu) > 7 {
			t.Fatal("oversized context menu", h.Menu)
		}
		h.Menu = nil
		for _, size := range [][2]int{{28, 10}, {50, 16}, {100, 26}} {
			view := m.helmBody(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal(fmt.Sprint(size), ansi.Strip(view))
			}
		}
	}
}

func TestHelmRefreshAndPausedWorkflowPreserveReadingState(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Scroll = 4
	h.OpenTimeline(inspectorFixture())
	_ = monitor.HelmTimelineBody(h, 50, 8)
	if h.Timeline.Scroll != 4 {
		t.Fatal("loading refresh discarded timeline position")
	}
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Tab, h.ValuesVariant, h.Subcursor = 2, 2, 8
	h.SaveInspectorPosition()
	h.BeginWorkflow()
	m.leaveHelmWorkflow(h, false)
	other := inspectorFixture()
	other.Name = "another"
	h.ApplyInspectorSnapshot(other)
	m.resumeHelmWorkflow(h)
	m.leaveHelmWorkflow(h, true)
	if h.Detail.Name != "shop" || h.Tab != 2 || h.ValuesVariant != 2 || h.Subcursor != 8 {
		t.Fatal("paused action returned to wrong document or position")
	}
}

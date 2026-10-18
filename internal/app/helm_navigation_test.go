package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
)

func TestHelmCollectionArrowsPreserveSectionAndSelection(t *testing.T) {
	for _, section := range monitor.HelmSections {
		t.Run(section, func(t *testing.T) {
			m, h := helmModel(t)
			m.chooseHelmSection(section)
			h.Preference.Filter = "shop"
			h.Preference.Selected = "apps/shop"
			h.Cursor = 1
			preference, treeCursor := h.Preference, m.treeCursor
			for _, key := range []string{"left", "right"} {
				press(t, m, key)
				if h.Preference != preference || h.Cursor != 1 || m.treeCursor != treeCursor || m.focus != 1 || h.Busy || h.Pending != "" {
					t.Fatalf("%s changed the collection: preference=%+v cursor=%d focus=%d pending=%q", key, h.Preference, h.Cursor, m.focus, h.Pending)
				}
			}
			if strings.Contains(m.helmHint(), "←→") {
				t.Fatal("collection hint advertises horizontal navigation")
			}
			view := ansi.Strip(m.helmBody(28, 10))
			if strings.Contains(view, "←") || strings.Contains(view, "→") {
				t.Fatal("compact collection header advertises horizontal navigation")
			}
		})
	}
}

func TestHelmSidebarSelectsSectionsAndFoldsBranches(t *testing.T) {
	m, h := helmModel(t)
	branches := []string{m.active, m.active + "/monitor", m.connection, m.connection + "/view/helm"}
	for _, key := range branches {
		m.expanded[key] = true
	}
	for _, section := range monitor.HelmSections {
		m.focus = 0
		m.treeCursor = treeIndexFor(t, m, m.connection+"/helm/"+section)
		press(t, m, "enter")
		if h.Preference.Section != section || m.focus != 1 {
			t.Fatalf("sidebar did not open %s", section)
		}
	}
	preference := h.Preference
	for _, key := range branches {
		m.setTreeExpanded(key, true)
		m.focus = 0
		m.treeCursor = treeIndexFor(t, m, key)
		press(t, m, "left")
		if m.expanded[key] {
			t.Fatalf("left did not collapse %s", key)
		}
		press(t, m, "right")
		if !m.expanded[key] {
			t.Fatalf("right did not expand %s", key)
		}
		if h.Preference != preference || m.focus != 0 {
			t.Fatal("sidebar folding changed the main view or focus")
		}
	}
}

func TestHelmReleaseArrowsStayWithinDetails(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Namespace: "apps"}
	h.Tab = 5
	preference, treeCursor := h.Preference, m.treeCursor
	for _, step := range []struct {
		key string
		tab int
	}{{"right", 2}, {"left", 5}, {"left", 1}, {"right", 5}} {
		expectedPosition := h.InspectorPositions[fmt.Sprintf("%d/0", step.tab)]
		h.Subcursor = 5
		press(t, m, step.key)
		if h.Tab != step.tab || h.Subcursor != expectedPosition || h.Detail == nil {
			t.Fatalf("%s did not select detail section %d", step.key, step.tab)
		}
		if h.Preference != preference || m.treeCursor != treeCursor || m.focus != 1 {
			t.Fatal("detail navigation changed the sidebar selection or focus")
		}
	}
}

func TestHelmRetiredPlanSectionRestoresReleases(t *testing.T) {
	m, h := helmModel(t)
	p := h.Preference
	p.Section, p.Filter, p.Selected = "Plan", "shop", "apps/shop"
	m.helmPreferences[m.active+"/"+m.connection] = p
	delete(m.helmStates, h.Key)
	h = m.helmer()
	if h.Preference.Section != "Releases" || h.Preference.Filter != "shop" || h.Preference.Selected != "apps/shop" {
		t.Fatal("retired section lost the saved inventory preferences", h.Preference)
	}
	if m.chooseHelmSection("Plan") || m.executeHelm("plan") || canonicalCommand("P") == "plan" {
		t.Fatal("retired Plan navigation is still available")
	}
}

func TestHelmChartActionReturnsToOriginAndResumesAfterNavigation(t *testing.T) {
	for _, section := range []string{"Charts"} {
		t.Run(section, func(t *testing.T) {
			m, h := helmModel(t)
			m.chooseHelmSection(section)
			release := &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 3}
			h.Detail, h.Tab = release, 2
			h.Cursor, h.Subcursor = 2, 4
			m.beginHelmDraft(h)
			h.Draft.Values = "replicas: 3\n"
			draft := h.Draft
			if !h.WorkflowVisible() || h.Preference.Section != section {
				t.Fatal("action left its originating section")
			}
			h.Draft.Form.Index = len(h.Draft.Form.Fields)
			m.updateHelm("esc")
			if h.WorkflowVisible() || h.Cursor != 2 || h.Subcursor != 4 || h.Draft != draft {
				t.Fatal("return lost the origin or draft")
			}
			if h.Detail != release || h.Tab != 2 {
				t.Fatal("return lost the originating chart or release")
			}
			m.chooseHelmSection("Releases")
			m.helmActions(h)
			if !monitor.ContainsString(h.Menu, "Resume unfinished action") {
				t.Fatal("retained action is inaccessible")
			}
			h.Menu = []string{"Resume unfinished action"}
			h.Choice = 0
			m.activateHelmMenu(h)
			if !h.WorkflowVisible() || h.Preference.Section != section || h.Draft != draft || h.Draft.Values != "replicas: 3\n" {
				t.Fatal("resume lost the action or target")
			}
			h.Draft.Form.Index = len(h.Draft.Form.Fields) + 2
			m.updateHelm("enter")
			if h.Workflow != nil || h.Draft != nil || h.Plan != nil || h.Pending != "" {
				t.Fatal("cancel did not discard the action without a write")
			}
		})
	}
}

func TestHelmRollbackPreviewReturnsToSelectedRevision(t *testing.T) {
	m, h := helmModel(t)
	snapshot := &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 4}
	h.Detail, h.Tab, h.Subcursor = snapshot, 5, 2
	plan := &helmtool.Plan{Operation: "rollback", Name: "shop", Namespace: "apps", CurrentRevision: 4, TargetRevision: 2}
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "rollback", Plan: plan})
	if !h.WorkflowVisible() || h.Preference.Section != "Releases" {
		t.Fatal("rollback preview left Releases")
	}
	m.updateHelm("esc")
	if h.Detail != snapshot || h.Tab != 5 || h.Subcursor != 2 || h.Plan != plan || h.WorkflowVisible() {
		t.Fatal("rollback return lost the selected revision")
	}
	m.executeHelm("rollback")
	if h.Pending != "" || h.Confirm {
		t.Fatal("another rollback replaced the retained preview")
	}
	m.resumeHelmWorkflow(h)
	m.executeHelm("rollback")
	if !h.Confirm || h.Choice != 0 || h.Plan != plan {
		t.Fatal("resumed rollback lost its preview or safe default")
	}
	m.updateHelm("enter")
	if h.Pending != "" || h.Confirm {
		t.Fatal("default confirmation submitted a write")
	}
}

func TestHelmPausedPreviewKeepsReviewedTargetAndRejectsLateResults(t *testing.T) {
	m, h := helmModel(t)
	m.chooseHelmSection("Charts")
	m.beginHelmDraft(h)
	h.Busy, h.Running, h.Run = true, "prepare", 10
	cancelled := false
	h.Cancel = func() { cancelled = true }
	m.chooseHelmSection("Releases")
	m.updateHelmResult(monitor.HelmResult{State: h, Run: 10, Kind: "prepare", Plan: &helmtool.Plan{Namespace: "wrong"}})
	if !cancelled || h.Plan != nil || h.WorkflowVisible() {
		t.Fatal("late preview replaced the selected view")
	}
	h.Plan = &helmtool.Plan{Operation: "rollback", Namespace: "reviewed", Name: "demo"}
	m.executeHelm("rollback")
	if h.Confirm || h.Pending != "" {
		t.Fatal("hidden preview opened an installation confirmation")
	}
	m.chooseHelmNamespace(h, "other")
	m.resumeHelmWorkflow(h)
	m.executeHelm("namespace moved")
	if h.Preference.Section != "Charts" || h.Preference.Namespace != "apps" || h.Plan.Namespace != "reviewed" || h.Namespaces != nil {
		t.Fatal("resuming the preview changed its captured target")
	}
	m.confirmHelm(h)
	if !h.Confirm || h.Choice != 0 {
		t.Fatal("resumed preview cannot be confirmed explicitly")
	}
}

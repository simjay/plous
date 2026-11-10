package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
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
			labels, _ := h.MenuChoices()
			if !monitor.ContainsString(labels, "Resume unfinished action") {
				t.Fatal("retained action is inaccessible")
			}
			h.Menu = []monitor.HelmMenuItem{monitor.HelmResumeWorkflow.Item()}
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

type helmExperienceRunner func(context.Context, helmtool.Config, []string, string) ([]byte, error)

func (f helmExperienceRunner) Run(c context.Context, cfg helmtool.Config, args []string, in string) ([]byte, error) {
	return f(c, cfg, args, in)
}

func TestHelmTimelineOrdersLoadsAndReturnsFromExactRevision(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "shop", Namespace: "apps", Revision: 4, Chart: "demo"}}
	m.updateHelm("enter")
	if h.Pending != "timeline" {
		t.Fatal("Releases did not open history")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "timeline", Snapshot: &helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 1}, {Revision: 4}, {Revision: 3}}}})
	h.Pending = ""
	timeline := h.Timeline
	if timeline.Selected() != 4 || timeline.Revisions[2].Revision != 1 {
		t.Fatal("history not newest first")
	}
	m.updateHelm("down")
	timeline.Scroll = 2
	m.updateHelm("enter")
	if h.Pending != "detail" || h.JobRevision != 3 || h.RequestedRelease.Name != "shop" {
		t.Fatal("wrong inspector target")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "detail", Snapshot: &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 3}})
	h.Pending = ""
	if h.Tab != 5 {
		t.Fatal("inspector does not start with resources")
	}
	m.updateHelm("esc")
	if h.Timeline != timeline || timeline.Cursor != 1 || timeline.Scroll != 2 || h.Detail != nil {
		t.Fatal("back lost timeline state")
	}
	m.updateHelm("B")
	if h.JobRevision != 3 || h.Pending != "rollback" {
		t.Fatal("timeline rollback target changed")
	}
	h.Pending = ""
	h.BeginWorkflow()
	m.leaveHelmWorkflow(h, true)
	if h.Timeline != timeline || timeline.Selected() != 3 {
		t.Fatal("workflow lost timeline origin")
	}
	m.updateHelm("esc")
	if h.Timeline != nil || h.Cursor != 0 {
		t.Fatal("timeline did not return to selected release")
	}
}

func TestHelmTimelineUnavailableBaselineStaleResultsAndReadScope(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(&helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 7}, {Revision: 1}}})
	var calls []string
	h.Client.Runner = helmExperienceRunner(func(_ context.Context, _ helmtool.Config, args []string, _ string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] != "status" || args[1] != "shop" || !strings.Contains(strings.Join(args, " "), "--namespace apps") {
			t.Fatal("unexpected read", args)
		}
		return json.Marshal(&helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 1, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo"}}})
	})
	cmd := h.SyncTimeline()
	if cmd == nil || !strings.Contains(h.Timeline.Entries[7].Err, "Baseline revision 6 unavailable") {
		t.Fatal("missing predecessor was silently substituted")
	}
	result := cmd().(monitor.HelmResult)
	if len(calls) != 1 || strings.Contains(calls[0], "--revision 7") {
		t.Fatal("wrong baseline read", calls)
	}
	old := h.Timeline
	m.chooseHelmNamespace(h, "production")
	m.updateHelmResult(result)
	if h.Timeline != nil || len(old.Entries) != 1 {
		t.Fatal("stale result restored another namespace")
	}
}

func TestHelmInspectorSelectorsKeepIdentityAndLatestRelease(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Filter = "first"
	h.Releases = []helmtool.Release{{Name: "first", Namespace: "apps", Chart: "demo", Revision: 4}, {Name: "second", Namespace: "apps", Chart: "demo", Revision: 7}, {Name: "unrelated", Namespace: "apps", Chart: "other", Revision: 1}}
	h.Detail = &helmtool.Snapshot{Name: "first", Namespace: "apps", Version: 3, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo"}}, History: []helmtool.Revision{{Revision: 3}, {Revision: 4}}}
	m.updateHelm("J")
	if len(h.Menu) != 2 {
		t.Fatal("release selector followed text filter or included unrelated charts", h.Menu)
	}
	m.updateHelm("down")
	m.updateHelm("enter")
	if h.RequestedRelease.Name != "second" || h.RequestedRelease.Revision != 0 || h.Pending != "detail" {
		t.Fatal("release switch did not request latest")
	}
	h.Pending = ""
	h.RequestedRelease = nil
	m.updateHelm("F")
	if !strings.HasPrefix(h.Menu[0].Label, "Revision 4") || h.Choice != 1 {
		t.Fatal("revision selector lost historical selection")
	}
	m.updateHelm("home")
	m.updateHelm("enter")
	if h.JobRevision != 4 {
		t.Fatal("revision selector ignored exact revision")
	}
}

func TestHelmChartsAndInspectorFitAndSeparateHooks(t *testing.T) {
	m, h := helmModel(t)
	m.chooseHelmSection("Charts")
	for i := 0; i < 12; i++ {
		h.Releases = append(h.Releases, helmtool.Release{Name: fmt.Sprintf("release-%02d", i), Namespace: "apps", Chart: fmt.Sprintf("chart-%d", i/4), Version: fmt.Sprint(i%2 + 1), Revision: i + 1, Status: "deployed"})
	}
	for _, size := range [][2]int{{28, 10}, {50, 16}, {90, 26}, {150, 40}} {
		for _, cursor := range []int{0, 5, 11} {
			h.Cursor = cursor
			view := m.helmBody(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), fmt.Sprintf("release-%02d", cursor)) {
				t.Fatalf("card selection clipped at %v: %s", size, ansi.Strip(view))
			}
		}
	}
	snapshot := &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 1, Manifest: "# Source: demo/templates/config.yaml\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: config}\ndata: {password: hidden}\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 2}\n"}
	hookData, _ := json.Marshal(map[string]any{"hooks": []map[string]any{{"name": "preflight", "events": []string{"pre-upgrade"}, "manifest": "apiVersion: batch/v1\nkind: Job\nmetadata: {name: preflight}\n"}}})
	if err := json.Unmarshal(hookData, snapshot); err != nil {
		t.Fatal(err)
	}
	h.Detail = snapshot
	h.Tab = 5
	resources, err := monitor.HelmResources(snapshot)
	if err != nil || resources[0].Kind != "Deployment" || resources[1].Group != "Configuration" || len(resources) != 3 || resources[2].Group != "Hooks" {
		t.Fatal("inventory not grouped by purpose", err, resources)
	}
	for _, size := range [][2]int{{28, 10}, {50, 16}, {100, 26}, {150, 40}} {
		view := m.helmBody(size[0], size[1])
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), "web") {
			t.Fatalf("inspector selection clipped at %v: %s", size, ansi.Strip(view))
		}
	}
	h.Subcursor = 1
	m.updateHelm("enter")
	m.updateHelm("enter")
	if h.Document == nil || strings.Contains(h.Document.Text(), "hidden") || !strings.Contains(h.Document.Text(), "config") {
		t.Fatal("resource selection or masking lost after grouping")
	}
}

func TestHelmChartSectionReturnPreservesCardAndTimelineCancellation(t *testing.T) {
	m, h := helmModel(t)
	m.chooseHelmSection("Charts")
	h.Cursor = 4
	m.chooseHelmSection("Releases")
	h.Cursor = 2
	m.chooseHelmSection("Charts")
	if h.Cursor != 4 {
		t.Fatal("return to Charts lost the card position")
	}
	h.OpenTimeline(&helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 3}, {Revision: 2}, {Revision: 1}}})
	cancelled := false
	job := &monitor.HelmTimelineLoad{Revision: 3, Cancel: func() { cancelled = true }}
	h.Timeline.Load = job
	m.helmTimelineKey(h, "down")
	if !cancelled || h.Timeline.Load != nil {
		t.Fatal("a slow previous comparison blocked the newly selected revision")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Kind: "revision-diff", TimelineLoad: job, Changes: &helmtool.RevisionChanges{Added: 42}})
	if len(h.Timeline.Entries) != 0 {
		t.Fatal("cancelled comparison was accepted")
	}
}

func TestHelmActionDispatchUsesIdentityInsteadOfDisplayedLabel(t *testing.T) {
	m, h := helmModel(t)
	m.helmActions(h)
	for i := range h.Menu {
		if h.Menu[i].Action == monitor.HelmChooseNamespace {
			h.Choice = i
			// A label that belongs to another action must not change execution.
			h.Menu[i].Label = "Refresh releases"
			pending := h.Pending
			m.activateHelmMenu(h)
			if h.Namespaces == nil || h.Pending != pending {
				t.Fatal("displayed text selected an action instead of its identity")
			}
			return
		}
	}
	t.Fatal("namespace action was not available")
}

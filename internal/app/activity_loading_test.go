package app

import (
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/workspace"
	"testing"
)

func completeActivity(t *testing.T, m *model) {
	t.Helper()
	if m.activityUI.Load == nil {
		m.syncActivity()
	}
	if m.activityUI.Load != nil {
		result := activity.ReadActivity(m.store, m.activityUI.Load)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		m.applyActivity(result)
	}
}

func TestActivityLoadsOnlyWhenRequestedAndRejectsStaleResult(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "First")
	first := m.active
	if !m.activityUI.Pending || m.syncActivity() != nil {
		t.Fatal("history read during another view")
	}
	m.execute("3")
	cmd := m.syncActivity()
	if cmd == nil {
		t.Fatal("activity did not start its read")
	}
	result := cmd().(activity.ActivityResult)
	saved, err := m.store.Save(workspace.Workspace{Name: "Second"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	m.workspaces = append(m.workspaces, saved)
	m.selectScope(saved.ID, "", "")
	second := m.active
	m.applyActivity(result)
	if len(m.activityUI.Changes) != 0 || m.activityUI.Workspace != second {
		t.Fatal("old workspace result escaped scope")
	}
	completeActivity(t, m)
	if len(m.activityUI.Changes) != 1 || m.activityUI.Changes[0].WorkspaceID == first {
		t.Fatal("current history not loaded")
	}
	cached := m.activityEntries()
	if len(cached) != 1 {
		t.Fatal(cached)
	}
	m.selectScope(second, "helm", "")
	if m.activityUI.Pending {
		t.Fatal("tool selection reread workspace history")
	}
	m.loadHistory()
	cmd = m.syncActivity()
	old := cmd().(activity.ActivityResult)
	m.loadHistory()
	m.applyActivity(old)
	if !m.activityUI.Pending {
		t.Fatal("cancelled reload accepted")
	}
}

func TestActivityRefreshKeepsSelectedIdentity(t *testing.T) {
	m := &model{active: "workspace", mode: 2, positions: map[string]int{}, activityUI: activity.Controller{Changes: []workspace.Change{{ID: "older", After: workspace.Workspace{Name: "Old"}}}}}
	run := &activity.ActivityLoad{Workspace: m.active}
	m.activityUI.Load = run
	m.applyActivity(activity.ActivityResult{Load: run, Changes: []workspace.Change{{ID: "newer", After: workspace.Workspace{Name: "New"}}, {ID: "older", After: workspace.Workspace{Name: "Old"}}}})
	if m.positions[m.viewKey()] != 1 {
		t.Fatal("new entries moved selection to a different change")
	}
}

func TestActivityCompletionDoesNotChangeOtherViewPosition(t *testing.T) {
	m := &model{active: "workspace", mode: 1, positions: map[string]int{}, activityUI: activity.Controller{Changes: []workspace.Change{{ID: "older", After: workspace.Workspace{Name: "Old"}}}}}
	run := &activity.ActivityLoad{Workspace: m.active}
	m.activityUI.Load = run
	m.positions[m.viewKey()] = 7
	m.applyActivity(activity.ActivityResult{Load: run, Changes: []workspace.Change{{ID: "newer", After: workspace.Workspace{Name: "New"}}, {ID: "older", After: workspace.Workspace{Name: "Old"}}}})
	if m.positions[m.viewKey()] != 7 {
		t.Fatal("background Activity completion changed Explore position")
	}
	m.mode = 2
	if m.positions[m.viewKey()] != 1 {
		t.Fatal("background completion lost Activity selection")
	}
}

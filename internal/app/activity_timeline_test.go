package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/workspace"
)

func activityTimelineModel(t *testing.T) *model {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Activity fixture")
	mainCommand(t, m, "3")
	completeActivity(t, m)
	at := time.Date(2026, 10, 3, 12, 30, 0, 0, time.UTC)
	m.activityUI.Operations = []workspace.Operation{
		{ID: "install", WorkspaceID: m.active, ConnectionID: "cluster", Tool: "helm", Kind: "install", Release: "checkout", Namespace: "apps", Context: "test", Status: "submitted", ObservedAt: at},
		{ID: "first", WorkspaceID: m.active, ConnectionID: "cluster", ConnectionFingerprint: "config", Tool: "helm", Kind: "revision", Release: "checkout", Namespace: "apps", Context: "test", Status: "deployed", Revision: 1, SourceTime: at.Add(-time.Hour).Format(time.RFC3339), ObservedAt: at},
		{ID: "second", WorkspaceID: m.active, ConnectionID: "cluster", ConnectionFingerprint: "config", Tool: "helm", Kind: "revision", Release: "checkout", Namespace: "apps", Context: "test", Status: "superseded", Revision: 1, SourceTime: at.Add(-time.Hour).Format(time.RFC3339), ObservedAt: at.Add(time.Hour)},
	}
	m.activityUI.Inventory = nil
	return m
}

func TestActivityTimelineFramesPreviewAndSelection(t *testing.T) {
	m := activityTimelineModel(t)
	for _, size := range [][2]int{{80, 16}, {100, 24}, {120, 30}, {160, 40}, {200, 50}, {240, 60}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			press(t, m, "end")
			view := ansi.Strip(m.render())
			rows := strings.Split(view, "\n")
			if len(rows) != size[1] {
				t.Fatalf("height %d", len(rows))
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != size[0] {
					t.Fatalf("row width %d: %q", ansi.StringWidth(row), row)
				}
			}
			if !strings.Contains(view, "›") {
				t.Fatal("selected row disappeared")
			}
			if size[0] >= 200 && !strings.Contains(view, "Preview") {
				t.Fatal("wide frame has no preview")
			}
			if size[0] <= 120 && strings.Contains(view, "Preview") {
				t.Fatal("preview crowded narrow frame")
			}
		})
	}
}

func TestActivityTimelineFiltersDetailsObservationsAndMouse(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 240, 60
	clickMouse(t, m, "main", "activity-type/1")
	if m.activityUI.Filter != activity.PlousActions || len(m.activityEntries()) != 1 {
		t.Fatal("type control did not apply")
	}
	press(t, m, "right")
	press(t, m, "right")
	if m.activityUI.Filter != activity.HelmHistory || len(m.activityEntries()) != 1 {
		t.Fatal("observations were not grouped")
	}
	typeText(m, "/checkout")
	listSearch := m.searches[1]
	selected := m.activityEntries()[0].Identity()
	clickMouse(t, m, "main", "activity/"+selected)
	press(t, m, "enter")
	if m.activityListSearch != listSearch || m.searches[1] != nil || m.activityRecord.Identity() != selected {
		t.Fatal("detail lost filtered entry")
	}
	clickMouse(t, m, "main", "command/observations")
	if !strings.Contains(m.detail, "Record: first") || !strings.Contains(m.detail, "Record: second") {
		t.Fatal("original observations inaccessible")
	}
	typeText(m, "O")
	if strings.Contains(m.detail, "Record: first") {
		t.Fatal("observations did not fold")
	}
	typeText(m, "/retained")
	press(t, m, "esc")
	if m.detail == "" {
		t.Fatal("detail find Escape closed record")
	}
	press(t, m, "esc")
	if m.detail != "" || m.searches[1] != listSearch || m.activityUI.Filter != activity.HelmHistory || m.activityEntries()[m.activityPosition()].Identity() != selected {
		t.Fatal("Back lost type, search, or selection")
	}
	press(t, m, "esc")
	if m.searches[1] != nil || m.detail != "" {
		t.Fatal("list search Escape failed")
	}
	press(t, m, "right")
	if m.activityUI.Filter != activity.All {
		t.Fatal("type navigation failed")
	}
	m.execute("g")
	if !m.activityUI.Pending {
		t.Fatal("refresh did not reload history")
	}
}

func TestActivityTypeNavigationPreservesOtherPanelForm(t *testing.T) {
	m := activityTimelineModel(t)
	m.beginWorkspace()
	form := m.form
	m.focus = 2
	press(t, m, "right")
	if m.form != form || m.activityUI.Filter != activity.All {
		t.Fatal("type navigation discarded form")
	}
}

func TestActivityTimelineLoadingEmptyAndErrorStayBounded(t *testing.T) {
	m := activityTimelineModel(t)
	m.activityUI.Changes = nil
	m.activityUI.Operations = nil
	m.activityUI.Inventory = nil
	for _, state := range []string{"empty", "loading", "error", "filtered"} {
		t.Run(state, func(t *testing.T) {
			m.activityUI.Pending = state == "loading"
			m.activityUI.Error = ""
			m.activityUI.Filter = activity.All
			want := "No activity yet"
			if state == "loading" {
				want = "Loading activity"
			}
			if state == "error" {
				want = "Activity unavailable"
				m.activityUI.Error = want + ": fixture unavailable"
			}
			if state == "filtered" {
				want = "No matching activity"
				m.activityUI.Filter = activity.PlousActions
			}
			for _, size := range [][2]int{{80, 16}, {240, 60}} {
				m.width, m.height = size[0], size[1]
				rows := strings.Split(ansi.Strip(m.render()), "\n")
				if len(rows) != size[1] {
					t.Fatal("overflowed height")
				}
				for _, row := range rows {
					if ansi.StringWidth(row) != size[0] {
						t.Fatal("overflowed width")
					}
				}
				if !strings.Contains(strings.Join(rows, "\n"), want) {
					t.Fatalf("missing %s", want)
				}
			}
		})
	}
}

package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/workspace"
)

func TestActivityOldPositionMovesImmediately(t *testing.T) {
	m := activityTimelineModel(t)
	m.positions[m.viewKey()] = 100
	press(t, m, "up")
	if got, want := m.positions[m.viewKey()], len(m.activityEntries())-2; got != want {
		t.Fatalf("Up did not move from the visible row: %d, want %d", got, want)
	}
}

func TestActivitySearchEscapeTracksIdentityAcrossRefreshAndType(t *testing.T) {
	m := activityTimelineModel(t)
	m.positions[m.viewKey()] = 1
	original := m.activityEntries()[1].Identity()
	typeText(m, "/checkout")
	run := &activity.ActivityLoad{Workspace: m.active}
	m.activityUI.Load = run
	changes := append([]workspace.Change{{ID: "newest", Time: time.Now().Add(time.Hour), After: workspace.Workspace{Name: "New"}}}, m.activityUI.Changes...)
	m.applyActivity(activity.ActivityResult{Load: run, Changes: changes, Operations: m.activityUI.Operations})
	press(t, m, "esc")
	if m.activityEntries()[m.positions[m.viewKey()]].Identity() != original {
		t.Fatal("Escape restored an index rather than the original record")
	}
	typeText(m, "/checkout")
	press(t, m, "right")
	press(t, m, "esc")
	if m.activityPosition() >= len(m.activityEntries()) {
		t.Fatal("type change restored an out-of-range selection")
	}
}

func TestActivityCompactObservationControlsAndContent(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 80, 16
	m.activityUI.Filter = activity.HelmHistory
	m.openActivityEntry(m.activityEntries()[0])
	view := ansi.Strip(m.render())
	if !strings.Contains(view, "O Observations") {
		t.Fatal("compact controls hid observations")
	}
	if strings.Contains(view, "Enter Open release") {
		t.Fatal("removed connection offers a dead release link")
	}
	typeText(m, "O")
	if !strings.Contains(ansi.Strip(m.render()), "superseded") {
		t.Fatal("expanding observations left their content below the viewport")
	}
}

func TestActivityShortErrorKeepsRetryAndFullDetailsAccessible(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 80, 16
	m.activityUI.Error = "Activity unavailable: " + strings.Repeat("long/", 40) + "permission denied"
	view := ansi.Strip(m.render())
	if !strings.Contains(view, "g Retry") || !strings.Contains(view, "d Error") {
		t.Fatal("retry/details controls clipped behind error")
	}
	m.execute("describe")
	if !strings.Contains(m.detail, "permission denied") {
		t.Fatal("complete error inaccessible")
	}
	press(t, m, "esc")
	if m.detail != "" {
		t.Fatal("error details did not return to timeline")
	}
}

func TestActivityHelpShowsOnlyAvailableActions(t *testing.T) {
	m := activityTimelineModel(t)
	m.activityUI.Filter = activity.PlousActions
	m.openActivityEntry(m.activityEntries()[0])
	m.execute("help")
	m.help.section = 1
	view := ansi.Strip(m.renderHelpOverlay())
	if strings.Contains(view, "observations") || strings.Contains(view, "Enter Open the recorded release") {
		t.Fatal("help offered unavailable record actions")
	}
}

func TestActivityDetailScrollDoesNotAccumulatePastEnd(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 80, 16
	m.activityUI.Filter = activity.HelmHistory
	m.openActivityEntry(m.activityEntries()[0])
	typeText(m, "O")
	m.updateMain("end")
	end := m.cursor
	for range 50 {
		m.updateMain("down")
	}
	if m.cursor != end {
		t.Fatal("scroll accumulated beyond visible end")
	}
	m.updateMain("up")
	if m.cursor != end-1 {
		t.Fatal("Up did not move immediately from the last page")
	}
	m.updateMain("home")
	if m.cursor != 0 || !strings.Contains(ansi.Strip(m.render()), "superseded") {
		t.Fatal("Home lost the expanded observation header")
	}
}

func TestActivityTypeControlsAreDirectAndKeepSearchLiteral(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 240, 60
	clickMouse(t, m, "main", "activity-type/3")
	if m.activityUI.Filter != activity.HelmHistory {
		t.Fatal("type chip did not choose Helm")
	}
	typeText(m, "/checkout")
	clickMouse(t, m, "main", "activity-type/1")
	if m.activityUI.Filter != activity.PlousActions || m.searches[1].input.Text != "checkout" {
		t.Fatal("type chip discarded search")
	}
	typeText(m, "T")
	if m.searches[1].input.Text != "checkoutT" {
		t.Fatalf("shortcut captured search text: %q (focus %d, filter %s)", m.searches[1].input.Text, m.focus, m.activityUI.Filter)
	}
	press(t, m, "esc")
	m.width, m.height = 80, 16
	clickMouse(t, m, "main", "activity-type/0")
	if m.activityUI.Filter != activity.All {
		t.Fatal("compact previous arrow advanced instead of going back")
	}
}

func TestActivityTransientTypesDoNotSaveFilteredPositions(t *testing.T) {
	m := activityTimelineModel(t)
	m.positions[m.activityKey()] = 2
	press(t, m, "right")
	m.updateMain("home")
	if m.activityPosition() != 0 || m.positions[m.activityKey()] != 2 {
		t.Fatal("filtered position replaced All activity's selection")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Positions[m.activityKey()] != 2 {
		t.Fatal("transient type cursor saved as an All activity position")
	}
}

func TestActivityMouseSelectionPausesAndResumesSearch(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 240, 60
	typeText(m, "/checkout")
	selected := m.activityEntries()[0].Identity()
	clickMouse(t, m, "main", "activity/"+selected)
	if !m.searches[1].finished {
		t.Fatal("row selection did not leave search editing")
	}
	if strings.Contains(m.hint(), "Type to filter") || strings.Contains(ansi.Strip(m.searchBar(1, 80, "Search activity")), "n/N") {
		t.Fatal("paused list search advertised text input or text-find keys")
	}
	typeText(m, "/T")
	if m.searches[1].finished || m.searches[1].input.Text != "checkoutT" {
		t.Fatal("resuming search cleared the query or captured its keys")
	}
}

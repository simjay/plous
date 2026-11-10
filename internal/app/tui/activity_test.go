package tui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func TestActivityDetailColorsWrappingAndNavigation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "3")
	before := *m.current()
	after := before
	after.Name = "Payments production"
	after.Connections = []workspace.Connection{{ID: "connection-id", Tool: "kustomize", Name: "Production", Settings: map[string]string{"path": strings.Repeat("/長い-path", 20) + "\nsecond line\x1b[2J"}}}
	change := workspace.Change{Before: &before, After: after, Revision: 2, Actor: "user", Time: time.Now()}
	m.activityUI.Changes = []workspace.Change{change}
	press(t, m, "enter")
	if strings.Contains(m.detail, "\x1b[2J") || !strings.Contains(ansi.Strip(m.detail), "second line") {
		t.Fatal("detail must preserve multiline values and strip terminal controls")
	}
	if !strings.Contains(m.viewContent(120, 100), ui.AddedStyle.Render("+ After   Payments production")) {
		t.Fatal("detail rendering stripped change colors")
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, offset := range []int{0, 8, 10000} {
				m.cursor = offset
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatal("detail overflowed the terminal height")
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != size[0] {
						t.Fatalf("detail overflowed a terminal row: %q", line)
					}
				}
			}
		})
	}
	press(t, m, "esc")
	if m.detail != "" || m.mode != 2 || m.positions[m.viewKey()] != 0 {
		t.Fatal("Esc must return to the selected Activity event")
	}
}

func activityReviewModel() (*model, workspace.Change) {
	before := workspace.Workspace{ID: "workspace-one", Name: "Shop", Revision: 5, Connections: []workspace.Connection{{
		ID: "cluster-one", Tool: "kubernetes", Name: "Production", Settings: map[string]string{"namespace": "old", "context": "production"},
	}}}
	after := before
	after.Revision = 9
	after.Connections = []workspace.Connection{{ID: "cluster-one", Tool: "kubernetes", Name: "Production", Settings: map[string]string{"namespace": "shop", "context": "production"}}}
	change := workspace.Change{ID: "change-nine", WorkspaceID: before.ID, Revision: 9, Actor: "user", Time: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), Before: &before, After: after}
	m := &model{active: after.ID, mode: 2, focus: 1, workspaces: []workspace.Workspace{after}, positions: map[string]int{}}
	return m, change
}

func TestActivityReviewCapturesExactImmutableSnapshots(t *testing.T) {
	m, change := activityReviewModel()
	m.setActivityDetail(change)
	r := m.activityReview
	if r.Comparison != nil || r.Open || !m.activityDetailActive() {
		t.Fatal("summary must remain the default and build comparison only on request")
	}
	for _, sample := range []struct {
		text string
		want workspace.Workspace
	}{{r.Before, *change.Before}, {r.After, change.After}} {
		var got workspace.Workspace
		if err := json.Unmarshal([]byte(sample.text), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, sample.want) {
			t.Fatal("comparison must include every stored workspace field")
		}
	}
	change.Before.Connections[0].Settings["namespace"] = "replaced-before"
	change.After.Connections[0].Settings["namespace"] = "replaced-after"
	if !m.openActivityReview() {
		t.Fatal("could not open comparison from captured summary")
	}
	if strings.Contains(r.Before+r.After, "replaced-") {
		t.Fatal("later history or workspace updates replaced the captured snapshots")
	}
	if r.Comparison.BeforeLabel != "Revision 5" || r.Comparison.AfterLabel != "Revision 9" {
		t.Fatal("comparison must label the actual snapshot revisions")
	}
	if r.Comparison.CanConfirm || r.Comparison.ConfirmLabel != "" || r.Comparison.Confirm().Action != "" {
		t.Fatal("Activity must never offer a confirmation/write path")
	}
	if !strings.Contains(r.Comparison.Target, "Read-only") {
		t.Fatal("read-only state must be visible")
	}
}

func TestActivityReviewCreationAndDeterministicIdentity(t *testing.T) {
	m, change := activityReviewModel()
	change.Before = nil
	change.After.Revision, change.Revision = 1, 1
	m.setActivityDetail(change)
	m.openActivityReview()
	if m.activityReview.Before != "" || m.activityReview.Comparison.BeforeLabel != "Empty workspace" {
		t.Fatal("creation must compare an empty workspace with revision one")
	}
	identity, after := m.activityReview.Identity, m.activityReview.After
	change.After.Connections[0].Settings = map[string]string{"context": "production", "namespace": "shop"}
	m.setActivityDetail(change)
	if m.activityReview.Identity != identity || m.activityReview.After != after {
		t.Fatal("map insertion order changed the stored comparison")
	}
	change.ID = "different-change"
	m.setActivityDetail(change)
	if m.activityReview.Identity == identity {
		t.Fatal("comparison identity must identify the selected history event")
	}
}

func TestActivityReviewBackPreservesSummaryPositionAndSharedReadingLayers(t *testing.T) {
	m, change := activityReviewModel()
	m.setActivityDetail(change)
	summary := m.detail
	m.cursor = 12
	m.positions[m.viewKey()] = 4
	filter := &panelSearch{kind: "activity", input: textview.NewField("updated")}
	m.searches[1] = filter
	m.openActivityReview()
	r := m.activityReview
	key := func(code rune, text string) {
		t.Helper()
		handled, _ := m.activityReviewKey(tea.KeyPressMsg{Code: code, Text: text})
		if !handled {
			t.Fatal("visible comparison did not handle its local key")
		}
	}
	key('/', "/")
	if !m.activityReviewSearching() || !m.activityReviewPaste("production") {
		t.Fatal("comparison search must use shared literal search input")
	}
	key(tea.KeyEnter, "")
	if r.Comparison.Reader.Query != "production" || r.Comparison.Reader.Searching {
		t.Fatal("search submission must retain query for navigation")
	}
	key(tea.KeyEscape, "")
	if !m.activityReviewVisible() || r.Comparison.Reader.Query != "" {
		t.Fatal("Esc must clear local search before leaving comparison")
	}
	key('v', "v")
	key(tea.KeyDown, "")
	if !r.Comparison.Reader.Selecting {
		t.Fatal("comparison selection unavailable")
	}
	key(tea.KeyEscape, "")
	if !m.activityReviewVisible() || r.Comparison.Reader.Selecting {
		t.Fatal("Esc must clear local selection before leaving comparison")
	}
	key(tea.KeyEnter, "")
	if !m.activityReviewVisible() {
		t.Fatal("Enter in comparison navigation must not activate an action")
	}
	key(tea.KeyEscape, "")
	if !m.activityDetailActive() || m.detail != summary || m.cursor != 12 || m.positions[m.viewKey()] != 4 || m.searches[1] != filter {
		t.Fatal("return from comparison changed summary/list/filter state")
	}
	if !m.openActivityReview() || m.activityReview.Comparison != r.Comparison {
		t.Fatal("reopening the same comparison should retain its reading position")
	}
}

func TestActivityReviewRejectsStaleTargetsAndDoesNotEditOnPaste(t *testing.T) {
	m, change := activityReviewModel()
	m.setActivityDetail(change)
	m.openActivityReview()
	r := m.activityReview
	before, after := r.Before, r.After
	m.activityReviewPaste("replace the workspace")
	if r.Before != before || r.After != after || r.Comparison.Reader.Query != "" {
		t.Fatal("paste outside search must not change comparison")
	}
	for _, state := range []struct {
		mode   int
		active string
		detail string
	}{{0, change.WorkspaceID, r.Summary}, {2, "different-workspace", r.Summary}, {2, change.WorkspaceID, "different event"}} {
		m.mode, m.active, m.detail = state.mode, state.active, state.detail
		if m.activityReviewVisible() || m.openActivityReview() || m.activityReviewPaste("ignored") {
			t.Fatal("comparison escaped its captured workspace and summary identity")
		}
	}
}

func TestActivityReviewUsesSharedCopyAndFitsPanels(t *testing.T) {
	m, change := activityReviewModel()
	m.setActivityDetail(change)
	m.openActivityReview()
	r := m.activityReview.Comparison
	for row, line := range r.Reader.Lines {
		if strings.Contains(line.Text, `"namespace": "shop"`) {
			r.Reader.Cursor = textview.Point{Row: row}
			handled, command := m.activityReviewKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
			if !handled || command == nil || r.Reader.SelectedText() != line.Text || strings.Contains(r.Reader.SelectedText(), "+ ") {
				t.Fatal("copy must use shared reader text without diff markers")
			}
			break
		}
	}
	if !strings.Contains(m.status, "terminal clipboard") {
		t.Fatal("copy action must report terminal clipboard support honestly")
	}
	for _, size := range [][2]int{{28, 10}, {60, 20}, {120, 36}} {
		view := ansi.Strip(m.activityReviewBody(size[0], size[1]))
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatal("comparison exceeds panel height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("comparison exceeds panel width")
			}
		}
	}
}

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

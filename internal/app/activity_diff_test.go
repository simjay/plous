package app

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"reflect"
	"strings"
	"testing"
	"time"
)

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

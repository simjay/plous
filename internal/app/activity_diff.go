package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/diffview"
	"github.com/simjay/plous/internal/textview"
	"github.com/simjay/plous/internal/workspace"
)

type activityReview struct {
	workspaceID, summary            string
	before, after, target, identity string
	beforeLabel, afterLabel         string
	revision                        uint64
	open                            bool
	comparison                      *diffview.Model
}

func workspaceSnapshot(w *workspace.Workspace) string {
	if w == nil {
		return ""
	}
	data, _ := json.MarshalIndent(w, "", "  ")
	return string(data) + "\n"
}

func (m *model) setActivityDetail(change workspace.Change) {
	workspaceID := change.WorkspaceID
	if workspaceID == "" {
		workspaceID = change.After.ID
	}
	if workspaceID == "" {
		workspaceID = m.active
	}
	data, _ := json.Marshal(change)
	summary := m.changeDetail(change)
	r := &activityReview{
		workspaceID: workspaceID,
		summary:     summary,
		before:      workspaceSnapshot(change.Before),
		after:       workspaceSnapshot(&change.After),
		target:      oneLine(change.After.Name) + " · " + changeSummary(change) + " · Read-only",
		identity:    fmt.Sprintf("%x", sha256.Sum256(append([]byte(workspaceID+"\x00"), data...))),
		beforeLabel: "Empty workspace",
		afterLabel:  fmt.Sprintf("Revision %d", change.After.Revision),
		revision:    change.Revision,
	}
	if change.Before != nil {
		r.beforeLabel = fmt.Sprintf("Revision %d", change.Before.Revision)
	}
	m.activityReview = r
	m.detail, m.cursor = summary, 0
}

func (m *model) activitySummaryVisible() bool {
	r := m.activityReview
	return r != nil && m.current() != nil && m.active == r.workspaceID && m.mode == 2 && m.page == "" && m.detail == r.summary
}

func (m *model) activityDetailActive() bool {
	return m.activitySummaryVisible() && !m.activityReview.open
}

func (m *model) activityReviewVisible() bool {
	return m.activitySummaryVisible() && m.activityReview.open
}

func (m *model) openActivityReview() bool {
	if !m.activitySummaryVisible() {
		return false
	}
	r := m.activityReview
	if r.comparison == nil {
		r.comparison = diffview.New(r.before, r.after, r.target, r.revision, false)
		r.comparison.ID = r.identity
		r.comparison.BeforeLabel, r.comparison.AfterLabel = r.beforeLabel, r.afterLabel
		r.comparison.BackLabel = "Back to summary"
	}
	r.open = true
	return true
}

func (m *model) activityReviewSearching() bool {
	return m.activityReviewVisible() && m.activityReview.comparison.Reader.Searching
}

func (m *model) activityReviewKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.activityReviewVisible() {
		return false, nil
	}
	r := m.activityReview
	decision, copied := r.comparison.Key(msg.String(), msg.Text)
	if decision.ID == r.identity && (decision.Action == "back" || decision.Action == "close") {
		r.open = false
	}
	return true, m.copyWorkspaceText(copied)
}

func (m *model) activityReviewPaste(text string) bool {
	if !m.activityReviewVisible() {
		return false
	}
	m.activityReview.comparison.Reader.Paste(text)
	return true
}

func (m *model) activityReviewBody(width, height int) string {
	if !m.activityReviewVisible() {
		return ""
	}
	m.activityReview.comparison.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{m.expandAction()}}}
	return m.activityReview.comparison.View(width, height, workspaceTheme())
}

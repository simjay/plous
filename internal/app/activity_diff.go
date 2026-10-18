package app

import (
	tea "charm.land/bubbletea/v2"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) setActivityDetail(change workspace.Change) {
	m.operationDetail = nil
	workspaceID := change.WorkspaceID
	if workspaceID == "" {
		workspaceID = change.After.ID
	}
	if workspaceID == "" {
		workspaceID = m.active
	}
	data, _ := json.Marshal(change)
	summary := activity.ChangeDetail(change)
	r := &activity.ActivityReview{
		WorkspaceID: workspaceID,
		Summary:     summary,
		Before:      activity.WorkspaceSnapshot(change.Before),
		After:       activity.WorkspaceSnapshot(&change.After),
		Target:      ui.OneLine(change.After.Name) + " · " + activity.ChangeSummary(change) + " · Read-only",
		Identity:    fmt.Sprintf("%x", sha256.Sum256(append([]byte(workspaceID+"\x00"), data...))),
		BeforeLabel: "Empty workspace",
		AfterLabel:  fmt.Sprintf("Revision %d", change.After.Revision),
		Revision:    change.Revision,
	}
	if change.Before != nil {
		r.BeforeLabel = fmt.Sprintf("Revision %d", change.Before.Revision)
	}
	m.activityReview = r
	m.detail, m.cursor = summary, 0
}

func (m *model) activitySummaryVisible() bool {
	r := m.activityReview
	return r != nil && m.current() != nil && m.active == r.WorkspaceID && m.mode == 2 && m.page == "" && m.detail == r.Summary
}

func (m *model) activityDetailActive() bool {
	return m.activitySummaryVisible() && !m.activityReview.Open
}

func (m *model) activityReviewVisible() bool {
	return m.activitySummaryVisible() && m.activityReview.Open
}

func (m *model) openActivityReview() bool {
	if !m.activitySummaryVisible() {
		return false
	}
	r := m.activityReview
	if r.Comparison == nil {
		r.Comparison = diffview.New(r.Before, r.After, r.Target, r.Revision, false)
		r.Comparison.ID = r.Identity
		r.Comparison.BeforeLabel, r.Comparison.AfterLabel = r.BeforeLabel, r.AfterLabel
		r.Comparison.BackLabel = "Back to summary"
	}
	r.Open = true
	return true
}

func (m *model) activityReviewSearching() bool {
	return m.activityReviewVisible() && m.activityReview.Comparison.Reader.Searching
}

func (m *model) activityReviewKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.activityReviewVisible() {
		return false, nil
	}
	r := m.activityReview
	decision, copied := r.Comparison.Key(msg.String(), msg.Text)
	if decision.ID == r.Identity && (decision.Action == "back" || decision.Action == "close") {
		r.Open = false
	}
	return true, m.copyWorkspaceText(copied)
}

func (m *model) activityReviewPaste(text string) bool {
	if !m.activityReviewVisible() {
		return false
	}
	m.activityReview.Comparison.Reader.Paste(text)
	return true
}

func (m *model) activityReviewBody(width, height int) string {
	if !m.activityReviewVisible() {
		return ""
	}
	m.activityReview.Comparison.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{m.expandAction()}}}
	return m.activityReview.Comparison.View(width, height, ui.WorkspaceTheme())
}

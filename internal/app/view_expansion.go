package app

import (
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/textview"
)

type viewExpansion struct {
	source  any
	restore func()
}

// Expansion belongs to the application layout. Components retain their own
// document, stream and comparison state throughout a layout change.
func (m *model) expandableView() (any, *textview.Viewer, *docedit.Model, bool) {
	if m.activityReviewVisible() {
		r := m.activityReview.comparison
		return r, &r.Reader, nil, r.ErrorActive || r.Reader.Searching || r.Reader.Selecting || r.Reader.Query != ""
	}
	if !m.exploreVisible() {
		return nil, nil, nil, false
	}
	if m.valueDocumentActive() {
		d := m.explore.action.valueDocument
		return d, nil, d, d.Searching || d.Query != "" || d.Mode == docedit.Select || d.Prefix() != ""
	}
	if m.documentActive() {
		a := m.explore.action
		if a.stage == "review" && a.review != nil {
			r := a.review
			return r, &r.Reader, nil, a.busy || r.ErrorActive || r.Reader.Searching || r.Reader.Selecting || r.Reader.Query != ""
		}
		if a.document == nil {
			a.initDocument()
		}
		d := a.document
		return d, nil, d, a.busy || a.discard || a.diagnostic != nil || d.Searching || d.Query != "" || d.Mode != docedit.Normal || d.Prefix() != ""
	}
	if t := m.explore.terminal; t != nil {
		if t.captured {
			return nil, nil, nil, true
		}
		if t.review == nil {
			t.beginReview()
		}
		r := &t.review.viewer.Reader
		return t.review, r, nil, r.Searching || r.Selecting || r.Query != ""
	}
	if m.yamlDocumentActive() {
		d := m.syncYAMLDocument().document
		return d, nil, d, d.Searching || d.Query != "" || d.Mode == docedit.Select || d.Prefix() != ""
	}
	if m.logWorkspaceActive() {
		v := m.podLogViewer()
		r := &v.Reader
		return v, r, nil, r.Searching || r.Selecting || r.Query != ""
	}
	return nil, nil, nil, false
}

func (m *model) viewExpanded() bool {
	return m.viewExpansion != nil && m.focus == 1 && m.help == nil && m.settingsOverlay == nil
}

func (m *model) restoreView() {
	if m.viewExpansion != nil {
		m.viewExpansion.restore()
		m.viewExpansion = nil
	}
}

func (m *model) syncViewExpansion() {
	if m.viewExpansion == nil || m.help != nil || m.settingsOverlay != nil {
		return
	}
	source, _, _, _ := m.expandableView()
	if m.focus != 1 || source != m.viewExpansion.source {
		m.restoreView()
	}
}

func (m *model) viewCommand(command string) bool {
	command = canonicalCommand(command)
	switch command {
	case "zoom", "linenumbers":
	default:
		return false
	}
	if m.help != nil || m.settingsOverlay != nil || m.form != nil {
		m.status = "Close the overlay to change the document view."
		return true
	}
	source, reader, document, _ := m.expandableView()
	if command == "linenumbers" {
		if document == nil {
			m.status = "Open YAML or an editor document to toggle line numbers."
		} else {
			document.LineNumbers = !document.LineNumbers
			m.status = ""
		}
		return true
	}
	if m.viewExpansion != nil {
		m.restoreView()
		m.status = ""
		return true
	}
	if source == nil {
		m.status = "Open Logs, a comparison, or an Editor document to expand it."
		return true
	}
	restore := func() {}
	if reader != nil {
		restore = reader.RestoreViewport()
	}
	if document != nil {
		restore = document.RestoreViewport()
	}
	m.viewExpansion = &viewExpansion{source: source, restore: restore}
	m.focus, m.shortcutPrefix, m.status = 1, "", ""
	return true
}

func (m *model) restoreExpandedOnEscape() bool {
	if !m.viewExpanded() || m.command != nil || m.shortcutPrefix != "" {
		return false
	}
	source, _, _, local := m.expandableView()
	if source != m.viewExpansion.source || local {
		return false
	}
	m.restoreView()
	return true
}

func (m *model) expandAction() textview.Action {
	label := "Expand"
	if m.viewExpanded() {
		label = "Restore"
	}
	return textview.Action{Key: ":z", Label: label}
}

func (m *model) documentDisplayActions(d *docedit.Model) textview.ActionGroup {
	value := "Off"
	if d.LineNumbers {
		value = "On"
	}
	return textview.ActionGroup{Label: "View", Actions: []textview.Action{m.expandAction(), {Key: ":L", Label: "Line numbers", Value: value}}}
}

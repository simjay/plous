package app

import (
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

type viewExpansion struct {
	source  any
	restore func()
}

// Components supply viewport snapshots and local Escape precedence; every main
// view can use the expanded layout, including views without a text component.
func (m *model) expandableView() (any, *textview.Viewer, *docedit.Model, bool) {
	if m.overviewVisible() {
		if v := m.overviewRead(); v != nil && v.State.MetricDetail != nil {
			d := v.State.MetricDetail
			return d, d.Reader(), nil, true
		}
	}
	if m.presentedDocumentVisible() {
		d := m.uiDocument.model
		return d, nil, d, d.Searching || d.Query != "" || d.Mode != docedit.Normal || d.Prefix() != ""
	}
	if m.codeVisible() {
		s := m.coder()
		local := s.Input != nil || s.Menu != "" || s.Busy || s.Discard || s.Filter != "" || s.BrowserDetail || len(s.Details) > 0 || s.View == "manifests" && s.Preference.Section != 1
		if d := s.Review; d != nil {
			return d, &d.Reader, nil, local || d.ErrorActive || d.Reader.Searching || d.Reader.Selecting || d.Reader.Query != ""
		}
		if d := s.Document; d != nil {
			return d, nil, d, local || d.Searching || d.Query != "" || d.Mode != docedit.Normal || d.Prefix() != ""
		}
		return nil, nil, nil, local
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		return nil, nil, nil, v.Namespaces != nil || v.State.Loading || v.State.HasLocalEscape()
	}
	if m.helmVisible() {
		h := m.helmer()
		local := h.Find != nil || h.Input != nil || h.Discard || h.Confirm || h.Menu != nil || h.Busy
		if h.Comparison != nil {
			r := h.Comparison
			return r, &r.Reader, nil, local || r.ErrorActive || r.Reader.Searching || r.Reader.Selecting || r.Reader.Query != ""
		}
		if h.Document != nil {
			d := h.Document
			return d, nil, d, local || d.Searching || d.Query != "" || d.Mode != docedit.Normal || d.Prefix() != ""
		}
		if h.Draft != nil && h.Plan == nil && h.WorkflowVisible() {
			local = local || h.Draft.Form.Index < len(h.Draft.Form.Fields)
		}
		return nil, nil, nil, local
	}
	if m.activityReviewVisible() {
		r := m.activityReview.Comparison
		return r, &r.Reader, nil, r.ErrorActive || r.Reader.Searching || r.Reader.Selecting || r.Reader.Query != ""
	}
	if !m.exploreVisible() {
		return nil, nil, nil, false
	}
	if m.valueDocumentActive() {
		d := m.explore.Action.ValueDocument
		return d, nil, d, d.Searching || d.Query != "" || d.Mode == docedit.Select || d.Prefix() != ""
	}
	if m.documentActive() {
		a := m.explore.Action
		if a.Stage == "review" && a.Review != nil {
			r := a.Review
			return r, &r.Reader, nil, a.Busy || r.ErrorActive || r.Reader.Searching || r.Reader.Selecting || r.Reader.Query != ""
		}
		if a.Document == nil {
			a.InitDocument()
		}
		d := a.Document
		return d, nil, d, a.Busy || a.Discard || a.Diagnostic != nil || d.Searching || d.Query != "" || d.Mode != docedit.Normal || d.Prefix() != ""
	}
	if t := m.explore.Terminal; t != nil {
		if t.Captured {
			return nil, nil, nil, true
		}
		if t.Review == nil {
			t.BeginReview()
		}
		r := &t.Review.Viewer.Reader
		return t.Review, r, nil, r.Searching || r.Selecting || r.Query != ""
	}
	if m.yamlDocumentActive() {
		d := m.syncYAMLDocument().Document
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
	return m.viewExpansion != nil && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil
}

func (m *model) restoreView() {
	if m.viewExpansion != nil {
		source, _, _, _ := m.expandableView()
		if source == m.viewExpansion.source && m.viewExpansion.restore != nil {
			m.viewExpansion.restore()
		}
		m.viewExpansion = nil
	}
}

func (m *model) syncViewExpansion() {
	if m.viewExpansion == nil || m.helpVisible() || m.settingsOverlay != nil {
		return
	}
	source, _, _, _ := m.expandableView()
	if m.focus != 1 || m.mainTerminalCaptured() {
		m.restoreView()
	} else if source != m.viewExpansion.source {
		// A new view has no compact viewport to restore.
		m.viewExpansion.source, m.viewExpansion.restore = source, nil
	}
}

func (m *model) viewCommand(command string) bool {
	command = canonicalCommand(command)
	switch command {
	case "fullscreen", "linenumbers":
	default:
		return false
	}
	if m.helpVisible() || m.settingsOverlay != nil || m.form != nil {
		m.status = "Close the overlay to change the main-panel layout."
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
	if m.mainTerminalCaptured() {
		m.status = "Use Ctrl+] to leave terminal input before expanding main."
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
	if !m.viewExpanded() || m.commandFocused() || m.shortcutPrefix != "" {
		return false
	}
	if m.mainViewLocalLayer() {
		return false
	}
	m.restoreView()
	return true
}

func (m *model) mainTerminalCaptured() bool {
	return m.resourceTerminalVisible() && m.explore.Terminal.Captured
}

func (m *model) mainViewLocalLayer() bool {
	if m.overviewBack != nil && m.mode == 1 {
		return true
	}
	if m.panelInteraction(1).localEscape || m.activePanelSearch(1) != nil {
		return true
	}
	if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		return true
	}
	_, _, _, local := m.expandableView()
	return local
}

func (m *model) expandAction() textview.Action {
	label := "Fullscreen"
	if m.viewExpanded() {
		label = "Restore layout"
	}
	return textview.Action{Key: ":f", Label: label}
}

func (m *model) documentDisplayActions(d *docedit.Model) textview.ActionGroup {
	value := "Off"
	if d.LineNumbers {
		value = "On"
	}
	return textview.ActionGroup{Label: "View", Actions: []textview.Action{m.expandAction(), {Key: ":L", Label: "Line numbers", Value: value}}}
}

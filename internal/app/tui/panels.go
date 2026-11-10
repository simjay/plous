package tui

import (
	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

// Park overlays at the panel being left without discarding their local state.
type overlayTravel struct {
	parked bool
	panel  int
}

func (o overlayTravel) visible(focus int) bool { return !o.parked || o.panel == focus }

func (m *model) helpVisible() bool {
	return m.help != nil && m.helpTravel.visible(m.focus)
}

func (m *model) chatSourcesVisible() bool {
	return m.chatSources != nil && m.sourcesTravel.visible(m.focus)
}

func (m *model) unparkOverlays() {
	if m.helpVisible() {
		m.helpTravel = overlayTravel{}
	}
	if m.chatSourcesVisible() {
		m.sourcesTravel = overlayTravel{}
	}
}

func (m *model) parkOverlays() {
	if m.helpVisible() {
		m.helpTravel = overlayTravel{parked: true, panel: m.focus}
	}
	if m.chatSourcesVisible() {
		m.sourcesTravel = overlayTravel{parked: true, panel: m.focus}
	}
}

func (m *model) travelPanel(backward bool) {
	m.parkOverlays()
	m.cycleFocus(backward)
	m.finishPanelTravel()
}

func (m *model) toggleTreeFocus() {
	m.restoreView()
	m.shortcutPrefix = ""
	if m.focus == 0 {
		m.focus = 1
		m.mouse.commandPaused = false
	} else {
		m.leftHidden = false
		m.focus = 0
	}
	m.finishPanelTravel()
}

func (m *model) toggleAgentFocus() {
	m.parkOverlays()
	m.restoreView()
	m.shortcutPrefix = ""
	if m.focus == 2 {
		m.focus = m.agentReturnFocus
		if m.focus != 0 || m.leftHidden {
			m.focus = 1
		}
	} else {
		m.agentReturnFocus = m.focus
		m.rightHidden = false
		m.focus = 2
		if m.externalAgentSelected() && !m.helpVisible() && !m.chatSourcesVisible() {
			m.nativeRequested = true
		}
	}
	if m.focus == 1 {
		m.mouse.commandPaused = false
	}
	m.finishPanelTravel()
}

func (m *model) finishPanelTravel() {
	// Hit areas still describe the panel and overlay from the preceding frame.
	m.mouse.context = ""
	m.status = ""
	m.persist()
}

type assistantSize string

const (
	assistantNormal assistantSize = ""
	assistantDouble assistantSize = "double"
)

func (m *model) cycleAssistantSize() {
	m.restoreView()
	switch {
	case m.rightHidden:
		m.rightHidden, m.assistantSize = false, assistantNormal
		m.status = "AI panel: normal. ii Double width"
	case m.assistantSize == assistantNormal:
		m.assistantSize = assistantDouble
		m.status = "AI panel: double width where space allows. ii Hide"
	default:
		m.rightHidden, m.assistantSize = true, assistantNormal
		m.status = "AI panel hidden. ii Show normal width"
	}
	m.ensureVisibleFocus()
}

func (m *model) panelVisible(panel int) bool {
	if m.viewExpanded() {
		return panel == 1
	}
	if widths, ok := m.overviewPanelWidths(); ok {
		return widths[panel] > 0
	}
	// Help keeps every section accessible, including hidden panels' commands.
	return m.helpVisible() || panel == 1 || (panel == 0 && !m.leftHidden) || (panel == 2 && !m.rightHidden)
}

func (m *model) ensureVisibleFocus() {
	if !m.panelVisible(m.focus) {
		m.focus = 1
	}
}

func (m *model) cycleFocus(backward bool) {
	m.shortcutPrefix = ""
	step := 1
	if backward {
		step = 2
	}
	for next := (m.focus + step) % 3; ; next = (next + step) % 3 {
		if next == 1 || next == 0 && !m.leftHidden || next == 2 && !m.rightHidden {
			if next != m.focus {
				m.restoreView()
			}
			m.focus = next
			if next == 1 {
				m.mouse.commandPaused = false
			}
			return
		}
	}
}

func (m *model) panelWidths() (widths [3]int) {
	if result, ok := m.overviewPanelWidths(); ok {
		return result
	}
	// Relax side-panel minimums on compact terminals to retain room for main.
	sidebarMin := min(32, max(24, m.width/4))
	assistantMin := min(40, max(24, m.width*3/10))
	if m.overviewActive() {
		sidebarMin = 28
		assistantMin = 28
	}
	if m.panelVisible(0) {
		widths[0] = max(sidebarMin, m.width/8)
	}
	if m.panelVisible(2) {
		widths[2] = max(assistantMin, m.width/4)
		if m.overviewActive() && m.width >= 150 {
			widths[2] = max(assistantMin, min(widths[2], m.width-widths[0]-94))
		}
		if m.assistantSize != assistantNormal {
			widths[2] = max(widths[2], min(2*widths[2], m.width-widths[0]-24))
		}
	}
	widths[1] = m.width - widths[0] - widths[2]
	return
}

// Side panels grow into spare width without taking columns back from Overview.
func (m *model) overviewPanelWidths() (widths [3]int, ok bool) {
	if !m.overviewActive() || m.helpVisible() || m.viewExpanded() {
		return widths, false
	}
	if m.width < 80 {
		focus := m.focus
		if focus == 0 && m.leftHidden || focus == 2 && m.rightHidden {
			focus = 1
		}
		widths[focus] = m.width
		return widths, true
	}
	left, right := 0, 0
	if !m.leftHidden {
		left = max(28, m.width/8)
	}
	if !m.rightHidden {
		right = max(28, m.width/4)
	}
	budget := min(left+right, max(0, m.width-76))
	if left > 0 && right > 0 {
		budget = m.width - max(76, m.width*5/8)
	}
	if left+right > 0 {
		widths[0] = budget * left / (left + right)
		if left > 0 && right > 0 {
			widths[0] = min(max(28, m.width/8), budget/2)
		}
		widths[2] = budget - widths[0]
	}
	if left > 0 {
		widths[0] = max(28, widths[0])
	}
	if right > 0 {
		widths[2] = max(28, widths[2])
	}
	if right > 0 && m.assistantSize != assistantNormal {
		main := min(max(24, m.width*3/8), max(24, m.width-widths[0]-32))
		widths[2] = m.width - widths[0] - main
	}
	widths[1] = m.width - widths[0] - widths[2]
	return widths, true
}

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

type interactionOwner uint8

const (
	interactionNavigation interactionOwner = iota
	interactionHelp
	interactionSources
	interactionCommand
	interactionSettings
	interactionForm
	interactionChatInput
	interactionFeature
	interactionCharts
	interactionTerminal
	interactionUnavailable
)

// Composition resolves ownership; feature controllers retain their local state.
// Panel resolution also supplies the underlying context shown by Help.
type interaction struct {
	owner        interactionOwner
	panelSearch  bool
	localEscape  bool
	resumeSearch func()
}

func (m *model) panelInteraction(panel int) interaction {
	if panel == 0 {
		return interaction{owner: interactionNavigation, panelSearch: true}
	}
	if panel == 2 {
		if m.externalAgentSelected() {
			if m.nativeAgent != nil && m.nativeAgent.Captured && !m.nativeAgent.Finished {
				return interaction{owner: interactionTerminal}
			}
			return interaction{owner: interactionNavigation}
		}
		if m.chat().Models || m.chatEditing && !m.chat().History {
			return interaction{owner: interactionChatInput}
		}
		return interaction{owner: interactionNavigation, panelSearch: true}
	}
	if m.form != nil {
		return interaction{owner: interactionForm, localEscape: true}
	}
	if m.presentedDocumentVisible() {
		return interaction{owner: interactionFeature, localEscape: true}
	}
	if m.settingsOverlay != nil || m.page != "" {
		return interaction{owner: interactionSettings, localEscape: true}
	}
	if m.mainTerminalCaptured() {
		return interaction{owner: interactionTerminal, localEscape: true}
	}
	if m.overviewVisible() {
		v := m.overviewRead()
		if v == nil {
			return interaction{owner: interactionNavigation}
		}
		if v.State.MetricDetail != nil {
			return interaction{owner: interactionFeature, localEscape: true}
		}
		return interaction{owner: interactionNavigation, localEscape: v.State.Input != nil || v.State.Query != "" || v.State.Expanded || v.State.Group != "" || v.Inspecting}
	}

	if m.helmVisible() || m.gitOpsVisible() || m.documentActive() || m.valueDocumentActive() || m.yamlDocumentActive() || m.activityReviewVisible() || m.logWorkspaceActive() || m.resourceTerminalVisible() {
		return interaction{owner: interactionFeature}
	}
	if m.exploreVisible() {
		e := m.explore
		if e.Input != nil || e.Picker != "" || e.Action != nil {
			return interaction{owner: interactionFeature, localEscape: true}
		}
		if m.chartsActive() {
			return interaction{owner: interactionCharts, localEscape: true, resumeSearch: func() { e.Usage.Charts.Inspect = false }}
		}
		return interaction{owner: interactionNavigation, panelSearch: e.Detail != nil}
	}
	return interaction{owner: interactionNavigation, panelSearch: true}
}

func (m *model) activeInteraction() interaction {
	if m.commandFocused() {
		return interaction{owner: interactionCommand}
	}
	if m.helpVisible() {
		return interaction{owner: interactionHelp}
	}
	if m.chatSourcesVisible() {
		return interaction{owner: interactionSources}
	}
	if m.mainTerminalCaptured() && m.focus == 1 {
		return m.panelInteraction(1)
	}
	if m.tooSmall() {
		return interaction{owner: interactionUnavailable}
	}
	return m.panelInteraction(m.focus)
}

// Keyboard and action buttons enter feature handlers through the same boundary.
func (m *model) featureKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	owner := m.activeInteraction().owner
	if owner != interactionFeature && owner != interactionTerminal && owner != interactionNavigation {
		return false, nil
	}
	if handled, cmd := m.presentedDocumentKey(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.overviewKey(msg); handled {
		return true, cmd
	}

	if m.gitOpsInputKey(msg) {
		return true, nil
	}
	if handled, cmd := m.helmKey(msg); handled {
		return true, cmd
	}
	return m.workspaceKey(msg)
}

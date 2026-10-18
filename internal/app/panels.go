package app

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

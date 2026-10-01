package app

func (m *model) panelVisible(panel int) bool {
	if m.viewExpanded() {
		return panel == 1
	}
	// Help keeps every section accessible, including hidden panels' commands.
	return m.help != nil || panel == 1 || (panel == 0 && !m.leftHidden) || (panel == 2 && !m.rightHidden)
}

func (m *model) ensureVisibleFocus() {
	if !m.panelVisible(m.focus) {
		m.focus = 1
	}
}

func (m *model) cycleFocus(backward bool) {
	m.restoreView()
	m.shortcutPrefix = ""
	step := 1
	if backward {
		step = 2
	}
	for {
		m.focus = (m.focus + step) % 3
		if m.panelVisible(m.focus) {
			return
		}
	}
}

func (m *model) panelWidths() (widths [3]int) {
	// Relax side-panel minimums on compact terminals to retain room for main.
	sidebarMin := min(32, max(24, m.width/4))
	assistantMin := min(40, max(24, m.width*3/10))
	if m.panelVisible(0) {
		widths[0] = max(sidebarMin, m.width/8)
	}
	if m.panelVisible(2) {
		widths[2] = max(assistantMin, m.width/4)
	}
	widths[1] = m.width - widths[0] - widths[2]
	return
}

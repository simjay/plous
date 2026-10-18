package editor

import "strings"

// ResumeSearch preserves the query and returns to the current mode on Escape.
func (m *Model) ResumeSearch() {
	if !m.Searching {
		m.searchReturn = m.Mode
	}
	m.Searching = true
}

// Scroll moves the viewport without editing text or extending a selection.
func (m *Model) Scroll(rows int) {
	m.top = max(0, min(m.top+rows, max(0, strings.Count(m.text, "\n")+1-m.height)))
	m.mouseScrolled = true
}

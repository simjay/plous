package app

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/textview"
	"github.com/simjay/plous/internal/workspace"
)

type panelSearch struct {
	finished, positioned         bool
	width                        int
	input                        editor
	scope, kind, treeKey         string
	cursor, match, total, scroll int
}

const searchBarHeight = 3

func (m *model) searchScope(panel int) string {
	switch panel {
	case 0:
		return "tree"
	case 1:
		if m.form != nil || m.page != "" || m.settingsOverlay != nil || m.documentActive() || m.valueDocumentActive() || m.yamlDocumentActive() || m.activityReviewVisible() || m.logWorkspaceActive() || m.resourceTerminalVisible() {
			return ""
		}
		if m.exploreVisible() {
			e := m.explorer()
			if e.input != nil || e.picker != "" || e.detail == nil {
				return ""
			}
			return fmt.Sprintf("%s/%s/%s/%s/%d", m.viewKey(), e.preference.Resource, e.detail.GetNamespace(), e.detail.GetName(), e.tab)
		}
		return m.viewKey() + "/" + m.detail
	case 2:
		c := m.chat()
		if c.models || m.chatEditing && !c.history {
			return ""
		}
		return fmt.Sprintf("%s/%s/%t", m.active, c.conversation.ID, c.history)
	}
	return ""
}

func (m *model) clearStaleSearches() {
	for panel, s := range m.searches {
		if s != nil && s.scope != m.searchScope(panel) {
			if s.kind == "activity" {
				m.positions[strings.TrimSuffix(s.scope, "/")] = s.cursor
			}
			m.searches[panel] = nil
		}
	}
}

func (m *model) beginPanelSearch() bool {
	if m.help != nil || m.focus == 1 && (m.form != nil || m.page != "" || m.settingsOverlay != nil) || m.focus == 2 && m.chat().models {
		return false
	}
	scope := m.searchScope(m.focus)
	if scope == "" {
		return false // Resource tables and pickers keep their existing filter controls.
	}
	s := &panelSearch{input: newEditor(""), scope: scope, kind: "find"}
	switch m.focus {
	case 0:
		s.kind = "tree"
		rows := m.treeRows()
		if m.treeCursor >= 0 && m.treeCursor < len(rows) {
			s.treeKey = rows[m.treeCursor].key
		}
	case 1:
		s.scroll = m.positions[m.viewKey()]
		if m.detail != "" {
			s.scroll = m.cursor
		}
		if m.exploreVisible() {
			s.scroll = m.explore.scroll
		}
		if m.mode == 2 && m.detail == "" {
			s.kind, s.cursor = "activity", m.positions[m.viewKey()]
		}
	case 2:
		s.scroll = m.chat().scroll
		if m.chat().history {
			s.kind, s.cursor = "history", m.chat().cursor
		} else {
			m.chat().reading = true
		}
	}
	m.searches[m.focus] = s
	m.shortcutPrefix, m.status = "", ""
	m.searchChanged()
	return true
}

func searchContains(value, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(query)))
}

func (m *model) searchQuery(panel int) string {
	if s := m.searches[panel]; s != nil {
		return s.input.text
	}
	return ""
}

func (m *model) filteredConversations() []workspace.Conversation {
	choices := m.chat().choices
	if s := m.searches[2]; s != nil && s.kind == "history" {
		filtered := make([]workspace.Conversation, 0, len(choices))
		for _, c := range choices {
			if searchContains(c.Title+" "+c.Created.Local().Format("Jan 02 15:04"), s.input.text) {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}
	return choices
}

func (m *model) filteredChanges() []workspace.Change {
	if s := m.searches[1]; s != nil && s.kind == "activity" {
		filtered := make([]workspace.Change, 0, len(m.changes))
		for _, c := range m.changes {
			text := fmt.Sprintf("%s r%d %s", changeSummary(c), c.Revision, c.Time.Local().Format("Jan 02 15:04"))
			if searchContains(text, s.input.text) {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}
	return m.changes
}

func (m *model) searchChanged() {
	s := m.searches[m.focus]
	s.match = 0
	s.positioned = false
	switch s.kind {
	case "tree":
		m.treeCursor = 0
		for i, row := range m.treeRows() {
			if row.kind != "settings" && searchContains(row.label+" "+row.resource, s.input.text) {
				m.treeCursor = i
				break
			}
		}
	case "history":
		m.chat().cursor = 0
	case "activity":
		m.positions[m.viewKey()] = 0
	}
}

func (m *model) updatePanelSearch(msg tea.KeyPressMsg) {
	s := m.searches[m.focus]
	key := msg.String()
	if key == "esc" {
		m.searches[m.focus] = nil
		switch s.kind {
		case "tree":
			m.restoreTreeCursor(s.treeKey)
		case "history":
			m.chat().cursor = s.cursor
		case "activity":
			m.positions[m.viewKey()] = s.cursor
		case "find":
			if m.focus == 2 {
				m.chat().scroll = s.scroll
			} else if m.exploreVisible() {
				m.explore.scroll = s.scroll
			} else if m.detail != "" {
				m.cursor = s.scroll
			} else {
				m.positions[m.viewKey()] = s.scroll
			}
		}
		m.persist()
		return
	}
	if s.kind == "find" && key == "enter" {
		s.finished = true
		return
	}
	if s.kind == "find" && (key == "down" || key == "shift+enter" || key == "up" || key == "n" && s.finished || key == "N" && s.finished) {
		s.positioned = false
		step := 1
		if key == "up" || key == "shift+enter" || key == "N" {
			step = -1
		}
		if s.total > 0 {
			s.match = (s.match + s.total + step) % s.total
		}
		return
	}
	if s.kind != "find" && (key == "up" || key == "down" || key == "pgup" || key == "pgdown" || key == "enter") {
		switch s.kind {
		case "tree":
			rows := m.treeRows()
			row := rows[max(0, min(m.treeCursor, len(rows)-1))]
			if key == "enter" && len(rows) == 1 && strings.TrimSpace(s.input.text) != "" {
				return
			}
			blocked := m.form != nil
			if key == "enter" && !blocked && strings.TrimSpace(s.input.text) != "" {
				parents := map[string]string{}
				for _, r := range rows {
					parents[r.key] = r.parent
				}
				for parent := row.parent; parent != ""; parent = parents[parent] {
					m.setTreeExpanded(parent, true)
				}
			}
			m.updateTree(key)
			if key == "enter" && !blocked {
				m.searches[0] = nil
				m.restoreTreeCursor(row.key, row.parent, row.connection, row.workspace)
			}
		case "history":
			choices := m.filteredConversations()
			m.updateChat(msg)
			if key == "enter" && !m.chat().history {
				m.searches[2] = nil
			} else if key == "pgup" || key == "pgdown" {
				step := 10
				if key == "pgup" {
					step = -step
				}
				m.chat().cursor = max(0, min(len(choices)-1, m.chat().cursor+step))
			}
		case "activity":
			changes := m.filteredChanges()
			selected := max(0, min(m.positions[m.viewKey()], len(changes)-1))
			if key == "pgup" || key == "pgdown" {
				step := 10
				if key == "pgup" {
					step = -step
				}
				m.positions[m.viewKey()] = max(0, min(len(m.filteredChanges())-1, m.positions[m.viewKey()]+step))
			} else {
				m.updateMain(key)
			}
			if m.detail != "" {
				m.searches[1] = nil
				for i, change := range m.changes {
					if change.Revision == changes[selected].Revision {
						m.positions[m.viewKey()] = i
						break
					}
				}
			}
		}
		return
	}
	before := s.input.text
	s.input.key(msg, false)
	if s.input.text != before {
		m.searchChanged()
	}
}

func (m *model) restoreTreeCursor(keys ...string) {
	rows := m.treeRows()
	for _, key := range keys {
		for i, row := range rows {
			if row.key == key {
				m.treeCursor = i
				return
			}
		}
	}
	m.treeCursor = max(0, min(m.treeCursor, len(rows)-1))
}

func (m *model) searchBar(panel, width int, label string) string {
	s := m.searches[panel]
	hint := ""
	if s.finished {
		hint = "n/N matches · / new search · Esc clear"
	}
	return searchField(s.input, width, m.focus == panel && !s.finished, label, "Type to search…", hint)
}

func searchField(input editor, width int, active bool, label, placeholder, hint string) string {
	innerWidth := max(1, width-4)
	text := editorLine(input, innerWidth, active)
	if input.text == "" {
		cursor := ""
		if active {
			cursor = "▏"
		}
		text = cursor + muted.Render(clipped(placeholder, max(0, innerWidth-ansi.StringWidth(cursor))))
	}
	if width < 4 {
		return clipped(text, width)
	}
	border := borderColor
	if active {
		border = focusedBorder
	}
	frame := lipgloss.NewStyle().Foreground(border)
	title := clipped(" / "+label+" ", width-3)
	top := frame.Render("╭─") + selected.Render(title) + frame.Render(strings.Repeat("─", max(0, width-3-ansi.StringWidth(title)))+"╮")
	middle := frame.Render("│") + " " + text + strings.Repeat(" ", max(0, innerWidth-ansi.StringWidth(text))) + " " + frame.Render("│")
	bottom := frame.Render("╰" + strings.Repeat("─", width-2) + "╯")
	if hint != "" {
		label := clipped(" "+hint+" ", width-3)
		bottom = frame.Render("╰"+strings.Repeat("─", max(0, width-3-ansi.StringWidth(label)))) + muted.Render(label) + frame.Render("─╯")
	}
	return strings.Join([]string{top, middle, bottom}, "\n")
}

func searchMatchRows(text string, spans [][]int, width int) []int {
	rows := make([]int, 0, len(spans))
	position, column, row := 0, 0, 0
	for _, span := range spans {
		for position <= span[0] && position < len(text) {
			_, cells, size, _ := ansi.DecodeSequence(text[position:], 0, nil)
			if column+cells > width {
				row++
				column = 0
			}
			column += cells
			position += size
		}
		rows = append(rows, row)
	}
	return rows
}

// Search the full text before wrapping so a match may cross a visual line break.
func (m *model) findText(panel int, text string, width, height, offset int) string {
	s := m.searches[panel]
	width, available := max(1, width), max(0, height-searchBarHeight)
	query := s.input.text
	var hits []int
	var lines []string
	var pattern *regexp.Regexp
	if query != "" {
		pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	}
	for _, line := range strings.Split(text, "\n") {
		clean := ansi.Strip(line)
		var spans [][]int
		if pattern != nil {
			spans = pattern.FindAllStringIndex(clean, -1)
		}
		marks := make([]textview.Span, 0, len(spans))
		position, runes := 0, 0
		for _, span := range spans {
			runes += utf8.RuneCountInString(clean[position:span[0]])
			end := runes + utf8.RuneCountInString(clean[span[0]:span[1]])
			marks = append(marks, textview.Span{Start: runes, End: end, Style: lipgloss.NewStyle().Background(highlight).Underline(true)})
			position, runes = span[1], end
		}
		painted := textview.Paint(clean, line, marks)
		if panel == 2 {
			rows := chatRows(painted, width)
			row := 0
			for _, span := range spans {
				for row+1 < len(rows) && rows[row+1].start <= span[0] {
					row++
				}
				hits = append(hits, len(lines)+row)
			}
			for _, row := range rows {
				lines = append(lines, row.text)
			}
			continue
		}
		for _, row := range searchMatchRows(clean, spans, width) {
			hits = append(hits, len(lines)+row)
		}
		lines = append(lines, strings.Split(ansi.Hardwrap(painted, width, true), "\n")...)
	}
	s.total = len(hits)
	label := "Find"
	if query != "" {
		label = "Find · No matches"
		if len(hits) > 0 {
			s.match = max(0, s.match) % len(hits)
			if !s.positioned || s.width != width {
				offset = max(0, hits[s.match]-available/2)
				s.positioned, s.width = true, width
				if panel == 2 {
					m.chat().reading = true
				}
			}
			label = fmt.Sprintf("Find · %d/%d", s.match+1, len(hits))
		}
	}
	offset = max(0, min(offset, max(0, len(lines)-available)))
	s.scroll = offset
	if panel == 2 {
		m.chat().scroll = s.scroll
	} else if m.exploreVisible() {
		m.explore.scroll = s.scroll
	} else if m.detail != "" {
		m.cursor = s.scroll
	} else {
		m.positions[m.viewKey()] = s.scroll
	}
	return styledView(m.searchBar(panel, width, label)+"\n"+strings.Join(lines[offset:min(len(lines), offset+available)], "\n"), width, height)
}

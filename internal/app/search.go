package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"regexp"
	"strings"
	"unicode/utf8"
)

type panelSearch struct {
	archive                      *chatFind
	finished, positioned         bool
	width                        int
	input                        textview.Field
	scope, kind, treeKey         string
	activityIdentity             string
	activityFilter               activity.Filter
	cursor, match, total, scroll int
}

func (m *model) searchScope(panel int) string {
	switch panel {
	case 0:
		return "tree"
	case 1:
		if m.overviewVisible() {
			return ""
		}
		if m.codeOverviewVisible() {
			return "code-overview/" + m.viewKey() + "/" + m.coder().Preference.BrowserItem
		}
		if m.codeVisible() || m.helmVisible() || m.gitOpsVisible() {
			return ""
		}
		if m.form != nil || m.page != "" || m.settingsOverlay != nil || m.documentActive() || m.valueDocumentActive() || m.yamlDocumentActive() || m.activityReviewVisible() || m.logWorkspaceActive() || m.resourceTerminalVisible() {
			return ""
		}
		if m.exploreVisible() {
			e := m.explorer()
			if e.Input != nil || e.Picker != "" || e.Detail == nil {
				return ""
			}
			return fmt.Sprintf("%s/%s/%s/%s/%d", m.viewKey(), e.Preference.Resource, e.Detail.GetNamespace(), e.Detail.GetName(), e.Tab)
		}
		return m.viewKey() + "/" + m.detail
	case 2:
		c := m.chat()
		if c.Models || m.chatEditing && !c.History {
			return ""
		}
		return fmt.Sprintf("%s/%s/%t", m.conversationScope(), c.Conversation.ID, c.History)
	}
	return ""
}

func (m *model) clearStaleSearches() {
	if search := m.activityListSearch; search != nil && search.scope != m.viewKey()+"/" && !m.helpVisible() && m.settingsOverlay == nil {
		m.restoreActivityPosition(search)
		m.activityListSearch = nil
	}
	for panel, s := range m.searches {
		if panel == 1 && (m.settingsOverlay != nil || m.helpVisible() || m.chatSourcesVisible()) {
			continue
		}
		if s != nil && s.scope != m.searchScope(panel) {
			if s.kind == "activity" {
				m.restoreActivityPosition(s)
			}
			s.cancelChatFind()
			m.searches[panel] = nil
		}
	}
}

func (m *model) beginPanelSearch() bool {
	if m.helpVisible() || m.focus == 1 && (m.form != nil || m.page != "" || m.settingsOverlay != nil) || m.focus == 2 && m.chat().Models {
		return false
	}
	if resume := m.activeInteraction().resumeSearch; resume != nil {
		resume()
		if search := m.searches[1]; search != nil && search.scope == m.searchScope(1) {
			search.finished = false
			m.shortcutPrefix, m.status = "", ""
			return true
		}
	}
	scope := m.searchScope(m.focus)
	if scope == "" {
		return false // Resource tables and pickers keep their existing filter controls.
	}
	if search := m.searches[m.focus]; search != nil && search.kind == "activity" && search.scope == scope && search.finished {
		search.finished = false
		m.shortcutPrefix, m.status = "", ""
		return true
	}
	s := &panelSearch{input: textview.NewField(""), scope: scope, kind: "find"}
	switch m.focus {
	case 0:
		s.kind = "tree"
		rows := m.treeRows()
		if m.treeCursor >= 0 && m.treeCursor < len(rows) {
			s.treeKey = rows[m.treeCursor].key
		}
	case 1:
		s.scroll = m.positions[m.viewKey()]
		if m.codeOverviewVisible() {
			s.scroll = m.coder().DetailOffset
		}
		if m.detail != "" {
			s.scroll = m.cursor
		}
		if m.exploreVisible() {
			s.scroll = m.explore.Scroll
		}
		if m.mode == 2 && m.detail == "" {
			s.kind, s.cursor, s.activityFilter = "activity", m.activityPosition(), m.activityUI.Filter
			entries := m.activityEntries()
			if len(entries) > 0 {
				s.cursor = min(max(0, s.cursor), len(entries)-1)
				s.activityIdentity = entries[s.cursor].Identity()
			}
		}
	case 2:
		s.scroll = m.chat().Scroll
		if m.chat().History {
			s.kind, s.cursor = "history", m.chat().Cursor
		} else {
			m.chat().Reading = true
		}
	}
	m.searches[m.focus].cancelChatFind()
	m.searches[m.focus] = s
	m.shortcutPrefix, m.status = "", ""
	m.searchChanged()
	return true
}

func (m *model) searchQuery(panel int) string {
	if s := m.searches[panel]; s != nil {
		return s.input.Text
	}
	return ""
}

func (m *model) filteredConversations() []history.Conversation {
	choices := m.chat().Choices
	if s := m.searches[2]; s != nil && s.kind == "history" {
		filtered := make([]history.Conversation, 0, len(choices))
		for _, c := range choices {
			if ui.SearchContains(c.Title+" "+c.Created.Local().Format("Jan 02 15:04"), s.input.Text) {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}
	return choices
}

func (m *model) searchChanged() {
	s := m.searches[m.focus]
	s.cancelChatFind()
	s.match = 0
	s.positioned = false
	switch s.kind {
	case "tree":
		m.treeCursor = 0
		for i, row := range m.treeRows() {
			if row.kind != "settings" && ui.SearchContains(row.label+" "+row.resource, s.input.Text) {
				m.treeCursor = i
				break
			}
		}
	case "history":
		m.chat().Cursor = 0
	case "activity":
		m.setActivityPosition(0)
	}
}

func (m *model) updatePanelSearch(msg tea.KeyPressMsg) {
	s := m.searches[m.focus]
	key := msg.String()
	if key == "esc" {
		s.cancelChatFind()
		m.searches[m.focus] = nil
		switch s.kind {
		case "tree":
			m.restoreTreeCursor(s.treeKey)
		case "history":
			m.chat().Cursor = s.cursor
		case "activity":
			m.restoreActivityPosition(s)
		case "find":
			if m.focus == 2 {
				m.chat().Scroll = s.scroll
			} else if m.codeOverviewVisible() {
				m.coder().DetailOffset = s.scroll
			} else if m.exploreVisible() {
				m.explore.Scroll = s.scroll
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
		if m.focus == 2 && m.stepChatFind(s, step) {
			return
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
			if key == "enter" && len(rows) == 1 && strings.TrimSpace(s.input.Text) != "" {
				return
			}
			blocked := m.form != nil
			if key == "enter" && !blocked && strings.TrimSpace(s.input.Text) != "" {
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
			if key == "enter" && !m.chat().History {
				m.searches[2] = nil
			} else if key == "pgup" || key == "pgdown" {
				step := 10
				if key == "pgup" {
					step = -step
				}
				m.chat().Cursor = max(0, min(len(choices)-1, m.chat().Cursor+step))
			}
		case "activity":
			m.updateMain(key)
		}
		return
	}
	before := s.input.Text
	s.input.Key(msg, false)
	if s.input.Text != before {
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
		if s.kind == "activity" {
			hint = "/ Edit search · Esc clear"
		}
	}
	return ui.SearchField(s.input, width, m.focus == panel && !s.finished, label, "Type to search…", hint)
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
	width, available := max(1, width), max(0, height-ui.SearchBarHeight)
	query := s.input.Text
	var hits []int
	var lines []string
	var pattern *regexp.Regexp
	if query != "" {
		pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	}
	for index, line := range strings.Split(text, "\n") {
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
			marks = append(marks, textview.Span{Start: runes, End: end, Style: lipgloss.NewStyle().Background(ui.Highlight).Underline(true)})
			position, runes = span[1], end
		}
		painted := textview.Paint(clean, line, marks)
		if panel == 2 {
			rows := m.chat().ChatLineRows(painted, index, width)
			row := 0
			for _, span := range spans {
				for row+1 < len(rows) && rows[row+1].Start <= span[0] {
					row++
				}
				hits = append(hits, len(lines)+row)
			}
			for _, row := range rows {
				lines = append(lines, row.Text)
			}
			continue
		}
		for _, row := range searchMatchRows(clean, spans, width) {
			hits = append(hits, len(lines)+row)
		}
		lines = append(lines, strings.Split(ansi.Hardwrap(painted, width, true), "\n")...)
	}
	if s.archive != nil && !s.archive.found {
		hits = nil
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
					m.chat().Reading = true
					rows := m.chat().ChatRows(text, width)
					m.chat().SelectReplyForLine(rows[min(hits[s.match], len(rows)-1)].Line)
				}
			}
			label = fmt.Sprintf("Find · %d/%d", s.match+1, len(hits))
		}
	}
	offset = max(0, min(offset, max(0, len(lines)-available)))
	s.scroll = offset
	if panel == 2 {
		m.chat().Scroll = s.scroll
	} else if m.codeOverviewVisible() {
		m.coder().DetailOffset = s.scroll
	} else if m.exploreVisible() {
		m.explore.Scroll = s.scroll
	} else if m.detail != "" {
		m.cursor = s.scroll
	} else {
		m.positions[m.viewKey()] = s.scroll
	}
	if panel == 2 {
		label = s.chatFindLabel(label)
	}
	return ui.StyledView(m.searchBar(panel, width, label)+"\n"+strings.Join(lines[offset:min(len(lines), offset+available)], "\n"), width, height)
}

func (s *panelSearch) ownsKey(key string) bool {
	return s != nil && (!s.finished || key == "esc" || s.kind == "find" && (key == "n" || key == "N"))
}

func (m *model) activePanelSearch(panel int) *panelSearch {
	if !m.panelInteraction(panel).panelSearch {
		return nil
	}
	return m.searches[panel]
}

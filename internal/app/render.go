package app

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func plain(s string) string              { return cleanInput(s, true) }
func oneLine(s string) string            { return strings.ReplaceAll(plain(s), "\n", " ") }
func clipped(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }
func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.BackgroundColor, v.ForegroundColor = background, foreground
	if m.resourceTerminalVisible() && m.explore.terminal.captured && m.explore.terminal.cursorVisible.Load() && !m.tooSmall() {
		pos := m.explore.terminal.emulator.CursorPosition()
		_, rows := m.terminalDimensions()
		v.Cursor = tea.NewCursor(pos.X+m.panelWidths()[0]+2, pos.Y+4+(m.height-6-rows))
	}
	return v
}
func (m *model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		text := "Plous " + m.version + "\nResize terminal to at least 80 x 16.\nCtrl+C to exit.\nFocus: " + []string{"tree", "main", "chat"}[m.focus]
		if m.command != nil {
			text += "\n:" + m.command.display(m.focus == 1)
		}
		if m.status != "" {
			text += "\n" + oneLine(m.status)
		}
		return lipgloss.NewStyle().Width(m.width).Height(m.height).MaxWidth(m.width).MaxHeight(m.height).Render(text)
	}
	if m.help != nil {
		return m.renderHelpOverlay()
	}
	widths := m.panelWidths()
	height := m.height - 2
	header := gradientText(" plous", lipgloss.Color("#D38BC3"), focusedBorder)
	label := muted.Render("v" + m.version + " ")
	header += strings.Repeat(" ", m.width-lipgloss.Width(header)-lipgloss.Width(label)) + label
	bodyHeight := height - 4
	visible := []string{}
	if m.panelVisible(0) {
		visible = append(visible, panel("Workspaces", m.treeBody(widths[0]-4, bodyHeight), widths[0], height, m.focus == 0))
	}
	base := *m
	if previous := m.settingsOverlay; previous != nil {
		base.page, base.detail, base.cursor = previous.page, previous.detail, previous.cursor
		base.form, base.command = nil, nil
	}
	visible = append(visible, base.mainPanel(widths[1], height))
	if m.panelVisible(2) {
		visible = append(visible, panel("AI assistant", m.chatBody(widths[2]-4, bodyHeight), widths[2], height, m.focus == 2))
	}
	panels := lipgloss.JoinHorizontal(lipgloss.Top, visible...)

	screen := lipgloss.JoinVertical(lipgloss.Left, header, panels, m.footer())
	if m.settingsOverlay != nil && m.focus == 1 && m.help == nil {
		return m.renderSettingsOverlay(screen)
	}
	if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		return m.renderResourceActionOverlay(screen)
	}
	return screen
}
func centeredOverlay(background, window string) string {
	width, height := lipgloss.Size(background)
	windowWidth, windowHeight := lipgloss.Size(window)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(canvasStyle.Foreground(lipgloss.Color("#56575E")).Render(ansi.Strip(background))),
		lipgloss.NewLayer(window).X((width-windowWidth)/2).Y((height-windowHeight)/2).Z(1),
	).Render()
}

func panel(title, content string, width, height int, focused bool) string {
	color := borderColor
	if focused {
		color = focusedBorder
	}
	heading := titleStyle.Render(title)
	rule := lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", width-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(heading + "\n" + rule + "\n" + content)
}
func listView(items []string, index, width, height int) string {
	return choiceList(items, index, width, height, choiceLine)
}

func choiceList(items []string, index, width, height int, renderRow func(string, bool, int) string) string {
	if height <= 0 {
		return ""
	}
	index = max(0, min(index, len(items)-1))
	start := max(0, index-height+1)
	end := min(len(items), start+height)
	lines := []string{}
	for i := start; i < end; i++ {
		lines = append(lines, renderRow(items[i], i == index, width))
	}
	return strings.Join(lines, "\n")
}
func textView(text string, width, height, offset int) string {
	return styledTextView(plain(text), width, height, offset)
}

// Callers sanitize user values before adding terminal styles.
func styledTextView(text string, width, height, offset int) string {
	if height <= 0 {
		return ""
	}
	wrapped := ansi.Wrap(text, max(1, width), "")
	lines := strings.Split(wrapped, "\n")
	offset = max(0, min(offset, max(0, len(lines)-height)))
	return strings.Join(lines[offset:min(len(lines), offset+height)], "\n")
}

// The tab cells share the full panel width; only names shorten on narrow screens.
func tabWidths(width int) []int {
	available := width - (len(modes) - 1)
	widths := make([]int, len(modes))
	for i := range widths {
		widths[i] = available / len(modes)
		if i < available%len(modes) {
			widths[i]++
		}
	}
	return widths
}
func (m *model) tabs(width int) string {
	tops, labels, bottoms := []string{}, []string{}, []string{}
	edge := lipgloss.NewStyle().Foreground(focusedBorder)
	for i, cellWidth := range tabWidths(width) {
		innerWidth := cellWidth - 2
		shortcut := fmt.Sprintf(":%d", i+1)
		name := clipped(modes[i], innerWidth-5)
		label := " " + name + strings.Repeat(" ", max(1, innerWidth-ansi.StringWidth(name)-len(shortcut)-2)) + shortcut + " "
		style := muted
		bottom := edge.Render("┴" + strings.Repeat("─", innerWidth) + "┴")
		if m.mode == i {
			style = titleStyle
			bottom = edge.Render("╯") + strings.Repeat(" ", innerWidth) + edge.Render("╰")
		}
		tops = append(tops, edge.Render("╭"+strings.Repeat("─", innerWidth)+"╮"))
		labels = append(labels, edge.Render("│")+style.Render(label)+edge.Render("│"))
		bottoms = append(bottoms, bottom)
	}
	return strings.Join(tops, " ") + "\n" + strings.Join(labels, " ") + "\n" + strings.Join(bottoms, edge.Render("─"))
}
func (m *model) mainPanel(width, height int) string {
	color := borderColor
	if m.focus == 1 {
		color = focusedBorder
	}
	frame := lipgloss.NewStyle().Foreground(color)
	lines := strings.Split(m.tabs(width), "\n")
	bodyHeight := height - 4
	if m.command != nil {
		bodyHeight -= 3
	}
	content := strings.Split(m.mainBody(width-4, bodyHeight), "\n")
	content = content[:min(len(content), bodyHeight)]
	for len(content) < bodyHeight {
		content = append(content, "")
	}
	if m.command != nil {
		content = append(content, strings.Split(m.commandBox(width-4), "\n")...)
	}
	for i := 0; i < height-4; i++ {
		text := ""
		if i < len(content) {
			text = clipped(content[i], width-4)
		}
		lines = append(lines, frame.Render("│")+" "+text+strings.Repeat(" ", max(0, width-4-ansi.StringWidth(text)))+" "+frame.Render("│"))
	}
	lines = append(lines, frame.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(lines, "\n")
}

func (m *model) commandBox(width int) string {
	color := borderColor
	if m.focus == 1 {
		color = accent
	}
	text := ":" + editorLine(*m.command, width-5, m.focus == 1)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Padding(0, 1).Width(width).Height(3).MaxWidth(width).MaxHeight(3).Render(text)
}

func (m *model) mainBody(width, height int) string {
	if m.activityReviewVisible() {
		return m.activityReviewBody(width, height)
	}
	if m.exploreVisible() {
		return m.exploreBody(width, height)
	}
	scope := "Start here"
	if w := m.current(); w != nil {
		scope = w.Name
		if m.tool != "" {
			scope += " / " + toolNames[m.tool]
		}
		if c := m.currentConnection(); c != nil {
			scope += " / " + c.Name
		}
	}
	heading := muted.Render(clipped(oneLine(scope), width)) + "\n\n"
	available := max(1, height-2)
	return heading + m.viewContent(width, available)
}

// Shared content keeps settings forms and pickers inside the same overlay.
func (m *model) viewContent(width, height int) string {
	available := max(1, height)
	var body string
	switch {
	case m.form != nil && m.form.kind == "kube":
		body = m.contextPickerBody(width, available)
	case m.form != nil:
		body = m.formBody(width, available)
	case m.page == "settings":
		body = m.settingsBody(width, available)
	case m.page == "tools":
		items := []string{}
		for _, tool := range toolTypes {
			items = append(items, toolNames[tool])
		}
		body = sectionHeadingStyle.Render("Choose a tool") + "\n\n" + choiceList(items, m.cursor, width, max(1, available-2), settingsChoiceLine)
	case m.page == "removeprovider":
		body = removedStyle.Bold(true).Render("Remove "+oneLine(m.removeProfile)+"?") + "\n" + choiceList([]string{"Cancel", "Remove AI connection"}, m.cursor, width, available-1, settingsChoiceLine)
	case m.page == "remove":
		name := "connection"
		if w := m.current(); w != nil {
			for _, c := range w.Connections {
				if c.ID == m.removeID {
					name = c.Name
				}
			}
		}
		body = removedStyle.Bold(true).Render("Remove "+oneLine(name)+"?") + "\n" + choiceList([]string{"Cancel", "Remove from workspace"}, m.cursor, width, available-1, settingsChoiceLine)
	case m.current() == nil:
		body = emptyState("Welcome to Plous", "Organize your tools and connections in a workspace.", "Enter  Create workspace\ncc     Start a conversation", available)
		if m.searches[1] != nil {
			body = m.findText(1, body, width, available, 0)
		}
	case m.activityDetailActive():
		control := selected.Render(clipped("[ d View changes ]", width)) + "\n\n"
		space := max(0, available-2)
		body = styledTextView(m.detail, width, space, m.cursor)
		if m.searches[1] != nil {
			body = m.findText(1, m.detail, width, space, m.cursor)
		}
		body = control + body
	case m.detail != "":
		body = styledTextView(m.detail, width, available, m.cursor)
		if m.searches[1] != nil {
			body = m.findText(1, m.detail, width, available, m.cursor)
		}
	case m.mode == 2:
		changes := m.filteredChanges()
		heading := gradientHeading("Configuration history") + "\n\n"
		if m.searches[1] != nil {
			heading = m.searchBar(1, width, "Search activity") + "\n"
		}
		if len(changes) == 0 && m.searches[1] != nil {
			body = heading + "No matching changes."
		} else if len(changes) == 0 {
			body = emptyState("No changes yet", "Workspace configuration changes will appear here.", ":s  Open Settings", available)
		} else {
			lines := []string{}
			pos := min(max(0, m.positions[m.viewKey()]), len(changes)-1)
			for i, c := range changes {
				lines = append(lines, activityRow(c, i == pos, width),
					muted.Render(clipped(fmt.Sprintf("    r%d · %s", c.Revision, c.Time.Local().Format("Jan 02 · 15:04")), width)))
			}
			body = heading + scrollSelection(lines, pos*2, max(1, available-strings.Count(heading, "\n")))
		}
	default:
		body = styledTextView(m.modeContent(width, available), width, available, m.positions[m.viewKey()])
		if m.searches[1] != nil {
			body = m.findText(1, m.modeContent(width, available), width, available, m.positions[m.viewKey()])
		}
	}
	return styledView(body, width, available)
}
func (m *model) modeContent(width, height int) string {
	w := m.current()
	if w == nil {
		return ""
	}
	switch m.mode {
	case 0:
		if len(w.Connections) == 0 {
			return gradientHeading("Workspace overview") + "\n" + muted.Render(fmt.Sprintf("revision %d", w.Revision)) + "\n\n" +
				emptyState("No connections yet", "Connect a tool to this workspace.", "Enter  Add tool", height-4)
		}
		count := 0
		groups := []string{}
		for _, tool := range toolTypes {
			if m.tool != "" && tool != m.tool {
				continue
			}
			connections := []string{}
			for _, c := range w.Connections {
				if c.Tool == tool {
					connections = append(connections, "  "+oneLine(c.Name))
				}
			}
			if len(connections) > 0 {
				count += len(connections)
				groups = append(groups, titleStyle.Render(toolNames[tool])+muted.Render(fmt.Sprintf("  %d", len(connections))), strings.Join(connections, "\n"), "")
			}
		}
		noun := "connections"
		if count == 1 {
			noun = "connection"
		}
		return gradientHeading("Workspace overview") + "\n" + muted.Render(fmt.Sprintf("%d %s · revision %d", count, noun, w.Revision)) + "\n\n" +
			strings.Join(groups, "\n") + "\n" + selected.Render(":s  Manage connections") + "\n" + muted.Render("Saved configuration · live status unavailable")
	case 1:
		c := m.currentConnection()
		if c == nil {
			// Commands cover the bottom rows without recentering the artwork.
			welcomeHeight := height
			if m.command != nil {
				welcomeHeight += 3
			}
			return styledTextView(exploreWelcome(width, welcomeHeight), width, height, 0)
		}
		text := gradientHeading("Connection settings") + "\n" + muted.Render("Saved configuration") + "\n\n"
		keys := []string{}
		for k := range c.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if c.Settings[k] != "" {
				label := settingLabel(k)
				value := oneLine(c.Settings[k])
				if width < 48 || ansi.StringWidth(value) > width-13 {
					text += muted.Render(label) + "\n" + value + "\n\n"
				} else {
					text += muted.Render(fmt.Sprintf("%-12s", label)) + value + "\n\n"
				}
			}
		}
		return text + selected.Render(":s  Edit in Settings") + "\n" + muted.Render("Live resource browsing is not available yet.")
	}
	return ""
}

func emptyState(title, description, action string, height int) string {
	text := gradientHeading(title) + "\n\n"
	if height >= 9 {
		text += description + "\n\n"
	}
	return text + selected.Render(action)
}
func (m *model) chatBody(width, height int) string {
	if m.chat().models {
		return m.modelPickerBody(width, height)
	}
	boxHeight := min(8, max(5, height/3))
	inputWidth, inputHeight := max(1, width-4), max(1, boxHeight-3)
	active := m.focus == 2 && m.chatEditing && !m.chat().history
	e := m.draft()
	text := e.display(active)
	if e.text == "" {
		text = ":c or cc to type"
		if active {
			text = "▏Ask anything…"
		}
	}
	lines := strings.Split(ansi.Wrap(text, inputWidth, ""), "\n")
	offset := max(0, len(lines)-inputHeight)
	if active {
		prefix := string([]rune(e.text)[:min(e.cursor, len([]rune(e.text)))]) + "▏"
		cursorLine := len(strings.Split(ansi.Wrap(prefix, inputWidth, ""), "\n")) - 1
		offset = max(0, cursorLine-inputHeight+1)
	}
	text = textView(text, inputWidth, inputHeight, offset)

	editorContent := lipgloss.NewStyle().Width(inputWidth).Height(inputHeight).MaxWidth(inputWidth).MaxHeight(inputHeight).Render(text)
	border := borderColor
	if active {
		border = accent
	}
	label := chatModelName(m.settings.Agent)
	if label != "" && !strings.HasSuffix(m.settings.Agent.Name, label) {
		label += " · " + m.settings.Agent.Name
	}
	if m.settings.Agent.Model == "" {
		label = "No model"
	}
	label = clipped(oneLine(label), max(1, inputWidth-7))
	label += strings.Repeat(" ", max(1, inputWidth-ansi.StringWidth(label)-6)) + "/model"
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(width).Height(boxHeight).MaxWidth(width).MaxHeight(boxHeight).
		Render(muted.Render(label) + "\n" + editorContent)
	transcriptHeight := max(0, height-boxHeight-2)
	transcript := m.chatTranscript(width, transcriptHeight)
	transcript = lipgloss.NewStyle().Width(width).Height(transcriptHeight).MaxWidth(width).MaxHeight(transcriptHeight).Render(transcript)
	return transcript + "\n\n" + box + "\n" + muted.Render(m.chatContextLine(width))
}
func (m *model) hint() string {
	if m.shortcutPrefix != "" {
		_, label := navigationShortcut(m.shortcutPrefix)
		return m.shortcutPrefix + " → " + m.shortcutPrefix + " " + label + " · Esc cancel"
	}
	if m.command != nil && m.focus == 1 {
		return "Enter run · Esc cancel"
	}
	if m.help != nil {
		return "←→ sections · ↑↓ / PgUp PgDn scroll · Esc close"
	}
	if s := m.searches[m.focus]; s != nil {
		if s.kind == "find" {
			if s.finished {
				return "n/N matches · / new search · Esc clear search"
			}
			return "Enter finish search · ↑↓ matches · Esc clear search"
		}
		return "Type to filter · ↑↓ select · Enter open · Esc close search"
	}
	if m.settingsOverlay != nil && m.focus != 1 {
		if m.focus != 2 || !m.chatEditing {
			return "Settings paused · Tab to resume · :c chat · :h help"
		}
	}
	if m.focus == 0 {
		return "↑↓ Navigate · Enter Open/fold · / Search · Tab Switch panel · hh Help"
	}
	if m.focus == 2 {
		if m.chat().models {
			return "↑↓ choose model · Enter select · Esc back"
		}
		if m.chatEditing && !m.chat().history {
			return "Enter Send · Shift+Enter Newline · Esc Read conversation"
		}
		if m.chat().history {
			return "↑↓ choose · Enter open · / search · Esc back · :h help"
		}
		return "cc Write message · / Find · End Latest · hh Help"
	}
	if m.form != nil {
		if m.form.kind == "kube" {
			return "↑↓ select · Space check · Enter next/activate · Esc back"
		}
		if m.form.index < len(m.form.fields) && len(m.form.fields[m.form.index].options) > 0 {
			return "←→ choose · ↑↓ fields · Enter next · Esc cancel"
		}
		return "↑↓ fields · Enter next/activate · Esc cancel"
	}
	if m.page == "settings" {
		return "↑↓ select · Enter open · Esc close"
	}
	if m.page != "" {
		return "↑↓ select/scroll · Enter open · Esc back"
	}
	if m.current() == nil {
		return "Enter New workspace · ss Settings · hh Help"
	}
	if m.mode == 2 {
		if m.detail != "" {
			return "/ find · ↑↓ / PgUp PgDn scroll · Esc back · :h help"
		}
		return "Enter Open change · / Search · " + m.sidebarHint() + " · hh Help"
	}
	if m.exploreVisible() {
		return m.exploreHint()
	}
	if m.mode == 0 && len(m.current().Connections) == 0 {
		return "Enter Add tool · " + m.sidebarHint() + " · hh Help"
	}
	return "/ Find · " + m.sidebarHint() + " · Tab Switch panel · hh Help"
}

// Body components already sanitize user text before applying terminal styles.
func styledView(text string, width, height int) string {
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	return strings.Join(lines[:min(len(lines), max(0, height))], "\n")
}

func editorLine(e editor, width int, active bool) string {
	text := e.display(active)
	if active {
		prefix := string([]rune(e.text)[:min(e.cursor, len([]rune(e.text)))]) + "▏"
		skip := max(0, ansi.StringWidth(prefix)-max(1, width)+1)
		text = ansi.TruncateLeft(text, skip, "")
	}
	return clipped(text, width)
}

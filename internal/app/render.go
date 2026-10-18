package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"sort"
	"strings"
)

func (m *model) View() tea.View {
	if m.nativeScroll.deferView && m.nativeScroll.view != nil {
		return *m.nativeScroll.view
	}
	rendered := m.render()
	if !m.tooSmall() && m.width > 0 && m.height > 0 {
		lines := strings.Split(rendered, "\n")
		lines[len(lines)-1] = textview.MouseScope(m.footer(), "footer")
		rendered = strings.Join(lines, "\n")
	}
	content, spans := textview.MouseLayout(rendered)
	m.mouse.spans, m.mouse.context = spans, m.mouseContext()
	v := tea.NewView(content)
	if !m.settings.MouseDisabled && !m.mainTerminalCaptured() && !m.tooSmall() {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.AltScreen = true
	v.BackgroundColor, v.ForegroundColor = ui.Background, ui.Foreground
	if m.resourceTerminalVisible() && m.explore.Terminal.Captured && m.explore.Terminal.CursorVisible.Load() && !m.tooSmall() {
		pos := m.explore.Terminal.Emulator.CursorPosition()
		_, rows := m.terminalDimensions()
		v.Cursor = tea.NewCursor(pos.X+m.panelWidths()[0]+2, pos.Y+4+(m.height-6-rows))
	}
	if m.nativeAgentCaptured() && !m.nativeAgent.Reading() && m.nativeAgent.CursorVisible.Load() {
		pos := m.nativeAgent.Screen.CursorPosition()
		widths := m.panelWidths()
		v.Cursor = tea.NewCursor(widths[0]+widths[1]+2+pos.X, 4+pos.Y)
	}
	m.nativeScroll.view = &v
	return v
}

func (m *model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.tooSmall() {
		text := "Plous " + m.version + "\nResize terminal to at least 80 x 16.\nCtrl+C to exit.\nFocus: " + []string{"tree", "main", "chat"}[m.focus]
		if m.command != nil {
			text += "\n:" + m.command.Display(m.focus == 1)
		}
		if m.status != "" {
			text += "\n" + ui.OneLine(m.status)
		}
		return lipgloss.NewStyle().Width(m.width).Height(m.height).MaxWidth(m.width).MaxHeight(m.height).Render(text)
	}
	if m.helpVisible() {
		return m.renderHelpOverlay()
	}
	if m.chatSourcesVisible() {
		return m.renderChatSourcesOverlay()
	}
	widths := m.panelWidths()
	height := m.height - 2
	header := ui.BrandStyle.Render(" plous")
	label := ui.Muted.Render("v" + m.version + " ")
	header += strings.Repeat(" ", m.width-lipgloss.Width(header)-lipgloss.Width(label)) + label
	bodyHeight := height - 4
	visible := []string{}
	if m.panelVisible(0) {
		if !m.nativeScroll.onlyNative || m.nativeScroll.panels[0] == "" {
			m.nativeScroll.panels[0] = textview.MouseScope(ui.Panel("Connections", m.treeBody(widths[0]-4, bodyHeight), widths[0], height, m.focus == 0), "tree")
		}
		visible = append(visible, m.nativeScroll.panels[0])
	}
	base := *m
	if previous := m.settingsOverlay; previous != nil {
		base.page, base.detail, base.cursor = previous.Page, previous.Detail, previous.Cursor
		base.form, base.command = nil, nil
	}
	if m.panelVisible(1) {
		if !m.nativeScroll.onlyNative || m.nativeScroll.panels[1] == "" {
			m.nativeScroll.panels[1] = textview.MouseScope(base.mainPanel(widths[1], height), "main")
		}
		visible = append(visible, m.nativeScroll.panels[1])
	}
	if m.panelVisible(2) {
		visible = append(visible, textview.MouseScope(ui.Panel(m.chatPanelTitle(widths[2]-4), m.chatBody(widths[2]-4, bodyHeight), widths[2], height, m.focus == 2), "chat"))
	}
	panels := lipgloss.JoinHorizontal(lipgloss.Top, visible...)

	screen := lipgloss.JoinVertical(lipgloss.Left, header, panels, textview.MouseScope(m.footer(), "footer"))
	if m.settingsOverlay != nil && m.focus == 1 && !m.helpVisible() {
		return m.renderSettingsOverlay(screen)
	}
	if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		return m.renderResourceActionOverlay(screen)
	}
	return screen
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

func (m *model) tabs(width int, edge lipgloss.Style) string {
	tops, labels, bottoms := []string{}, []string{}, []string{}
	for i, cellWidth := range tabWidths(width) {
		innerWidth := cellWidth - 2
		shortcut := fmt.Sprintf(":%d", i+1)
		name := ui.Clipped(modes[i], innerWidth-5)
		label := " " + name + strings.Repeat(" ", max(1, innerWidth-ansi.StringWidth(name)-len(shortcut)-2)) + shortcut + " "
		style := ui.Muted
		bottom := edge.Render("┴" + strings.Repeat("─", innerWidth) + "┴")
		if m.mode == i {
			style = ui.TitleStyle
			bottom = edge.Render("╯") + strings.Repeat(" ", innerWidth) + edge.Render("╰")
		}
		tops = append(tops, ui.MouseCommand(edge.Render("╭"+strings.Repeat("─", innerWidth)+"╮"), fmt.Sprint(i+1)))
		labels = append(labels, ui.MouseCommand(edge.Render("│")+style.Render(label)+edge.Render("│"), fmt.Sprint(i+1)))
		bottoms = append(bottoms, ui.MouseCommand(bottom, fmt.Sprint(i+1)))
	}
	return strings.Join(tops, " ") + "\n" + strings.Join(labels, " ") + "\n" + strings.Join(bottoms, edge.Render("─"))
}

func (m *model) mainPanel(width, height int) string {
	color := ui.BorderColor
	if m.focus == 1 {
		color = ui.FocusedBorder
	}
	frame := lipgloss.NewStyle().Foreground(color)
	lines := strings.Split(m.tabs(width, frame), "\n")
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
			text = ui.Clipped(content[i], width-4)
		}
		lines = append(lines, frame.Render("│")+" "+text+strings.Repeat(" ", max(0, width-4-ansi.StringWidth(text)))+" "+frame.Render("│"))
	}
	lines = append(lines, frame.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(lines, "\n")
}

func (m *model) commandBox(width int) string {
	color := ui.BorderColor
	if m.commandFocused() {
		color = ui.Accent
	}
	text := ":" + ui.EditorLine(*m.command, width-5, m.commandFocused(), "command")
	return ui.MouseTarget(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Padding(0, 1).Width(width).Height(3).MaxWidth(width).MaxHeight(3).Render(text), "command")
}

func (m *model) mainBody(width, height int) string {
	if m.presentedDocumentVisible() {
		return m.uiDocument.model.View(width, height, ui.WorkspaceTheme())
	}
	if m.overviewVisible() {
		return m.overviewBody(width, height)
	}
	if m.mode == 2 && m.current() != nil && m.page == "" && m.form == nil && m.detail == "" {
		return m.activityBody(width, height)
	}
	if m.codeVisible() {
		return m.codeBody(width, height)
	}

	if m.gitOpsVisible() {
		return m.gitOpsBody(width, height)
	}
	if m.helmVisible() {
		return m.helmBody(width, height)
	}
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
			scope += " / " + appsettings.ToolNames[m.tool]
		}
		if c := m.currentConnection(); c != nil {
			scope += " / " + c.Name
		}
	}
	heading := ui.Muted.Render(ui.Clipped(ui.OneLine(scope), width)) + "\n\n"
	available := max(1, height-2)
	return heading + m.viewContent(width, available)
}

// Shared content keeps settings forms and pickers inside the same overlay.
func (m *model) viewContent(width, height int) string {
	available := max(1, height)
	var body string
	switch {
	case m.form != nil && m.form.Directory != nil:
		body = m.directoryPickerBody(width, available)
	case m.form != nil && m.form.Kind == "kube":
		body = m.contextPickerBody(width, available)
	case m.form != nil:
		body = m.formBody(width, available)
	case m.page == "settings":
		body = m.settingsBody(width, available)
	case m.page == "tools":
		body = m.connectionPickerBody(width, available)
	case m.page == "removeprovider":
		body = ui.RemovedStyle.Bold(true).Render("Remove "+ui.OneLine(m.removeProfile)+"?") + "\n" + ui.ChoiceList([]string{"Cancel", "Remove provider connection"}, m.cursor, width, available-1, ui.SettingsChoiceLine)
	case m.page == "remove":
		name := "connection"
		if w := m.current(); w != nil {
			for _, c := range w.Connections {
				if c.ID == m.removeID {
					name = c.Name
				}
			}
		}
		body = ui.RemovedStyle.Bold(true).Render("Remove "+ui.OneLine(name)+"?") + "\n" + ui.ChoiceList([]string{"Cancel", "Remove from workspace"}, m.cursor, width, available-1, ui.SettingsChoiceLine)
	case m.currentRoot && m.current() == nil:
		body = m.currentBody(width, available)
	case m.current() == nil:
		body = ui.EmptyState("Welcome to Plous", "Organize your tools and connections in a workspace.", "Enter  Create Workspace\ncc     Start a conversation", available)
		if m.mode == 1 {
			body = m.welcomeBody(width, available)
		}
		if m.searches[1] != nil {
			body = m.findText(1, body, width, available, 0)
		}
	case m.activityDetailActive():
		control := ui.MouseAction(ui.Selected.Render(ui.Clipped("[ d View changes ]", width)), "d") + "\n\n"
		space := max(0, available-2)
		body = ui.StyledTextView(m.detail, width, space, m.cursor)
		if m.searches[1] != nil {
			body = m.findText(1, m.detail, width, space, m.cursor)
		}
		body = control + body
	case m.mode == 2 && m.operationDetail != nil && m.detail != "":
		control := m.activityOperationControls(width)
		space := max(0, available-strings.Count(control, "\n")-2)
		body = ui.StyledTextView(m.detail, width, space, m.cursor)
		if m.searches[1] != nil {
			body = m.findText(1, m.detail, width, space, m.cursor)
		}
		body = control + "\n\n" + body

	case m.detail != "":
		body = ui.StyledTextView(m.detail, width, available, m.cursor)
		if m.searches[1] != nil {
			body = m.findText(1, m.detail, width, available, m.cursor)
		}
	default:
		body = ui.StyledTextView(m.modeContent(width, available), width, available, m.positions[m.viewKey()])
		if m.searches[1] != nil {
			body = m.findText(1, m.modeContent(width, available), width, available, m.positions[m.viewKey()])
		}
	}
	return ui.StyledView(body, width, available)
}

func (m *model) modeContent(width, height int) string {
	w := m.current()
	if w == nil {
		return ""
	}
	switch m.mode {
	case 0:
		if len(w.Connections) == 0 {
			return ui.Heading("Workspace overview") + "\n" + ui.Muted.Render(fmt.Sprintf("revision %d", w.Revision)) + "\n\n" +
				ui.EmptyState("No connections yet", "Add a cluster or source to this workspace.", "Enter  Add Connection", height-4)
		}
		count := 0
		groups := []string{}
		for _, area := range []string{"monitor", "code"} {
			if m.tool != "" && connectionArea(m.tool) != area {
				continue
			}
			connections := []string{}
			for _, c := range w.Connections {
				if connectionArea(c.Tool) != area || m.connection != "" && c.ID != m.connection {
					continue
				}
				label := appsettings.ToolNames[c.Tool]
				if monitorCluster(c.Tool) {
					label = "Kubernetes + Helm"
				}
				if sourceConnection(c.Tool) {
					label = "Source"
				}
				connections = append(connections, "  "+ui.OneLine(c.Name)+" · "+label)
			}
			if len(connections) > 0 {
				count += len(connections)
				label := "Monitor"
				if area == "code" {
					label = "Code"
				}
				groups = append(groups, ui.TitleStyle.Render(label)+ui.Muted.Render(fmt.Sprintf("  %d", len(connections))), strings.Join(connections, "\n"), "")
			}
		}
		noun := "connections"
		if count == 1 {
			noun = "connection"
		}
		return ui.Heading("Workspace overview") + "\n" + ui.Muted.Render(fmt.Sprintf("%d %s · revision %d", count, noun, w.Revision)) + "\n\n" +
			strings.Join(groups, "\n") + "\n" + ui.Selected.Render(":s  Manage connections") + "\n" + ui.Muted.Render("Saved configuration · live status unavailable")
	case 1:
		c := m.currentConnection()
		if c == nil {
			return m.welcomeBody(width, height)
		}
		text := ui.Heading("Connection settings") + "\n" + ui.Muted.Render("Saved configuration") + "\n\n"
		keys := []string{}
		for k := range c.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if c.Settings[k] != "" {
				label := activity.SettingLabel(k)
				value := ui.OneLine(c.Settings[k])
				if width < 48 || ansi.StringWidth(value) > width-13 {
					text += ui.Muted.Render(label) + "\n" + value + "\n\n"
				} else {
					text += ui.Muted.Render(fmt.Sprintf("%-12s", label)) + value + "\n\n"
				}
			}
		}
		return text + ui.Selected.Render(":s  Edit in Settings") + "\n" + ui.Muted.Render("Live resource browsing is not available yet.")
	}
	return ""
}

func (m *model) hint() string {
	if m.shortcutPrefix != "" {
		_, label := navigationShortcut(m.shortcutPrefix)
		return m.shortcutPrefix + " → " + m.shortcutPrefix + " " + label + " · Esc cancel"
	}
	if m.externalAgentSelected() && m.focus == 2 && !m.helpVisible() && !m.chatSourcesVisible() && !m.commandFocused() {
		if m.nativeAgent != nil && m.nativeAgent.Reading() {
			return "Wheel / PgUp PgDn Scroll output · End / Esc Live · Alt+←/→ Panels · Ctrl+] Return to panel"
		}
		if m.nativeAgentCaptured() {
			return "Alt+←/→ Panels · Ctrl+] Return to panel · Ctrl+C Interrupt agent"
		}
		return "Ctrl+] Return to panel · cc Enter terminal · ii Resize panel · Alt+←/→ Panels · :X Close session · hh Help"
	}
	if m.activeInteraction().owner == interactionCommand {
		return "Enter run · Esc cancel"
	}
	if m.activeInteraction().owner == interactionHelp {
		return "←→ sections · ↑↓ / PgUp PgDn scroll · Esc close"
	}
	if s := m.activePanelSearch(m.focus); s != nil {
		if s.kind == "find" {
			if s.finished {
				return "n/N matches · / new search · Esc clear search"
			}
			return "Enter finish search · ↑↓ matches · Esc clear search"
		}
		if s.kind == "activity" && s.finished {
			return "↑↓ Select · Enter Details · / Edit search · Esc Clear search · hh Help"
		}
		return "Type to filter · ↑↓ select · Enter open · Esc close search"
	}
	if m.settingsOverlay != nil && m.focus != 1 {
		if m.focus != 2 || !m.chatEditing {
			return "Settings paused · Alt+←/→ to resume · :c chat · :h help"
		}
	}
	if m.focus == 0 {
		return "↑↓ Navigate · ←→ Collapse/expand · Enter Open/fold · g Refresh · / Search · Alt+←/→ Panels · hh Help"
	}
	if m.focus == 2 {
		if m.chat().Models {
			return "↑↓ choose model · Enter select · Esc back"
		}
		if m.chatEditing && !m.chat().History {
			return "Enter Send · Shift+Enter Newline · Esc Read conversation"
		}
		if m.chat().History {
			return "↑↓ choose · Enter open · / search · Esc back · :h help"
		}
		return "cc Write message · [ ] Reply · Enter Sources · ↑↓ Scroll · / Find · End Latest · hh Help"
	}
	if m.form != nil {
		if m.form.Directory != nil {
			if m.form.Directory.Editing {
				return "Enter Open path · Esc Cancel typing"
			}
			return "↑↓ Select · Enter Open/select · ← Parent · / Path · Esc Back"
		}
		if m.form.Kind == "kube" {
			if m.form.Index == 0 {
				return "Enter Next · Esc Back"
			}
			index := m.form.Index - 2
			if index >= 0 && index < len(m.contexts) {
				if m.contextAlreadyAdded(index) {
					return "↑↓ Select · Already added · Esc Back"
				}
				return "↑↓ Select · Space Check · → Name · Enter Check · Esc Back"
			}
			return "↑↓ Select · Enter Activate · Esc Back"
		}
		if m.form.Index < len(m.form.Fields) && len(m.form.Fields[m.form.Index].Options) > 0 {
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
		return "Enter New Workspace · ss Settings · hh Help"
	}
	if m.codeVisible() {
		return m.codeHint()
	}

	if m.gitOpsVisible() {
		return m.gitOpsHint()
	}
	if m.helmVisible() {
		return m.helmHint()
	}
	if m.mode == 2 {
		if m.detail != "" {
			return "/ find · ↑↓ / PgUp PgDn scroll · Esc back · :h help"
		}
		return "↑↓ Select · Enter Details · / Search · " + m.sidebarHint() + " · hh Help"
	}
	if m.exploreVisible() {
		return m.exploreHint()
	}
	if m.overviewVisible() {
		action := "Inspect"
		navigation := "←→ Tiles · ↑↓ Rows/sections"
		v := m.overviewRead()
		if v != nil && v.State.MetricDetail != nil {
			d := v.State.MetricDetail
			back := "Overview"
			if d.Sources {
				back = "Chart"
			}
			if d.Reader().Searching {
				return "Type to find · Enter Finish · Esc Cancel search"
			}
			hint := "↑↓ / PgUp PgDn Scroll · / Find · Esc " + d.Reader().EscapeLabel(back)
			if action := d.EnterLabel(); action != "" {
				hint = "Enter " + action + " · " + hint
			}
			return hint + " · g Refresh · ff Fullscreen · hh Help"
		}
		if v != nil {
			action = v.State.EnterLabel()
			navigation = v.State.NavigationHint()
		}
		if v != nil && v.State.Input != nil {
			return "Type to filter · ↑↓ Select · Enter " + action + " · Esc Clear search"
		}
		if v != nil && v.State.Pane == 3 {
			if v.State.InventoryTool != "" {
				return navigation + " · Enter " + action + " · Esc Back · / Search · e Expand · g Refresh · hh Help"
			}
			return navigation + " · ↑↓ Counters · Tab Tree · Enter " + action + " · Esc Back · e Expand · g Refresh · hh Help"
		}
		return navigation + " · Enter " + action + " · e Expand · / Search · g Refresh · ff Fullscreen · hh Help"
	}
	if m.mode == 0 && len(m.current().Connections) == 0 {
		return "Enter Add Connection · " + m.sidebarHint() + " · hh Help"
	}
	return "/ Find · " + m.sidebarHint() + " · Alt+←/→ Panels · hh Help"
}

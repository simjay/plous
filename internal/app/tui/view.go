package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) View() tea.View {
	if m.nativeScroll.deferView && m.nativeScroll.view != nil {
		return *m.nativeScroll.view
	}
	if m.mouse.screenScope == "footer" && (m.status != m.mouse.screenStatus || m.currentNotice != m.mouse.screenNotice) {
		m.mouse.reset()
	}
	content, spans := textview.MouseLayout(m.renderMouseFrame())
	context := m.mouseContext()
	if s := m.mouse.screen; s != nil && (context != m.mouse.context || m.focus != m.mouse.screenFocus || !s.Matches(content)) {
		m.mouse.reset()
		content, spans = textview.MouseLayout(m.renderMouseFrame())
	}
	m.mouse.frame, m.mouse.spans, m.mouse.context = content, spans, context
	if m.screenSelectionActive() {
		content = m.mouse.screen.View(content, ui.WorkspaceTheme())
	}
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

func (m *model) renderMouseFrame() string {
	rendered := m.render()
	if !m.tooSmall() && m.width > 0 && m.height > 0 {
		lines := strings.Split(rendered, "\n")
		lines[len(lines)-1] = textview.MouseScope(m.footer(), "footer")
		rendered = strings.Join(lines, "\n")
	}
	if m.screenSelectionActive() && m.mouse.screenScope == "footer" {
		// Keep selected footer text stationary and place Copy in the top bar.
		copy := textview.MouseScope(textview.ActionChip(textview.Action{Label: "Copy selection", Mouse: "clipboard/copy"}, m.width, ui.WorkspaceTheme()), "footer")
		lines := strings.Split(rendered, "\n")
		lines[0] = ansi.Truncate(lines[0], max(0, m.width-ansi.StringWidth(copy)), "") + copy
		rendered = strings.Join(lines, "\n")
	}
	return rendered
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
		if len(m.sidebarConnections(*w)) == 0 {
			return ui.Heading("Workspace overview") + "\n" + ui.Muted.Render(fmt.Sprintf("revision %d", w.Revision)) + "\n\n" +
				ui.EmptyState("No connections yet", "Add a cluster to this workspace.", "Enter  Add Connection", height-4)
		}
		connections := []string{}
		for _, c := range w.Connections {
			if !monitorCluster(c.Tool) || m.connection != "" && c.ID != m.connection {
				continue
			}
			connections = append(connections, "  "+ui.OneLine(c.Name)+" · Kubernetes + Helm")
		}
		count := len(connections)
		groups := []string{}
		if count > 0 {
			groups = append(groups, ui.TitleStyle.Render("Monitor")+ui.Muted.Render(fmt.Sprintf("  %d", count)), strings.Join(connections, "\n"), "")
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
	if m.mode == 0 && len(m.sidebarConnections(*m.current())) == 0 {
		return "Enter Add Connection · " + m.sidebarHint() + " · hh Help"
	}
	return "/ Find · " + m.sidebarHint() + " · Alt+←/→ Panels · hh Help"
}

func (m *model) footerContext() string {
	switch m.focus {
	case 0:
		return "Connections"
	case 2:
		return "AI assistant"
	}
	if m.presentedDocumentVisible() {
		return "Assistant document"
	}

	if m.gitOpsVisible() {
		return m.gitOpsCurrent().State.Title()
	}
	if m.helmVisible() {
		return "Helm " + m.helmer().Preference.Section
	}
	if m.exploreVisible() && m.explore != nil && m.explore.Resource.Name != "" {
		return monitor.ResourceTitle(m.explore.Resource)
	}
	return modes[m.mode]
}

func (m *model) sidebarHint() string {
	if m.leftHidden {
		return "bb Show connections"
	}
	return "Esc Connections"
}

func documentBackHint(d *docedit.Model, back string) string {
	switch {
	case d.Searching:
		return "Cancel search"
	case d.Prefix() != "":
		return "Cancel pending key"
	case d.Mode != docedit.Normal:
		return "Normal mode"
	case d.Query != "":
		return "Clear search"
	}
	return back
}

func (m *model) footer() string {
	if m.screenSelectionActive() && m.mouse.screenScope == "footer" {
		return m.mouse.screenFooter
	}
	label, message := m.footerContext(), m.hint()
	if m.helpVisible() {
		label = "Help"
	} else if m.chatSourcesVisible() {
		label, message = "Sources", m.chatSourcesHint()
		if m.chatSources.ShowReply {
			label = "Read reply"
		}
	} else if m.settingsOverlay != nil && m.focus == 1 {
		label = "Settings"
		if m.toolSetup != nil {
			label = "Add Connection"
			if m.toolSetup.ContextsOnly {
				label = "Add Context"
			}
		}
		if m.form != nil {
			if m.form.Kind == "kube" || m.form.Kind == "contextname" {
				label = "Add Context"
			}
		}
	} else if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		label, message = "Actions", "↑↓ Choose · Enter Select · Esc Back · hh Help"
		a := m.explore.Action
		if a.Busy {
			message = "Esc Cancel request"
		} else if a.Stage == "scale" || a.Stage == "export" || a.Stage == "execcommand" && a.ExecEditing {
			message = "Enter Continue · Esc Back"
		} else if a.Stage == "confirm" {
			message = "←→ Choose · Enter Confirm · Esc Back · hh Help"
		} else if a.Stage == "done" {
			message = "Esc Return to resource · hh Help"
			if a.Progress != nil {
				message = monitor.OperationProgressFooter(a.Progress, m.width)
			}
		}
	} else if m.focus == 1 {
		switch {
		case m.presentedDocumentVisible():
			message = "/ Find · y Copy · Esc " + documentBackHint(m.uiDocument.model, "Back") + " · hh Help"
		case m.activityReviewVisible():
			label, message = "Changes", "[ ] Change · / Find · Esc "+m.activityReview.Comparison.EscapeLabel()+" · hh Help"
			if m.activityReviewSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		case m.activityDetailActive() && m.searches[1] == nil:
			message = "d View changes · / Find · Esc Activity · hh Help"
		case m.mode == 2 && m.detail != "" && m.operationDetail != nil && m.searches[1] == nil:
			message = "↑↓ Scroll · / Find · Esc Activity · hh Help"
			if m.activityHasObservations() {
				message = "O Observations · " + message
			}
			if m.activityReleaseUnavailable(m.operationDetail) == "" {
				message = "Enter Open release · " + message
			}
		case m.valueDocumentActive():
			message = "/ Find · y Copy · a Actions · Esc " + documentBackHint(m.explore.Action.ValueDocument, "Hide value") + " · hh Help"
			if m.explore.Action.ValueDocument.Searching {
				message = "Enter Finish search · Esc Cancel search"
			}
		case m.yamlDocumentActive():
			back := documentBackHint(m.syncYAMLDocument().Document, strings.TrimPrefix(m.resourceBackLabel(), "Esc "))
			message = "/ Find · y Copy · E Edit · Esc " + back + " · hh Help"
			if m.yamlDocumentSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		}
	}
	if m.documentActive() && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil {
		label, message = "Editor", m.documentHint()
		a := m.explore.Action
		if a.Stage == "review" {
			label = "Review changes"
		} else if a.Document != nil {
			label += " " + string(a.Document.Mode)
		}
	}
	if m.resourceTerminalVisible() {
		label, message = "Exec", "Alt+←/→ Panels · Ctrl+] Review output · Ctrl+C Exit Plous"
		if !m.explore.Terminal.Captured {
			label = "Exec review"
			message = "r Resume input · End Latest · / Find · Esc Close session · hh Help"
			if m.explore.Terminal.Done {
				message = "e Edit command · R Retry · End Latest · Esc Close session · hh Help"
			}
			if m.explore.Terminal.Review != nil {
				reader := &m.explore.Terminal.Review.Viewer.Reader
				message = strings.Replace(message, "Esc Close session", "Esc "+reader.EscapeLabel("Close session"), 1)
				if reader.Searching {
					message = "Enter Finish search · Esc Cancel search"
				} else if reader.Selecting {
					message = "↑↓←→ Select text · y Copy · Esc Clear selection · hh Help"
				}
			}
		}
	}
	if m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil && !m.commandFocused() {
		if !m.mainViewLocalLayer() {
			controls := strings.Split(message, " · ")
			if m.viewExpanded() {
				hasBack := false
				for i, control := range controls {
					if strings.HasPrefix(control, "Esc ") {
						controls[i], hasBack = "Esc Restore layout", true
					}
				}
				if !hasBack {
					controls = append(controls, "Esc Restore layout")
				}
			}
			// Text components show fullscreen in their action bars. Other main views
			// need a footer hint, without repeating the expanded Escape action.
			if source, _, _, _ := m.expandableView(); source == nil && !m.viewExpanded() && !strings.Contains(message, "ff Fullscreen") {
				index := min(2, len(controls))
				controls = append(controls[:index], append([]string{":f " + m.expandAction().Label}, controls[index:]...)...)
			}
			message = strings.Join(controls, " · ")
		}
	}
	if m.chatSourcesVisible() && !m.helpVisible() {
		label, message = "Sources", m.chatSourcesHint()
		if m.chatSources.ShowReply {
			label = "Read reply"
		}
	}
	if m.commandFocused() {
		label, message = "Command", "Enter Run · Esc Cancel"
	}
	if m.selectedInput() != nil || m.screenSelectionActive() {
		controls := strings.Split(message, " · ")
		hasEscape := false
		for i, control := range controls {
			if strings.HasPrefix(control, "Esc ") {
				controls[i] = "Esc Clear selection"
				hasEscape = true
			}
		}
		if !hasEscape && m.screenSelectionActive() {
			controls = append(controls, "Esc Clear selection")
		}
		message = strings.Join(controls, " · ")
	}
	selected := m.screenSelectionActive() || m.clipboardSurface().hasSelection()
	if selected {
		message = "[ Copy selection ] · " + message
	}
	if m.shortcutPrefix != "" {
		message = m.hint()
	} else if !selected && m.currentRoot && m.currentNotice != "" && m.status == "" && !m.nativeAgentCaptured() && !(m.resourceTerminalVisible() && m.explore.Terminal.Captured) {
		return renderFooter(label, ui.OneLine(m.currentNotice), m.width, true)
	} else if m.status != "" && (!m.nativeAgentCaptured() || selected) && !(m.resourceTerminalVisible() && m.explore.Terminal.Captured) {
		return renderFooter(label, ui.OneLine(m.status), m.width, true)
	}
	return renderFooter(label, message, m.width, false)
}

func renderFooter(label, message string, width int, notice bool) string {
	prefix := ui.Selected.Render(" "+ui.Clipped(ui.OneLine(label), min(20, max(0, width/4)))) + "  "
	available := max(0, width-ansi.StringWidth(prefix))
	if notice {
		return lipgloss.NewStyle().Width(width).Render(prefix + ui.Muted.Render(ui.Clipped(message, available)))
	}
	var controls []string
	back, help := "", ""
	for _, control := range strings.Split(message, " · ") {
		switch {
		case strings.HasPrefix(control, "Esc ") || control == "bb Show connections":
			back = control
		case control == "hh Help" || control == ":h help" || control == ":h Help":
			help = control
		default:
			controls = append(controls, control)
		}
	}
	var tail []string
	if back != "" {
		tail = append(tail, back)
	}
	if help != "" {
		tail = append(tail, help)
	}
	var shown []string
	limit := 5
	if label == "Overview" {
		limit = 9
	}
	for _, control := range controls {
		candidate := append(append(append([]string{}, shown...), control), tail...)
		if len(shown)+len(tail) < limit && ansi.StringWidth(strings.Join(candidate, " · ")) <= available {
			shown = append(shown, control)
		}
	}
	shown = append(shown, tail...)
	for i, control := range shown {
		if control == "[ Copy selection ]" {
			shown[i] = textview.ActionChip(textview.Action{Label: "Copy selection", Mouse: "clipboard/copy"}, available, ui.WorkspaceTheme())
			continue
		}
		key, action, _ := strings.Cut(control, " ")
		shown[i] = ui.Selected.Render(key) + ui.Muted.Render(" "+action)
		if ui.MouseFooterKey(key) {
			shown[i] = ui.MouseAction(shown[i], key)
		}
	}
	return lipgloss.NewStyle().Width(width).Render(prefix + ui.Clipped(strings.Join(shown, ui.Muted.Render(" · ")), available))
}

var loadingFrames = []string{
	"●····", "·●···", "··●··", "···●·", "····●",
	"····●", "···●·", "··●··", "·●···", "●····",
}

type loadingTickMsg struct{}

func (m *model) loading(label string) string {
	frame := loadingFrames[m.loadingFrame%len(loadingFrames)]
	return frame + " " + ui.OneLine(label)
}

func (m *model) loadingVisible() bool {
	if m.width < minWidth || m.height < minHeight || m.exploreClosed || m.helpVisible() || m.settingsOverlay != nil && m.focus == 1 || m.resourceTerminalVisible() {
		return false
	}
	if m.resourceActionVisible() {
		return m.explore.Action.Busy
	}
	if m.chat().Restore != nil && !m.rightHidden && !m.chat().History && !m.chat().Models {
		return true
	}
	if m.run != nil && m.run.Chat == m.chat() && !m.rightHidden && !m.chat().History && !m.chat().Models {
		return true
	}
	if m.mode == 2 && m.page == "" && (m.activityUI.Pending || m.activityUI.Load != nil) {
		return true
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		return !v.Cluster.Loaded || v.State.Loading || v.Namespaces != nil && !v.Namespaces.Loaded
	}
	if m.helmVisible() {
		h := m.helmer()
		return h.Busy || h.Namespaces != nil && !h.Namespaces.Manual && !h.Namespaces.Loaded
	}
	if !m.exploreVisible() || m.explore == nil {
		return false
	}
	e := m.explore
	if e.Picker == "namespace" && e.Namespaces != nil && !e.Namespaces.Manual && !e.Namespaces.Loaded {
		return true
	}
	if e.Connecting {
		return true
	}
	if e.Picker != "" {
		return false
	}
	if e.Detail != nil {
		return e.DetailRun != nil || (e.Tab == 4 && e.Log != nil && !e.LogConnected && e.LogError == "")
	}
	return e.Stream != nil && !e.Listed && e.Err == ""
}

func (m *model) syncLoading() tea.Cmd {
	if m.loadingTickPending || !m.loadingVisible() {
		return nil
	}
	m.loadingTickPending = true
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return loadingTickMsg{} })
}

// A1's open course and departure point, adapted to terminal cells.
const plousMark = `    ▄▄▄▄▄▄
  ███▀▀▀▀██▄
 ██▀      ▀██
 ██        ██
 ██      ▄██▀
 ██  ██████▀
 ▄▄
 ▀▀`

const plousMarkCompact = `▄█▀▀█▄
██ ▄█▀
▄▄`

func (m *model) welcomeBody(width, height int) string {
	message := "Choose a connection to start exploring."
	if m.current() == nil {
		message = "Create a workspace to start exploring."
	}
	// Commands cover the bottom rows without recentering the artwork.
	welcomeHeight := height
	if m.command != nil {
		welcomeHeight += 3
	}
	return ui.StyledTextView(exploreWelcome(width, welcomeHeight, message), width, height, 0)
}

func exploreWelcome(width, height int, message string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	message = ansi.Wrap(message, width, "")
	content := ui.TitleStyle.Render("plous") + "\n\n" + ui.Muted.Width(width).Align(lipgloss.Center).Render(message)
	mark := plousMark
	if width < lipgloss.Width(mark) || height < lipgloss.Height(content)+lipgloss.Height(mark)+1 {
		mark = plousMarkCompact
	}
	if width >= lipgloss.Width(mark) && height >= lipgloss.Height(content)+lipgloss.Height(mark)+1 {
		content = ui.BrandStyle.Width(lipgloss.Width(mark)).Render(mark) + "\n\n" + content
	}
	content = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

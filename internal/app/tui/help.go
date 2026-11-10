package tui

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	appactivity "github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"github.com/simjay/plous/internal/ui"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
)

type helpState struct {
	previousFocus, section int
	scroll                 [3]int
	sections               [3]helpview.Section
}

func (m *model) openHelp() {
	if m.help != nil {
		if !m.helpVisible() {
			m.help.previousFocus = m.focus
			m.helpTravel = overlayTravel{}
		}
		return
	}
	m.helpTravel = overlayTravel{}
	sections := m.helpSections()
	m.help = &helpState{previousFocus: m.focus, section: m.focus, sections: sections}
	if m.chatSourcesVisible() {
		m.help.section = 2
	} else if m.settingsOverlay != nil || m.form != nil {
		m.help.section = 1
	}
}

func (m *model) closeHelp() {
	if m.help != nil {
		if m.helpVisible() {
			m.focus = m.help.previousFocus
		}
		m.help = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) helpDimensions() (width, height int) {
	margin := 2
	if m.height < 22 {
		margin = 1
	}
	return min(164, m.width-4), min(44, m.height-margin*2)
}

func (m *model) helpSizes() (widths [3]int, topHeight, bottomHeight int) {
	width, height := m.helpDimensions()
	widths[0] = max(18, width/4)
	widths[2] = max(24, width/3)
	widths[1] = width - widths[0] - widths[2]
	bottomHeight = 8
	topHeight = height - bottomHeight
	return
}

func (m *model) helpBodyHeight(panel, topHeight int) int {
	height := topHeight - 3
	if panel == 1 && m.command != nil {
		height -= 3
	}
	return max(1, height)
}

func (m *model) updateHelp(key string) {
	if key == "esc" {
		m.closeHelp()
		m.status = ""
		return
	}
	if key == "left" {
		m.help.section = (m.help.section + 2) % 3
	} else if key == "right" {
		m.help.section = (m.help.section + 1) % 3
	}
	section := m.help.section
	widths, height, _ := m.helpSizes()
	available := m.helpBodyHeight(section, height)
	lines := helpLines(m.panelHelpSection(section), widths[section]-4)
	last := max(0, len(lines)-available)
	offset := min(m.help.scroll[section], last)
	switch key {
	case "up":
		offset--
	case "down":
		offset++
	case "pgup":
		offset -= available
	case "pgdown":
		offset += available
	case "home":
		offset = 0
	case "end":
		offset = last
	}
	m.help.scroll[section] = max(0, min(last, offset))
}

func helpBox(title, content string, width, height int, focused bool) string {
	color := ui.BorderColor
	if focused {
		color = ui.FocusedBorder
	}
	headingStyle := ui.OverlayTitleStyle.Background(ui.Surface)
	if focused {
		headingStyle = headingStyle.Background(ui.Highlight).Width(width - 4)
	}
	heading := headingStyle.Render(ui.Clipped(title, width-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Foreground(ui.Foreground).Background(ui.Surface).
		Padding(0, 1).Width(width).Height(height).MaxWidth(width).MaxHeight(height).
		Render(heading + "\n" + content)
}

func (m *model) helpPanels() string {
	widths, topHeight, bottomHeight := m.helpSizes()
	panels := make([]string, 3)
	for i, width := range widths {
		section := m.panelHelpSection(i)
		height := m.helpBodyHeight(i, topHeight)
		lines := helpLines(section, width-4)
		offset := min(m.help.scroll[i], max(0, len(lines)-height))
		title := section.Title
		if len(lines) > height {
			label := fmt.Sprintf("%d/%d", offset+1, len(lines))
			title = ui.Clipped(title, max(1, width-5-len(label))) + " " + label
		}
		end := min(len(lines), offset+height)
		body := strings.Join(lines[offset:end], "\n")
		if i == 1 && m.command != nil {
			body = lipgloss.NewStyle().Width(width-4).Height(height).Render(body) + "\n" + m.commandBox(width-4)
		}
		panels[i] = textview.MouseScope(ui.MouseTarget(helpBox(title, body, width, topHeight, m.help.section == i), "help/"+strconv.Itoa(i)), "help")
	}
	width, _ := m.helpDimensions()
	global := textview.MouseScope(helpBox("Global · Navigation", m.globalHelpBody(), width, bottomHeight, false), "help")
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, panels...), global)
}

func (m *model) globalHelpBody() string {
	key := func(k, label string) string { return ui.ShortcutStyle.Render(k) + " " + label }
	return strings.Join([]string{
		key(":1 / 11", "Overview") + "   " + key(":2 / 22", "Explore") + "   " + key(":3 / 33", "Activity"),
		key(navigationBinding("sidebar"), "Sidebar") + "   " + key(navigationBinding("aiinteraction"), "AI panel") + "   " + key(navigationBinding("fullscreen"), "Fullscreen*"),
		key(navigationBinding("settings"), "Settings") + "   " + key(navigationBinding("help"), "Help") + "   " + key(":quit", "Exit"),
		key("Alt+← / Alt+→", "Panels") + "   " + key("Ctrl+]", "Agent / return") + "   " + key("Ctrl+C", "Save and exit"),
		key(":", "Command, then Enter") + "   " + key("Tab", "Tree / main") + "   " + ui.Muted.Render("* Outside overlays"),
	}, "\n")
}

func (m *model) panelHelpSection(panel int) helpview.Section {
	if m.help != nil && m.help.sections[panel].Title != "" {
		return m.help.sections[panel]
	}
	return m.helpSections()[panel]
}

func (m *model) renderHelpOverlay() string {
	base := *m
	base.help, base.command = nil, nil
	base.focus = m.help.previousFocus
	base.ensureVisibleFocus()
	lines := strings.Split(base.render(), "\n")
	lines[len(lines)-1] = m.footer()
	return ui.CenteredOverlay(strings.Join(lines, "\n"), m.helpPanels())
}

func (m *model) helpSections() [3]helpview.Section {
	sections := [3]helpview.Section{m.workspaceHelp(), m.mainHelp(), m.chatHelp()}
	for i := range sections {
		sections[i].Note("Drag text in tables, labels, Help, chat, and native agent output, then click Copy selection in the footer. Viewers also use y. Paste into active text input with the terminal Paste command. Ctrl+C exits outside active native agent input.")
		m.resolveHelpCommands(&sections[i])
	}
	return sections
}

func (m *model) resolveHelpCommands(s *helpview.Section) {
	commands := map[string]commandDefinition{}
	for _, c := range applicationCommands {
		commands[c.name] = c
	}
	for _, c := range monitor.ResourceActionDefinitions {
		commands[c.Command] = commandDefinition{key: c.Alias, name: c.Command}
	}
	for gi := range s.Groups {
		for ei := range s.Groups[gi].Entries {
			e := &s.Groups[gi].Entries[ei]
			if c, ok := commands[e.Command]; ok {
				for _, binding := range []string{":" + c.key, ":" + c.name} {
					if binding != e.Key {
						e.Alternatives = append(e.Alternatives, binding)
					}
				}
			}
		}
	}
}

func (m *model) workspaceHelp() helpview.Section {
	if search := m.searches[0]; search != nil && !search.finished {
		s := helpview.Search("Workspaces · Search", "Open selected result")
		s.Add("Results", helpview.Key("select", "↑↓", "Select result"), helpview.Key("page", "PgUp / PgDn", "Jump ten rows"))
		return s
	}
	s := helpview.New("Workspaces", helpview.Key("select", "↑↓", "Select row"), helpview.Key("open", "Enter", "Select / open"), helpview.Key("filter", "/", "Search tree"))
	rows := m.treeRowsRead()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		r := rows[m.treeCursor]
		s.Title = "Workspaces · " + r.label
		if r.isBranch() {
			s.Add("Branch", helpview.Key("fold", "←→", "Fold / expand"))
		} else if r.kind == "current" || r.kind == "capability" || r.kind == "gitopssection" || r.kind == "helmsection" || r.kind == "discovery" || r.kind == "connection" && monitorCluster(r.tool) {
			s.Add("Connection", helpview.Command("refresh", "g", "Refresh tool discovery"))
		}
		if r.isAddAction() || r.kind == "savecurrent" || r.kind == "choosecurrent" || r.kind == "settings" {
			s.Groups[0].Entries[1].Label = r.label
		}
	}
	s.Add("Manage", helpview.Command("new", ":n", "New workspace"), helpview.Command("connections", ":K", "Choose connection"))
	s.Add("Navigation", helpview.Key("ends", "Home / End", "First / last"), helpview.Key("page", "PgUp / PgDn", "Jump ten rows"))
	if m.searches[0] != nil {
		s.Add("Search", helpview.Key("back", "Esc", "Clear tree search"))
	}
	// Global alternatives remain reachable without filling the fixed global box.
	for _, e := range globalHelpSection().Groups[0].Entries {
		s.Add("Global commands", e)
	}
	return s
}

func (m *model) mainHelp() helpview.Section {
	if m.presentedDocumentVisible() && m.form == nil && m.settingsOverlay == nil {
		return m.uiDocument.model.Help("Assistant document", m.uiDocument.model.Target, "Back")
	}
	if m.form != nil {
		return m.form.Help()
	}
	if m.settingsOverlay != nil {
		switch m.page {
		case "tools":
			return helpview.Picker("Add connection", "Open tool setup")
		case "remove", "removeprovider":
			return helpview.Picker("Remove connection", "Activate selected choice")
		}
		return (*appsettings.Form)(nil).Help()
	}
	if s := m.activePanelSearch(1); s != nil && !s.finished {
		return helpview.Search("Find / filter", "Finish search / open selection")
	}
	if m.mode == 2 && m.page == "" {
		if m.activityReview != nil {
			return m.activityReview.Comparison.Help("Activity comparison (read only)", "", "Return to saved summary")
		}
		state := appactivity.HelpContext{Detail: m.detail != "", LoadError: m.activityUI.Error != ""}
		if m.operationDetail != nil {
			state.OpenRelease = m.activityReleaseUnavailable(m.operationDetail) == ""
			state.Observations = m.activityHasObservations()
		} else {
			state.Compare = m.activityDetailActive()
		}
		return state.Help()
	}
	if m.overviewVisible() {
		if v := m.overviewRead(); v != nil {
			return v.State.Help()
		}
		return (&monitor.OverviewState{}).Help()
	}
	if m.mode != 1 || m.page != "" {
		s := helpview.New("Overview", helpview.Key("back", "Esc", "Back / sidebar"))
		if m.current() == nil || len(m.current().Connections) == 0 {
			s.Add("Workspace", helpview.Key("setup", "Enter", "Add connection"))
		}
		s.Add("Read", helpview.Find()...)
		s.Add("Navigation", helpview.Scroll()...)
		return s
	}
	if m.explore != nil && m.explore.Picker == "connections" {
		return m.explore.Help()
	}
	c := m.selectedConnection()

	if c != nil && monitorCluster(c.Tool) && m.tool == "helm" {
		connection := *c
		connection.Settings = maps.Clone(c.Settings)
		if connection.Settings == nil {
			connection.Settings = map[string]string{}
		}
		connection.Settings["driver"] = m.helmDriver(m.active, *c)
		c = &connection
		key := m.active + "/" + c.ID + "/" + monitor.HelmConfig(c).Fingerprint()
		return m.helmStates[key].Help()
	}
	if c != nil && monitorCluster(c.Tool) && gitOpsTool(m.tool) {
		if c := m.selectedConnection(); c != nil {
			root := m.active + "/" + c.ID + "/" + m.tool
			preference := m.gitOpsPreferences[root]
			section := gitops.NormalizeSection(gitops.Tool(m.tool), preference.Section)
			if preference.Section == "" && strings.Contains(preference.Selected, "/HelmRelease/") && m.tool == string(gitops.Flux) {
				section = gitops.HelmReleases
			}
			for _, v := range m.gitOpsViews {
				if v.Cluster.Key == clusterKey(m.active, *c) && v.State.Tool == gitops.Tool(m.tool) && v.State.Section == section {
					if v.Namespaces != nil {
						return v.Namespaces.Help(v.State.Title() + " · Namespace")
					}
					return v.State.Help()
				}
			}
		}
		return helpview.New("Monitor · "+m.tool, helpview.Command("refresh", "g", "Refresh connection"))
	}
	s := m.explore.Help()
	if m.explore != nil && m.explore.Detail != nil && m.explore.Tab != 1 && m.explore.Tab != 4 && m.explore.Action == nil && m.explore.Terminal == nil {
		s.Note(strings.TrimSpace(m.usageHelp()))
	}
	return s
}

func (m *model) chatHelp() helpview.Section {
	if m.externalAgentSelected() {
		s := helpview.New(m.settings.MainAgent.Label() + " terminal")
		s.Note("The installed CLI owns input, menus and conversation history. The session keeps its captured connection.")
		s.Add("", helpview.Command("chat", "cc / :c", "Enter terminal"))
		s.Add("", helpview.Command("stopchat", ":X", "Close native session"))
		s.Note("Alt+Left/Right move panels directly. Ctrl+] switches between the agent and previous panel. Ctrl+C goes to the active native terminal for its interrupt behavior. To save and exit Plous, press Ctrl+] then Ctrl+C. Use the CLI's /resume for history.")
		s.Note("Wheel or PgUp/PgDn reads terminal output. End or Esc returns to live output while reading; typing or paste also returns live. Up/Down stays with the native prompt while captured. Fullscreen CLI views handle their own scrolling.")
		return s
	}
	if m.chatSourcesVisible() {
		return m.chatSources.Help()
	}
	if s := m.searches[2]; s != nil && !s.finished {
		return helpview.Search("AI chat · Find", "Finish search / open conversation")
	}
	return m.chat().Help(m.chatEditing, m.run != nil && !m.run.Finished, m.settings.MainAgent)
}

func globalHelpSection() helpview.Section {
	s := helpview.New("Global")
	for _, command := range []struct{ name, label string }{{"settings", "Settings"}, {"help", "Help"}, {"sidebar", "Sidebar"}, {"aiinteraction", "AI panel"}, {"fullscreen", "Fullscreen"}} {
		s.Add("", helpview.Command(command.name, navigationBinding(command.name), command.label))
	}
	s.Add("", helpview.Command("quit", ":quit", "Save and exit"))
	return s
}

// Style before wrapping so continuation lines preserve their semantic role.
func helpLines(s helpview.Section, width int) []string {
	width = max(1, width)
	var lines []string
	wrap := func(text string) { lines = append(lines, strings.Split(ansi.Wrap(text, width, ""), "\n")...) }
	entry := func(key, label, reason string) {
		if reason != "" {
			label += " (" + reason + ")"
		}
		keyWidth := ansi.StringWidth(key)
		column := 8
		if width < 28 {
			column = 6
		}
		if keyWidth+2 >= width/2 {
			wrap(ui.ShortcutStyle.Render(key))
			for _, line := range strings.Split(ansi.Wrap(label, max(1, width-2), ""), "\n") {
				lines = append(lines, "  "+line)
			}
		} else {
			column = max(column, keyWidth+2)
			for i, line := range strings.Split(ansi.Wrap(label, max(1, width-column), ""), "\n") {
				prefix := strings.Repeat(" ", column)
				if i == 0 {
					prefix = ui.ShortcutStyle.Render(key) + strings.Repeat(" ", column-keyWidth)
				}
				lines = append(lines, prefix+line)
			}
		}
	}
	for _, g := range s.Groups {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		if g.Title != "" {
			wrap(ui.SectionHeadingStyle.Render(g.Title))
		}
		for _, e := range g.Entries {
			entry(e.Key, e.Label, e.Reason)
		}
		if g.Note != "" {
			wrap(ui.Muted.Render(g.Note))
		}
	}
	var alternatives []helpview.Entry
	for _, g := range s.Groups {
		for _, e := range g.Entries {
			if len(e.Alternatives) > 0 {
				alternatives = append(alternatives, e)
			}
		}
	}
	if len(alternatives) > 0 {
		lines = append(lines, "")
		wrap(ui.SectionHeadingStyle.Render("Alternative keys"))
		for _, e := range alternatives {
			entry(strings.Join(e.Alternatives, " / "), e.Label, "")
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

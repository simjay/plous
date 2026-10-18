package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/simjay/plous/internal/ui"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
	"strings"
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

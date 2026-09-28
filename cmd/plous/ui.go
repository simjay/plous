package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	minWidth  = 80
	minHeight = 12
)

var (
	muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B949E"))
	accent = lipgloss.Color("#7AA2F7")
)

type model struct {
	width, height int
	focus         int
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focus = (m.focus + 1) % 3
		case "shift+tab":
			m.focus = (m.focus + 2) % 3
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).
			MaxWidth(m.width).MaxHeight(m.height).
			Render("Plous " + version() + "\nResize terminal to at least 80 x 12.\nq / ctrl+c quit")
	}

	left := min(26, max(18, m.width/5))
	right := min(48, max(28, m.width*3/10))
	center := m.width - left - right
	height := m.height - 2

	header := lipgloss.NewStyle().Bold(true).Render(" plous")
	label := muted.Render("v" + version() + " ")
	header += strings.Repeat(" ", m.width-lipgloss.Width(header)-lipgloss.Width(label)) + label

	panels := lipgloss.JoinHorizontal(lipgloss.Top,
		panel("List", muted.Render("No items yet"), left, height, m.focus == 0, false),
		panel("Main", "welcome to plous", center, height, m.focus == 1, true),
		panel("AI assistant", muted.Render("No conversation yet.\n\nAI is not connected."), right, height, m.focus == 2, false),
	)
	footer := muted.Width(m.width).Render(" tab / shift+tab switch panel    q / ctrl+c quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, panels, footer)
}

func panel(title, content string, width, height int, focused, centered bool) string {
	color := lipgloss.Color("#444C56")
	if focused {
		color = accent
	}
	// Border and horizontal padding leave four columns for the frame.
	innerWidth, bodyHeight := width-4, height-4
	body := lipgloss.NewStyle().Width(innerWidth).Height(bodyHeight).
		MaxWidth(innerWidth).MaxHeight(bodyHeight)
	if centered {
		body = body.Align(lipgloss.Center, lipgloss.Center)
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(color).Render(title)
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(color).
		Padding(0, 1).Width(width - 2).Height(height - 2).
		MaxWidth(width).MaxHeight(height).
		Render(heading + "\n\n" + body.Render(content))
}

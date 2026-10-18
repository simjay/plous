package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
)

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

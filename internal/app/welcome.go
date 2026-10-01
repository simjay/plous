package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The three blocks follow assets/logo/mark-dark.svg in terminal cells.
const plousMark = `▄▄▄▄▄▄▄▄▄▄▄▄▄▄
██████████████
▄▄▄▄▄▄  ▄▄▄▄▄▄
██████  ██████
██████  ██████
██████  ██████
▀▀▀▀▀▀  ▀▀▀▀▀▀`

const plousMarkCompact = `▀▀▀▀▀▀▀▀▀▀
███    ███
███    ███`

func exploreWelcome(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	message := ansi.Wrap("Choose a connection to start exploring.", width, "")
	content := titleStyle.Render("plous") + "\n\n" + muted.Width(width).Align(lipgloss.Center).Render(message)
	mark := plousMark
	if width < lipgloss.Width(mark) || height < lipgloss.Height(content)+lipgloss.Height(mark)+1 {
		mark = plousMarkCompact
	}
	if width >= lipgloss.Width(mark) && height >= lipgloss.Height(content)+lipgloss.Height(mark)+1 {
		lines := strings.Split(mark, "\n")
		for i, line := range lines {
			lines[i] = gradientText(line, lipgloss.Color("#4F8DF7"), lipgloss.Color("#9066F0"))
		}
		content = strings.Join(lines, "\n") + "\n\n" + content
	}
	content = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

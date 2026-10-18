package ui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

const SearchBarHeight = 3

func SearchContains(value, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(query)))
}

func SearchField(input textview.Field, width int, active bool, label, placeholder, hint string) string {
	innerWidth := max(1, width-4)
	text := EditorLine(input, innerWidth, active)
	if input.Text == "" {
		cursor := ""
		if active {
			cursor = "▏"
		}
		text = cursor + Muted.Render(Clipped(placeholder, max(0, innerWidth-ansi.StringWidth(cursor))))
	}
	if width < 4 {
		return Clipped(text, width)
	}
	border := BorderColor
	if active {
		border = FocusedBorder
	}
	frame := lipgloss.NewStyle().Foreground(border)
	title := Clipped(" / "+label+" ", width-3)
	top := frame.Render("╭─") + Selected.Render(title) + frame.Render(strings.Repeat("─", max(0, width-3-ansi.StringWidth(title)))+"╮")
	middle := frame.Render("│") + " " + text + strings.Repeat(" ", max(0, innerWidth-ansi.StringWidth(text))) + " " + frame.Render("│")
	bottom := frame.Render("╰" + strings.Repeat("─", width-2) + "╯")
	if hint != "" {
		label := Clipped(" "+hint+" ", width-3)
		bottom = frame.Render("╰"+strings.Repeat("─", max(0, width-3-ansi.StringWidth(label)))) + Muted.Render(label) + frame.Render("─╯")
	}
	return MouseTarget(strings.Join([]string{top, middle, bottom}, "\n"), "search")
}

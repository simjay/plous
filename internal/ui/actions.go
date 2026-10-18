package ui

import (
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

func ActionButton(key, label string, unavailable bool) string {
	theme := WorkspaceTheme()
	width := ansi.StringWidth(key+label) + 3
	action := textview.Action{Key: key, Label: label}
	if unavailable {
		theme.Keycap, theme.Text = Muted, Muted
		action.Mouse = "disabled"
	}
	return textview.ActionChip(action, width, theme)
}

func ActionButtonRows(buttons []string, width int) []string {
	var rows []string
	line := ""
	for _, button := range buttons {
		if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(button) > width {
			rows = append(rows, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += Clipped(button, width)
	}
	if line != "" {
		rows = append(rows, line)
	}
	return rows
}

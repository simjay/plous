package text

import (
	lipgloss "charm.land/lipgloss/v2"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type Action struct {
	Mouse                      string
	Key, Label, Value          string
	Focused, Primary, Disabled bool
}

type ActionGroup struct {
	Label   string
	Actions []Action
}

func ActionChip(action Action, width int, theme Theme) string {
	if width <= 0 {
		return ""
	}
	body, key, edge := theme.Control, theme.Keycap, theme.Muted
	if action.Primary {
		edge = theme.Accent
	}
	marker := ""
	if action.Focused {
		edge = theme.Accent
		marker = edge.Render("› ")
		key = key.Background(theme.ControlFocus.GetBackground())
	}
	if action.Disabled {
		body = body.Foreground(theme.Muted.GetForeground())
		key = body
	}
	label := action.Label
	value := action.Value
	if action.Disabled {
		value = "Unavailable"
	}
	prefix, suffix := edge.Render("[ "), edge.Render(" ]")
	if action.Key != "" {
		prefix, suffix = key.Render("["+action.Key+"]")+" ", ""
	}
	prefix = marker + prefix
	valueText := ""
	if value != "" {
		style := theme.Muted
		if value == "On" && !action.Disabled {
			style = theme.Added.Bold(true)
		}
		valueText = " " + style.Render(value)
	}
	room := width - ansi.StringWidth(prefix+valueText+suffix)
	if room < 1 {
		return mouseActionChip(ansi.Truncate(prefix+label+valueText+suffix, width, "…"), action)
	}
	labelStyle := theme.Text.UnsetBackground().Bold(false)
	if action.Disabled {
		labelStyle = labelStyle.Foreground(theme.Muted.GetForeground())
	}
	content := prefix + labelStyle.Render(ansi.Truncate(label, room, "…")) + valueText + suffix
	if action.Key == "" {
		return mouseActionChip(body.Render(content), action)
	}
	return mouseActionChip(content, action)
}

func mouseActionChip(content string, action Action) string {
	if action.Disabled {
		return Mouse(content, "disabled")
	}
	if action.Mouse != "" {
		return Mouse(content, action.Mouse)
	}
	if action.Key == "" {
		return Mouse(content, "disabled")
	}
	return Mouse(content, "action/"+action.Key)
}

// Keep the focused action visible, or the first primary action while reading.
func ActionBar(groups []ActionGroup, width, maxRows int, theme Theme) string {
	if width <= 0 || maxRows <= 0 {
		return ""
	}
	labelWidth := 0
	if width >= 48 {
		for _, group := range groups {
			labelWidth = max(labelWidth, ansi.StringWidth(group.Label))
		}
		labelWidth = min(labelWidth, 10)
	}
	var rows []string
	focusedRow := -1
	primaryRow := -1
	for _, group := range groups {
		if len(group.Actions) == 0 {
			continue
		}
		prefix := ""
		if labelWidth > 0 {
			prefix = theme.Muted.Render(lipgloss.NewStyle().Width(labelWidth).Render(ansi.Truncate(group.Label, labelWidth, "…"))) + "  "
		}
		indent := strings.Repeat(" ", ansi.StringWidth(prefix))
		line, used := prefix, false
		for _, action := range group.Actions {
			chip := ActionChip(action, width-ansi.StringWidth(prefix), theme)
			if used && ansi.StringWidth(line)+2+ansi.StringWidth(chip) > width {
				rows = append(rows, line)
				line, used = indent, false
			}
			if used {
				line += "  "
			}
			line += chip
			used = true
			if action.Focused {
				focusedRow = len(rows)
			}
			if action.Primary && primaryRow == -1 {
				primaryRow = len(rows)
			}
		}
		rows = append(rows, line)
	}
	if len(rows) <= maxRows {
		return strings.Join(rows, "\n")
	}
	visible := max(1, maxRows-1)
	if focusedRow == -1 {
		focusedRow = primaryRow
	}
	start := max(0, focusedRow-visible+1)
	shown := append([]string(nil), rows[start:start+visible]...)
	if maxRows > 1 {
		shown = append(shown, theme.Muted.Render(ansi.Truncate("… More controls in Help", width, "…")))
	}
	return strings.Join(shown, "\n")
}

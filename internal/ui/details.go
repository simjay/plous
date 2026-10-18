package ui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func DetailHeading(title string, width int) string {
	if width <= 0 {
		return ""
	}
	heading := SectionHeadingStyle
	text := ansi.Wrap(OneLine(title), max(1, width-2), "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		prefix := "  "
		if i == 0 {
			prefix = "▎ "
		}
		if width < 3 {
			prefix = ""
		}
		remaining := width - ansi.StringWidth(prefix+line)
		rule := ""
		if i == len(lines)-1 && remaining >= 3 {
			rule = " " + lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", remaining-1))
		}
		lines[i] = heading.Render(prefix+line) + rule
	}
	return strings.Join(lines, "\n")
}

func DetailCallout(title, body, actions string, width int, tone lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	inner := max(1, width-3)
	lines := []string{tone.Bold(true).Render(ansi.Wrap(title, inner, ""))}
	if body != "" {
		lines = append(lines, ansi.Wrap(body, inner, ""))
	}
	if actions != "" {
		lines = append(lines, "", actions)
	}
	content := strings.Join(lines, "\n")
	if width < 8 {
		return StyledView(content, width, 10000)
	}
	return lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(tone.GetForeground()).PaddingLeft(1).PaddingRight(1).
		Width(width).MaxWidth(width).Render(content)
}

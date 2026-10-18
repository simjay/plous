package ui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func Plain(s string) string { return textview.CleanInput(s, true) }

func OneLine(s string) string { return strings.ReplaceAll(Plain(s), "\n", " ") }

func Clipped(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }

func CenteredOverlay(background, window string) string {
	width, height := lipgloss.Size(background)
	windowWidth, windowHeight := lipgloss.Size(window)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(CanvasStyle.Foreground(lipgloss.Color("#56575E")).Render(ansi.Strip(background))),
		lipgloss.NewLayer(textview.MouseScope(window, "main")).X((width-windowWidth)/2).Y((height-windowHeight)/2).Z(1),
	).Render()
}

func Panel(title, content string, width, height int, focused bool) string {
	color := BorderColor
	if focused {
		color = FocusedBorder
	}
	heading := TitleStyle.Render(title)
	rule := lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", width-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(heading + "\n" + rule + "\n" + content)
}

func ListView(items []string, index, width, height int) string {
	return ChoiceList(items, index, width, height, ChoiceLine)
}

func ChoiceList(items []string, index, width, height int, renderRow func(string, bool, int) string) string {
	if height <= 0 {
		return ""
	}
	index = max(0, min(index, len(items)-1))
	start := max(0, index-height+1)
	end := min(len(items), start+height)
	lines := []string{}
	for i := start; i < end; i++ {
		lines = append(lines, MouseChoice(renderRow(items[i], i == index, width), i, items[i]))
	}
	return strings.Join(lines, "\n")
}

func TextView(text string, width, height, offset int) string {
	return StyledTextView(Plain(text), width, height, offset)
}

// Callers sanitize user values before adding terminal styles.
func StyledTextView(text string, width, height, offset int) string {
	return MeasureViewport(text, width, height).View(offset)
}

func EmptyState(title, description, action string, height int) string {
	text := Heading(title) + "\n\n"
	if height >= 9 {
		text += description + "\n\n"
	}
	lines := strings.Split(action, "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		lines[i] = Selected.Render(line)
		if len(fields) > 0 && MouseFooterKey(fields[0]) {
			lines[i] = MouseAction(lines[i], fields[0])
		}
	}
	return text + strings.Join(lines, "\n")
}

// Body components already sanitize user text before applying terminal styles.
func StyledView(text string, width, height int) string {
	return MeasureViewport(text, width, height).View(0)
}

func EditorLine(e textview.Field, width int, active bool, owners ...string) string {
	if !active && e.Text == "" {
		return ""
	}
	owner := "input"
	if len(owners) > 0 {
		owner = owners[0]
	}
	text := e.MouseDisplay(active, owner, WorkspaceTheme())
	if active {
		prefix := string([]rune(e.Text)[:min(e.Cursor, len([]rune(e.Text)))]) + "▏"
		skip := max(0, ansi.StringWidth(prefix)-max(1, width)+1)
		text = ansi.TruncateLeft(text, skip, "")
	}
	return ansi.Truncate(text, max(0, width), "")
}

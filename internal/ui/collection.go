package ui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func ScopeHeader(context, namespace string, selectable bool, width int) string {
	if namespace == "" || namespace == "*" {
		namespace = "all namespaces"
	}
	contextLabel := Muted.Render("ctx ") + TitleStyle.Render(OneLine(context))
	scope := MouseAction(Selected.Render("[ "+OneLine(namespace)+" ▾ ]"), "n")
	hint := Muted.Render(" n")
	if !selectable {
		scope, hint = Muted.Render(OneLine(namespace)), ""
	}
	header := contextLabel + "  " + scope + hint
	if ansi.StringWidth(header) > width {
		return Clipped(contextLabel, width) + "\n" + Clipped(scope, max(1, width-ansi.StringWidth(hint))) + hint
	}
	return header
}

func CollectionTableRow(columns, cells []string, indices, widths []int, width int, heading, active bool, cellStyle func(string, string) lipgloss.Style) string {
	parts := make([]string, len(indices))
	for i, index := range indices {
		value := ""
		if index < len(cells) {
			value = cells[index]
		}
		text := Clipped(OneLine(value), widths[i])
		if heading && ansi.StringWidth(value) > widths[i] && (strings.HasSuffix(value, " ↑") || strings.HasSuffix(value, " ↓")) {
			mark := " " + string([]rune(value)[len([]rune(value))-1])
			text = Clipped(strings.TrimSuffix(OneLine(value), mark), max(1, widths[i]-2)) + mark
		}
		style := cellStyle(columns[index], value)
		if heading {
			style = Muted
		} else if active && index == 0 {
			style = TitleStyle
		}
		if active {
			style = style.Background(Highlight)
		}
		parts[i] = style.Render(text + strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(text))))
		if heading {
			parts[i] = MouseTarget(parts[i], "sort/"+columns[index])
		}
	}
	separator := "  "
	if active {
		separator = lipgloss.NewStyle().Background(Highlight).Render(separator)
	}
	text := Clipped(strings.Join(parts, separator), width)
	if active {
		text = lipgloss.NewStyle().Foreground(Foreground).Background(Highlight).Width(width).Render(text)
	}
	return text
}

func PickerOverlay(base, body string, width, height int) string {
	base = lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(base)
	pickerWidth, pickerHeight := PickerSize(width, height)
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Accent).Background(Surface).Padding(0, 1).Width(pickerWidth).Height(pickerHeight).MaxWidth(pickerWidth).MaxHeight(pickerHeight).Render(body)
	return CenteredOverlay(base, window)
}

func PickerSize(width, height int) (int, int) {
	return max(12, min(width, 70)), max(3, min(height, 18))
}

// Variable-height rows need only enough neighbors to fill the viewport on each
// side of the selection. The renderer receives the original index for identity.
func CollectionWindow(count, cursor, height int, render func(int) []string) string {
	if count == 0 || height <= 0 {
		return ""
	}
	cursor = max(0, min(cursor, count-1))
	current := render(cursor)
	var before [][]string
	size := 0
	for i := cursor - 1; i >= 0 && size < height; i-- {
		row := render(i)
		before = append(before, row)
		size += len(row)
	}
	lines := make([]string, 0, size+len(current)+height)
	for i := len(before) - 1; i >= 0; i-- {
		lines = append(lines, before[i]...)
	}
	selected := len(lines)
	lines = append(lines, current...)
	size = 0
	for i := cursor + 1; i < count && size < height; i++ {
		row := render(i)
		lines = append(lines, row...)
		size += len(row)
	}
	return ScrollSelection(lines, selected, height)
}

func ChoiceLine(label string, active bool, width int) string {
	prefix := "  "
	style := lipgloss.NewStyle().Width(width)
	if active {
		prefix = "› "
		style = Selected.Background(Highlight).Width(width)
	}
	return style.Render(Clipped(prefix+OneLine(label), width))
}

func SelectionWindow(count, selectedRow, height int) (int, int) {
	if height <= 0 {
		return 0, 0
	}
	start := max(0, selectedRow-height+min(3, height))
	start = min(start, max(0, count-height))
	return start, min(count, start+height)
}

func ScrollSelection(lines []string, selectedRow, height int) string {
	start, end := SelectionWindow(len(lines), selectedRow, height)
	return strings.Join(lines[start:end], "\n")
}

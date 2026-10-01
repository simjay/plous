package editor

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/textview"
)

// Lexical coloring continues to work while the document is temporarily invalid.
func YAMLLine(line string, theme textview.Theme) string {
	r := []rune(line)
	spans := []textview.Span{}
	for i := 0; i < len(r); i++ {
		start := i
		if r[i] == '#' && (i == 0 || unicode.IsSpace(r[i-1])) {
			spans = append(spans, textview.Span{Start: i, End: len(r), Style: theme.Muted})
			break
		}
		if r[i] == '\'' || r[i] == '"' {
			quote := r[i]
			i++
			for i < len(r) {
				if r[i] == quote {
					break
				}
				if r[i] == '\\' && quote == '"' {
					i++
				}
				i++
			}
			spans = append(spans, textview.Span{Start: start, End: min(i+1, len(r)), Style: theme.Added})
			continue
		}
		if r[i] == ':' && (i+1 == len(r) || unicode.IsSpace(r[i+1])) {
			key := 0
			for key < i && (unicode.IsSpace(r[key]) || r[key] == '-') {
				key++
			}
			spans = append(spans, textview.Span{Start: key, End: i, Style: theme.Accent})
		}
		if unicode.IsDigit(r[i]) && (i == 0 || unicode.IsSpace(r[i-1]) || strings.ContainsRune("[:,", r[i-1])) {
			for i+1 < len(r) && (unicode.IsDigit(r[i+1]) || r[i+1] == '.') {
				i++
			}
			spans = append(spans, textview.Span{Start: start, End: i + 1, Style: theme.Added})
		}
	}
	return textview.Paint(line, "", spans)
}

func (m *Model) View(width, height int, theme textview.Theme) string {
	return m.view(width, height, theme, true)
}

// ViewEmbedded leaves document identity to the containing resource view.
func (m *Model) ViewEmbedded(width, height int, theme textview.Theme) string {
	return m.view(width, height, theme, false)
}

func (m *Model) view(width, height int, theme textview.Theme, identity bool) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	m.clamp()
	r := []rune(m.text)
	row := strings.Count(string(r[:m.Cursor]), "\n")
	start, _ := m.bounds()
	column := textview.Cells(string(r[start:m.Cursor]))
	state := string(m.Mode)
	if m.Controls {
		state = "Plous controls active"
	}
	if m.ReadOnly {
		state = "Read only"
		if m.Mode == Select {
			state += " · Select"
		}
	} else if m.Dirty() {
		state += " · Unsaved changes"
	} else if m.SavedStatus != "" {
		state += " · " + m.SavedStatus
	} else {
		state += " · Unchanged"
	}
	header := ""
	if identity {
		header = theme.Accent.Render(ansi.Truncate("Plous Editor · "+m.Name, width, "…")) + "\n"
		if height >= 8 {
			for _, line := range strings.SplitN(m.Target, "\n", 3)[:min(2, strings.Count(m.Target, "\n")+1)] {
				header += theme.Muted.Render(ansi.Truncate(line, width, "…")) + "\n"
			}
		}
	}
	stateStyle := theme.Accent
	if !m.ReadOnly && !m.Dirty() && m.SavedStatus != "" {
		stateStyle = theme.Added
	}
	header += stateStyle.Render(ansi.Truncate(fmt.Sprintf("%s · %d:%d", state, row+1, column+1), width, "…"))
	if m.Diagnostic != "" {
		lines := strings.Split(ansi.Wrap(m.Diagnostic, width, ""), "\n")
		budget := min(len(lines), max(0, min(3, height-lipgloss.Height(header)-4)))
		if budget > 0 {
			header += "\n" + theme.Removed.Render(strings.Join(lines[:budget], "\n"))
		}
	}
	if len(m.Actions) > 0 && height >= 6 {
		budget := max(1, height-lipgloss.Height(header)-3)
		if m.Searching || m.Query != "" {
			budget = max(1, budget-4)
		}
		header += "\n" + textview.ActionBar(m.Actions, width, min(4, budget), theme)
	}
	if m.Notice != "" && (m.Query == "" || m.Notice != m.SearchStatus()) {
		header += "\n" + theme.Removed.Render(ansi.Truncate(m.Notice, width, "…"))
	}
	if m.Searching || m.Query != "" {
		header += "\n" + textview.SearchBar(m.Query, m.Searching, m.SearchStatus(), width, theme)
	}
	header += "\n" + theme.Muted.Render(strings.Repeat("─", width))
	available := max(0, height-lipgloss.Height(header))
	m.height = available
	if available == 0 {
		return strings.Join(strings.Split(header, "\n")[:min(height, lipgloss.Height(header))], "\n")
	}
	lines := strings.Split(m.text, "\n")
	if row < m.top {
		m.top = row
	}
	if row >= m.top+available {
		m.top = row - available + 1
	}
	m.top = max(0, min(m.top, max(0, len(lines)-available)))
	gutter := min(width-1, max(4, len(fmt.Sprint(len(lines)))+2))
	if !m.LineNumbers {
		gutter = 0
	}
	bodyWidth := max(1, width-gutter)
	if column < m.left {
		m.left = column
	}
	if column >= m.left+bodyWidth {
		m.left = column - bodyWidth + 1
	}
	selectStart, selectEnd := 0, 0
	if m.Mode == Select {
		selectStart, selectEnd = m.selection()
	}
	var pattern *regexp.Regexp
	if m.Query != "" {
		pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(m.Query))
	}
	bracket := m.matchingBracket()
	var out []string
	offset := 0
	for i, line := range lines {
		length := utf8.RuneCountInString(line)
		if i >= m.top && i < m.top+available {
			spans := []textview.Span{}
			if pattern != nil {
				for _, hit := range pattern.FindAllStringIndex(line, -1) {
					spans = append(spans, textview.Span{Start: utf8.RuneCountInString(line[:hit[0]]), End: utf8.RuneCountInString(line[:hit[1]]), Style: theme.Match})
				}
			}
			if m.Mode == Select && selectEnd > offset && selectStart <= offset+length {
				spans = append(spans, textview.Span{Start: max(0, selectStart-offset), End: min(length, selectEnd-offset), Style: theme.Selection})
			}
			styled := textview.Visible(line)
			if m.Syntax == "yaml" {
				styled = YAMLLine(line, theme)
			}
			if bracket >= offset && bracket < offset+length {
				spans = append(spans, textview.Span{Start: bracket - offset, End: bracket - offset + 1, Style: theme.Match})
			}
			if i == row && !m.Controls {
				if m.Cursor-offset == length {
					line += " "
					styled += " "
				}
				spans = append(spans, textview.Span{Start: m.Cursor - offset, End: textview.Next(line, m.Cursor-offset), Style: theme.Cursor})
			}
			painted := textview.Paint(line, styled, spans)
			prefix := ""
			if gutter > 0 {
				prefix = theme.Muted.Render(ansi.Truncate(fmt.Sprintf("%*d │", gutter-2, i+1), gutter, ""))
			}
			out = append(out, prefix+ansi.Cut(painted, m.left, m.left+bodyWidth))
		}
		offset += length + 1
	}
	return textview.Fit(header+"\n"+strings.Join(out, "\n"), width, height)
}

// Restore the viewport after a temporary layout change only if the caller has
// not moved through or replaced the document in the expanded view.
func (m *Model) RestoreViewport() func() {
	top, left, cursor, revision := m.top, m.left, m.Cursor, m.Revision
	return func() {
		if m.Cursor == cursor && m.Revision == revision {
			m.top, m.left = top, left
		}
	}
}

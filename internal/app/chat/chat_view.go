package chat

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	markdownview "github.com/simjay/plous/internal/ui/markdown"
	"regexp"
	"strings"
)

type ChatRow struct {
	Text         string
	Line, Column int
	Start        int
}

type ChatQuestion struct {
	First, Last int
	Width       int
}

var ChatYouStyle = ui.Muted

var ChatAssistantStyle = lipgloss.NewStyle().Foreground(ui.Accent).Bold(true)

var ChatIndent = regexp.MustCompile(`^(?: *(?:[•*-] |[0-9]+[.)] |│ )| +)`)

func ChatMessage(text string, user bool) string {
	text = ui.Plain(strings.ReplaceAll(text, "\t", "    "))
	if user {
		return text
	}
	return markdownview.Transcript(text, ui.WorkspaceTheme())
}

type ChatRenderedReply struct{ Source, Text string }

func (c *ChatState) RenderedReply(index int) string {
	if len(c.Rendered) > len(c.Turns) {
		c.Rendered = c.Rendered[:len(c.Turns)]
	}
	for len(c.Rendered) < len(c.Turns) {
		c.Rendered = append(c.Rendered, ChatRenderedReply{})
	}
	r := &c.Rendered[index]
	source := c.Turns[index].Response.Text
	if r.Source != source {
		r.Source, r.Text = source, ChatMessage(source, false)
	}
	return r.Text
}

func ChatRows(body string, width int) []ChatRow {
	var rows []ChatRow
	for i, line := range strings.Split(body, "\n") {
		clean := ansi.Strip(line)
		position, column := 0, 0
		for _, row := range strings.Split(ansi.Wrap(line, max(1, width), ""), "\n") {
			visible := ansi.Strip(row)
			// Word wrapping can omit the spaces between visual rows.
			skipped := max(0, strings.Index(clean[position:], visible))
			column += ansi.StringWidth(clean[position : position+skipped])
			position += skipped
			rows = append(rows, ChatRow{Text: ansi.Cut(line, column, column+ansi.StringWidth(row)), Line: i, Column: column, Start: position})
			position += len(visible)
			column += ansi.StringWidth(visible)
		}
	}
	return rows
}

// Bubble decoration lives in visual rows so search and resize anchors still
// refer to the original question text, including phrases across wrapped rows.
func (c *ChatState) ChatLineRows(line string, index, width int) []ChatRow {
	if width >= 6 {
		for _, question := range c.Questions {
			if index < question.First-1 {
				break
			}
			if index > question.Last {
				continue
			}
			limit := width
			if width >= 20 {
				limit = width * 85 / 100
			}
			boxWidth := min(limit, max(6, question.Width+4))
			inset := strings.Repeat(" ", width-boxWidth)
			if index == question.First-1 {
				return []ChatRow{{Text: strings.Repeat(" ", max(0, width-ansi.StringWidth(line)-2)) + line, Line: index}}
			}
			innerWidth := boxWidth - 4
			rows := ChatRows(line, innerWidth)
			frame := lipgloss.NewStyle().Foreground(ui.BorderColor).Background(ui.Background)
			fill := ui.CanvasStyle.Background(ui.Highlight)
			for i := range rows {
				rows[i].Line = index
				body := rows[i].Text + strings.Repeat(" ", max(0, innerWidth-ansi.StringWidth(rows[i].Text)))
				rows[i].Text = inset + frame.Render("│") + fill.Render(" "+body+" ") + frame.Render("│")
			}
			if index == question.First {
				top := ChatRow{Text: inset + frame.Render("╭"+strings.Repeat("─", boxWidth-2)+"╮"), Line: index, Column: -1, Start: -1}
				rows = append([]ChatRow{top}, rows...)
			}
			if index == question.Last {
				bottom := ChatRow{Text: inset + frame.Render("╰"+strings.Repeat("─", boxWidth-2)+"╯"), Line: index, Column: ansi.StringWidth(line) + 1, Start: len(ansi.Strip(line)) + 1}
				rows = append(rows, bottom)
			}
			return rows
		}
	}
	rows := ChatRows(line, width)
	prefix := ChatIndent.FindString(ansi.Strip(line))
	indent := ansi.StringWidth(prefix)
	if indent > 0 && indent < width-4 {
		rows = ChatRows(ansi.Cut(line, indent, ansi.StringWidth(line)), width-indent)
		for i := range rows {
			rows[i].Column += indent
			rows[i].Start += len(prefix)
			decoration := strings.Repeat(" ", indent)
			if i == 0 {
				decoration = ansi.Cut(line, 0, indent)
				rows[i].Column, rows[i].Start = 0, 0
			}
			rows[i].Text = decoration + rows[i].Text
		}
	}
	for i := range rows {
		rows[i].Line = index
	}
	return rows
}

func (c *ChatState) ChatRows(body string, width int) []ChatRow {
	var rows []ChatRow
	for i, line := range strings.Split(body, "\n") {
		rows = append(rows, c.ChatLineRows(line, i, width)...)
	}
	return rows
}

// Retain only visible rows while traversing a bounded transcript window.
func (c *ChatState) WalkChatRows(body string, width int, visit func(ChatRow) bool) {
	for index := 0; ; index++ {
		line, rest, more := strings.Cut(body, "\n")
		for _, row := range c.ChatLineRows(line, index, width) {
			if !visit(row) {
				return
			}
		}
		if !more {
			return
		}
		body = rest
	}
}

// Package markdown provides the read-only Plous Markdown Viewer.
package markdown

import (
	lipgloss "charm.land/lipgloss/v2"
	"strings"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

type Model struct {
	Reader      textview.Viewer
	Name        string
	Actions     []textview.Action
	Source      bool
	source      string
	rendered    []textview.Line
	sourceLines []textview.Line
	saved       [2]textview.Viewer
	prepared    bool
	palette     markdownPalette
	notice      string
}

func New(source string) *Model {
	lines := strings.Split(ansi.Strip(source), "\n")
	for i, line := range lines {
		lines[i] = textview.Visible(strings.TrimSuffix(line, "\r"))
	}
	source = strings.Join(lines, "\n")
	m := &Model{source: source, Reader: textview.Viewer{Wrap: true, ShowPosition: true}}
	for _, line := range strings.Split(source, "\n") {
		m.sourceLines = append(m.sourceLines, textview.Line{Text: line})
	}
	return m
}

func (m *Model) prepare(theme textview.Theme) {
	palette := paletteFor(theme)
	if m.prepared && m.palette == palette {
		return
	}
	m.prepared, m.palette = true, palette
	var err error
	output := ""
	if len(m.source) <= 1<<20 {
		output, err = renderMarkdown(m.source, palette)
	}
	if err != nil || len(m.source) > 1<<20 {
		m.notice = "Rendering unavailable; showing Markdown source."
		m.Source = true
		m.Reader.SetLines(m.sourceLines)
		return
	}
	m.rendered = nil
	for _, line := range strings.Split(strings.Trim(output, "\n"), "\n") {
		plain := strings.TrimRight(ansi.Strip(line), " ")
		styled := ansi.Truncate(line, ansi.StringWidth(plain), "")
		m.rendered = append(m.rendered, textview.Line{Text: plain, Styled: styled})
	}
	if m.Source {
		m.Reader.SetLines(m.sourceLines)
	} else {
		m.Reader.SetLines(m.rendered)
	}
}

func ptr[T any](value T) *T { return &value }

func (m *Model) Key(key, text string) (bool, string) {
	if !m.Reader.Searching {
		switch key {
		case "R":
			if m.notice != "" {
				return true, ""
			}
			i := 0
			if m.Source {
				i = 1
			}
			m.saved[i] = m.Reader
			m.Source = !m.Source
			m.Reader = m.saved[1-i]
			m.Reader.Wrap, m.Reader.ShowPosition = true, true
			if m.Source {
				m.Reader.SetLines(m.sourceLines)
			} else {
				m.Reader.SetLines(m.rendered)
			}
			return true, ""
		case "Y":
			return true, m.source
		}
	}
	return m.Reader.Key(key, text)
}

func (m *Model) View(width, height int, theme textview.Theme) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	m.prepare(theme)
	mode, toggle := "Rendered", "Source"
	if m.Source {
		mode, toggle = "Source", "Rendered"
	}
	title := "Plous Markdown Viewer"
	if m.Name != "" {
		title += " · " + textview.Visible(m.Name)
	}
	header := theme.Accent.Render(ansi.Truncate(title, width, "…")) + "\n" + theme.Muted.Render(mode)
	if m.notice != "" {
		header += " · " + theme.Muted.Render(ansi.Truncate(m.notice, max(1, width-len(mode)-3), "…"))
	}
	actions := append([]textview.Action{}, m.Actions...)
	actions = append(actions, textview.Action{Key: "R", Label: toggle, Disabled: m.notice != ""}, textview.Action{Key: "/", Label: "Find"}, textview.Action{Key: "v", Label: "Select"}, textview.Action{Key: "y", Label: "Copy"}, textview.Action{Key: "Y", Label: "Copy source"}, textview.Action{Key: "Esc", Label: m.Reader.EscapeLabel("Back")})
	if height >= 6 {
		header += "\n" + textview.ActionBar([]textview.ActionGroup{{Actions: actions}}, width, min(2, max(1, height-5)), theme)
	}
	available := height - lipgloss.Height(header) - 1
	if available < 1 {
		return textview.Fit(header, width, height)
	}
	return textview.Fit(header+"\n\n"+m.Reader.View(width, available, theme), width, height)
}

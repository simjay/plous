package logs

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

type Model struct {
	Reader                        textview.Viewer
	Text                          string
	Message                       string
	MaxBytes, MaxLines            int
	Dropped, Unseen               int
	Following                     bool
	formatLine                    func(string) textview.Line
	preparedText, preparedMessage string
	prepared                      bool
	preparedLines                 []string
	preparedRows                  []textview.Line
}

func New(bytes, lines int) *Model {
	return &Model{MaxBytes: bytes, MaxLines: lines, Following: true, Reader: textview.Viewer{Wrap: true}}
}

func (m *Model) Reset() {
	m.Text = ""
	m.Message = ""
	m.Dropped = 0
	m.Unseen = 0
	m.Following = true
	m.Reader = textview.Viewer{Wrap: m.Reader.Wrap}
	m.preparedText, m.preparedMessage, m.prepared = "", "", false
	m.preparedLines, m.preparedRows = nil, nil
}

// Formatting changes displayed text, never retained source or byte limits.
func (m *Model) SetFormatter(format func(string) textview.Line) {
	m.formatLine, m.prepared = format, false
	m.preparedRows = nil
	m.preparedLines = nil
	m.Refresh()
}

func (m *Model) Append(text string) {
	if text == "" {
		return
	}
	if !m.Following {
		m.Unseen += strings.Count(text, "\n")
	}
	m.Text += text
	start := 0
	if m.MaxBytes > 0 && len(m.Text) > m.MaxBytes {
		start = len(m.Text) - m.MaxBytes
		if m.Text[start-1] != '\n' {
			if index := strings.IndexByte(m.Text[start:], '\n'); index >= 0 {
				start += index + 1
			} else {
				for start < len(m.Text) && !utf8.RuneStart(m.Text[start]) {
					start++
				}
			}
		}
	}
	if m.MaxLines > 0 {
		for remaining := strings.Count(m.Text[start:], "\n") - m.MaxLines; remaining > 0; remaining-- {
			start += strings.IndexByte(m.Text[start:], '\n') + 1
		}
	}
	if start > 0 {
		removed := strings.Count(m.Text[:start], "\n")
		if !strings.HasSuffix(m.Text[:start], "\n") {
			removed++
		}
		m.Dropped += removed
		m.Reader.Shift(removed)
		if removed < len(m.preparedLines) {
			m.preparedLines = m.preparedLines[removed:]
			m.preparedRows = m.preparedRows[removed:]
		} else {
			m.preparedLines, m.preparedRows = nil, nil
		}
		m.Text = strings.Clone(m.Text[start:])
	}
	m.Refresh()
}

func (m *Model) Refresh() {
	if m.prepared && m.preparedText == m.Text && m.preparedMessage == m.Message {
		return
	}
	var lines []string
	if m.Text != "" {
		lines = strings.Split(m.Text, "\n")
	}
	rows := make([]textview.Line, len(lines))
	for i, line := range lines {
		if i < len(m.preparedLines) && m.preparedLines[i] == line {
			rows[i] = m.preparedRows[i]
		} else {
			rows[i] = textview.Line{Text: line}
			if m.formatLine != nil {
				rows[i] = m.formatLine(line)
			}
		}
	}
	m.preparedLines, m.preparedRows = lines, rows
	if m.Message != "" {
		for _, line := range strings.Split(m.Message, "\n") {
			rows = append(rows, textview.Line{Text: line})
		}
	}
	m.Reader.SetLines(rows)
	m.preparedText, m.preparedMessage, m.prepared = m.Text, m.Message, true
	if m.Following {
		m.Reader.Cursor = textview.Point{Row: max(0, len(rows)-1)}
	}
}

func (m *Model) Snapshot(rows []textview.Line) {
	bytes, start := 0, len(rows)
	for start > 0 {
		size := len(rows[start-1].Text) + len(rows[start-1].Styled)
		if m.MaxBytes > 0 && bytes+size > m.MaxBytes || m.MaxLines > 0 && len(rows)-start >= m.MaxLines {
			break
		}
		bytes += size
		start--
	}
	m.Dropped = start
	m.Reader.SetLines(append([]textview.Line(nil), rows[start:]...))
	m.Reader.Cursor = textview.Point{Row: max(0, len(m.Reader.Lines)-1)}
	m.Following = false
}

func (m *Model) Follow() {
	m.Reader.RevealCursor()
	m.Following = true
	m.Reader.Caret = false
	m.Unseen = 0
	m.Reader.Selecting = false
	m.Reader.Query = ""
	m.Reader.Searching = false
	m.Reader.SetLines(m.Reader.Lines)
	m.Reader.Cursor = textview.Point{Row: max(0, len(m.Reader.Lines)-1)}
}
func (m *Model) Pause() { m.Following = false }

func (m *Model) Key(key, text string) (bool, string) {
	if !m.Reader.Searching && (key == " " || key == "space") && !m.Reader.Selecting {
		if m.Following {
			m.Pause()
		} else {
			m.Follow()
		}
		return true, ""
	}
	handled, copy := m.Reader.Key(key, text)
	if handled {
		m.Pause()
	}
	return handled, copy
}

type Header struct {
	Title, Target, State, Note string
	Actions                    []textview.ActionGroup
}

func HeaderView(h Header, width, height int, theme textview.Theme) string {
	rows := []string{theme.Accent.Render(ansi.Truncate(h.Title, width, "…")), theme.Muted.Render(ansi.Truncate(h.Target, width, "…")), theme.Accent.Render(ansi.Truncate(h.State, width, "…"))}
	if h.Note != "" && height >= 8 {
		notes := strings.Split(h.Note, "\n")
		for _, line := range notes[:min(len(notes), max(0, height-7))] {
			rows = append(rows, theme.Muted.Render(ansi.Truncate(line, width, "…")))
		}
	}
	if len(h.Actions) > 0 {
		// Reserve output, its position line, and the optional divider.
		reserved := 2
		if height >= 8 {
			reserved++
		}
		budget := max(1, height-len(rows)-reserved)
		if height >= 12 {
			budget = min(budget, max(2, height/3))
		}
		if width < 40 && height < 24 {
			budget = min(budget, 2)
		}
		rows = append(rows, textview.ActionBar(h.Actions, width, budget, theme))
	}
	if height >= 8 {
		rows = append(rows, theme.Muted.Render(strings.Repeat("─", max(0, width))))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) View(h Header, width, height int, theme textview.Theme) string {
	m.Reader.ShowPosition = true
	if m.Unseen > 0 {
		noun := "lines"
		if m.Unseen == 1 {
			noun = "line"
		}
		h.State += fmt.Sprintf(" · %d new %s", m.Unseen, noun)
	}
	if m.Dropped > 0 {
		h.Note += fmt.Sprintf(" · %d older lines removed", m.Dropped)
	}
	if m.Reader.Notice != "" {
		h.Note = m.Reader.Notice
	}
	header := HeaderView(h, width, height, theme)
	return textview.Fit(header+"\n"+m.Reader.View(width, max(0, height-lipgloss.Height(header)), theme), width, height)
}

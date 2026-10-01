package diffview

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/simjay/plous/internal/textview"
)

type Decision struct{ ID, Action string }
type Model struct {
	ID, Target, BeforeLabel, AfterLabel string
	Name, ConfirmKey                    string
	BackLabel, ConfirmLabel             string
	Notice, EscapeOverride              string
	ErrorReader                         textview.Viewer
	ErrorActive                         bool
	Reader                              textview.Viewer
	Actions                             bool
	ViewActions                         []textview.ActionGroup
	Action                              int
	CanConfirm, Stale                   bool
	Error                               string
	Added, Removed                      int
	hunks                               []int
	before, after                       string
	kinds                               []byte
	prefixes                            []string
	themeKey                            [5]string
}

func New(before, after, target string, revision uint64, confirm bool) *Model {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", target, revision, before, after))))
	m := &Model{ID: id, Target: target, BeforeLabel: "Before", AfterLabel: "After", BackLabel: "Back", ConfirmLabel: "Confirm", CanConfirm: confirm && before != after, before: before, after: after}
	if !confirm {
		m.ConfirmLabel = ""
	}
	m.build()
	return m
}

func (m *Model) build() {
	a, b := splitLines(m.before), splitLines(m.after)
	rows := []textview.Line{}
	appendLine := func(text string, kind byte, old, new int) {
		if kind == '+' {
			m.Added++
		}
		if kind == '-' {
			m.Removed++
		}
		left, right := "    ", "    "
		if old > 0 {
			left = fmt.Sprintf("%4d", old)
		}
		if new > 0 {
			right = fmt.Sprintf("%4d", new)
		}
		prefix := left + " " + right + " " + string(kind) + " "
		line := strings.TrimSuffix(text, "\n")
		rows = append(rows, textview.Line{Text: line, Prefix: prefix})
		m.kinds = append(m.kinds, kind)
		if !strings.HasSuffix(text, "\n") {
			rows = append(rows, textview.Line{Metadata: true, Text: "No newline at end of file", Prefix: "           \\ "})
			m.kinds = append(m.kinds, '\\')
		}
	}
	for _, op := range difflib.NewMatcher(a, b).GetOpCodes() {
		if op.Tag != 'e' {
			m.hunks = append(m.hunks, len(rows))
			label := fmt.Sprintf("@@ -%d,%d +%d,%d @@", op.I1+1, op.I2-op.I1, op.J1+1, op.J2-op.J1)
			rows = append(rows, textview.Line{Metadata: true, Text: label})
			m.kinds = append(m.kinds, '@')
		}
		switch op.Tag {
		case 'e':
			for i, j := op.I1, op.J1; i < op.I2; i, j = i+1, j+1 {
				appendLine(a[i], ' ', i+1, j+1)
			}
		case 'd':
			for i := op.I1; i < op.I2; i++ {
				appendLine(a[i], '-', i+1, 0)
			}
		case 'i':
			for j := op.J1; j < op.J2; j++ {
				appendLine(b[j], '+', 0, j+1)
			}
		case 'r':
			for i := op.I1; i < op.I2; i++ {
				appendLine(a[i], '-', i+1, 0)
			}
			for j := op.J1; j < op.J2; j++ {
				appendLine(b[j], '+', 0, j+1)
			}
		}
	}
	m.Reader.Caret = true
	for _, row := range rows {
		m.prefixes = append(m.prefixes, row.Prefix)
	}
	m.Reader.SetLines(rows)
	if len(m.hunks) > 0 {
		m.Reader.Cursor.Row = m.hunks[0]
	}
}

func (m *Model) style(theme textview.Theme) {
	key := [5]string{theme.Text.Render("x"), theme.Muted.Render("x"), theme.Accent.Render("x"), theme.Added.Render("x"), theme.Removed.Render("x")}
	if m.themeKey == key {
		return
	}
	m.themeKey = key
	for i := range m.Reader.Lines {
		line := &m.Reader.Lines[i]
		style := theme.Text
		switch m.kinds[i] {
		case '+':
			style = theme.Added
		case '-':
			style = theme.Removed
		case '@':
			style = theme.Accent
		case '\\':
			style = theme.Muted
		}
		line.Styled = style.Render(textview.Visible(line.Text))
		prefix := m.prefixes[i]
		if line.Metadata {
			line.Prefix = theme.Muted.Render(prefix)
		} else {
			line.Prefix = theme.Muted.Render(prefix[:len(prefix)-2]) + style.Render(prefix[len(prefix)-2:])
		}
	}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (m *Model) SetError(message string) {
	m.Error, m.ErrorActive = message, message != ""
	rows := []textview.Line{}
	for _, line := range strings.Split(message, "\n") {
		rows = append(rows, textview.Line{Text: line})
	}
	m.ErrorReader = textview.Viewer{Wrap: true}
	m.ErrorReader.SetLines(rows)
}

func (m *Model) Invalidate() { m.Stale = true; m.CanConfirm = false }
func (m *Model) Confirm() Decision {
	if m.CanConfirm && !m.Stale {
		return Decision{m.ID, "confirm"}
	}
	return Decision{}
}

func (m *Model) Key(key, text string) (Decision, string) {
	if m.ErrorActive {
		handled, copied := m.ErrorReader.Key(key, text)
		if key == "esc" && !handled {
			m.ErrorActive = false
		}
		return Decision{}, copied
	}
	if m.Reader.Searching || m.Reader.Selecting {
		_, copy := m.Reader.Key(key, text)
		return Decision{}, copy
	}
	if key == "!" && m.Error != "" {
		m.ErrorActive = true
		return Decision{}, ""
	}
	if key == "esc" {
		if handled, _ := m.Reader.Key(key, text); handled {
			return Decision{}, ""
		}
		return Decision{m.ID, "back"}, ""
	}
	if key == "a" {
		m.Actions = !m.Actions
		return Decision{}, ""
	}
	if m.Actions {
		switch key {
		case "left", "right":
			if m.ConfirmLabel != "" {
				m.Action = 1 - m.Action
			}
		case "enter":
			if m.Action == 0 {
				return Decision{m.ID, "back"}, ""
			}
			return m.Confirm(), ""
		}
		return Decision{}, ""
	}
	if key == "[" || key == "]" {
		row := m.Reader.Cursor.Row
		found := -1
		if key == "]" {
			for _, h := range m.hunks {
				if h > row {
					found = h
					break
				}
			}
			if found < 0 && len(m.hunks) > 0 {
				found = m.hunks[0]
			}
		} else {
			for i := len(m.hunks) - 1; i >= 0; i-- {
				if m.hunks[i] < row {
					found = m.hunks[i]
					break
				}
			}
			if found < 0 && len(m.hunks) > 0 {
				found = m.hunks[len(m.hunks)-1]
			}
		}
		if found >= 0 {
			m.Reader.Cursor = textview.Point{Row: found}
		}
		return Decision{}, ""
	}
	_, copy := m.Reader.Key(key, text)
	return Decision{}, copy
}

func (m *Model) EscapeLabel() string {
	if m.ErrorActive {
		return m.ErrorReader.EscapeLabel("Back to comparison")
	}
	return m.Reader.EscapeLabel(m.BackLabel)
}

func (m *Model) View(width, height int, theme textview.Theme) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	m.style(theme)
	state := fmt.Sprintf("%d additions · %d removals", m.Added, m.Removed)
	if m.before == m.after {
		state = "No manifest changes."
	}
	if m.Stale {
		state = "Stale comparison · Validate again"
	}
	title := "Plous Diff Reviewer"
	if m.Name != "" {
		title += " · " + m.Name
	}
	header := theme.Accent.Render(ansi.Truncate(title, width, "…"))
	for _, line := range strings.SplitN(m.Target, "\n", 3)[:min(2, strings.Count(m.Target, "\n")+1)] {
		header += "\n" + theme.Muted.Render(ansi.Truncate(line, width, "…"))
	}
	header += "\n" + theme.Muted.Render(ansi.Truncate(m.BeforeLabel+" → "+m.AfterLabel, width, "…")) + "\n" + theme.Accent.Render(ansi.Truncate(state, width, "…"))
	if m.Notice != "" {
		lines := strings.Split(ansi.Wrap(m.Notice, width, ""), "\n")
		budget := min(len(lines), max(0, height-lipgloss.Height(header)-4))
		if budget > 0 {
			header += "\n" + theme.Muted.Render(strings.Join(lines[:budget], "\n"))
		}
	}
	escape := m.EscapeLabel()
	if m.EscapeOverride != "" {
		escape = m.EscapeOverride
	}
	actions := []textview.Action{{Label: m.BackLabel, Focused: m.Actions && m.Action == 0}}
	if escape == m.BackLabel {
		actions[0].Key = "Esc"
	}
	if m.ConfirmLabel != "" {
		actions = append(actions, textview.Action{Key: m.ConfirmKey, Label: m.ConfirmLabel, Focused: m.Actions && m.Action == 1, Primary: true, Disabled: !m.CanConfirm})
	}
	for i := range actions {
		if actions[i].Focused && (i > 0 || actions[i].Key == "") {
			actions[i].Key = "Enter"
		}
	}
	if escape != m.BackLabel {
		actions = append([]textview.Action{{Key: "Esc", Label: escape}}, actions...)
	}
	groups := []textview.ActionGroup{{Label: "Decision", Actions: actions}}
	groups = append(groups, m.ViewActions...)
	if !m.Actions {
		groups = append(groups, textview.ActionGroup{Label: "Navigate", Actions: []textview.Action{{Key: "[ ]", Label: "Change"}, {Key: "/", Label: "Search"}, {Key: "a", Label: "Actions"}}})
	}
	header += "\n" + textview.ActionBar(groups, width, max(1, min(4, height-lipgloss.Height(header)-3)), theme)
	if m.Error != "" && !m.ErrorActive {
		header += "\n" + theme.Removed.Render(ansi.Truncate("! Operation error · Read details before retrying", width, "…"))
	}
	header += "\n" + theme.Muted.Render(strings.Repeat("─", width))
	if m.ErrorActive {
		m.ErrorReader.ShowPosition = true
		return textview.Fit(header+"\n"+m.ErrorReader.View(width, max(0, height-lipgloss.Height(header)), theme), width, height)
	}
	m.Reader.ShowPosition = true
	return textview.Fit(header+"\n"+m.Reader.View(width, max(0, height-lipgloss.Height(header)), theme), width, height)
}

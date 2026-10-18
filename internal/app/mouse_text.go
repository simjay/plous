package app

import (
	tea "charm.land/bubbletea/v2"

	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"slices"
	"strings"
	"time"
)

type mouseTextSurface struct {
	input        *textview.Field
	query        *textview.QueryInput
	queryText    *string
	document     *docedit.Model
	reader       *textview.Viewer
	scope, owner string
}

type mouseTextGesture struct {
	surface    mouseTextSurface
	inputValue string
	lines      []string
	spans      []textview.MouseSpan
	anchor     textview.Point
	context    string
	x, y       int
	dragged    bool
}

type mouseTextClick struct {
	surface    mouseTextSurface
	inputValue string
	lines      []string
	point      textview.Point
	context    string
	x, y       int
	at         time.Time
}

func (s mouseTextSurface) inputValue() string {
	if s.input != nil {
		return s.input.Text
	}
	return ""
}

func (s mouseTextSurface) lines() []string {
	if s.query != nil {
		return []string{*s.queryText}
	}
	if s.input != nil {
		text := s.input.Text
		if strings.HasPrefix(s.owner, "secret/") {
			text = strings.Repeat("•", len([]rune(text)))
		}
		return []string{text}
	}
	if s.document != nil {
		return strings.Split(s.document.Text(), "\n")
	}
	var lines []string
	if s.reader != nil {
		for _, line := range s.reader.Lines {
			lines = append(lines, line.Text)
		}
	}
	return lines
}

func (m *model) mouseTextSurface(scope, owner string) (s mouseTextSurface) {
	s.scope, s.owner = scope, owner
	if m.helpVisible() {
		return s
	}
	if owner == "reader" || owner == "document" || owner == "query" {
		if c := m.chatSources; m.chatSourcesVisible() {
			if scope != "sources" {
				return s
			}
			if c.ShowReply {
				s.reader = &c.Reply.Reader
			} else {
				s.document = c.Document
			}
		} else if scope == "main" && m.form == nil && m.settingsOverlay == nil {
			source, reader, document, _ := m.expandableView()
			if comparison, ok := source.(*diffview.Model); ok && comparison.ErrorActive {
				reader = &comparison.ErrorReader
			}
			if m.documentActive() {
				a := m.explore.Action
				if a.Busy || a.Discard {
					return s
				}
				if a.Diagnostic != nil {
					reader, document = a.Diagnostic, nil
				}
			}
			s.reader, s.document = reader, document
		}
		if owner == "query" {
			if s.document != nil {
				s.query, s.queryText = &s.document.SearchInput, &s.document.Query
			} else if s.reader != nil {
				s.query, s.queryText = &s.reader.SearchInput, &s.reader.Query
			}
		} else if owner == "reader" {
			s.document = nil
		} else {
			s.reader = nil
		}
		return s
	}
	if m.chatSourcesVisible() {
		return s
	}
	switch owner {
	case "command":
		if scope == "main" {
			s.input = m.command
		}
	case "composer":
		if scope == "chat" && !m.chat().History && !m.chat().Models {
			s.input = m.draft()
		}
	case "execfield":
		if scope == "main" && m.resourceActionVisible() && m.explore.Action.Stage == "execcommand" && !m.explore.Action.Busy {
			s.input = &m.explore.Action.Input
		}
	case "input":
		panel := map[string]int{"tree": 0, "main": 1, "chat": 2}[scope]
		if search := m.activePanelSearch(panel); search != nil {
			s.input = &search.input
			return s
		}
		if scope != "main" {
			return s
		}
		if m.form != nil {
			if p := m.form.Directory; p != nil && p.Editing {
				s.input = &p.Input
			}
			return s
		}
		if m.overviewVisible() {
			if v := m.overviewRead(); v != nil {
				s.input = v.State.Input
			}
			return s
		}
		if m.codeVisible() {
			s.input = m.coder().Input
			return s
		}

		if m.helmVisible() {
			s.input = m.helmer().Input
			if f := m.helmer().Find; f != nil && s.input == nil {
				s.reader, s.query, s.queryText = &f.Reader, &f.Reader.SearchInput, &f.Reader.Query
			}
			return s
		}
		if m.resourceActionVisible() && !m.explore.Action.Busy {
			a := m.explore.Action
			if a.Stage == "scale" || a.Stage == "export" {
				s.input = &a.Input
			}
		} else if m.exploreVisible() {
			s.input = m.explore.Input
		}
	default:
		kind, key, _ := strings.Cut(owner, "/")
		if scope != "main" || kind != "field" && kind != "secret" {
			return s
		}
		f := m.form
		if f == nil && m.helmVisible() && m.helmer().Draft != nil {
			f = m.helmer().Draft.Form
		}
		if f != nil && f.Directory == nil {
			for i := range f.Fields {
				if f.Fields[i].Key == key && len(f.Fields[i].Options) == 0 {
					s.input = &f.Fields[i].Value
					break
				}
			}
		}
	}
	return s
}

func (s mouseTextSurface) valid() bool {
	return s.input != nil || s.document != nil || s.reader != nil || s.query != nil
}

func textPoint(span textview.MouseSpan, target textview.TextTarget, lines []string, x int) (textview.Point, bool) {
	if target.Row >= len(lines) || textview.TextHash(lines[target.Row]) != target.Hash {
		return textview.Point{}, false
	}
	col := target.Offset
	if target.Cells {
		col = textview.ColumnAt(lines[target.Row], col+x-span.X)
	}
	return textview.Point{Row: target.Row, Col: col}, true
}

func (m *model) applyTextPointer(s mouseTextSurface, point textview.Point, anchor *textview.Point, word bool) {
	m.status = ""
	m.mouseFocus(s.scope)
	m.mouse.commandPaused = s.owner != "command"
	if s.query != nil {
		if s.document != nil {
			s.document.ResumeSearch()
		} else {
			s.reader.Searching = true
		}
		s.query.Pointer(*s.queryText, point, anchor, word)
	} else if s.input != nil {
		switch {
		case s.owner == "composer":
			m.focusChat(true)
		case s.owner == "execfield":
			m.explore.Action.ExecEditing = true
		case strings.HasPrefix(s.owner, "field/") || strings.HasPrefix(s.owner, "secret/"):
			_, key, _ := strings.Cut(s.owner, "/")
			m.mouseForm("field", key)
		case s.owner == "input":
			if search := m.activePanelSearch(m.focus); search != nil {
				search.finished = false
			}
		}
		s.input.Pointer(point.Col, anchor, word)
	} else if s.document != nil {
		s.document.Pointer(point, anchor, word)
	} else if s.reader != nil {
		if m.logWorkspaceActive() && s.scope == "main" {
			m.podLogViewer().Pause()
		}
		if s.scope == "main" {
			source, _, _, _ := m.expandableView()
			if comparison, ok := source.(*diffview.Model); ok {
				comparison.Actions = false
			}
		}
		s.reader.Pointer(point, anchor, word)
	}
}

func (m *model) updateTextMouse(msg tea.MouseMsg, span textview.MouseSpan, hit bool, now time.Time, context string) bool {
	p := msg.Mouse()
	if _, wheel := msg.(tea.MouseWheelMsg); wheel {
		return false
	}
	if g := m.mouse.text; g != nil {
		if _, press := msg.(tea.MouseClickMsg); press {
			m.mouse.text = nil
			return false
		}
		if context != g.context || m.mouseTextSurface(g.surface.scope, g.surface.owner) != g.surface || g.inputValue != g.surface.inputValue() || !slices.Equal(g.lines, g.surface.lines()) {
			m.mouse.reset()
			return true
		}
		// Use the pressed frame while dragging. Cursor decoration and selection
		// redraws must not move the source positions under the pointer.
		point, found := g.anchor, false
		for _, old := range g.spans {
			if old.Y != p.Y || p.X < old.X || p.X >= old.X+old.Width || old.Scope != g.surface.scope {
				continue
			}
			t, ok := textview.ParseTextTarget(old.Target)
			if ok && t.Owner == g.surface.owner {
				point, found = textPoint(old, t, g.lines, p.X)
			}
			break
		}
		switch msg.(type) {
		case tea.MouseMotionMsg:
			if p.Button != tea.MouseLeft {
				m.mouse.reset()
				return true
			}
			if p.X != g.x || p.Y != g.y {
				g.dragged = true
			}
			if g.dragged && found {
				m.applyTextPointer(g.surface, point, &g.anchor, false)
			}
		case tea.MouseReleaseMsg:
			m.mouse.text = nil
			if p.Button != tea.MouseLeft && p.Button != tea.MouseNone {
				m.mouse.lastText = nil
				return true
			}
			if g.dragged {
				if found {
					m.applyTextPointer(g.surface, point, &g.anchor, false)
				}
				m.mouse.lastText = nil
			} else if found && absMouse(g.x-p.X) <= 1 && absMouse(g.y-p.Y) <= 1 {
				double := false
				if last := m.mouse.lastText; last != nil {
					double = last.surface == g.surface && last.inputValue == g.inputValue && slices.Equal(last.lines, g.lines) && last.point == point && last.context == context && now.Sub(last.at) >= 0 && now.Sub(last.at) <= 400*time.Millisecond && absMouse(last.x-p.X) <= 1 && absMouse(last.y-p.Y) <= 1
				}
				m.applyTextPointer(g.surface, point, nil, double)
				m.mouse.lastText = &mouseTextClick{g.surface, g.inputValue, g.lines, point, m.mouseContext(), p.X, p.Y, now}
				if double {
					m.mouse.lastText = nil
				}
			}
			m.persist()
		}
		return true
	}
	if _, press := msg.(tea.MouseClickMsg); !press || p.Button != tea.MouseLeft || !hit {
		return false
	}
	target, ok := textview.ParseTextTarget(span.Target)
	if !ok {
		m.mouse.lastText = nil
		return false
	}
	surface := m.mouseTextSurface(span.Scope, target.Owner)
	if !surface.valid() {
		return false
	}
	lines := surface.lines()
	point, ok := textPoint(span, target, lines, p.X)
	if !ok {
		return true
	}
	m.mouse.press, m.mouse.last = nil, nil
	m.shortcutPrefix = ""
	m.clearDocumentPrefix()
	m.mouse.text = &mouseTextGesture{surface: surface, inputValue: surface.inputValue(), lines: lines, spans: slices.Clone(m.mouse.spans), anchor: point, context: context, x: p.X, y: p.Y}
	return true
}

func (m *model) inputSurface() mouseTextSurface {
	if m.helpVisible() || m.chatSourcesVisible() {
		return mouseTextSurface{}
	}
	scope := []string{"tree", "main", "chat"}[m.focus]
	owner := "input"
	if m.commandFocused() {
		owner = "command"
	} else if m.focus == 2 && m.chatEditing {
		owner = "composer"
	} else if m.focus == 1 {
		f := m.form
		if f == nil && m.helmVisible() && m.helmer().Draft != nil && m.helmer().WorkflowVisible() {
			f = m.helmer().Draft.Form
		}
		if f != nil && f.Directory == nil && f.Index < len(f.Fields) {
			owner = "field/" + f.Fields[f.Index].Key
			if f.Fields[f.Index].Secret {
				owner = "secret/" + f.Fields[f.Index].Key
			}
		}
		if m.resourceActionVisible() && m.explore.Action.ExecEditing {
			owner = "execfield"
		}
	}
	return m.mouseTextSurface(scope, owner)
}

func (m *model) selectedInput() *textview.Field {
	s := m.inputSurface()
	if s.input != nil && s.input.Selecting {
		return s.input
	}
	return nil
}

func (m *model) clearInputSelection() bool {
	if input := m.selectedInput(); input != nil {
		input.Selecting = false
		return true
	}
	return false
}

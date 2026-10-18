package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"

	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
	"strings"
	"time"
)

type mouseState struct {
	commandPaused bool
	spans         []textview.MouseSpan
	context       string
	press, last   *mouseGesture
	now           func() time.Time
	text          *mouseTextGesture
	lastText      *mouseTextClick
}

type mouseGesture struct {
	scope, target, context string
	x, y                   int
	at                     time.Time
}

func (s *mouseState) reset() { s.press, s.last, s.text, s.lastText = nil, nil, nil, nil }

func (s *mouseState) hit(x, y int) (textview.MouseSpan, bool) {
	for _, span := range s.spans {
		if span.Y == y && x >= span.X && x < span.X+span.Width {
			return span, true
		}
	}
	return textview.MouseSpan{}, false
}

func (m *model) mouseContext() string {
	s := fmt.Sprintf("%dx%d/%s/%s/%d/%s/%p/%p/%p/%p/%t/%t/%p", m.width, m.height, m.active, m.connection, m.mode, m.page, m.form, m.help, m.settingsOverlay, m.chatSources, m.leftHidden, m.rightHidden, m.viewExpansion)
	s += "/assistant:" + string(m.assistantSize)
	if m.overviewVisible() {
		if v := m.overviewRead(); v != nil {
			s += fmt.Sprintf("/overview:%p/%d/%t/%p", v, v.State.Pane, v.State.Expanded, v.State.MetricDetail)
			if d := v.State.MetricDetail; d != nil {
				s += fmt.Sprintf("/sources:%t", d.Sources)
			}
		}
	}
	if m.form != nil && m.form.Directory != nil {
		s += m.form.Directory.Identity()
	}
	if e := m.explore; e != nil {
		s += fmt.Sprintf("/k:%s/%s/%d/%s/%p/%p", e.Key, e.Resource.Key(), e.Tab, e.Picker, e.Log, e.Terminal)
		if e.Detail != nil {
			s += "/" + string(e.Detail.GetUID()) + "/" + e.Detail.GetName()
		}
		if a := e.Action; a != nil {
			s += fmt.Sprintf("/a:%p/%s/%t/%t/%p", a, a.Stage, a.Busy, a.Discard, a.Diagnostic)
			if a.Document != nil {
				s += fmt.Sprintf("/d:%d", a.Document.Revision)
			}
			if a.Review != nil {
				s += "/r:" + a.Review.ID
			}
		}
	}
	if m.codeVisible() {
		c := m.coder()
		entry := c.Preference.Selected
		if c.Preference.Section == 0 {
			entry = ""
		}
		s += fmt.Sprintf("/code:%s/%s/%p/%s/%d/%s/%s/%p/%p/%t/%t/%s", c.View, c.Key, c.Inventory, entry, c.Preference.Section, c.Menu, c.InputKind, c.Document, c.Review, c.Discard, c.Busy && c.Running != "preview", c.Filter)
		s += fmt.Sprintf("/files:%s/%v", c.FileRoot, c.FileExpanded)
		s += fmt.Sprintf("/layout:%s/%t/%t", c.RenderRoot+"/"+c.Group+"/"+c.NodeGraph, c.BrowserDetail, c.ComparisonUnified)
		if c.Document != nil {
			s += fmt.Sprintf("/revision:%d", c.Document.Revision)
		}
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		s += fmt.Sprintf("/gitops:%s/%s/%p/%p/%t/%s", v.State.Tool, v.State.Section, v.State.Detail, v.Namespaces, v.State.Loading, v.State.Query)
	}
	if m.helmVisible() {
		h := m.helmer()
		s += fmt.Sprintf("/h:%s/%s/%p/%p/%p/%d/%s/%s/%t/%t/%t/%p/%p", h.Key, h.Preference.Section, h.Detail, h.Artifact, h.Timeline, h.Tab, h.MenuKind, h.InputKind, h.Confirm, h.Discard, h.Busy, h.Document, h.Comparison)
	}
	if c := m.chat(); c != nil {
		s += fmt.Sprintf("/c:%s/%t/%t", c.Conversation.ID, c.History, c.Models)
	}
	s += fmt.Sprintf("/detail:%s/%p", m.detail, m.activityReview)
	if sources := m.chatSources; sources != nil {
		s += fmt.Sprintf("/sources:%t/%p/%t/%p", sources.List, sources.Document, sources.ShowReply, sources.Reply)
	}
	return s
}

func (m *model) mouseAllowed(msg tea.MouseMsg) bool {
	if m.settings.MouseDisabled || m.tooSmall() || m.mainTerminalCaptured() {
		return false
	}
	return msg.Mouse().Mod == 0 && m.mouseContext() == m.mouse.context
}

func (m *model) updateMouse(msg tea.MouseMsg) tea.Cmd {
	if !m.mouseAllowed(msg) {
		m.mouse.reset()
		return nil
	}
	point := msg.Mouse()
	span, hit := m.mouse.hit(point.X, point.Y)
	context := m.mouse.context
	now := time.Now()
	if m.mouse.now != nil {
		now = m.mouse.now()
	}
	if m.updateTextMouse(msg, span, hit, now, context) {
		return nil
	}
	if !hit {
		m.mouse.reset()
		return nil
	}
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if p := m.mouse.press; p != nil && (p.x != point.X || p.y != point.Y) {
			m.mouse.reset()
		}
	case tea.MouseWheelMsg:
		m.mouse.reset()
		prefixChanged := m.shortcutPrefix != ""
		m.shortcutPrefix = ""
		prefixChanged = m.clearDocumentPrefix() || prefixChanged
		if span.Scope == "chat" && m.externalAgentSelected() && !m.helpVisible() && !m.chatSourcesVisible() {
			cmd := m.nativeAgentWheel(point)
			if prefixChanged {
				m.nativeScroll.onlyNative, m.nativeScroll.deferView = false, false
			}
			return cmd
		}
		return m.mouseWheel(span, point.Button)
	case tea.MouseClickMsg:
		if point.Button != tea.MouseLeft {
			m.mouse.reset()
			return nil
		}
		m.shortcutPrefix = ""
		m.clearDocumentPrefix()
		m.mouse.press = &mouseGesture{span.Scope, span.Target, context, point.X, point.Y, now}
	case tea.MouseReleaseMsg:
		p := m.mouse.press
		m.mouse.press = nil
		if p == nil || (point.Button != tea.MouseLeft && point.Button != tea.MouseNone) || p.scope != span.Scope || p.target != span.Target || p.context != context || absMouse(p.x-point.X) > 1 || absMouse(p.y-point.Y) > 1 {
			m.mouse.last = nil
			return nil
		}
		double := false
		if last := m.mouse.last; last != nil {
			double = last.scope == p.scope && last.target == p.target && last.context == context && now.Sub(last.at) >= 0 && now.Sub(last.at) <= 400*time.Millisecond && absMouse(last.x-point.X) <= 1 && absMouse(last.y-point.Y) <= 1
		}
		p.at = now
		m.mouse.last = p
		cmd := m.activateMouse(span, double)
		if double || m.mouseContext() != context {
			m.mouse.reset()
		}
		m.persist()
		return cmd
	}
	return nil
}

func absMouse(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *model) mouseFocus(scope string) {
	focus := m.focus
	switch scope {
	case "tree":
		focus = 0
	case "main":
		focus = 1
	case "chat":
		focus = 2
		if n := m.nativeAgent; m.externalAgentSelected() && n != nil && !n.Finished {
			n.Captured = true
		}
	}
	if focus != m.focus {
		m.restoreView()
		m.focus = focus
	}
}

func (m *model) activateMouse(span textview.MouseSpan, double bool) tea.Cmd {
	if span.Target == "disabled" {
		return nil
	}
	m.mouseFocus(span.Scope)
	if span.Scope != "footer" {
		m.mouse.commandPaused = span.Target != "command"
		if span.Target != "search" && !strings.HasPrefix(span.Target, "command/") && !strings.HasPrefix(span.Target, "activity-type/") {
			if search := m.activePanelSearch(m.focus); search != nil {
				search.finished = true
			}
		}
	}
	kind, value, _ := strings.Cut(span.Target, "/")
	switch kind {
	case "overview-tile":
		if m.overviewVisible() && !m.helpVisible() && m.settingsOverlay == nil {
			s := &m.overviewCurrent().State
			index, err := strconv.Atoi(value)
			if err == nil && index >= 0 && index < 5 && s.MetricDetail == nil {
				if s.Input != nil {
					s.Query, s.Input = s.Input.Text, nil
				}
				s.Pane, s.Cursor[0] = 0, index
				_, cmd := m.overviewKey(localMouseKey("enter"))
				return cmd
			}
		}
	case "usage-sample":
		m.chartSampleClick(value)
	case "clipboard":
		if value == "copy" {
			return m.copySelection()
		}
	case "text":
		if target, ok := textview.ParseTextTarget(span.Target); ok && target.Owner == "input" {
			span.Target = "search"
			return m.activateMouse(span, false)
		}
	case "":
		return nil
	case "command":
		if value == "" {
			m.focus = 1
			return nil
		}
		m.command = nil
		if m.execute(value) {
			return tea.Quit
		}
	case "action":
		return m.mouseControl(value)
	case "composer":
		m.focusChat(true)
	case "models":
		m.openModelPicker()
	case "tree", "fold":
		for i, row := range m.treeRows() {
			if row.key != value {
				continue
			}
			m.treeCursor = i
			if kind == "fold" {
				m.expandTreeRow(row, !row.open)
			} else {
				m.updateTree("enter")
			}
			break
		}
	case "directory":
		return m.mouseDirectory(value, double)
	case "field", "option", "formaction":
		return m.mouseForm(kind, value)
	case "search":
		if m.gitOpsVisible() && m.focus == 1 {
			m.updateGitOps("/")
			return nil
		}
		if m.codeVisible() && m.focus == 1 {
			m.updateCode("/")
			return nil
		}

		if m.helmVisible() && m.focus == 1 {
			h := m.helmer()
			if h.Input == nil {
				m.updateHelm("/")
			}
		} else if m.exploreVisible() && m.focus == 1 && m.explore.Input != nil {
			return nil
		} else if s := m.activePanelSearch(m.focus); s != nil {
			s.finished = false
		} else {
			m.beginPanelSearch()
		}
	case "help":
		if m.helpVisible() {
			m.help.section, _ = strconv.Atoi(value)
		}
	case "choice":
		index, _, _ := strings.Cut(value, "/")
		i, err := strconv.Atoi(index)
		if err == nil {
			return m.mouseChoose(i, double)
		}
	default:
		return m.mouseItem(kind, value, double)
	}
	return nil
}

// Controls enter the owning adapter directly, never the global key dispatcher.
func (m *model) mouseControl(key string) tea.Cmd {
	if key == ":" {
		m.beginCommand()
		return nil
	}
	if strings.HasPrefix(key, ":") {
		m.command = nil
		if m.execute(strings.TrimPrefix(key, ":")) {
			return tea.Quit
		}
		return nil
	}
	switch key {
	case "hh":
		if m.execute("help") {
			return tea.Quit
		}
		return nil
	case "cc":
		m.focusChat(true)
		return nil
	case "bb":
		m.execute("sidebar")
		return nil
	case "ii":
		m.execute("aiinteraction")
		return nil
	case "ss":
		m.execute("settings")
		return nil
	case "ff":
		m.execute("fullscreen")
		return nil
	case "Enter":
		key = "enter"
	case "Esc":
		key = "esc"
	case "Space":
		key = "space"
	case "Backspace":
		key = "backspace"
	case "End":
		key = "end"
	case "Home":
		key = "home"
	}
	msg := localMouseKey(key)
	if key == "esc" && m.clearInputSelection() {
		return nil
	}
	if m.commandFocused() {
		if key == "esc" || key == "enter" {
			m.unparkOverlays()
		}
		if key == "esc" {
			m.command = nil
			m.focus = m.commandFocus
		}
		if key == "enter" {
			command := strings.TrimSpace(m.command.Text)
			m.command = nil
			m.focus = m.commandFocus
			if m.execute(command) {
				return tea.Quit
			}
		}
		return nil
	}
	if m.helpVisible() {
		m.updateHelp(key)
		return nil
	}
	if m.chatSourcesVisible() {
		return m.chatSourcesKey(msg)
	}
	if key == "esc" && m.restoreExpandedOnEscape() {
		return nil
	}
	if m.activePanelSearch(m.focus).ownsKey(key) {
		m.updatePanelSearch(msg)
		return nil
	}
	if m.focus == 2 {
		return m.updateChat(msg)
	}
	if m.focus == 0 {
		m.updateTree(key)
		return nil
	}
	if m.form != nil {
		m.updateForm(msg)
		return nil
	}
	if handled, cmd := m.featureKey(msg); handled {
		return cmd
	}
	if m.resourceActionVisible() {
		return m.updateResourceActionKey(msg)
	}
	if m.exploreVisible() && m.explore.Input != nil {
		m.updateExploreInput(msg)
		return nil
	}
	if m.gitOpsVisible() {
		m.updateGitOps(key)
		return nil
	}
	if key == "/" {
		m.beginPanelSearch()
		return nil
	}
	m.updateMain(key)
	return nil
}

func (m *model) commandFocused() bool {
	return m.command != nil && m.focus == 1 && !m.mouse.commandPaused
}

func localMouseKey(key string) tea.KeyPressMsg {
	code := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight, "space": ' ', "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "end": tea.KeyEnd, "home": tea.KeyHome}[key]
	if code == 0 {
		r := []rune(key)
		if len(r) == 1 {
			code = r[0]
		}
	}
	return tea.KeyPressMsg{Code: code}
}

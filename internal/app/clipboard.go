package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

type clipboardAccess struct {
	write func(context.Context, string) error
	read  func(context.Context) (string, error)
}

type clipboardCopyResult struct {
	fallback string
	native   bool
}

type clipboardPasteRequest struct {
	surface  mouseTextSurface
	field    textview.Field
	query    string
	revision uint64
	cancel   context.CancelFunc
}

type clipboardPasteResult struct {
	request *clipboardPasteRequest
	text    string
	err     error
}

func (m *model) cancelClipboardPaste() {
	if p := m.clipboardPaste; p != nil {
		p.cancel()
		m.clipboardPaste = nil
	}
}

func (m *model) copyWorkspaceText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	if m.clipboard.write == nil {
		m.status = "Sent to the terminal clipboard. Clipboard support depends on your terminal."
		return tea.SetClipboard(text)
	}
	m.status = "Copying to clipboard…"
	write := m.clipboard.write
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if write(ctx, text) == nil {
			return clipboardCopyResult{native: true}
		}
		return clipboardCopyResult{fallback: text}
	}
}

// Clipboard actions use the same focused text owner as mouse selection.
func (m *model) clipboardSurface() mouseTextSurface {
	owner := m.activeInteraction().owner
	if owner == interactionHelp || owner == interactionUnavailable || owner == interactionTerminal {
		return mouseTextSurface{}
	}
	main := m.focus == 1 && !m.chatSourcesVisible() && m.form == nil && m.settingsOverlay == nil && !m.commandFocused()
	if main && m.codeVisible() {
		s := m.coder()
		if s.Menu != "" || s.Discard || s.Busy {
			return mouseTextSurface{}
		}
	}
	if main && m.helmVisible() {
		h := m.helmer()
		if h.Menu != nil || h.Discard || h.Confirm || h.Busy {
			return mouseTextSurface{}
		}
	}
	if s := m.inputSurface(); s.input != nil || s.query != nil && s.reader != nil && s.reader.Searching {
		return s
	}
	if m.commandFocused() || !m.chatSourcesVisible() && m.focus == 1 && (m.form != nil || m.settingsOverlay != nil) {
		return mouseTextSurface{}
	}
	scope := "main"
	if m.chatSourcesVisible() {
		scope = "sources"
	} else if m.focus != 1 {
		return mouseTextSurface{}
	}
	s := m.mouseTextSurface(scope, "query")
	if s.document != nil && s.document.Searching || s.reader != nil && s.reader.Searching {
		return s
	}
	s.query, s.queryText = nil, nil
	return s
}

func (s mouseTextSurface) hasSelection() bool {
	switch {
	case s.query != nil:
		_, _, selected := s.query.Position(*s.queryText)
		return selected
	case s.input != nil:
		return s.input.Selecting && !strings.HasPrefix(s.owner, "secret/")
	case s.document != nil:
		return s.document.Mode == docedit.Select
	case s.reader != nil:
		return s.reader.Selecting
	}
	return false
}

func (s mouseTextSurface) selectedText() string {
	if !s.hasSelection() {
		return ""
	}
	switch {
	case s.query != nil:
		cursor, anchor, _ := s.query.Position(*s.queryText)
		field := textview.Field{Text: *s.queryText, Cursor: cursor, Anchor: anchor, Selecting: true}
		a, b := field.Selection()
		return string([]rune(field.Text)[a:b])
	case s.input != nil:
		a, b := s.input.Selection()
		return string([]rune(s.input.Text)[a:b])
	case s.document != nil:
		return s.document.SelectedText()
	case s.reader != nil:
		return s.reader.SelectedText()
	}
	return ""
}

func (m *model) copySelection() tea.Cmd {
	text := m.clipboardSurface().selectedText()
	if text == "" {
		m.status = "Select text to copy."
		return nil
	}
	return m.copyWorkspaceText(text)
}

func (m *model) requestClipboardPaste() tea.Cmd {
	s := m.clipboardSurface()
	allowed := s.query != nil || s.input != nil && s.owner != "command" || s.document != nil && !s.document.ReadOnly && !s.document.Controls
	if search := m.searches[m.focus]; s.input != nil && search != nil && s.input == &search.input && search.finished {
		allowed = false
	}
	if !allowed {
		return nil
	}
	if m.clipboard.read == nil {
		m.status = "Use your terminal's Paste command to insert clipboard text."
		return nil
	}
	m.cancelClipboardPaste()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	p := &clipboardPasteRequest{surface: s, cancel: cancel}
	if s.input != nil {
		p.field = *s.input
	}
	if s.query != nil {
		p.query = *s.queryText
	}
	if s.document != nil {
		p.revision = s.document.Revision
	}
	m.clipboardPaste = p
	read := m.clipboard.read
	return func() tea.Msg {
		defer cancel()
		text, err := read(ctx)
		return clipboardPasteResult{p, text, err}
	}
}

func (m *model) finishClipboardPaste(result clipboardPasteResult) {
	p := result.request
	if p == nil || m.clipboardPaste != p {
		return
	}
	m.cancelClipboardPaste()
	s := m.clipboardSurface()
	if s != p.surface || s.input != nil && *s.input != p.field || s.query != nil && *s.queryText != p.query || s.document != nil && s.document.Revision != p.revision {
		return
	}
	if result.err != nil {
		m.status = "Could not read clipboard. Use your terminal's Paste command."
		return
	}
	if result.text == "" {
		return
	}
	m.pasteText(result.text)
}

func (m *model) clipboardKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.mainTerminalCaptured() {
		return false, nil
	}
	mod := msg.Mod &^ (tea.ModCapsLock | tea.ModNumLock)
	if mod != tea.ModSuper && mod != tea.ModCtrl|tea.ModShift {
		return false, nil
	}
	switch msg.Code {
	case 'c', 'C':
		m.shortcutPrefix = ""
		m.clearDocumentPrefix()
		return true, m.copySelection()
	case 'v', 'V':
		m.shortcutPrefix = ""
		m.clearDocumentPrefix()
		return true, m.requestClipboardPaste()
	}
	return false, nil
}

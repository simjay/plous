package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"slices"
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
	if m.screenSelectionActive() {
		text := m.mouse.screen.Text()
		if m.mouse.screenScope == "footer" {
			m.mouse.reset()
		}
		return m.copyWorkspaceText(text)
	}
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

func clipboardShortcut(msg tea.KeyPressMsg) string {
	mod := msg.Mod &^ (tea.ModCapsLock | tea.ModNumLock)
	if mod != tea.ModSuper && mod != tea.ModCtrl|tea.ModShift {
		return ""
	}
	switch msg.Code {
	case 'c', 'C':
		return "copy"
	case 'v', 'V':
		return "paste"
	}
	return ""
}

func (m *model) clipboardKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.mainTerminalCaptured() {
		return false, nil
	}
	switch clipboardShortcut(msg) {
	case "copy":
		m.shortcutPrefix = ""
		m.clearDocumentPrefix()
		return true, m.copySelection()
	case "paste":
		m.shortcutPrefix = ""
		m.clearDocumentPrefix()
		return true, m.requestClipboardPaste()
	}
	return false, nil
}

func (m *model) pasteText(text string) {
	m.mouse.reset()
	m.shortcutPrefix = ""
	if m.nativeAgentCaptured() {
		m.nativeAgent.Follow()
		m.nativeAgent.Screen.Paste(text)
		return
	}
	owner := m.activeInteraction().owner
	if owner == interactionHelp || owner == interactionCommand || owner == interactionUnavailable {
		return
	}
	if owner == interactionSources {
		if s := m.chatSources; s.ShowReply && s.Reply.Reader.Searching && m.command == nil {
			s.Reply.Reader.Paste(text)
		}
		if d := m.chatSources.Document; d != nil && d.Searching && m.command == nil {
			d.Paste(text)
		}
		return
	}
	if m.focus == 1 && m.overviewVisible() {
		if d := m.overviewCurrent().State.MetricDetail; d != nil {
			d.Reader().Paste(text)
		} else if input := m.overviewCurrent().State.Input; input != nil {
			input.Insert(text, false)
			m.overviewCurrent().State.Cursor = [4]int{}
		}
		return
	}
	if m.focus == 1 && m.presentedDocumentVisible() {
		m.uiDocument.model.Paste(text)
		return
	}

	if m.gitOpsPaste(text) {
		return
	}
	if m.helmPaste(text) {
		return
	}
	if m.workspacePaste(text) {
		return
	}
	if m.resourceTerminalVisible() && m.explore.Terminal.Captured {
		m.explore.Terminal.Emulator.Paste(text)
		return
	}
	if m.resourceActionVisible() {
		a := m.explore.Action
		if !a.Busy && (slices.Contains([]string{"scale", "export"}, a.Stage) || a.Stage == "execcommand" && a.ExecEditing) {
			a.Input.Insert(text, a.Stage == "edit")
		}
		return
	}
	// Handle paste before key dispatch so pasted commands stay literal.
	if search := m.activePanelSearch(m.focus); search != nil {
		if !search.finished {
			search.input.Insert(text, false)
			m.searchChanged()
		}
	} else if m.focus == 2 && m.chatEditing && !m.chat().History && !m.chat().Models {
		m.draft().Insert(text, true)
		m.draft().Literal = true
		m.persist()
	} else if m.focus == 1 && m.form != nil {
		before := m.form.Fields[0].Value.Text
		m.form.Paste(text)
		if m.form.Kind == "mainagent" && m.form.Index < len(m.form.Fields) && m.form.Fields[m.form.Index].Key == "entry_command" {
			m.nativeChange = nil
			m.status = ""
		}
		if m.form.Kind == "kube" && m.form.Fields[0].Value.Text != before {
			m.invalidateContexts()
		}
	} else if m.exploreEditing() && m.explore.Input != nil {
		m.explore.Input.Insert(text, false)
	}
}

const clipboardLimit = 2 * 1024 * 1024

func systemClipboard() clipboardAccess {
	return clipboardFor(runtime.GOOS, os.Getenv, exec.LookPath)
}

func clipboardFor(platform string, getenv func(string) string, lookPath func(string) (string, error)) clipboardAccess {
	// Remote sessions must address the user's terminal, not the server's desktop.
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" || getenv("MOSH_IP") != "" {
		return clipboardAccess{}
	}
	var writer, reader []string
	find := func(name string, args ...string) []string {
		path, err := lookPath(name)
		if err != nil {
			return nil
		}
		return append([]string{path}, args...)
	}
	switch platform {
	case "darwin":
		writer, reader = find("pbcopy"), find("pbpaste", "-Prefer", "txt")
	case "linux":
		if getenv("WAYLAND_DISPLAY") != "" {
			writer, reader = find("wl-copy"), find("wl-paste", "--no-newline")
		}
		if getenv("DISPLAY") != "" {
			if writer == nil {
				writer = find("xclip", "-selection", "clipboard", "-in")
			}
			if reader == nil {
				reader = find("xclip", "-selection", "clipboard", "-out")
			}
			if writer == nil {
				writer = find("xsel", "--clipboard", "--input")
			}
			if reader == nil {
				reader = find("xsel", "--clipboard", "--output")
			}
		}
	}
	var access clipboardAccess
	if len(writer) > 0 {
		access.write = func(ctx context.Context, text string) error {
			cmd := exec.CommandContext(ctx, writer[0], writer[1:]...)
			cmd.WaitDelay = 100 * time.Millisecond
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	if len(reader) > 0 {
		access.read = func(ctx context.Context) (string, error) {
			cmd := exec.CommandContext(ctx, reader[0], reader[1:]...)
			cmd.WaitDelay = 100 * time.Millisecond
			var buffer clipboardBuffer
			cmd.Stdout = &buffer
			if err := cmd.Run(); err != nil {
				return "", err
			}
			return buffer.data.String(), nil
		}
	}
	return access
}

type clipboardBuffer struct{ data bytes.Buffer }

func (b *clipboardBuffer) Write(p []byte) (int, error) {
	if len(p) > clipboardLimit-b.data.Len() {
		return 0, errors.New("clipboard exceeds document limit")
	}
	return b.data.Write(p)
}

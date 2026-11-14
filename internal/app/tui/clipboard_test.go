package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func copyWithShortcut(t *testing.T, m *model, mod tea.KeyMod) string {
	t.Helper()
	var copied string
	m.clipboard.write = func(_ context.Context, text string) error { copied = text; return nil }
	handled, cmd := m.clipboardKey(tea.KeyPressMsg{Code: 'c', Mod: mod})
	if !handled || cmd == nil {
		t.Fatal("copy shortcut did not reach the clipboard")
	}
	result := cmd()
	m.Update(result)
	if m.status != "Copied to clipboard." {
		t.Fatal(m.status)
	}
	return copied
}

func TestClipboardCopiesFocusedUnicodeSelectionWithoutEditing(t *testing.T) {
	for _, mod := range []tea.KeyMod{tea.ModSuper, tea.ModCtrl | tea.ModShift} {
		t.Run(fmt.Sprint(mod), func(t *testing.T) {
			m, _ := testModel(t)
			m.focusChat(true)
			*m.draft() = textview.NewField("one 界cafe\u0301 two")
			m.draft().Anchor, m.draft().Cursor, m.draft().Selecting = 4, 8, true
			before := *m.draft()
			if got := copyWithShortcut(t, m, mod); got != "界cafe\u0301" {
				t.Fatal(got)
			}
			if *m.draft() != before || m.quitting || m.run != nil {
				t.Fatal("copy changed or submitted the draft")
			}
		})
	}

	m, a := documentModel(t)
	a.Document = docedit.New("first 界\nsecond text\n")
	a.Document.Mode, a.Document.Anchor, a.Document.Cursor = docedit.Select, 6, 13
	before := a.Document.Text()
	if got := copyWithShortcut(t, m, tea.ModSuper); got != "界\nsecond" {
		t.Fatal(got)
	}
	if a.Document.Text() != before || a.SaveRequested || a.Document.Mode != docedit.Select {
		t.Fatal("copy changed editor selection or supplied save intent")
	}
}

func TestClipboardCopyButtonToleratesPointerMovement(t *testing.T) {
	for _, surface := range []string{"screen", "field"} {
		t.Run(surface, func(t *testing.T) {
			m, _ := testModel(t)
			m.width, m.height = 160, 36
			if surface == "screen" {
				selectScreenText(t, m, "Current Workspace")
			} else {
				m.focusChat(true)
				*m.draft() = textview.Field{Text: "selected text", Cursor: 12, Selecting: true}
			}
			var copied string
			m.clipboard.write = func(_ context.Context, text string) error { copied = text; return nil }
			p := mousePoint(t, m, "footer", "clipboard/copy")
			m.updateMouse(tea.MouseClickMsg{X: p.X + 3, Y: p.Y, Button: tea.MouseLeft})
			m.View()
			m.updateMouse(tea.MouseMotionMsg{X: p.X + 4, Y: p.Y, Button: tea.MouseLeft})
			m.View()
			cmd := m.updateMouse(tea.MouseReleaseMsg{X: p.X + 4, Y: p.Y, Button: tea.MouseLeft})
			if cmd == nil {
				t.Fatal("a one-cell pointer movement cancelled Copy")
			}
			m.Update(cmd())
			want := "selected text"
			if surface == "screen" {
				want = "Current Workspace"
			}
			if copied != want {
				t.Fatalf("copied %q, want %q", copied, want)
			}
		})
	}
}

func TestClipboardCancelledCopyClickPreservesSelection(t *testing.T) {
	m, _ := testModel(t)
	selectScreenText(t, m, "Current Workspace")
	p := mousePoint(t, m, "footer", "clipboard/copy")
	m.updateMouse(tea.MouseClickMsg{X: p.X + 3, Y: p.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseMotionMsg{X: p.X + 6, Y: p.Y, Button: tea.MouseLeft})
	m.View()
	cmd := m.updateMouse(tea.MouseReleaseMsg{X: p.X + 6, Y: p.Y, Button: tea.MouseLeft})
	if cmd != nil || !m.screenSelectionActive() {
		t.Fatal("a cancelled Copy click copied text or discarded the selection")
	}
	copyScreenButton(t, m, "Current Workspace")
}

func TestClipboardCopyButtonUsesLogSourceAndRespectsEviction(t *testing.T) {
	m := exploreModel(t)
	m.width, m.height = 200, 50
	m.openExploreSelection(4)
	m.explore.DetailValid = true
	m.explore.Log = &monitor.ExploreLogRun{Cancel: func() {}}
	v := m.podLogViewer()
	v.Append("first evidence\nsecond evidence\n")
	v.Pause()
	v.Reader.Anchor, v.Reader.Cursor, v.Reader.Selecting = textview.Point{}, textview.Point{Row: 1, Col: 5}, true
	if !m.logWorkspaceActive() || !m.clipboardSurface().hasSelection() {
		t.Fatalf("log=%v surface=%+v status=%s", m.logWorkspaceActive(), m.clipboardSurface(), m.status)
	}
	m.status = ""
	cmd := clickMouse(t, m, "footer", "clipboard/copy")
	if cmd == nil || fmt.Sprint(cmd()) != "first evidence\nsecond" || m.explore.Log == nil {
		t.Fatal("Copy button lost source text or stopped collection")
	}
	v.Reader.Shift(1)
	if m.clipboardSurface().hasSelection() || m.copySelection() != nil {
		t.Fatal("evicted selection copied replacement text")
	}
}

func TestClipboardNeverCopiesHiddenFieldsOrBackgroundSelections(t *testing.T) {
	m, _ := testModel(t)
	m.beginWorkspace()
	f := &m.form.Fields[0]
	f.Secret = true
	f.Value = textview.Field{Text: "private fixture", Cursor: 6, Selecting: true}
	if m.copySelection() != nil || m.clipboardSurface().hasSelection() {
		t.Fatal("masked credential field was copied")
	}
	f.Secret = false
	m.openHelp()
	if m.copySelection() != nil {
		t.Fatal("Help copied an underlying field")
	}
	m.closeHelp()
	m.form.Index = len(m.form.Fields)
	if m.copySelection() != nil {
		t.Fatal("form actions copied an unfocused field")
	}
}

func TestClipboardPasteIsLiteralAndOneEditorUndo(t *testing.T) {
	m, _ := testModel(t)
	m.focusChat(true)
	m.clipboard.read = func(context.Context) (string, error) { return "/model\n:quit 界", nil }
	_, cmd := m.clipboardKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModSuper})
	if cmd == nil {
		t.Fatal("paste was not requested")
	}
	m.Update(cmd())
	if m.draft().Text != "/model\n:quit 界" || !m.draft().Literal || m.command != nil || m.quitting || m.run != nil {
		t.Fatal("clipboard content was interpreted as a command")
	}
	m, a := documentModel(t)
	a.Document = docedit.New("old text")
	a.Document.Mode, a.Document.Anchor, a.Document.Cursor = docedit.Select, 0, 2
	m.clipboard.read = func(context.Context) (string, error) { return "new\n界", nil }
	cmd = m.requestClipboardPaste()
	if cmd == nil {
		t.Fatal("editor paste was not requested")
	}
	m.Update(cmd())
	if a.Document.Text() != "new\n界 text" || a.SaveRequested {
		t.Fatal("paste did not replace selection literally")
	}
	a.Document.Undo()
	if a.Document.Text() != "old text" || a.Document.Mode != docedit.Select {
		t.Fatal("paste was not one undoable edit")
	}
	m.clipboard.read = func(context.Context) (string, error) { return "", nil }
	m.Update(m.requestClipboardPaste()())
	if a.Document.Text() != "old text" || a.Document.Mode != docedit.Select {
		t.Fatal("empty clipboard deleted the selection")
	}
}

func TestClipboardPasteCannotReachChangedInputOrHiddenViews(t *testing.T) {
	for _, change := range []string{"key", "paste", "mouse", "resize", "blur", "replace", "text"} {
		t.Run(change, func(t *testing.T) {
			m, _ := testModel(t)
			m.focusChat(true)
			m.clipboard.read = func(context.Context) (string, error) { return "late clipboard", nil }
			cmd := m.requestClipboardPaste()
			request := m.clipboardPaste
			switch change {
			case "key":
				m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			case "paste":
				m.Update(tea.PasteMsg{Content: "native paste"})
			case "mouse":
				m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
			case "resize":
				m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			case "blur":
				m.Update(tea.BlurMsg{})
			case "replace":
				m.drafts[m.conversationScope()] = &textview.Field{}
			case "text":
				m.draft().Insert("new draft", true)
			}
			before := m.draft().Text
			m.Update(cmd())
			m.finishClipboardPaste(clipboardPasteResult{request: request, text: "duplicate reply"})
			if m.draft().Text != before {
				t.Fatal("late or duplicate clipboard result reached another input")
			}
		})
	}
	m, _ := testModel(t)
	m.clipboard.read = func(context.Context) (string, error) { t.Fatal("unexpected clipboard read"); return "", nil }
	if m.requestClipboardPaste() != nil {
		t.Fatal("navigation requested clipboard")
	}
	m.beginCommand()
	if m.requestClipboardPaste() != nil {
		t.Fatal("colon input requested clipboard")
	}
}

func TestClipboardFallbackAndFailureDoNotClaimSuccessOrExposeContent(t *testing.T) {
	m, _ := testModel(t)
	m.clipboard.write = func(context.Context, string) error { return errors.New("private helper output") }
	result := m.copyWorkspaceText("fixture selected text")().(clipboardCopyResult)
	if result.native || result.fallback != "fixture selected text" {
		t.Fatal("native failure lost terminal fallback")
	}
	m.Update(result)
	if !strings.Contains(m.status, "depends on your terminal") || strings.Contains(m.status, "private") {
		t.Fatal(m.status)
	}
	m.focusChat(true)
	m.clipboard.read = func(context.Context) (string, error) { return "private clipboard", errors.New("private error") }
	m.Update(m.requestClipboardPaste()())
	if m.draft().Text != "" || strings.Contains(m.status, "private") {
		t.Fatal("failed clipboard read exposed content")
	}
}

func TestClipboardSearchAndSourcesKeepTheirInputOwnership(t *testing.T) {
	m, a := documentModel(t)
	a.Document = docedit.New("underlying document")
	a.Document.Key("/", "")
	a.Document.Paste("find 界 text")
	a.Document.SearchInput.Pointer(a.Document.Query, textview.Point{Col: 5}, nil, true)
	if got := m.clipboardSurface().selectedText(); got != "界" {
		t.Fatal("copy did not use the active query", got)
	}
	m.clipboard.read = func(context.Context) (string, error) { return ":quit", nil }
	m.Update(m.requestClipboardPaste()())
	if a.Document.Query != "find :quit text" || a.Document.Text() != "underlying document" || m.quitting {
		t.Fatal("search paste changed the document or dispatched a command")
	}

	m, _ = testModel(t)
	m.beginWorkspace()
	d := docedit.New("saved source")
	d.Mode, d.Anchor, d.Cursor, d.ReadOnly = docedit.Select, 0, 4, true
	m.chatSources = &appchat.ChatSources{Document: d}
	if got := m.clipboardSurface().selectedText(); got != "saved" {
		t.Fatal("Sources did not own copy above an unfinished form", got)
	}
	if m.requestClipboardPaste() != nil {
		t.Fatal("read-only Sources requested a paste")
	}
}

func TestClipboardShortcutsPreserveCapturedExecKeys(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.Terminal = reviewTerminal(t, 80, 24)
	for _, code := range []rune{'c', 'v'} {
		if handled, _ := m.clipboardKey(tea.KeyPressMsg{Code: code, Mod: tea.ModSuper}); handled {
			t.Fatal("clipboard intercepted captured Exec input")
		}
	}
	if m.clipboardSurface().valid() {
		t.Fatal("captured Exec exposed a background clipboard surface")
	}
}

func TestClipboardBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		name, platform string
		env            map[string]string
		available      []string
		lookups        []string
		read, write    bool
	}{
		{"macOS", "darwin", nil, []string{"pbcopy", "pbpaste"}, []string{"pbcopy", "pbpaste"}, true, true},
		{"Wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-copy", "wl-paste"}, []string{"wl-copy", "wl-paste"}, true, true},
		{"X11", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xclip"}, []string{"xclip", "xclip"}, true, true},
		{"X11 xsel", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, []string{"xclip", "xclip", "xsel", "xsel"}, true, true},
		{"SSH", "darwin", map[string]string{"SSH_CONNECTION": "fixture"}, nil, nil, false, false},
		{"SSH tty", "linux", map[string]string{"SSH_TTY": "/dev/pts/1", "DISPLAY": ":0"}, nil, nil, false, false},
		{"Mosh", "linux", map[string]string{"MOSH_IP": "fixture", "WAYLAND_DISPLAY": "wayland-0"}, nil, nil, false, false},
		{"headless", "linux", nil, nil, nil, false, false},
		{"missing helpers", "linux", map[string]string{"DISPLAY": ":0"}, nil, []string{"xclip", "xclip", "xsel", "xsel"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var looked []string
			a := clipboardFor(tc.platform, func(key string) string { return tc.env[key] }, func(name string) (string, error) {
				looked = append(looked, name)
				for _, available := range tc.available {
					if name == available {
						return "/fixture/" + name, nil
					}
				}
				return "", errors.New("not installed")
			})
			if (a.read != nil) != tc.read || (a.write != nil) != tc.write || !reflect.DeepEqual(looked, tc.lookups) {
				t.Fatalf("read=%v write=%v lookups=%v", a.read != nil, a.write != nil, looked)
			}
		})
	}
}

func TestClipboardHelpersPreserveUnicodeAndBoundReads(t *testing.T) {
	dir := t.TempDir()
	storage := filepath.Join(dir, "clipboard")
	t.Setenv("PLOUS_TEST_CLIPBOARD", storage)
	for name, script := range map[string]string{
		"pbcopy":  "#!/bin/sh\ncat > \"$PLOUS_TEST_CLIPBOARD\"\n",
		"pbpaste": "#!/bin/sh\ncat \"$PLOUS_TEST_CLIPBOARD\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	a := clipboardFor("darwin", func(string) string { return "" }, func(name string) (string, error) { return filepath.Join(dir, name), nil })
	text := "literal $(touch forbidden) `command`\n界 cafe\u0301\n"
	if err := a.write(t.Context(), text); err != nil {
		t.Fatal(err)
	}
	if got, err := a.read(t.Context()); err != nil || got != text {
		t.Fatalf("clipboard = %q, %v", got, err)
	}
	if err := os.WriteFile(storage, []byte(strings.Repeat("x", clipboardLimit+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := a.read(t.Context()); err == nil || got != "" {
		t.Fatal("oversized clipboard was silently truncated")
	}
	if err := os.WriteFile(filepath.Join(dir, "pbpaste"), []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got, err := a.read(ctx); err == nil || got != "" || time.Since(start) > time.Second {
		t.Fatal("clipboard helper ignored cancellation")
	}
}

package editor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestReadOnlyRejectsMutationAndRetainsSearchAndCopy(t *testing.T) {
	m := New("name: original\nitems: [one, two]\n")
	m.Paste("# history\n")
	m.Undo()
	m.ReadOnly = true
	before, revision := m.Text(), m.Revision
	for _, key := range []string{"i", "a", "o", "x", "d", "d", "p", "u", "U", ">", "<", "ctrl+s", "enter", "tab", "backspace", "delete"} {
		event := m.Key(key, key)
		if event.Action != "" || event.Copy != "" {
			t.Fatalf("read-only key emitted write action: %s %#v", key, event)
		}
	}
	m.Paste("replace")
	m.Undo()
	m.Redo()
	m.Indent(false)
	m.Indent(true)
	m.Mode = Insert
	m.Key("x", "x")
	if m.Text() != before || m.Revision != revision || m.Dirty() || m.Mode == Insert {
		t.Fatal("read-only mutation changed document")
	}
	m.Key("/", "")
	m.Paste("original")
	m.Key("enter", "")
	m.Key("v", "")
	m.Key("e", "")
	if got := m.Key("y", "").Copy; got != "original" {
		t.Fatalf("copy changed source: %q", got)
	}
	if m.Text() != before {
		t.Fatal("search/copy changed text")
	}
}

func TestReadOnlyRefreshClearsRevealedStateAndGuardsEditableDraft(t *testing.T) {
	m := New("secret: private\nsecond: value\n")
	m.ReadOnly = true
	m.Key("/", "")
	m.Paste("private")
	m.Key("enter", "")
	m.Key("v", "")
	m.RefreshReadOnly("secret: hidden\nsecond: value\n")
	if m.Query != "" || m.Searching || m.Mode != Normal || m.Dirty() || strings.Contains(m.SelectedText(), "private") || len(m.undo) > 0 || len(m.redo) > 0 {
		t.Fatal("refresh retained revealed state")
	}
	m.ReadOnly = false
	m.Paste("draft")
	text, revision := m.Text(), m.Revision
	m.RefreshReadOnly("replacement")
	if m.Text() != text || m.Revision != revision {
		t.Fatal("refresh replaced editing draft")
	}
}

func TestLineNumbersAndViewportArePresentationOnly(t *testing.T) {
	m := New(strings.Repeat("long: "+strings.Repeat("界", 80)+"\n", 80))
	m.ReadOnly, m.Syntax = true, "yaml"
	m.Cursor = 2400
	before := m.Text()
	m.View(35, 15, textview.DefaultTheme())
	top, left := m.top, m.left
	restore := m.RestoreViewport()
	m.View(160, 35, textview.DefaultTheme())
	restore()
	if m.top != top || m.left != left {
		t.Fatal("viewport not restored")
	}
	for _, numbers := range []bool{true, false} {
		m.LineNumbers = numbers
		view := ansi.Strip(m.View(35, 15, textview.DefaultTheme()))
		if strings.Contains(view, "│") != numbers || m.Text() != before || m.Dirty() {
			t.Fatal("gutter toggle changed document")
		}
	}
}

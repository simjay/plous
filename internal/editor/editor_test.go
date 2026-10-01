package editor

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/textview"
	"strings"
	"testing"
)

func TestTransactionsModesAndLiteralPaste(t *testing.T) {
	m := New("name: old\n")
	m.Key("i", "")
	m.Key("x", "x")
	m.Key("x", "x")
	m.Key("esc", "")
	m.Key("u", "u")
	if m.Text() != "name: old\n" || m.Mode != Normal {
		t.Fatal("insert transaction not restored")
	}
	m.Key("U", "U")
	if m.Text() != "xxname: old\n" {
		t.Fatal("redo lost insert")
	}
	m.Key("V", "")
	m.Key("G", "")
	m.Paste("bb cc :quit\n\t界👩‍💻é")
	if m.Text() != "bb cc :quit\n\t界👩‍💻é" {
		t.Fatalf("paste changed %q", m.Text())
	}
	m.Undo()
	if m.Text() != "xxname: old\n" || m.Mode != Select {
		t.Fatal("undo did not restore selection")
	}
}
func TestUnicodeMotionSelectionAndBracketMatching(t *testing.T) {
	m := New("界👩‍💻é [one]\nnext")
	m.Key("l", "")
	m.Key("l", "")
	if m.Cursor != 4 {
		t.Fatalf("split emoji: %d", m.Cursor)
	}
	m.Key("v", "")
	if m.SelectedText() != "é" {
		t.Fatalf("split grapheme: %q", m.SelectedText())
	}
	m.Key("esc", "")
	m.Key("l", "")
	m.Key("l", "")
	if m.matchingBracket() != 11 {
		t.Fatalf("matching bracket: %d", m.matchingBracket())
	}
}
func TestIndentSelectedLinesAndUndo(t *testing.T) {
	m := New("one\ntwo\nthree")
	m.Key("V", "")
	m.Key("j", "")
	m.Indent(false)
	if m.Text() != "  one\n  two\nthree" {
		t.Fatalf("indent leaked: %q", m.Text())
	}
	m.Undo()
	if m.Text() != "one\ntwo\nthree" {
		t.Fatal("indent undo")
	}
}
func TestSearchLimitsAndRenderingDoNotChangeDocument(t *testing.T) {
	m := New("name: [界, 'value'] # comment\nnext: 42\n")
	m.Syntax = "yaml"
	m.Key("/", "")
	m.Paste("value")
	m.Key("enter", "")
	if m.Cursor != 11 {
		t.Fatalf("search cursor: %d", m.Cursor)
	}
	before := m.Text()
	m.Paste(strings.Repeat("a", documentLimit))
	if m.Text() != before || !strings.Contains(m.Notice, "2 MiB") {
		t.Fatal("oversized paste changed document")
	}
	for _, w := range []int{28, 48, 100} {
		output := m.View(w, 16, textview.DefaultTheme())
		for _, line := range strings.Split(output, "\n") {
			if ansi.StringWidth(line) > w {
				t.Fatal("width overflow")
			}
		}
	}
	if m.Text() != before {
		t.Fatal("render changed document")
	}
}

func TestRedoKeyKeepsInsertAndSearchLiteral(t *testing.T) {
	m := New("name: old\n")
	m.Paste("draft ")
	m.Key("u", "u")
	m.Key("/", "/")
	m.Key("U", "U")
	if m.Query != "U" || m.Text() != "name: old\n" {
		t.Fatal("U during search changed the document instead of the query")
	}
	m.Key("esc", "")
	m.Key("v", "v")
	m.Key("U", "U")
	if m.Text() != "draft name: old\n" {
		t.Fatal("U in Select did not redo the edit")
	}
	m.Key("u", "u")
	m.Key("i", "i")
	m.Key("U", "U")
	if m.Text() != "Uname: old\n" || m.Mode != Insert {
		t.Fatal("U during Insert triggered redo instead of typing")
	}
}

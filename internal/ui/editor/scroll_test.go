package editor

import (
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func TestMouseScrollPreservesInsertSelectionAndUndo(t *testing.T) {
	m := New(strings.Repeat("line\n", 80))
	m.Mode = Insert
	m.Paste("draft")
	m.View(40, 12, textview.DefaultTheme())
	before, cursor, revision := m.Text(), m.Cursor, m.Revision
	m.Scroll(20)
	m.View(40, 12, textview.DefaultTheme())
	if m.top == 0 || m.Cursor != cursor || m.Text() != before || m.Revision != revision || m.Mode != Insert {
		t.Fatal("scroll changed document or followed cursor")
	}
	m.ResumeSearch()
	m.Key("esc", "")
	if m.Mode != Insert {
		t.Fatal("search lost mode")
	}
	m.Undo()
	if m.Text() == before {
		t.Fatal("scroll lost undo")
	}
}

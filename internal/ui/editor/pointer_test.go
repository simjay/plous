package editor

import (
	"testing"

	textview "github.com/simjay/plous/internal/ui/text"
)

func TestPointerPreservesDraftModeAndUndo(t *testing.T) {
	m := New("key: cafe\u0301\nnext: 界")
	m.Mode = Insert
	m.Paste("# initial\n")
	before, revision := m.Text(), m.Revision
	m.Pointer(textview.Point{Row: 1, Col: 5}, nil, false)
	if m.Mode != Insert || m.Text() != before || m.Revision != revision {
		t.Fatal("click changed draft or insert mode")
	}
	m.Pointer(textview.Point{Row: 1, Col: 6}, nil, true)
	if m.SelectedText() != "cafe\u0301" {
		t.Fatal("word split a grapheme", m.SelectedText())
	}
	m.Paste("tea")
	if m.Text() != "# initial\nkey: tea\nnext: 界" {
		t.Fatal(m.Text())
	}
	m.Undo()
	if m.Text() != before {
		t.Fatal("selection edit lost undo history")
	}
	m.Undo()
	if m.Text() != "key: cafe\u0301\nnext: 界" {
		t.Fatal("pointer cleared earlier undo")
	}
}

func TestReadOnlyPointerSelectionAndGutter(t *testing.T) {
	m := New("alpha\n界e\u0301\tend")
	m.ReadOnly = true
	_, spans := textview.MouseLayout(textview.MouseScope(m.View(20, 12, textview.DefaultTheme()), "main"))
	for _, span := range spans {
		if target, ok := textview.ParseTextTarget(span.Target); ok && span.X < 4 {
			t.Fatal("gutter mapped as text", span, target)
		}
	}
	anchor := textview.Point{Row: 1, Col: 0}
	m.Pointer(textview.Point{Row: 1, Col: 1}, &anchor, false)
	if m.SelectedText() != "界e\u0301" {
		t.Fatal(m.SelectedText())
	}
	m.Paste("overwrite")
	if m.Text() != "alpha\n界e\u0301\tend" || m.Dirty() {
		t.Fatal("read-only selection permitted editing")
	}
	m.Key("esc", "")
	if m.Mode != Normal {
		t.Fatal("Escape did not clear selection")
	}
}

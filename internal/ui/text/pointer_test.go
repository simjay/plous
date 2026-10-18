package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPointerColumnsAndWordsUseGraphemeBoundaries(t *testing.T) {
	text := "A界e\u0301\tZ"
	for cell, want := range []int{0, 1, 1, 2, 4, 4, 4, 4, 5, 6, 6} {
		if got := ColumnAt(text, cell); got != want {
			t.Fatalf("cell %d: got %d want %d", cell, got, want)
		}
	}
	for _, test := range []struct {
		text            string
		col, start, end int
	}{
		{"one cafe\u0301 two", 6, 4, 9}, {"界面 ready", 1, 0, 2}, {"a 👩‍💻 b", 3, 2, 5}, {"a  b", 1, 1, 3},
	} {
		a, b := WordBounds(test.text, test.col)
		if a != test.start || b != test.end {
			t.Fatalf("%q: %d:%d", test.text, a, b)
		}
	}
}

func TestPointerMapFollowsWrappedTextAndExcludesPrefixes(t *testing.T) {
	v := Viewer{Wrap: true}
	v.SetLines([]Line{{Text: "a界bc\tde\u0301f", Prefix: "1+", ContinuationIndent: 2}, {Text: "tail"}})
	view, spans := MouseLayout(MouseScope(v.View(9, 8, DefaultTheme()), "main"))
	rows := strings.Split(ansi.Strip(view), "\n")
	seen := false
	for _, span := range spans {
		target, ok := ParseTextTarget(span.Target)
		if !ok || target.Owner != "reader" {
			continue
		}
		if target.Row == 0 && span.X < 2 {
			t.Fatal("line number became source text", span)
		}
		if target.Row == 0 && target.Offset > 0 {
			seen = true
			col := ColumnAt(v.Lines[0].Text, target.Offset)
			v.Pointer(Point{0, col}, nil, false)
			if col == 0 || !strings.Contains(rows[span.Y], " ") {
				t.Fatal("wrapped row lost source offset", span)
			}
		}
	}
	if !seen {
		t.Fatal("missing continuation target")
	}
	v.Pointer(Point{0, 1}, nil, false)
	anchor := v.Cursor
	v.Pointer(Point{1, 1}, &anchor, false)
	if got := v.SelectedText(); got != "界bc\tde\u0301f\nta" {
		t.Fatalf("copied decorations instead of source: %q", got)
	}
	v.Pointer(Point{0, 3}, nil, true)
	if got := v.SelectedText(); got != "a界bc" {
		t.Fatalf("word: %q", got)
	}
}

func TestPointerMapKeepsHorizontalOffsetAndSearchFrame(t *testing.T) {
	v := Viewer{Cursor: Point{0, 14}, Query: "needle", Searching: true}
	v.SetLines([]Line{{Text: "0123456789needle"}})
	_, spans := MouseLayout(MouseScope(v.View(8, 8, DefaultTheme()), "main"))
	for _, span := range spans {
		target, ok := ParseTextTarget(span.Target)
		if !ok || target.Owner != "reader" {
			continue
		}
		if target.Offset != v.Left || target.Offset == 0 || span.Y < 3 {
			t.Fatal("lost clipping or search heading", span, target)
		}
		return
	}
	t.Fatal("missing body target")
}

package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestScreenSelectionCopiesVisibleGraphemesWithinPanel(t *testing.T) {
	frame := "left | 界cafe\u0301   |right\nleft | second   |right"
	spans := []MouseSpan{{X: 7, Y: 0, Width: 9, Scope: "main"}, {X: 7, Y: 1, Width: 9, Scope: "main"}}
	s := NewScreenSelection(frame, spans, "main", Point{Row: 1, Col: 9})
	s.Move(Point{Row: 0, Col: 8}) // The second cell of 界 still selects the whole grapheme.
	if got := s.Text(); got != "界cafe\u0301\nsec" {
		t.Fatalf("selected %q", got)
	}
	painted := s.View(frame, DefaultTheme())
	if ansi.Strip(painted) != frame || painted == frame {
		t.Fatal("selection changed screen text or failed to highlight")
	}
	s.Move(Point{Row: 0, Col: 19})
	if s.Text() != "界cafe\u0301\nsec" {
		t.Fatal("drag crossed into another panel")
	}
}

func TestScreenSelectionRejectsChangedSelectedCells(t *testing.T) {
	frame := "selected text\nother row"
	s := NewScreenSelection(frame, nil, "", Point{Col: 0})
	s.Move(Point{Col: 7})
	if s.Text() != "selected" || !s.Matches("selected text\nnew output") {
		t.Fatal("unselected output invalidated the selection")
	}
	if s.Matches("replaced text\nother row") || s.Matches("selected text") {
		t.Fatal("replacement text retained the selection")
	}
	styled := DefaultTheme().Accent.Render(frame)
	if !s.Matches(styled) || strings.Contains(s.Text(), "\x1b") {
		t.Fatal("styling changed clipboard text")
	}
}

package text

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ScreenSelection uses the final measured frame for content without source text
// targets. It stays inside the pressed scope and never reads hidden content.
type ScreenSelection struct {
	Anchor, Cursor Point
	Active         bool
	lines          []string
	bounds         [][2]int
}

func NewScreenSelection(frame string, spans []MouseSpan, scope string, point Point) *ScreenSelection {
	s := &ScreenSelection{Anchor: point, Cursor: point, lines: strings.Split(ansi.Strip(frame), "\n")}
	s.bounds = make([][2]int, len(s.lines))
	for row := range s.bounds {
		s.bounds[row] = [2]int{-1, -1}
		if scope == "" {
			s.bounds[row] = [2]int{0, ansi.StringWidth(s.lines[row])}
		}
	}
	for _, span := range spans {
		if scope == "" || span.Scope != scope || span.Y < 0 || span.Y >= len(s.bounds) {
			continue
		}
		b := &s.bounds[span.Y]
		if b[0] < 0 {
			*b = [2]int{span.X, span.X + span.Width}
		} else {
			b[0], b[1] = min(b[0], span.X), max(b[1], span.X+span.Width)
		}
	}
	if !s.contains(point) {
		return nil
	}
	return s
}

func (s *ScreenSelection) contains(p Point) bool {
	return p.Row >= 0 && p.Row < len(s.bounds) && s.bounds[p.Row][0] >= 0 && p.Col >= s.bounds[p.Row][0] && p.Col < s.bounds[p.Row][1]
}

func (s *ScreenSelection) Move(point Point) {
	s.Active = s.Active || point != s.Anchor
	if s.contains(point) {
		s.Cursor = point
	}
}

func (s *ScreenSelection) rowRange(row int) (int, int) {
	a, b := Ordered(s.Anchor, s.Cursor)
	if row < a.Row || row > b.Row || s.bounds[row][0] < 0 {
		return 0, 0
	}
	left, right := s.bounds[row][0], s.bounds[row][1]
	if row == a.Row {
		left = a.Col
	}
	if row == b.Row {
		right = b.Col
		return ColumnAt(s.lines[row], left), Next(s.lines[row], ColumnAt(s.lines[row], right))
	}
	return ColumnAt(s.lines[row], left), ColumnAt(s.lines[row], right)
}

func (s *ScreenSelection) Text() string {
	if !s.Active {
		return ""
	}
	a, b := Ordered(s.Anchor, s.Cursor)
	var lines []string
	for row := a.Row; row <= b.Row; row++ {
		if s.bounds[row][0] < 0 {
			continue
		}
		start, end := s.rowRange(row)
		lines = append(lines, strings.TrimRight(string([]rune(s.lines[row])[start:end]), " \t"))
	}
	return strings.Join(lines, "\n")
}

// Matches rejects replacement text even when a live view keeps the same shape.
func (s *ScreenSelection) Matches(frame string) bool {
	lines := strings.Split(ansi.Strip(frame), "\n")
	if len(lines) != len(s.lines) {
		return false
	}
	a, b := Ordered(s.Anchor, s.Cursor)
	for row := a.Row; row <= b.Row; row++ {
		start, end := s.rowRange(row)
		if start == end {
			continue
		}
		left := ansi.StringWidth(string([]rune(s.lines[row])[:start]))
		right := ansi.StringWidth(string([]rune(s.lines[row])[:end]))
		if ansi.Cut(lines[row], left, right) != string([]rune(s.lines[row])[start:end]) {
			return false
		}
	}
	return true
}

func (s *ScreenSelection) View(frame string, theme Theme) string {
	if !s.Active {
		return frame
	}
	lines := strings.Split(frame, "\n")
	a, b := Ordered(s.Anchor, s.Cursor)
	for row := a.Row; row <= b.Row; row++ {
		start, end := s.rowRange(row)
		lines[row] = Paint(s.lines[row], lines[row], []Span{{start, end, theme.Selection}})
	}
	return strings.Join(lines, "\n")
}

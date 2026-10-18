package text

import (
	"unicode/utf8"
)

func (e *Field) Selection() (int, int) {
	n := utf8.RuneCountInString(e.Text)
	a, b := min(e.Anchor, e.Cursor), max(e.Anchor, e.Cursor)
	return max(0, min(a, n)), Next(e.Text, max(0, min(b, n)))
}

func (e *Field) Pointer(col int, anchor *Point, word bool) {
	e.Cursor = max(0, min(col, utf8.RuneCountInString(e.Text)))
	if e.Cursor < utf8.RuneCountInString(e.Text) {
		e.Cursor = Previous(e.Text, Next(e.Text, e.Cursor))
	}
	e.Selecting = false
	if anchor != nil {
		e.Anchor, e.Selecting = anchor.Col, true
	} else if word {
		r := []rune(e.Text)
		start, end := e.Cursor, e.Cursor
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		for end < len(r) && r[end] != '\n' {
			end++
		}
		line := string(r[start:end])
		a, b := WordBounds(line, e.Cursor-start)
		e.Anchor, e.Cursor, e.Selecting = start+a, start+Previous(line, b), b > a
	}
}

// Input targets carry rune offsets through wrapping and cursor decoration.
func (e Field) MouseDisplay(active bool, owner string, theme Theme) string {
	text := CleanInput(e.Text, true)
	a, b := e.Selection()
	if !e.Selecting {
		a, b = 0, 0
	}
	return InputText(text, e.Cursor, active, a, b, owner, theme)
}

func QueryField(reader *Viewer) Field {
	e := NewField(reader.Query)
	e.Cursor, e.Anchor, e.Selecting = reader.SearchInput.Position(reader.Query)
	return e
}

package text

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// EditingKey accepts terminals configured to send BS instead of DEL for
// Backspace. Apply it only while editing text; Ctrl+H has no navigation action.
func EditingKey(key string) string {
	if key == "ctrl+h" {
		return "backspace"
	}
	return key
}

// InputText keeps literal rune positions attached while a field wraps or clips.
func InputText(text string, cursor int, active bool, start, end int, owner string, theme Theme) string {
	var out strings.Builder
	remaining := text
	position := 0
	target := TextTarget{Owner: owner, Hash: TextHash(text)}
	for remaining != "" {
		target.Offset = position
		if active && position == cursor {
			out.WriteString(Mouse("▏", target.String()))
		}
		cluster, _ := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
		rendered := Visible(cluster)
		if position >= start && position < end {
			rendered = theme.Selection.Render(rendered)
		}
		if cluster == "\n" {
			out.WriteString("\n")
		} else {
			out.WriteString(Mouse(rendered, target.String()))
		}
		position += utf8.RuneCountInString(cluster)
		remaining = remaining[len(cluster):]
	}
	target.Offset = position
	tail := " "
	if active && cursor >= position {
		tail = "▏"
	}
	out.WriteString(Mouse(tail, target.String()))
	return out.String()
}

// QueryInput preserves the public query string used by search and its adapters.
// External query replacement starts at the end and clears any old selection.
type QueryInput struct {
	value          string
	cursor, anchor int
	selecting      bool
}

func (q *QueryInput) sync(value string) {
	if q.value != value {
		q.value, q.cursor, q.selecting = value, utf8.RuneCountInString(value), false
	}
}

func (q *QueryInput) Pointer(value string, point Point, anchor *Point, word bool) {
	q.sync(value)
	q.cursor, q.selecting = max(0, min(point.Col, utf8.RuneCountInString(value))), false
	if anchor != nil {
		q.anchor, q.selecting = anchor.Col, true
	} else if word {
		a, b := WordBounds(value, q.cursor)
		q.anchor, q.cursor, q.selecting = a, Previous(value, b), b > a
	}
}

func (q *QueryInput) bounds(value string) (int, int) {
	if !q.selecting {
		return q.cursor, q.cursor
	}
	return min(q.cursor, q.anchor), Next(value, max(q.cursor, q.anchor))
}

func (q *QueryInput) ClearSelection() bool {
	was := q.selecting
	q.selecting = false
	return was
}

func (q *QueryInput) Position(value string) (cursor, anchor int, selecting bool) {
	q.sync(value)
	return q.cursor, q.anchor, q.selecting
}

func (q *QueryInput) Key(value *string, key, text string) {
	key = EditingKey(key)
	q.sync(*value)
	r := []rune(*value)
	a, b := q.bounds(*value)
	switch key {
	case "left":
		q.cursor = Previous(*value, q.cursor)
		if q.selecting {
			q.cursor = a
		}
		q.selecting = false
		return
	case "right":
		q.cursor = Next(*value, q.cursor)
		if q.selecting {
			q.cursor = b
		}
		q.selecting = false
		return
	case "home":
		q.cursor, q.selecting = 0, false
		return
	case "end":
		q.cursor, q.selecting = len(r), false
		return
	case "backspace":
		if !q.selecting {
			a = Previous(*value, q.cursor)
		}
	case "delete":
		if !q.selecting {
			b = Next(*value, q.cursor)
		}
	default:
		if text == "" {
			return
		}
		text = Visible(strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), "\r", ""))
	}
	*value = string(r[:a]) + text + string(r[b:])
	q.value, q.cursor, q.selecting = *value, a+utf8.RuneCountInString(text), false
}

func (q *QueryInput) view(value string, active bool, owner string, width int, theme Theme) string {
	q.sync(value)
	a, b := q.bounds(value)
	text := InputText(value, q.cursor, active, a, b, owner, theme)
	skip := 0
	if active {
		skip = max(0, Cells(string([]rune(value)[:q.cursor]))+2-width)
	}
	return ansi.Cut(text, skip, skip+width)
}

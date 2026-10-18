package text

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"unicode"
)

type Field struct {
	Text      string
	Cursor    int
	Literal   bool
	Limit     int
	Anchor    int
	Selecting bool
}

func NewField(text string) Field { return Field{Text: text, Cursor: len([]rune(text))} }

func CleanInput(s string, multiline bool) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' && multiline {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func (e *Field) Insert(s string, multiline bool) {
	s = CleanInput(s, multiline)
	r := []rune(e.Text)
	e.Cursor = max(0, min(e.Cursor, len(r)))
	if e.Selecting {
		a, b := e.Selection()
		r = append(r[:a:a], r[b:]...)
		e.Cursor, e.Selecting = a, false
	}
	add := []rune(s)
	limit := e.Limit
	if limit == 0 {
		limit = 16384
	}
	if len(r)+len(add) > limit {
		add = add[:max(0, limit-len(r))]
	}
	e.Text = string(r[:e.Cursor]) + string(add) + string(r[e.Cursor:])
	e.Cursor += len(add)
}

func (e *Field) Key(k tea.KeyPressMsg, multiline bool) {
	key := EditingKey(k.String())
	defer func() {
		if e.Text == "" {
			e.Literal = false
		}
	}()
	r := []rune(e.Text)
	e.Cursor = max(0, min(e.Cursor, len(r)))
	if e.Selecting {
		a, b := e.Selection()
		switch key {
		case "backspace", "delete":
			e.Text, e.Cursor, e.Selecting = string(r[:a])+string(r[b:]), a, false
			return
		case "left", "right":
			e.Cursor, e.Selecting = a, false
			if key == "right" {
				e.Cursor = b
			}
			return
		case "home", "end", "ctrl+a", "ctrl+e", "ctrl+u":
			e.Selecting = false
		}
	}
	switch key {
	case "left":
		e.Cursor = Previous(e.Text, e.Cursor)
	case "right":
		e.Cursor = Next(e.Text, e.Cursor)
	case "home", "ctrl+a":
		e.Cursor = 0
	case "end", "ctrl+e":
		e.Cursor = len(r)
	case "backspace":
		if e.Cursor > 0 {
			previous := Previous(e.Text, e.Cursor)
			e.Text = string(r[:previous]) + string(r[e.Cursor:])
			e.Cursor = previous
		}
	case "delete":
		if e.Cursor < len(r) {
			e.Text = string(r[:e.Cursor]) + string(r[Next(e.Text, e.Cursor):])
		}
	case "ctrl+u":
		e.Text = string(r[e.Cursor:])
		e.Cursor = 0
	case "enter":
		if multiline {
			e.Insert("\n", true)
		}
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 {
			e.Insert(k.Text, multiline)
		}
	}
}

func (e Field) Display(active bool) string {
	if !active {
		return CleanInput(e.Text, true)
	}
	r := []rune(CleanInput(e.Text, true))
	c := max(0, min(e.Cursor, len(r)))
	return string(r[:c]) + "▏" + string(r[c:])
}

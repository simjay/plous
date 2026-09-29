package app

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"unicode"
)

type editor struct {
	text   string
	cursor int
}

func newEditor(text string) editor { return editor{text: text, cursor: len([]rune(text))} }
func cleanInput(s string, multiline bool) string {
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
func (e *editor) insert(s string, multiline bool) {
	s = cleanInput(s, multiline)
	r := []rune(e.text)
	e.cursor = max(0, min(e.cursor, len(r)))
	add := []rune(s)
	if len(r)+len(add) > 16384 {
		add = add[:max(0, 16384-len(r))]
	}
	e.text = string(r[:e.cursor]) + string(add) + string(r[e.cursor:])
	e.cursor += len(add)
}
func (e *editor) key(k tea.KeyPressMsg, multiline bool) {
	r := []rune(e.text)
	e.cursor = max(0, min(e.cursor, len(r)))
	switch k.String() {
	case "left":
		e.cursor = max(0, e.cursor-1)
	case "right":
		e.cursor = min(len(r), e.cursor+1)
	case "home", "ctrl+a":
		e.cursor = 0
	case "end", "ctrl+e":
		e.cursor = len(r)
	case "backspace":
		if e.cursor > 0 {
			e.text = string(r[:e.cursor-1]) + string(r[e.cursor:])
			e.cursor--
		}
	case "delete":
		if e.cursor < len(r) {
			e.text = string(r[:e.cursor]) + string(r[e.cursor+1:])
		}
	case "ctrl+u":
		e.text = string(r[e.cursor:])
		e.cursor = 0
	case "enter":
		if multiline {
			e.insert("\n", true)
		}
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 {
			e.insert(k.Text, multiline)
		}
	}
}
func (e editor) display(active bool) string {
	if !active {
		return cleanInput(e.text, true)
	}
	r := []rune(cleanInput(e.text, true))
	c := max(0, min(e.cursor, len(r)))
	return string(r[:c]) + "▏" + string(r[c:])
}

package editor

import (
	"strings"
	"unicode/utf8"

	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *Model) Offset(point textview.Point) int {
	lines := strings.Split(m.text, "\n")
	row := max(0, min(point.Row, len(lines)-1))
	offset := 0
	for _, line := range lines[:row] {
		offset += utf8.RuneCountInString(line) + 1
	}
	return offset + max(0, min(point.Col, utf8.RuneCountInString(lines[row])))
}

func (m *Model) Pointer(point textview.Point, anchor *textview.Point, word bool) {
	m.Cursor = m.Offset(point)
	m.Controls, m.Searching, m.LineSelection, m.group = false, false, false, false
	m.prefix, m.Notice = "", ""
	m.mouseScrolled = true
	if m.Mode == Select {
		m.Mode = Normal
	}
	if anchor != nil {
		m.Anchor, m.Mode = m.Offset(*anchor), Select
	} else if word {
		start, end := m.bounds()
		line := string([]rune(m.text)[start:end])
		a, b := textview.WordBounds(line, m.Cursor-start)
		if b > a {
			m.Anchor, m.Cursor, m.Mode = start+a, start+textview.Previous(line, b), Select
		}
	}
}

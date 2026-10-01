package editor

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/simjay/plous/internal/textview"
)

type Mode string

const (
	Normal Mode = "NORMAL"
	Insert Mode = "INSERT"
	Select Mode = "SELECT"
)
const documentLimit = 2 * 1024 * 1024
const historyLimit = 16 * 1024 * 1024

type snapshot struct {
	text           string
	cursor, anchor int
	mode           Mode
	line           bool
}
type Event struct{ Action, Copy string }

type Model struct {
	Name, Target           string
	Syntax                 string
	Actions                []textview.ActionGroup
	Mode                   Mode
	Controls               bool
	ReadOnly, LineNumbers  bool
	Cursor, Anchor         int
	LineSelection          bool
	Revision               uint64
	Notice, Query          string
	SavedStatus            string
	Diagnostic             string
	Searching              bool
	text, baseline, prefix string
	undo, redo             []snapshot
	group                  bool
	top, left, height      int
	searchReturn           Mode
}

func New(text string) *Model {
	m := &Model{text: text, baseline: text, Mode: Normal, LineNumbers: true}
	if len(text) > documentLimit {
		m.Notice = "Document exceeds the 2 MiB editing limit."
	}
	return m
}
func (m *Model) Text() string { return m.text }
func (m *Model) Dirty() bool  { return m.text != m.baseline }
func (m *Model) MarkSaved(text string) {
	m.text = text
	m.baseline = text
	m.Revision++
	m.undo = nil
	m.redo = nil
	m.group = false
	m.clamp()
}

// RefreshReadOnly replaces a displayed snapshot and clears stale selections and
// searches. Editable buffers can only be replaced by their editing workflow.
func (m *Model) RefreshReadOnly(text string) {
	if !m.ReadOnly {
		return
	}
	m.clamp()
	row := strings.Count(string([]rune(m.text)[:m.Cursor]), "\n")
	start, _ := m.bounds()
	column := m.Cursor - start
	m.MarkSaved(text)
	lines := strings.Split(text, "\n")
	row = min(row, len(lines)-1)
	m.Cursor = 0
	for _, line := range lines[:row] {
		m.Cursor += utf8.RuneCountInString(line) + 1
	}
	m.Cursor += min(column, utf8.RuneCountInString(lines[row]))
	m.Anchor, m.Mode = m.Cursor, Normal
	m.Searching, m.Controls, m.LineSelection = false, false, false
	m.Query, m.prefix = "", ""
}
func (m *Model) state() snapshot {
	return snapshot{m.text, m.Cursor, m.Anchor, m.Mode, m.LineSelection}
}
func (m *Model) restore(s snapshot) {
	m.text, m.Cursor, m.Anchor, m.Mode, m.LineSelection = s.text, s.cursor, s.anchor, s.mode, s.line
	if m.Mode == Insert {
		m.Mode = Normal
	}
	m.Revision++
	m.group = false
	m.clamp()
}

func bounded(history []snapshot) []snapshot {
	size, start := 0, len(history)
	for start > 0 && len(history)-start < 100 {
		size += len(history[start-1].text)
		if size > historyLimit {
			break
		}
		start--
	}
	return append([]snapshot(nil), history[start:]...)
}

func (m *Model) Undo() {
	if m.ReadOnly {
		return
	}
	m.group = false
	if len(m.undo) == 0 {
		m.Notice = "Nothing to undo."
		return
	}
	m.redo = bounded(append(m.redo, m.state()))
	s := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.restore(s)
}
func (m *Model) Redo() {
	if m.ReadOnly {
		return
	}
	m.group = false
	if len(m.redo) == 0 {
		m.Notice = "Nothing to redo."
		return
	}
	m.undo = bounded(append(m.undo, m.state()))
	s := m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.restore(s)
}

func (m *Model) replace(start, end int, text string) bool {
	if m.ReadOnly {
		return false
	}
	r := []rune(m.text)
	start = max(0, min(start, len(r)))
	end = max(start, min(end, len(r)))
	if len(m.text)-len(string(r[start:end]))+len(text) > documentLimit {
		m.Notice = "Edit exceeds 2 MiB. No text was changed."
		return false
	}
	if !m.group {
		m.undo = bounded(append(m.undo, m.state()))
		m.redo = nil
	}
	m.text = string(r[:start]) + text + string(r[end:])
	m.Cursor = start + utf8.RuneCountInString(text)
	m.Revision++
	m.group = m.Mode == Insert
	return true
}

func (m *Model) Paste(text string) {
	m.prefix = ""
	if m.Searching {
		m.Query += strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), "\r", "")
		m.find(0)
		return
	}
	m.group = false
	start, end := m.Cursor, m.Cursor
	if m.Mode == Select {
		start, end = m.selection()
	}
	if m.replace(start, end, text) && m.Mode == Select {
		m.Mode = Normal
	}
	m.group = false
}

func (m *Model) clamp() { m.Cursor = max(0, min(m.Cursor, utf8.RuneCountInString(m.text))) }
func (m *Model) bounds() (int, int) {
	r := []rune(m.text)
	m.clamp()
	start, end := m.Cursor, m.Cursor
	for start > 0 && r[start-1] != '\n' {
		start--
	}
	for end < len(r) && r[end] != '\n' {
		end++
	}
	return start, end
}

func (m *Model) vertical(step int) {
	r := []rune(m.text)
	start, end := m.bounds()
	column := textview.Cells(string(r[start:m.Cursor]))
	for ; step < 0; step++ {
		if start == 0 {
			return
		}
		end = start - 1
		start = end
		for start > 0 && r[start-1] != '\n' {
			start--
		}
	}
	for ; step > 0; step-- {
		if end == len(r) {
			return
		}
		start = end + 1
		end = start
		for end < len(r) && r[end] != '\n' {
			end++
		}
	}
	pos := start
	for pos < end {
		next := start + textview.Next(string(r[start:end]), pos-start)
		if textview.Cells(string(r[start:next])) > column {
			break
		}
		pos = next
	}
	m.Cursor = pos
}

func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
		return 1
	}
	return 2
}
func (m *Model) word(key string) {
	r := []rune(m.text)
	p := m.Cursor
	if key == "b" {
		if p > 0 {
			p--
		}
		for p > 0 && wordClass(r[p]) == 0 {
			p--
		}
		if p < len(r) {
			class := wordClass(r[p])
			for p > 0 && wordClass(r[p-1]) == class {
				p--
			}
		}
	} else if key == "w" {
		if p < len(r) {
			class := wordClass(r[p])
			for p < len(r) && wordClass(r[p]) == class {
				p++
			}
			for p < len(r) && wordClass(r[p]) == 0 {
				p++
			}
		}
	} else {
		if p < len(r) {
			p++
		}
		for p < len(r) && wordClass(r[p]) == 0 {
			p++
		}
		if p < len(r) {
			class := wordClass(r[p])
			for p+1 < len(r) && wordClass(r[p+1]) == class {
				p++
			}
		}
	}
	m.Cursor = min(p, len(r))
	if m.Cursor < len(r) {
		m.Cursor = textview.Previous(m.text, textview.Next(m.text, m.Cursor))
	}
}

func (m *Model) selection() (int, int) {
	start, end := min(m.Anchor, m.Cursor), max(m.Anchor, m.Cursor)
	r := []rune(m.text)
	if m.LineSelection {
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		for end < len(r) && r[end] != '\n' {
			end++
		}
		if end < len(r) {
			end++
		}
	} else {
		end = textview.Next(m.text, end)
	}
	return start, end
}
func (m *Model) SelectedText() string {
	start, end := m.selection()
	return string([]rune(m.text)[start:end])
}

func (m *Model) find(step int) {
	if m.Query == "" {
		return
	}
	pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(m.Query))
	matches := pattern.FindAllStringIndex(m.text, -1)
	if len(matches) == 0 {
		m.Notice = "No matches"
		return
	}
	positions := make([]int, len(matches))
	byteOffset, runeOffset := 0, 0
	for i, span := range matches {
		runeOffset += utf8.RuneCountInString(m.text[byteOffset:span[0]])
		byteOffset = span[0]
		positions[i] = runeOffset
	}
	index := 0
	if step < 0 {
		index = len(positions) - 1
		for i := len(positions) - 1; i >= 0; i-- {
			if positions[i] < m.Cursor {
				index = i
				break
			}
		}
	} else {
		for i, p := range positions {
			if p > m.Cursor || step == 0 && p >= m.Cursor {
				index = i
				break
			}
		}
	}
	m.Cursor = positions[index]
	m.Notice = fmt.Sprintf("%d/%d matches", index+1, len(positions))
}

func (m *Model) SearchStatus() string {
	if m.Query == "" {
		return ""
	}
	pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(m.Query))
	matches := pattern.FindAllStringIndex(m.text, -1)
	if len(matches) == 0 {
		return "No matches"
	}
	index := 0
	cursorByte := len(string([]rune(m.text)[:m.Cursor]))
	for i, span := range matches {
		if span[0] <= cursorByte {
			index = i
		}
	}
	return fmt.Sprintf("%d/%d matches", index+1, len(matches))
}

func (m *Model) Indent(outdent bool) {
	r := []rune(m.text)
	start, end := m.bounds()
	if m.Mode == Select {
		start, end = m.selection()
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		if end > start && r[end-1] == '\n' {
			end--
		} else {
			for end < len(r) && r[end] != '\n' {
				end++
			}
		}
	}
	text := string(r[start:end])
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if outdent {
			if strings.HasPrefix(line, "\t") {
				lines[i] = line[1:]
			} else {
				n := min(2, len(line)-len(strings.TrimLeft(line, " ")))
				lines[i] = line[n:]
			}
		} else {
			lines[i] = "  " + line
		}
	}
	m.group = false
	if m.replace(start, end, strings.Join(lines, "\n")) {
		m.Mode = Normal
		m.Cursor = start
	}
	m.group = false
}

func (m *Model) Key(key, text string) Event {
	m.clamp()
	m.Notice = ""
	if m.Searching {
		switch key {
		case "esc":
			m.Searching = false
			m.Query = ""
			m.Mode = m.searchReturn
		case "enter":
			m.Searching = false
			m.Mode = m.searchReturn
		case "backspace":
			r := []rune(m.Query)
			if len(r) > 0 {
				m.Query = string(r[:textview.Previous(m.Query, len(r))])
			}
			m.find(0)
		case "up", "shift+enter":
			m.find(-1)
		case "down":
			m.find(1)
		default:
			if text != "" {
				m.Paste(text)
			}
		}
		return Event{}
	}
	if m.ReadOnly {
		if m.Mode == Insert {
			m.Mode = Normal
		}
		switch key {
		case "i", "a", "o", "x", "d", "p", "u", "U", ">", "<":
			m.prefix = ""
			return Event{}
		}
	}
	if m.Mode == Insert {
		start, end := m.bounds()
		switch key {
		case "esc":
			m.Mode = Normal
			m.group = false
		case "left":
			m.Cursor = textview.Previous(m.text, m.Cursor)
			m.group = false
		case "right":
			m.Cursor = textview.Next(m.text, m.Cursor)
			m.group = false
		case "up":
			m.vertical(-1)
			m.group = false
		case "down":
			m.vertical(1)
			m.group = false
		case "home":
			m.Cursor = start
			m.group = false
		case "end":
			m.Cursor = end
			m.group = false
		case "backspace":
			if m.Cursor > 0 {
				m.replace(textview.Previous(m.text, m.Cursor), m.Cursor, "")
			}
		case "delete":
			m.replace(m.Cursor, textview.Next(m.text, m.Cursor), "")
		case "tab":
			m.replace(m.Cursor, m.Cursor, "  ")
		case "enter":
			line := string([]rune(m.text)[start:m.Cursor])
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			m.replace(m.Cursor, m.Cursor, "\n"+indent)
		default:
			if text != "" {
				m.replace(m.Cursor, m.Cursor, text)
			}
		}
		return Event{}
	}
	previous := m.prefix
	m.prefix = ""
	start, end := m.bounds()
	switch key {
	case "esc":
		if previous != "" {
			return Event{}
		}
		if m.Mode == Select {
			m.Mode = Normal
		} else if m.Query != "" {
			m.Query = ""
		} else if m.ReadOnly {
			return Event{Action: "back"}
		} else {
			m.Controls = true
		}
		return Event{}
	case "left", "h":
		m.Cursor = textview.Previous(m.text, m.Cursor)
	case "right", "l":
		m.Cursor = textview.Next(m.text, m.Cursor)
	case "up", "k":
		m.vertical(-1)
	case "down", "j":
		m.vertical(1)
	case "pgup":
		m.vertical(-max(1, m.height-1))
	case "pgdown":
		m.vertical(max(1, m.height-1))
	case "home", "0":
		m.Cursor = start
	case "end", "$":
		m.Cursor = end
	case "w", "b", "e":
		m.word(key)
	case "G":
		m.Cursor = utf8.RuneCountInString(m.text)
	case "g":
		if previous == "g" {
			m.Cursor = 0
		} else {
			m.prefix = "g"
		}
	case "i":
		m.Mode = Insert
		m.group = false
	case "a":
		m.Cursor = min(end, textview.Next(m.text, m.Cursor))
		m.Mode = Insert
		m.group = false
	case "o":
		m.Cursor = end
		m.Mode = Insert
		m.group = false
		return m.Key("enter", "")
	case "v", "V":
		if m.Mode == Select {
			m.Mode = Normal
		} else {
			m.Mode = Select
			m.Anchor = m.Cursor
			m.LineSelection = key == "V"
		}
	case "x":
		if m.Mode == Select {
			a, b := m.selection()
			if m.replace(a, b, "") {
				m.Mode = Normal
			}
		} else {
			m.replace(m.Cursor, textview.Next(m.text, m.Cursor), "")
		}
	case "d":
		if m.Mode == Select {
			a, b := m.selection()
			copy := m.SelectedText()
			if m.replace(a, b, "") {
				m.Mode = Normal
			}
			return Event{Copy: copy}
		}
		if previous == "d" {
			if end < utf8.RuneCountInString(m.text) {
				end++
			}
			copy := string([]rune(m.text)[start:end])
			m.replace(start, end, "")
			return Event{Copy: copy}
		} else {
			m.prefix = "d"
		}
	case "y":
		if m.Mode == Select {
			copy := m.SelectedText()
			m.Mode = Normal
			return Event{Copy: copy}
		}
		if previous == "y" || m.ReadOnly {
			if end < utf8.RuneCountInString(m.text) {
				end++
			}
			return Event{Copy: string([]rune(m.text)[start:end])}
		}
		m.prefix = "y"
	case "p":
		return Event{Action: "paste"}
	case "u":
		m.Undo()
	case "U":
		m.Redo()
	case ">":
		m.Indent(false)
	case "<":
		m.Indent(true)
	case "/":
		m.Query = ""
		m.Searching = true
		m.searchReturn = m.Mode
	case "n":
		m.find(1)
	case "N":
		m.find(-1)
	}
	m.clamp()
	return Event{}
}

func (m *Model) Prefix() string { return m.prefix }

func (m *Model) ClearPrefix() { m.prefix = ""; m.group = false }

// Match only brackets outside YAML strings and comments.
func (m *Model) matchingBracket() int {
	r := []rune(m.text)
	if m.Cursor >= len(r) || !strings.ContainsRune("()[]{}", r[m.Cursor]) {
		return -1
	}
	type bracket struct {
		value rune
		index int
	}
	stack := []bracket{}
	quote := rune(0)
	comment, escaped := false, false
	for i, c := range r {
		if c == '\n' {
			comment = false
		}
		if comment {
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '#' && (i == 0 || unicode.IsSpace(r[i-1])) {
			comment = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if strings.ContainsRune("([{", c) {
			stack = append(stack, bracket{c, i})
			continue
		}
		close := strings.IndexRune(")]}", c)
		if close < 0 || len(stack) == 0 {
			continue
		}
		b := stack[len(stack)-1]
		if b.value != []rune("([{")[close] {
			stack = nil
			continue
		}
		stack = stack[:len(stack)-1]
		if i == m.Cursor {
			return b.index
		}
		if b.index == m.Cursor {
			return i
		}
	}
	return -1
}

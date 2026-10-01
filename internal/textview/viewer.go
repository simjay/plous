package textview

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Line struct {
	Text, Styled, Prefix string
	Metadata             bool
	ContinuationIndent   int
}

func (line Line) wrapIndent(width int) int {
	if line.ContinuationIndent <= 0 {
		return 0
	}
	if width-line.ContinuationIndent < 16 {
		return min(2, max(0, width-1))
	}
	return line.ContinuationIndent
}

// Keep the first row full width; subsequent rows leave room for the source label.
func wrapLine(text string, width, indent int) []string {
	rows := strings.Split(ansi.Hardwrap(text, width, true), "\n")
	if indent == 0 || len(rows) < 2 {
		return rows
	}
	return append(rows[:1:1], strings.Split(ansi.Hardwrap(strings.Join(rows[1:], ""), max(1, width-indent), true), "\n")...)
}

type hit struct {
	Point
	End int
}

type Viewer struct {
	Lines                      []Line
	Cursor, Anchor             Point
	Top, Left                  int
	Wrap, Selecting, Searching bool
	Caret                      bool
	ShowPosition               bool
	Query, Notice              string
	hits                       []hit
	match, width, height       int
}

func (v *Viewer) SetLines(lines []Line) {
	v.Lines = lines
	v.clamp()
	v.find()
}

func (v *Viewer) Shift(removed int) {
	if removed <= 0 {
		return
	}
	if v.Cursor.Row < removed || v.Selecting && v.Anchor.Row < removed {
		v.Selecting = false
		v.Notice = "Older output removed; selection cleared."
	}
	v.Cursor.Row = max(0, v.Cursor.Row-removed)
	v.Anchor.Row = max(0, v.Anchor.Row-removed)
	visual := removed
	if v.Wrap && v.width > 0 {
		visual = 0
		for _, line := range v.Lines[:min(removed, len(v.Lines))] {
			available := max(1, v.width-ansi.StringWidth(line.Prefix))
			visual += len(wrapLine(strings.ReplaceAll(Visible(line.Text), "\t", "    "), available, line.wrapIndent(available)))
		}
	}
	v.Top = max(0, v.Top-visual)
}

func (v *Viewer) clamp() {
	v.Cursor.Row = max(0, min(v.Cursor.Row, len(v.Lines)-1))
	if len(v.Lines) > 0 {
		v.Cursor.Col = max(0, min(v.Cursor.Col, utf8.RuneCountInString(v.Lines[v.Cursor.Row].Text)))
	}
}

func (v *Viewer) find() {
	v.hits = nil
	if v.Query == "" {
		return
	}
	pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(v.Query))
	for row, line := range v.Lines {
		byteOffset, runeOffset := 0, 0
		for _, span := range pattern.FindAllStringIndex(line.Text, -1) {
			start := runeOffset + utf8.RuneCountInString(line.Text[byteOffset:span[0]])
			end := start + utf8.RuneCountInString(line.Text[span[0]:span[1]])
			byteOffset, runeOffset = span[1], end
			v.hits = append(v.hits, hit{Point{row, start}, end})
		}
	}
	v.match = min(v.match, max(0, len(v.hits)-1))
	for i, h := range v.hits {
		if h.Point == v.Cursor {
			v.match = i
			break
		}
	}
}

func (v *Viewer) nextMatch(step int) {
	if len(v.hits) == 0 {
		return
	}
	v.match = (v.match + step + len(v.hits)) % len(v.hits)
	v.Cursor = v.hits[v.match].Point
}

func (v *Viewer) Paste(text string) {
	if v.Searching {
		v.Query += Visible(strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), "\r", ""))
		v.match = 0
		v.find()
		v.nextMatch(0)
	}
}

// Key returns copied text separately; the host owns clipboard access.
func (v *Viewer) Key(key, text string) (bool, string) {
	if v.Searching {
		switch key {
		case "esc":
			v.Searching, v.Query = false, ""
			v.find()
		case "enter":
			v.Searching = false
		case "backspace":
			r := []rune(v.Query)
			if len(r) > 0 {
				v.Query = string(r[:Previous(v.Query, len(r))])
			}
			v.find()
			v.nextMatch(0)
		case "up", "shift+enter":
			v.nextMatch(-1)
		case "down":
			v.nextMatch(1)
		default:
			if text != "" {
				v.Paste(text)
			}
		}
		return true, ""
	}
	v.Notice = ""
	line := ""
	if len(v.Lines) > 0 {
		line = v.Lines[v.Cursor.Row].Text
	}
	switch key {
	case "/":
		v.Searching = true
		v.Query = ""
		v.find()
	case "esc":
		if v.Selecting {
			v.Selecting = false
		} else if v.Query != "" {
			v.Query = ""
			v.find()
		} else {
			return false, ""
		}
	case "up", "k":
		v.Cursor.Row--
	case "down", "j":
		v.Cursor.Row++
	case "left":
		v.Cursor.Col = Previous(line, v.Cursor.Col)
	case "right":
		v.Cursor.Col = Next(line, v.Cursor.Col)
	case "pgup":
		v.Cursor.Row -= max(1, v.height-1)
		v.Top = max(0, v.Top-max(1, v.height-1))
	case "pgdown":
		v.Cursor.Row += max(1, v.height-1)
		v.Top += max(1, v.height-1)
	case "home":
		v.Cursor = Point{}
	case "end":
		v.Cursor = Point{Row: max(0, len(v.Lines)-1)}
	case "n", "enter":
		if v.Query == "" {
			return false, ""
		}
		v.nextMatch(1)
	case "N":
		if v.Query == "" {
			return false, ""
		}
		v.nextMatch(-1)
	case "v":
		v.Selecting = !v.Selecting
		v.Anchor = v.Cursor
	case "y":
		copied := v.SelectedText()
		v.Selecting = false
		return true, copied
	default:
		return false, ""
	}
	v.clamp()
	v.Caret = true
	return true, ""
}

func (v *Viewer) EscapeLabel(back string) string {
	if v.Searching {
		return "Cancel search"
	}
	if v.Selecting {
		return "Clear selection"
	}
	if v.Query != "" {
		return "Clear search"
	}
	return back
}

func (v *Viewer) SelectedText() string {
	if len(v.Lines) == 0 {
		return ""
	}
	if !v.Selecting {
		if v.Lines[v.Cursor.Row].Metadata {
			return ""
		}
		return v.Lines[v.Cursor.Row].Text
	}
	start, end := Ordered(v.Anchor, v.Cursor)
	start.Row = max(0, min(start.Row, len(v.Lines)-1))
	end.Row = max(start.Row, min(end.Row, len(v.Lines)-1))
	var parts []string
	for row := start.Row; row <= end.Row; row++ {
		if v.Lines[row].Metadata {
			continue
		}
		line := v.Lines[row].Text
		r := []rune(line)
		left, right := 0, len(r)
		if row == start.Row {
			left = min(start.Col, len(r))
		}
		if row == end.Row {
			right = Next(line, end.Col)
		}
		parts = append(parts, string(r[left:max(left, min(right, len(r)))]))
	}
	return strings.Join(parts, "\n")
}

func (v *Viewer) Status() string {
	if v.Notice != "" {
		return v.Notice
	}
	if v.Query != "" {
		if len(v.hits) == 0 {
			return "No matches"
		}
		return fmt.Sprintf("%d/%d matches", v.match+1, len(v.hits))
	}
	if v.Selecting {
		return "Select · y copy · Esc cancel"
	}
	return fmt.Sprintf("Line %d/%d", min(len(v.Lines), v.Cursor.Row+1), len(v.Lines))
}

func (v *Viewer) View(width, height int, theme Theme) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	v.clamp()
	v.width = width
	if v.ShowPosition {
		height--
	}
	heading := ""
	if v.Searching || v.Query != "" {
		heading = SearchBar(v.Query, v.Searching, v.Status(), width, theme) + "\n"
		if lipgloss.Height(heading) > height {
			heading = theme.Accent.Render(ansi.Truncate("/ "+v.Query+" · "+v.Status(), width, "…")) + "\n"
		}
		height = max(0, height-lipgloss.Height(strings.TrimSuffix(heading, "\n")))
	}
	v.height = height
	if height == 0 {
		if v.ShowPosition {
			return theme.Muted.Render(ansi.Truncate(v.Status(), width, "…"))
		}
		return strings.TrimSuffix(heading, "\n")
	}
	// Measure wrapping without styling hidden lines or building their search spans.
	var starts []int
	total := len(v.Lines)
	if v.Wrap {
		starts = make([]int, len(v.Lines)+1)
		for row, line := range v.Lines {
			prefix := ansi.Truncate(line.Prefix, max(0, width-1), "")
			available := max(1, width-ansi.StringWidth(prefix))
			plain := strings.ReplaceAll(Visible(line.Text), "\t", "    ")
			starts[row+1] = starts[row] + len(wrapLine(plain, available, line.wrapIndent(available)))
		}
		total = starts[len(v.Lines)]
	}
	lineStart := func(row int) int {
		if v.Wrap {
			return starts[row]
		}
		return row
	}
	cursorRow := v.Cursor.Row
	if len(v.Lines) > 0 {
		line := v.Lines[v.Cursor.Row]
		prefix := ansi.Truncate(line.Prefix, max(0, width-1), "")
		available := max(1, width-ansi.StringWidth(prefix))
		column := Cells(string([]rune(line.Text)[:v.Cursor.Col]))
		if v.Wrap {
			wrappedRow := 0
			if column >= available {
				wrappedRow = 1 + (column-available)/(available-line.wrapIndent(available))
			}
			cursorRow = starts[v.Cursor.Row] + min(starts[v.Cursor.Row+1]-starts[v.Cursor.Row]-1, wrappedRow)
		} else {
			if column < v.Left {
				v.Left = column
			}
			if column >= v.Left+available {
				v.Left = column - available + 1
			}
		}
	}
	if cursorRow < v.Top {
		v.Top = cursorRow
	}
	if cursorRow >= v.Top+height {
		v.Top = cursorRow - height + 1
	}
	v.Top = max(0, min(v.Top, max(0, total-height)))
	endRow := min(total, v.Top+height)
	first := v.Top
	if v.Wrap {
		first = sort.Search(len(v.Lines), func(row int) bool { return starts[row+1] > v.Top })
	}
	last := first
	var rendered []string
	start, end := Ordered(v.Anchor, v.Cursor)
	for row := first; row < len(v.Lines) && lineStart(row) < endRow; row++ {
		line := v.Lines[row]
		spans := []Span{}
		index := sort.Search(len(v.hits), func(i int) bool { return v.hits[i].Row >= row })
		for ; index < len(v.hits) && v.hits[index].Row == row; index++ {
			h := v.hits[index]
			spans = append(spans, Span{h.Col, h.End, theme.Match})
		}
		if v.Selecting && row >= start.Row && row <= end.Row {
			left, right := 0, utf8.RuneCountInString(line.Text)
			if row == start.Row {
				left = start.Col
			}
			if row == end.Row {
				right = Next(line.Text, end.Col)
			}
			spans = append(spans, Span{left, right, theme.Selection})
		}
		if v.Caret && row == v.Cursor.Row && v.Cursor.Col < utf8.RuneCountInString(line.Text) {
			spans = append(spans, Span{v.Cursor.Col, Next(line.Text, v.Cursor.Col), theme.Cursor})
		}
		prefix := ansi.Truncate(line.Prefix, max(0, width-1), "")
		available := max(1, width-ansi.StringWidth(prefix))
		painted := Paint(line.Text, line.Styled, spans)
		if v.Wrap {
			indent := line.wrapIndent(available)
			segments := wrapLine(painted, available, indent)
			for i := max(0, v.Top-starts[row]); i < min(len(segments), endRow-starts[row]); i++ {
				p := prefix
				if i > 0 {
					p = strings.Repeat(" ", ansi.StringWidth(prefix)+indent)
				}
				rendered = append(rendered, p+segments[i])
			}
		} else {
			rendered = append(rendered, prefix+ansi.Cut(painted, v.Left, v.Left+available))
		}
		last = row + 1
	}
	body := strings.Join(rendered, "\n")
	if v.ShowPosition {
		if total > 0 {
			first++
		}
		position := fmt.Sprintf("Lines %d-%d / %d", first, last, len(v.Lines))
		body = Fit(body, width, height) + "\n" + theme.Muted.Render(ansi.Truncate(position, width, "…"))
	}
	return heading + body
}

func (v *Viewer) RestoreViewport() func() {
	top, left, cursor := v.Top, v.Left, v.Cursor
	return func() {
		if v.Cursor == cursor {
			v.Top, v.Left = top, left
		}
	}
}

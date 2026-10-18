package text

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type lineMeasure struct {
	text, prefix        string
	indent, width, rows int
}

// Text, prefixes and continuation indentation can change independently of
// selection or styling. Cache geometry only; visible text is painted each frame.
func (v *Viewer) wrapStarts(width int) []int {
	count := len(v.Lines)
	if cap(v.measures) < count || cap(v.measures) > max(64, count*2) {
		v.measures = make([]lineMeasure, count)
	} else {
		v.measures = v.measures[:count]
	}
	if cap(v.starts) < count+1 {
		v.starts = make([]int, count+1)
	} else {
		v.starts = v.starts[:count+1]
	}
	v.starts[0] = 0
	for row, line := range v.Lines {
		cached := &v.measures[row]
		if cached.width != width || cached.text != line.Text || cached.prefix != line.Prefix || cached.indent != line.ContinuationIndent {
			prefix := ansi.Truncate(line.Prefix, max(0, width-1), "")
			available := max(1, width-ansi.StringWidth(prefix))
			plain := strings.ReplaceAll(Visible(line.Text), "\t", "    ")
			*cached = lineMeasure{text: line.Text, prefix: line.Prefix, indent: line.ContinuationIndent, width: width, rows: len(wrapLine(plain, available, line.wrapIndent(available)))}
		}
		v.starts[row+1] = v.starts[row] + cached.rows
	}
	return v.starts
}

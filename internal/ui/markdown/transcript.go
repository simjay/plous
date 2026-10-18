package markdown

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	tableast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Transcript keeps logical lines stable across resizing. Tables use labeled
// records in the narrow chat panel; the full viewer retains their columns.
func Transcript(source string, theme textview.Theme) string {
	m := New(source)
	if len(m.source) > 1<<20 {
		return m.source
	}
	output, err := renderMarkdown(transcriptTables(m.source), paletteFor(theme))
	if err != nil {
		return m.source
	}
	lines := strings.Split(strings.Trim(output, "\n"), "\n")
	for i, line := range lines {
		width := ansi.StringWidth(strings.TrimRight(ansi.Strip(line), " "))
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

func transcriptTables(source string) string {
	if !strings.Contains(source, "|") {
		return source
	}
	data := []byte(source)
	document := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser().Parse(text.NewReader(data))
	var out strings.Builder
	offset := 0
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		if node.Kind() != tableast.KindTable || node.ChildCount() < 2 {
			continue
		}
		start, end := len(data), 0
		var rows [][]string
		for row := node.FirstChild(); row != nil; row = row.NextSibling() {
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				for i := 0; i < cell.Lines().Len(); i++ {
					segment := cell.Lines().At(i)
					start, end = min(start, segment.Start), max(end, segment.Stop)
				}
				cells = append(cells, strings.TrimSpace(string(cell.Lines().Value(data))))
			}
			rows = append(rows, cells)
		}
		if start > end || start < offset {
			continue
		}
		start = strings.LastIndex(source[:start], "\n") + 1
		if n := strings.IndexByte(source[end:], '\n'); n >= 0 {
			end += n + 1
		} else {
			end = len(source)
		}
		out.WriteString(source[offset:start])
		for _, cells := range rows[1:] {
			for i, value := range cells {
				prefix := "  "
				if i == 0 {
					prefix = "- "
				}
				label := ""
				if i < len(rows[0]) && rows[0][i] != "" {
					label = rows[0][i] + ": "
				}
				out.WriteString(prefix + label + value + "\n")
			}
			out.WriteString("\n")
		}
		offset = end
	}
	if offset == 0 {
		return source
	}
	out.WriteString(source[offset:])
	return out.String()
}

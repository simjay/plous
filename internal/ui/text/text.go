package text

import (
	lipgloss "charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui/palette"
)

type Theme struct {
	Text, Muted, Accent, Selection, Match, Cursor, Added, Removed lipgloss.Style
	Control, Keycap, ControlFocus                                 lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		Text:         lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Foreground)),
		Muted:        lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Muted)),
		Accent:       lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Accent)).Bold(true),
		Selection:    lipgloss.NewStyle().Background(lipgloss.Color(palette.TextSelection)),
		Match:        lipgloss.NewStyle().Background(lipgloss.Color("#66522F")).Underline(true),
		Cursor:       lipgloss.NewStyle().Reverse(true),
		Added:        lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Success)),
		Removed:      lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Error)),
		Control:      lipgloss.NewStyle().Background(lipgloss.Color("#232A35")),
		Keycap:       lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Accent)).Background(lipgloss.Color("#242C37")).Bold(true),
		ControlFocus: lipgloss.NewStyle().Background(lipgloss.Color("#334257")),
	}
}

type Point struct{ Row, Col int }

func Before(a, b Point) bool { return a.Row < b.Row || a.Row == b.Row && a.Col < b.Col }

func Ordered(a, b Point) (Point, Point) {
	if Before(b, a) {
		return b, a
	}
	return a, b
}

// Text offsets count runes; movement uses grapheme boundaries and rendering uses cells.
func Next(text string, col int) int {
	n := 0
	for text != "" {
		cluster, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		n += utf8.RuneCountInString(cluster)
		if n > col {
			return n
		}
		text = text[len(cluster):]
	}
	return n
}

func Previous(text string, col int) int {
	n, previous := 0, 0
	for text != "" {
		cluster, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		n += utf8.RuneCountInString(cluster)
		if n >= col {
			return previous
		}
		previous = n
		text = text[len(cluster):]
	}
	return previous
}

func Visible(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, text)
}

func Cells(text string) int { return ansi.StringWidth(strings.ReplaceAll(Visible(text), "\t", "    ")) }

type Span struct {
	Start, End int
	Style      lipgloss.Style
}

func Paint(text, styled string, spans []Span) string {
	if styled == "" {
		styled = Visible(text)
	}
	if len(spans) == 0 {
		return strings.ReplaceAll(styled, "\t", "    ")
	}
	type indexedSpan struct {
		Span
		order int
	}
	pending := make([]indexedSpan, len(spans))
	for i, span := range spans {
		pending[i] = indexedSpan{span, i}
	}
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].Start < pending[j].Start })
	active := []indexedSpan{}
	index, position := 0, 0
	source := text
	cluster := ""
	clusterRunes, clusterEnd, cell := 0, 0, 0
	parser := ansi.NewParser()
	var state byte
	var pen uv.Style
	var out strings.Builder
	// Decode each styled grapheme once. Slicing from the start for every match
	// makes a long line with many matches quadratic.
	for styled != "" {
		seq, width, n, next := ansi.DecodeSequence(styled, state, parser)
		if n == 0 {
			break
		}
		state = next
		styled = styled[n:]
		if width == 0 && strings.HasPrefix(seq, "\x1b") {
			if strings.HasSuffix(seq, "m") {
				uv.ReadStyle(parser.Params(), &pen)
			}
			continue
		}
		if seq == "\t" {
			width = 4
		}
		for cell >= clusterEnd && source != "" {
			position += clusterRunes
			cluster, _ = ansi.FirstGraphemeCluster(source, ansi.GraphemeWidth)
			source = source[len(cluster):]
			clusterRunes = utf8.RuneCountInString(cluster)
			clusterEnd += Cells(cluster)
		}
		end := position + clusterRunes
		kept := active[:0]
		for _, span := range active {
			if span.End > position {
				kept = append(kept, span)
			}
		}
		active = kept
		for index < len(pending) && pending[index].Start < end {
			if pending[index].End > position {
				active = append(active, pending[index])
			}
			index++
		}
		sort.SliceStable(active, func(i, j int) bool { return active[i].order < active[j].order })
		paint := pen
		for _, span := range active {
			style := span.Style
			fg := style.GetForeground()
			if _, unset := fg.(lipgloss.NoColor); !unset {
				paint.Fg = fg
			}
			bg := style.GetBackground()
			if _, unset := bg.(lipgloss.NoColor); !unset {
				paint.Bg = bg
			}
			if style.GetBold() {
				paint.Attrs |= uv.AttrBold
			}
			if style.GetReverse() {
				paint.Attrs |= uv.AttrReverse
			}
			if underline := style.GetUnderlineStyle(); underline != 0 {
				paint.Underline = uv.Underline(underline)
			}
		}
		out.WriteString(paint.Styled(strings.ReplaceAll(seq, "\t", "    ")))
		cell += width
	}

	return out.String()
}

func SearchBar(query string, editing bool, status string, width int, theme Theme, input ...*QueryInput) string {
	label := "Search"
	if status != "" {
		label += " · " + status
	}
	value := query
	if editing {
		value += "▏"
	}
	if value == "" {
		value = "Type to search…"
	}
	if width < 5 {
		return ansi.Truncate(value, width, "")
	}
	value = ansi.Truncate(Visible(value), width-2, "…")
	if len(input) > 0 && query != "" {
		value = input[0].view(query, editing, "query", width-2, theme)
	}
	return Mouse(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Accent.GetForeground()).Width(width).MaxWidth(width).Render(theme.Muted.Render(label)+"\n"+value), "readersearch")
}

// CompactSearchBar retains editable query hit targets when space is scarce.
func CompactSearchBar(query string, editing bool, status string, width int, theme Theme, input *QueryInput) string {
	if width <= 0 {
		return ""
	}
	suffix := ""
	if status != "" {
		suffix = " · " + ansi.Truncate(status, max(0, width/2-3), "…")
	}
	room := max(1, width-2-ansi.StringWidth(suffix))
	value := input.view(query, editing, "query", room, theme)
	return Mouse(ansi.Truncate(theme.Accent.Render("/ ")+value+theme.Muted.Render(suffix), width, ""), "readersearch")
}

func Fit(text string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	rows := strings.Split(text, "\n")
	rows = rows[:min(height, len(rows))]
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "")
	}
	return strings.Join(rows, "\n")
}

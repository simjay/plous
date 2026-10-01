package textview

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"strings"
	"testing"
)

func TestPagingMovesViewportAndAppendKeepsItStill(t *testing.T) {
	v := Viewer{}
	rows := []Line{}
	for range 60 {
		rows = append(rows, Line{Text: "older record"})
	}
	v.SetLines(rows)
	v.Key("end", "")
	v.View(50, 8, DefaultTheme())
	before := v.Top
	v.Key("pgup", "")
	v.View(50, 8, DefaultTheme())
	if v.Top >= before {
		t.Fatal("Page Up did not move the view")
	}
	rows = append(rows, Line{Text: "new output"})
	v.SetLines(rows)
	if strings.Contains(ansi.Strip(v.View(50, 8, DefaultTheme())), "new output") {
		t.Fatal("new output moved review")
	}
}
func TestWrappedEvictionKeepsRetainedSelectionAndMatch(t *testing.T) {
	v := Viewer{Wrap: true}
	rows := []Line{{Text: strings.Repeat("x", 30)}, {Text: "needle one"}, {Text: "needle two"}, {Text: "last"}}
	v.SetLines(rows)
	v.Key("/", "")
	v.Paste("needle")
	v.Key("enter", "")
	v.Key("n", "")
	v.View(10, 2, DefaultTheme())
	v.Shift(1)
	v.SetLines(rows[1:])
	if v.Cursor.Row != 1 || v.Status() != "2/2 matches" {
		t.Fatalf("match drift: %+v %s", v.Cursor, v.Status())
	}
	v.Shift(1)
	v.SetLines(rows[2:])
	if v.Cursor.Row != 0 || v.Status() != "1/1 matches" {
		t.Fatal("eviction changed selected match")
	}
}
func TestStylingPreservesTabsCombiningCharactersAndWideText(t *testing.T) {
	text := "\té界 end"
	theme := DefaultTheme()
	styled := Paint(text, "", []Span{{1, 3, theme.Added}})
	highlighted := Paint(text, styled, []Span{{1, 3, theme.Selection}, {3, 4, theme.Cursor}})
	if got := ansi.Strip(highlighted); got != "    é界 end" {
		t.Fatalf("styling changed content: %q", got)
	}
	v := Viewer{Lines: []Line{{Text: text, Styled: highlighted}}, Selecting: true, Anchor: Point{Col: 0}, Cursor: Point{Col: 3}}
	if v.SelectedText() != "\té界" {
		t.Fatalf("copy normalized source: %q", v.SelectedText())
	}
}

func TestSelectionOverridesTerminalBackgroundAndCursorPreservesColors(t *testing.T) {
	theme := DefaultTheme()
	source := "\x1b[31;44mx\x1b[0m"
	selected := Paint("x", source, []Span{{0, 1, theme.Selection}})
	readStyle := func(text string) uv.Style {
		terminal := vt.NewSafeEmulator(20, 2)
		defer terminal.Close()
		if _, err := terminal.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
		return terminal.CellAt(0, 0).Style
	}
	selection := readStyle(selected)
	wantR, wantG, wantB, _ := theme.Selection.GetBackground().RGBA()
	r, g, b, _ := selection.Bg.RGBA()
	if r != wantR || g != wantG || b != wantB {
		t.Fatal("terminal background hid selection")
	}
	cursor := Paint("x", source, []Span{{0, 1, theme.Cursor}})
	cell := readStyle(cursor)
	if cell.Attrs&uv.AttrReverse == 0 || cell.Fg == nil || cell.Bg == nil {
		t.Fatal("cursor lost underlying style")
	}
	r, _, _, _ = cell.Fg.RGBA()
	if r == 0 {
		t.Fatal("unset foreground replaced terminal color")
	}
}

func TestVisibleRowsPreserveWrappedPrefixesSelectionAndResize(t *testing.T) {
	v := Viewer{Wrap: true}
	v.SetLines([]Line{{Text: "界é\tXYZabcdef", Prefix: "> "}, {Text: "next", Prefix: "> "}, {Text: "last", Prefix: "> "}})
	if got := ansi.Strip(v.View(10, 2, DefaultTheme())); got != "> 界é    X\n  YZabcdef" {
		t.Fatalf("wrapped source changed: %q", got)
	}
	v.Selecting, v.Anchor, v.Cursor = true, Point{}, Point{Row: 1, Col: 3}
	if got := ansi.Strip(v.View(10, 2, DefaultTheme())); got != "  YZabcdef\n> next" {
		t.Fatalf("partial first line or next line lost: %q", got)
	}
	if v.SelectedText() != "界é\tXYZabcdef\nnext" {
		t.Fatal("visible-row rendering changed the selected source")
	}
	v.Selecting = false
	v.Key("/", "")
	v.Paste("last")
	v.Key("enter", "")
	for _, width := range []int{8, 20, 10} {
		view := ansi.Strip(v.View(width, 8, DefaultTheme()))
		if !strings.Contains(view, "last") || v.Status() != "1/1 matches" || v.Cursor.Row != 2 {
			t.Fatalf("resize lost the hidden-line search result: %q", view)
		}
	}
}

func TestHangingLogLinesPreserveSearchCopyAndNarrowContent(t *testing.T) {
	prefix := "[pod/app] "
	text := prefix + strings.Repeat("x", 65) + " 界é end"
	v := Viewer{Wrap: true}
	v.SetLines([]Line{{Text: text, ContinuationIndent: len(prefix)}})
	rows := strings.Split(ansi.Strip(v.View(32, 12, DefaultTheme())), "\n")
	for _, row := range rows[1:] {
		if !strings.HasPrefix(row, strings.Repeat(" ", len(prefix))) {
			t.Fatalf("continuation lost source column: %q", row)
		}
	}
	for _, width := range []int{9, 20, 32, 80} {
		v.Cursor, v.Top = Point{}, 0
		rows = strings.Split(ansi.Strip(v.View(width, 100, DefaultTheme())), "\n")
		var recovered strings.Builder
		indent := v.Lines[0].wrapIndent(width)
		for i, row := range rows {
			if ansi.StringWidth(row) > width {
				t.Fatalf("line overflowed width %d: %q", width, row)
			}
			if i > 0 {
				row = strings.TrimPrefix(row, strings.Repeat(" ", indent))
			}
			recovered.WriteString(row)
		}
		if recovered.String() != text || v.SelectedText() != text {
			t.Fatalf("wrapping lost content at width %d: %q", width, recovered.String())
		}
	}
	v.Key("/", "")
	v.Paste("end")
	v.Key("enter", "")
	if got := ansi.Strip(v.View(32, 7, DefaultTheme())); !strings.Contains(got, "end") || v.Status() != "1/1 matches" {
		t.Fatalf("search did not reveal wrapped match: %q", got)
	}
	v.Wrap = false
	if !strings.Contains(ansi.Strip(v.View(32, 7, DefaultTheme())), "end") || v.SelectedText() != text {
		t.Fatal("horizontal scrolling lost the match or source")
	}
}

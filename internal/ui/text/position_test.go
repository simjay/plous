package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPositionTracksLogicalLinesWhenWrappedAndSearching(t *testing.T) {
	v := Viewer{Wrap: true, ShowPosition: true}
	v.SetLines([]Line{{Text: "abcdefghij"}, {Text: "second"}, {Text: "last"}})
	view := ansi.Strip(v.View(20, 3, DefaultTheme()))
	if !strings.Contains(view, "Lines 1-2 / 3") {
		t.Fatal(view)
	}
	v.Key("end", "")
	view = ansi.Strip(v.View(20, 3, DefaultTheme()))
	if !strings.Contains(view, "Lines 2-3 / 3") {
		t.Fatal(view)
	}
	v.SetLines([]Line{{Text: strings.Repeat("x", 60)}, {Text: "last"}})
	v.Key("home", "")
	view = ansi.Strip(v.View(20, 3, DefaultTheme()))
	if !strings.Contains(view, "Lines 1-1 / 2") {
		t.Fatal("wrapped lines counted as source lines", view)
	}
	v.Key("/", "")
	v.Paste("last")
	view = ansi.Strip(v.View(20, 8, DefaultTheme()))
	if !strings.Contains(view, "/ 2") || len(strings.Split(view, "\n")) > 8 {
		t.Fatal("search lost position or overflowed", view)
	}
	v.SetLines(nil)
	v.Query = ""
	v.Searching = false
	if view = ansi.Strip(v.View(20, 3, DefaultTheme())); !strings.Contains(view, "Lines 0-0 / 0") {
		t.Fatal(view)
	}
	for _, size := range [][2]int{{1, 1}, {4, 2}, {20, 6}} {
		v.Key("/", "")
		view = v.View(size[0], size[1], DefaultTheme())
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("compact search overflow")
		}
	}
}

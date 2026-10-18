package ui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestViewportUsesWrappedRowsForBoundsAndMouseTargets(t *testing.T) {
	content := MouseAction("first 界界界界界界界界", "A") + "\n" + MouseAction("last visible row", "B")
	for _, size := range [][2]int{{8, 2}, {18, 3}, {80, 12}, {8, 0}} {
		layout := MeasureScroll("Read details", content, size[0], size[1])
		start, end := layout.Bounds(1 << 30)
		if end != len(layout.Rows) || start < 0 {
			t.Fatal(start, end, layout)
		}
		before := strings.Join(layout.Rows, "\n")
		got := layout.View(1 << 30)
		if layout.Height > 0 && !strings.Contains(ansi.Strip(got), "row") {
			t.Fatal("last row unreachable", got)
		}
		for _, line := range strings.Split(got, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("overflow", size, got)
			}
		}
		if strings.Join(layout.Rows, "\n") != before {
			t.Fatal("drawing changed measured rows")
		}
		if size[1] > 0 && len(strings.Split(got, "\n")) > size[1] {
			t.Fatal("heading exceeded viewport", size, got)
		}
	}
}

package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Viewport measures styled rows once, preserving the mouse markers in them.
// Offsets are display rows after wrapping, not source lines.
type Viewport struct {
	Rows          []string
	Height, Limit int
}

func MeasureViewport(content string, width, height int) Viewport {
	rows := strings.Split(ansi.Wrap(content, max(1, width), ""), "\n")
	height = max(0, height)
	return Viewport{Rows: rows, Height: height, Limit: max(0, len(rows)-height)}
}

func (v Viewport) Bounds(offset int) (int, int) {
	start := max(0, min(offset, v.Limit))
	return start, min(len(v.Rows), start+v.Height)
}

func (v Viewport) View(offset int) string {
	start, end := v.Bounds(offset)
	return strings.Join(v.Rows[start:end], "\n")
}

// ScrollLayout reserves the wrapped heading before measuring the body.
type ScrollLayout struct {
	Heading string
	Viewport
}

func MeasureScroll(heading, content string, width, height int) ScrollLayout {
	top := MeasureViewport(heading, width, height)
	heading = top.View(0)
	return ScrollLayout{Heading: heading, Viewport: MeasureViewport(content, width, height-len(top.Rows))}
}

func (l ScrollLayout) View(offset int) string {
	if l.Height == 0 {
		return l.Heading
	}
	return l.Heading + "\n" + l.Viewport.View(offset)
}

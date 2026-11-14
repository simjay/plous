package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
)

type logDragTick struct{ gesture *mouseTextGesture }

func (g *mouseTextGesture) logScrollTick() tea.Cmd {
	g.logPending = true
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return logDragTick{g} })
}

func (m *model) updateLogDrag(tick logDragTick) tea.Cmd {
	g := tick.gesture
	if m.mouse.text != g {
		return nil
	}
	g.logPending = false
	context := m.mouseContext()
	if m.focus != 1 || context != m.mouse.context || !m.validTextGesture(g, context) {
		m.mouse.reset()
		return nil
	}
	point, found, scroll := m.logDragPoint(g.log, g.logPointer)
	if !found || scroll == 0 {
		return nil
	}
	reader := &g.log.Reader
	reader.Pointer(point, &g.anchor, false)
	top := reader.Top
	reader.Scroll(scroll)
	if reader.Top == top {
		return nil
	}
	return g.logScrollTick()
}

func (m *model) logDragPoint(log *logview.Model, p tea.Mouse) (textview.Point, bool, int) {
	widths := m.panelWidths()
	if p.X < widths[0] || p.X >= widths[0]+widths[1] {
		return textview.Point{}, false, 0
	}
	var first, last, row textview.MouseSpan
	for _, span := range m.mouse.spans {
		if span.Scope != "main" || !strings.HasPrefix(span.Target, "text/reader/") {
			continue
		}
		if first.Width == 0 {
			first = span
		}
		last = span
		if span.Y == p.Y {
			row = span
		}
	}
	scroll := 0
	switch {
	case first.Width == 0:
		return textview.Point{}, false, 0
	case p.Y < first.Y || p.Y == first.Y && first.Y != last.Y:
		row, scroll = first, -1
	case p.Y > last.Y || p.Y == last.Y && first.Y != last.Y:
		row, scroll = last, 1
	}
	target, ok := textview.ParseTextTarget(row.Target)
	lines := log.Reader.Lines
	if !ok || target.Row >= len(lines) {
		return textview.Point{}, false, 0
	}
	x := max(row.X, min(p.X, row.X+row.Width-1))
	point, found := textPointOnLine(row, target, lines[target.Row].Text, x)
	return point, found, scroll
}

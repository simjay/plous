package monitor

import "time"

func (s *OverviewState) NavigationHint() string {
	if s.Pane == 0 {
		return "←→ Tiles · ↑↓ Rows/sections"
	}
	l := s.layout(s.Width, s.Height)
	if l.compact || l.columns == 1 {
		return "↑↓ Rows/sections"
	}
	return "←→ Columns · ↑↓ Rows/sections"
}

func (s *OverviewState) moveArrow(key string) bool {
	switch key {
	case "j":
		key = "down"
	case "k":
		key = "up"
	case "l":
		key = "right"
	}
	if key != "left" && key != "right" && key != "up" && key != "down" {
		return false
	}
	l := s.layout(s.Width, s.Height)
	if s.Pane == 0 {
		s.movePulse(key, l)
		return true
	}
	if key == "left" || key == "right" {
		s.moveColumn(key, l)
		return true
	}
	step := 1
	if key == "up" {
		step = -1
	}
	if s.Pane == 3 && s.InventoryTool == "" {
		rows := s.Filtered(3)
		if len(rows) > 0 {
			counters := len(rows[min(s.Cursor[3], len(rows)-1)].Counters)
			if step > 0 && s.Counter < counters || step < 0 && s.Counter > 0 {
				s.Counter += step
				return true
			}
		}
	}
	next := s.Cursor[s.Pane] + step
	if next >= 0 && next < len(s.Filtered(s.Pane)) {
		s.Cursor[s.Pane] = next
		if s.Pane == 3 {
			s.Counter = 0
			if step < 0 && s.InventoryTool == "" {
				s.Counter = len(s.Filtered(3)[next].Counters)
			}
		}
		return true
	}
	if l.compact || l.columns == 1 {
		s.Pane = max(0, min(3, s.Pane+step))
	} else if l.columns == 2 && (s.Pane == 2 && step > 0 || s.Pane == 3 && step < 0) {
		s.Pane += step
	} else if step < 0 {
		s.moveAbove(l)
	}
	return true
}

func (s *OverviewState) movePulse(key string, l overviewLayout) {
	height := l.pulse
	if l.compact {
		height = l.lower
	}
	pulse := s.pulseLayout(s.Width, height)
	index := min(max(0, s.Cursor[0]), 4)
	switch key {
	case "left":
		if pulse.rows == 1 || index%pulse.columns > 0 {
			index = max(0, index-1)
		}
	case "right":
		if pulse.rows == 1 || index%pulse.columns < pulse.columns-1 {
			index = min(4, index+1)
		}
	case "up":
		if pulse.rows > 1 && index >= pulse.columns {
			index -= pulse.columns
		}
	case "down":
		if pulse.rows > 1 && (index/pulse.columns+1)*pulse.columns < 5 {
			index = min(4, index+pulse.columns)
		} else {
			s.Pane = 1
			if !l.compact && l.columns > 1 {
				bounds := pulse.tileBounds(index, s.Width, height)
				x := (bounds.Min.X + bounds.Max.X) / 2
				for pane := 2; pane <= l.columns; pane++ {
					if x >= l.paneBounds(pane, s.Width).Min.X {
						s.Pane = pane
					}
				}
			}
		}
	}
	s.Cursor[0] = index
}

func (s *OverviewState) moveColumn(key string, l overviewLayout) {
	if l.compact || l.columns == 1 {
		return
	}
	if l.columns == 3 {
		step := 1
		if key == "left" {
			step = -1
		}
		s.Pane = max(1, min(3, s.Pane+step))
		return
	}
	if key == "left" {
		s.Pane = 1
	} else if s.Pane == 1 {
		bounds := l.paneBounds(1, s.Width)
		p := s.paneContent(1, bounds.Dx(), bounds.Dy(), false, time.Now())
		y := 0
		if len(p.rows) > 0 {
			index := min(s.Cursor[1], len(p.rows)-1)
			y = 2 + (p.starts[index]+min(p.ends[index], p.starts[index]+p.height))/2 - s.paneOffset(p, 1)
		}
		s.Pane = 2
		if y >= l.paneBounds(3, s.Width).Min.Y {
			s.Pane = 3
		}
	}
}

func (s *OverviewState) moveAbove(l overviewLayout) {
	bounds := l.paneBounds(s.Pane, s.Width)
	pulse := s.pulseLayout(s.Width, l.pulse)
	index := min(max(0, s.Cursor[0]), 4)
	selected := pulse.tileBounds(index, s.Width, l.pulse)
	x := (selected.Min.X + selected.Max.X) / 2
	// Retain the selected tile when it is directly above this column.
	if x < bounds.Min.X || x > bounds.Max.X {
		center := (bounds.Min.X + bounds.Max.X) / 2
		distance := s.Width + 1
		for i := 0; i < 5; i++ {
			tile := pulse.tileBounds(i, s.Width, l.pulse)
			delta := (tile.Min.X+tile.Max.X)/2 - center
			if delta < 0 {
				delta = -delta
			}
			if delta < distance {
				index, distance = i, delta
			}
		}
	}
	s.Pane, s.Cursor[0] = 0, index
}

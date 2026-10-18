package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type ChartSample struct {
	Time  time.Time
	Value float64
}

type ChartOptions struct {
	Now, Selected time.Time
	Window, Gap   time.Duration
	Width, Height int
	Maximum       float64
	Line          lipgloss.Style
	Fill          color.Color
	Target        func(time.Time) string
	Bars          bool
}

// ChartMaximum leaves headroom and rounds to a readable 1, 2, or 5 multiple.
func ChartMaximum(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 1
	}
	value *= 1.1
	base := math.Pow(10, math.Floor(math.Log10(value)))
	for _, step := range []float64{1, 2, 5, 10} {
		if value <= base*step {
			return base * step
		}
	}
	return value
}

// TimeChart preserves bucket peaks and joins only consecutive observations.
// Extra cells add drawing resolution, never observations or clickable values.
func TimeChart(samples []ChartSample, o ChartOptions) []string {
	if o.Width < 1 || o.Height < 1 || o.Window <= 0 {
		return nil
	}
	values := make([]float64, o.Width)
	first, last, targets := make([]time.Time, o.Width), make([]time.Time, o.Width), make([]time.Time, o.Width)
	start := o.Now.Add(-o.Window)
	columnAt := func(t time.Time) int {
		return min(o.Width-1, int(float64(t.Sub(start))/float64(o.Window)*float64(o.Width)))
	}
	for _, p := range samples {
		if p.Time.Before(start) || p.Time.After(o.Now) || p.Value < 0 || math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
			continue
		}
		x := columnAt(p.Time)
		if first[x].IsZero() || p.Value >= values[x] {
			values[x], targets[x] = p.Value, p.Time
		}
		if first[x].IsZero() || p.Time.Before(first[x]) {
			first[x] = p.Time
		}
		if p.Time.After(last[x]) {
			last[x] = p.Time
		}
	}
	if o.Bars {
		previous := -1
		for x := range values {
			if first[x].IsZero() {
				continue
			}
			if previous >= 0 && first[x].Sub(last[previous]) <= o.Gap {
				for at := previous + 1; at < x; at++ {
					values[at], first[at] = values[previous], first[previous]
				}
			}
			previous = x
		}
		lines := make([]string, o.Height)
		for row := range lines {
			var line strings.Builder
			for x, value := range values {
				glyph := " "
				if !first[x].IsZero() && o.Maximum > 0 {
					units := int(math.Ceil(value / o.Maximum * float64(o.Height*8)))
					level := min(8, max(0, units-(o.Height-row-1)*8))
					glyph = string([]rune(" ▁▂▃▄▅▆▇█")[level])
					if value == 0 && row == o.Height-1 {
						glyph = "─"
					}
				}
				line.WriteString(o.Line.Render(glyph))
			}
			lines[row] = line.String()
		}
		return lines
	}
	dots, filled := make([][]rune, o.Height), make([][]bool, o.Height)
	for y := range dots {
		dots[y], filled[y] = make([]rune, o.Width), make([]bool, o.Width)
	}
	bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	pixel := func(x, y int) { dots[y/4][x/2] |= bits[y%4][x%2] }
	fill := func(x, y int) {
		for row := y/4 + 1; row < o.Height; row++ {
			filled[row][x] = true
		}
	}
	previousX, previousY := -1, 0
	for x, v := range values {
		if first[x].IsZero() {
			continue
		}
		y := o.Height*4 - 1
		if o.Maximum > 0 {
			y -= min(y, max(0, int(math.Round(v/o.Maximum*float64(y)))))
		}
		pixel(x*2, y)
		pixel(x*2+1, y)
		fill(x, y)
		if previousX >= 0 && first[x].Sub(last[previousX]) <= o.Gap {
			for at := previousX + 1; at < x; at++ {
				pixel(at*2, previousY)
				pixel(at*2+1, previousY)
				fill(at, previousY)
			}
			for at := min(previousY, y); at <= max(previousY, y); at++ {
				pixel(x*2, at)
			}
		}
		previousX, previousY = x, y
	}
	selected := -1
	if !o.Selected.IsZero() && !o.Selected.Before(start) && !o.Selected.After(o.Now) {
		selected = columnAt(o.Selected)
	}
	lines := make([]string, o.Height)
	for y := range dots {
		var line strings.Builder
		for x, cell := range dots[y] {
			glyph, style := " ", o.Line
			if cell != 0 {
				glyph = string(0x2800 + cell)
			}
			if filled[y][x] {
				style = style.Background(o.Fill)
			}
			if x == selected {
				style = style.Background(Highlight)
				if cell == 0 {
					glyph = "┊"
				}
			}
			text := style.Render(glyph)
			if o.Target != nil && !targets[x].IsZero() {
				text = MouseTarget(text, o.Target(targets[x]))
			}
			line.WriteString(text)
		}
		lines[y] = line.String()
	}
	return lines
}

func ChartTimeAxis(axisWidth, width int, window time.Duration) []string {
	if width < 4 {
		return nil
	}
	ticks, labels := []rune(strings.Repeat("─", width)), []rune(strings.Repeat(" ", width))
	minutes := int(window / time.Minute)
	place := func(start int, label string) bool {
		if start < 0 || start+len(label) > width {
			return false
		}
		for _, cell := range labels[max(0, start-1):min(width, start+len(label)+1)] {
			if cell != ' ' {
				return false
			}
		}
		copy(labels[start:], []rune(label))
		return true
	}
	place(width-3, "Now")
	if width >= 12 {
		place(0, fmt.Sprintf("%dm ago", minutes))
	}
	if width >= 30 {
		count := 3
		if minutes == 5 {
			count = 5
		}
		for i := 1; i < count; i++ {
			column := (width - 1) * i / count
			label := fmt.Sprintf("%dm", minutes*(count-i)/count)
			if place(column-len(label)/2, label) {
				ticks[column] = '┬'
			}
		}
	}
	return []string{lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat(" ", axisWidth+1) + "└" + string(ticks) + "┘"), Muted.Render(strings.Repeat(" ", axisWidth+2) + string(labels))}
}

// ComparisonBar uses a common denominator supplied by the caller. Unknown or
// zero denominators remain textual; over-budget values keep their actual ratio.
func ComparisonBar(label, detail string, value, total *float64, width int, style lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	heading := ansi.Wrap(OneLine(label)+"  "+OneLine(detail), width, "")
	if width < 12 || value == nil || total == nil || *value < 0 || *total <= 0 || math.IsNaN(*value+*total) || math.IsInf(*value+*total, 0) {
		return heading
	}
	ratio := *value / *total
	percent := fmt.Sprintf(" %.0f%%", ratio*100)
	barWidth := max(1, width-len(percent)-2)
	filled := min(barWidth, max(0, int(math.Round(ratio*float64(barWidth)))))
	if *value > 0 && filled == 0 {
		filled = 1
	}
	if ratio > 1 {
		style = ChangedStyle
	}
	bar := style.Render(strings.Repeat("━", filled)) + Muted.Render(strings.Repeat("─", barWidth-filled))
	return heading + "\n" + Clipped(" "+bar+Muted.Render(percent), width)
}

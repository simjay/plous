package ui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"math"
	"strings"
	"testing"
	"time"
)

func TestUsageWideChartKeepsGapsAndSampleTargets(t *testing.T) {
	now := time.Now()
	points := []ChartSample{{now.Add(-45 * time.Second), 1}, {now.Add(-30 * time.Second), 2}, {now.Add(-time.Second), 3}}
	o := ChartOptions{Now: now, Window: time.Minute, Gap: 22500 * time.Millisecond, Width: 120, Height: 8, Maximum: 4, Target: func(t time.Time) string { return t.Format(time.RFC3339Nano) }}
	lines := TimeChart(points, o)
	_, spans := textview.MouseLayout(textview.MouseScope(strings.Join(lines, "\n"), "chart"))
	if len(spans) == 0 {
		t.Fatal("no sample targets")
	}
	for _, span := range spans {
		if span.Target != "" && span.X != 30 && span.X != 60 && span.X != 118 {
			t.Fatalf("drawing cell became fake sample: %+v", span)
		}
	}
	for _, line := range lines {
		plain := []rune(ansi.Strip(line))
		if len(plain) != 120 || strings.TrimSpace(string(plain[61:118])) != "" {
			t.Fatal("missing poll was connected")
		}
	}
}

func TestTimeChartBarsPreserveGapsZeroAndBucketPeaks(t *testing.T) {
	now := time.Now()
	points := []ChartSample{{now.Add(-14 * time.Minute), 0}, {now.Add(-time.Minute), 2}, {now.Add(-50 * time.Second), 1}, {now.Add(time.Second), 100}}
	lines := TimeChart(points, ChartOptions{Now: now, Window: 15 * time.Minute, Width: 15, Height: 4, Maximum: 2, Bars: true})
	for y, line := range lines {
		cells := []rune(ansi.Strip(line))
		if len(cells) != 15 || cells[14] != '█' || strings.TrimSpace(string(cells[2:14])) != "" {
			t.Fatalf("bar lost peak or fabricated sample: %q", line)
		}
		if y == 3 && cells[1] != '─' {
			t.Fatal("real zero disappeared")
		}
	}
}

func TestTimeChartBarsConnectObservationsWithoutFillingOutages(t *testing.T) {
	now := time.Now()
	points := []ChartSample{{now.Add(-50 * time.Second), 4}, {now.Add(-35 * time.Second), 2}, {now.Add(-2 * time.Second), 3}}
	lines := TimeChart(points, ChartOptions{Now: now, Window: time.Minute, Gap: 20 * time.Second, Width: 60, Height: 4, Maximum: 4, Bars: true})
	for y, line := range lines {
		cells := []rune(ansi.Strip(line))
		if len(cells) != 60 || strings.TrimSpace(string(cells[:10])) != "" || strings.TrimSpace(string(cells[26:58])) != "" || cells[59] != ' ' {
			t.Fatal("filled before observation, across outage, or beyond last sample", line)
		}
		if y == 3 && strings.Contains(string(cells[10:26]), " ") {
			t.Fatal("adjacent observations became disconnected bars", line)
		}
	}
}

func TestUsageChartBarsKeepUnknownAndOverBudgetValues(t *testing.T) {
	for _, width := range []int{12, 26, 80} {
		for _, total := range []float64{-1, 0, 1, math.NaN()} {
			text := ansi.Strip(ComparisonBar("long resource name", "2 / reported capacity", new(2.0), new(total), width, ChartCPU))
			for _, line := range strings.Split(text, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatal("bar overflow", line)
				}
			}
			if total == 1 && !strings.Contains(text, "200%") {
				t.Fatal("over-budget value was clamped", text)
			}
			if total != 1 && strings.Contains(text, "%") {
				t.Fatal("unknown capacity became a percentage", text)
			}
		}
	}
	for _, v := range []float64{.0004, .1, 1, 10, 123, 1024 * 1024} {
		if bound := ChartMaximum(v); bound < v*1.1 {
			t.Fatalf("no headroom: %v %v", v, bound)
		}
	}
}

func TestUsagePlotKeepsTimeGapsAndRealZero(t *testing.T) {
	now := time.Now()
	points := []ChartSample{
		{Time: now.Add(-20 * time.Minute), Value: 100},
		{Time: now.Add(-14 * time.Minute), Value: 0},
		{Time: now.Add(-time.Minute), Value: 2},
		{Time: now.Add(time.Minute), Value: 100},
	}
	for _, height := range []int{2, 4} {
		plot := TimeChart(points, ChartOptions{Now: now, Window: 15 * time.Minute, Gap: 22500 * time.Millisecond, Width: 15, Height: height, Maximum: 2, Line: lipgloss.NewStyle()})
		if len(plot) != height || []rune(ansi.Strip(plot[height-1]))[1] == ' ' || []rune(ansi.Strip(plot[0]))[14] == ' ' {
			t.Fatalf("zero or peak misrepresented: %q", plot)
		}
		for row, line := range plot {
			cells := []rune(ansi.Strip(line))
			if len(cells) != 15 || cells[0] != ' ' || strings.TrimSpace(string(cells[2:14])) != "" {
				t.Fatalf("missing intervals were filled: %q", line)
			}
			if row < height-1 && cells[1] != ' ' {
				t.Fatalf("zero was drawn above the baseline: %q", line)
			}
		}
	}
}

func TestUsagePlotShowsSmallChangesAndKeepsBucketPeaks(t *testing.T) {
	now := time.Now()
	points := []ChartSample{
		{Time: now.Add(-21 * time.Second), Value: 2.4},
		{Time: now.Add(-20 * time.Second), Value: 2.9},
		{Time: now.Add(-19 * time.Second), Value: 2.4},
		{Time: now.Add(-5 * time.Second), Value: 2.7},
	}
	plot := TimeChart(points, ChartOptions{Now: now, Window: 15 * time.Minute, Gap: 22500 * time.Millisecond, Width: 60, Height: 4, Maximum: 2.9, Line: lipgloss.NewStyle()})
	cells := []rune(ansi.Strip(plot[0]))
	if cells[58] != '⠉' || cells[59] == ' ' || cells[58] == cells[59] {
		t.Fatal("nearby values or a peak within one bucket were flattened", plot)
	}
}

func TestChartTimeAxisAvoidsLabelOverlap(t *testing.T) {
	axis := ansi.Strip(strings.Join(ChartTimeAxis(5, 32, 5*time.Minute), "\n"))
	t.Log("\n" + axis)
	if strings.Contains(axis, "agom") {
		t.Error("left and interior time labels overlap")
	}
}

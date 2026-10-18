package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	"strings"
	"time"
)

func usageSampleTime(sample kubernetes.Usage) string {
	if sample.Aggregated {
		return "Collected " + sample.Collected.Local().Format("15:04:05") + " · Source samples " + sample.Timestamp.Local().Format("15:04:05") + "–" + sample.SourceEnd.Local().Format("15:04:05")
	}
	return "Sampled " + sample.Timestamp.Local().Format("15:04:05")
}

func (e *ExploreState) UsageSummaryFit(width, available int, expanded bool, now time.Time) string {
	maximum := 8
	if expanded {
		maximum = 12
	}
	if width < 40 {
		maximum = 2
	}
	overhead := lipgloss.Height(e.UsageSummary(width, 0, now))
	height := min(maximum, max(2, (available-overhead)/2))
	return e.UsageSummary(width, height, now)
}

func (e *ExploreState) UsageSummary(width, height int, now time.Time) string {
	if width <= 0 || !kubernetes.HasUsage(e.Resource) {
		return ""
	}
	lines := []string{SummaryHeading("Live usage", width)}
	run := e.Usage
	if run == nil {
		return strings.Join(append(lines, ui.Muted.Render("Waiting for metrics…")), "\n")
	}
	if !run.Charts.Inspect {
		lines = append(lines, ui.ActionButton("I", "Inspect charts", false))
	}
	current := [2]string{"N/A", "N/A"}
	valid := run.Err == "" && run.Latest.Fresh(now)
	sample := run.Latest
	if !run.Charts.Selected.IsZero() {
		for _, p := range run.Points {
			if usagePlotTime(p).Equal(run.Charts.Selected) {
				sample = p
				valid = true
				break
			}
		}
		lines = append(lines, ui.Selected.Render(ui.Clipped("Inspecting "+usagePlotTime(sample).Local().Format("15:04:05"), width)))
	}
	if valid {
		current = [2]string{kubernetes.CPUUsage(sample.CPU), kubernetes.MemoryUsage(sample.Memory)}
	}
	lines = append(lines, ui.Selected.Render(ansi.Wrap("CPU "+current[0]+" · Memory "+current[1], width, "")))
	note := run.Err
	if !run.Loaded {
		note = "Waiting for metrics…"
	} else if note == "" && !run.Latest.Fresh(now) {
		note = "Metrics are stale; waiting for a fresh sample."
		if run.Latest.Timestamp.IsZero() {
			note = "Waiting for a fresh sample."
		}
	}
	for _, text := range []string{note, run.Note, run.Charts.Notice} {
		if text != "" {
			lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap(ui.Plain(text), width, "")))
		}
	}
	maximumValues := run.Charts.Maximum
	if maximumValues[0] == 0 {
		maximumValues[0] = .001
	}
	if maximumValues[1] == 0 {
		maximumValues[1] = 1048576
	}
	maximum := [2]string{kubernetes.CPUUsage(maximumValues[0]), kubernetes.MemoryUsage(maximumValues[1])}
	axisWidth := max(3, ansi.StringWidth(maximum[0]), ansi.StringWidth(maximum[1]))
	plotWidth := width - axisWidth - 3
	for i, label := range []string{"CPU", "Memory"} {
		style, fill := ui.ChartCPU, ui.ChartCPUFill
		if i == 1 {
			style, fill = ui.ChartMemory, ui.ChartMemoryFill
			lines = append(lines, "")
		}
		lines = append(lines, style.Bold(true).Render(ui.Clipped(label, width)))
		if plotWidth < 4 {
			continue
		}
		points := make([]ui.ChartSample, 0, len(run.Points))
		for _, p := range run.Points {
			v := p.CPU
			if i == 1 {
				v = p.Memory
			}
			points = append(points, ui.ChartSample{Time: usagePlotTime(p), Value: v})
		}
		plot := ui.TimeChart(points, ui.ChartOptions{Now: now, Window: run.Charts.Range(), Gap: UsageInterval * 3 / 2, Width: plotWidth, Height: height, Maximum: maximumValues[i], Selected: run.Charts.Selected, Line: style, Fill: fill, Target: func(t time.Time) string { return "usage-sample/" + t.Format(time.RFC3339Nano) }})
		for row, line := range plot {
			value, tick := "", "│"
			if row == 0 {
				value, tick = maximum[i], "┤"
			} else if row == height-1 {
				value, tick = "0", "┤"
			}
			lines = append(lines, ui.Muted.Render(fmt.Sprintf("%*s %s", axisWidth, value, tick))+line+ui.Muted.Render("│"))
		}
	}
	if plotWidth >= 4 {
		lines = append(lines, ui.ChartTimeAxis(axisWidth, plotWidth, run.Charts.Range())...)
	}
	status := "Refresh 15s"
	if !run.Latest.Timestamp.IsZero() {
		status += " · " + usageSampleTime(sample)
	}
	if points := run.VisiblePoints(now); len(points) > 0 {
		status += " · History " + usagePlotTime(points[0]).Local().Format("15:04:05") + "–" + usagePlotTime(points[len(points)-1]).Local().Format("15:04:05")
	}
	lines = append(lines, ui.Muted.Render(ansi.Wrap(status, width, "")))

	return strings.Join(lines, "\n")
}

func (e *ExploreState) UsageHelp() string {
	if e == nil || !kubernetes.HasUsage(e.Resource) {
		return ""
	}
	text := "\n\nLive usage\nCPU and memory use independent scales from zero to the labeled maximum, not resource limits.\nW switches 5 / 15 minutes. Blank intervals have no samples.\nI opens charts; Left/Right inspects actual samples; P follows latest.\nSource: Metrics API (metrics.k8s.io), polled every 15 seconds.\nWorkload totals are collection snapshots; source timestamps may differ across Pods.\nHistory is collected only while this resource is open and is not saved."
	if run := e.Usage; run != nil && !run.Latest.Timestamp.IsZero() {
		text += "\n" + usageSampleTime(run.Latest)
		if !run.Latest.Aggregated {
			text += " · measurement window: " + run.Latest.Window.String()
		}
	}
	return text
}

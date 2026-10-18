package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"cmp"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	"slices"
	"strings"
	"time"
)

func (e *ExploreState) ChartLayout(width, height int, expanded bool, now time.Time) ui.ScrollLayout {
	buttons := []string{ui.ActionButton("I", "Back", false)}
	if e.Detail != nil {
		buttons = append(buttons, ui.ActionButton("W", fmt.Sprintf("Range %dm", int(e.Usage.Charts.Range()/time.Minute)), false), ui.ActionButton("P", "Latest", false))
	}
	heading := strings.Join(ui.ActionButtonRows(buttons, width), "\n")
	if e.Detail != nil {
		heading += "\n" + ui.Muted.Render(ui.Clipped(ui.OneLine(e.Detail.GetName())+" · ←/→ inspect · Esc back", width))
	}
	available := max(0, height-lipgloss.Height(heading))
	content := ""
	if e.Detail == nil {
		content = e.TopConsumers(width, now)
	} else {
		parts := []string{e.UsageSummaryFit(width, available, expanded, now)}
		if run := e.Usage; run.Err == "" && run.Latest.Fresh(now) {
			for _, section := range append(kubernetes.ContainerUsageSections(run.Latest), kubernetes.UsageBudgets(e.Resource, *e.Detail, run.Latest)...) {
				parts = append(parts, RenderChartSection(section, width))
			}
		}
		for _, section := range kubernetes.StatusCharts(e.Resource, *e.Detail) {
			parts = append(parts, RenderChartSection(section, width))
		}
		content = strings.Join(parts, "\n\n")
	}
	return ui.MeasureScroll(heading, content, width, height)
}

func RenderDetailBars(bars []kubernetes.DetailBar, width int) []string {
	var lines []string
	for _, bar := range bars {
		style := ui.InfoStyle
		if bar.Metric == "cpu" {
			style = ui.ChartCPU
		} else if bar.Metric == "memory" {
			style = ui.ChartMemory
		}
		lines = append(lines, ui.ComparisonBar(bar.Label, bar.Detail, bar.Value, bar.Total, width, style))
	}
	return lines
}

func RenderChartSection(section kubernetes.DetailSection, width int) string {
	lines := []string{SummaryHeading(section.Title, width)}
	lines = append(lines, RenderDetailBars(section.Bars, width)...)
	for _, field := range section.Fields {
		lines = append(lines, ui.Muted.Render(ansi.Wrap(field.Label+": "+ui.Plain(field.Value), width, "")))
	}
	return strings.Join(lines, "\n")
}

func (e *ExploreState) TopConsumers(width int, now time.Time) string {
	rows := e.PrepareInventory(now).Rows
	type item struct {
		name  string
		usage kubernetes.Usage
	}
	var items []item
	if e.Usage.Err == "" && e.Err == "" {
		for _, row := range rows {
			u, ok := e.Usage.Samples[kubernetes.UsageKey(row.Object)]
			if !ok || !u.Matches(row.Object, now) {
				continue
			}
			name := row.Object.GetName()
			if row.Object.GetNamespace() != "" {
				name = row.Object.GetNamespace() + "/" + name
			}
			items = append(items, item{name, u})
		}
	}
	lines := []string{SummaryHeading("Top consumers", width), ui.Muted.Render(ansi.Wrap(fmt.Sprintf("Fresh metrics for %d/%d filtered objects · top 10 per metric · bars relative to largest", len(items), len(rows)), width, ""))}
	for _, note := range []string{e.Usage.Err, e.Err, e.Warning} {
		if note != "" {
			lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap(ui.Plain(note), width, "")))
		}
	}
	if e.Collection.Truncated {
		lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap("Object list is partial; rankings cover returned objects only.", width, "")))
	}
	if len(items) == 0 {
		return strings.Join(append(lines, ui.Muted.Render("No fresh measurements available.")), "\n")
	}
	for metric := 0; metric < 2; metric++ {
		value := func(i item) float64 {
			if metric == 0 {
				return i.usage.CPU
			}
			return i.usage.Memory
		}
		slices.SortFunc(items, func(a, b item) int {
			if c := cmp.Compare(value(b), value(a)); c != 0 {
				return c
			}
			return strings.Compare(a.name, b.name)
		})
		label, format, style := "CPU", kubernetes.CPUUsage, ui.ChartCPU
		if metric == 1 {
			label, format, style = "Memory", kubernetes.MemoryUsage, ui.ChartMemory
		}
		lines = append(lines, "", SummaryHeading(label, width))
		for _, i := range items[:min(10, len(items))] {
			lines = append(lines, ui.ComparisonBar(i.name, format(value(i))+" · "+i.usage.Timestamp.Local().Format("15:04:05"), new(value(i)), new(value(items[0])), width, style))
		}
	}
	return strings.Join(lines, "\n")
}

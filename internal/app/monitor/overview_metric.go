package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

// Both readers keep their position while source details cover the chart.
type OverviewMetricDetail struct {
	Index   int
	Sources bool
	readers [2]textview.Viewer
}

func (d *OverviewMetricDetail) Title() string {
	return []string{"CPU", "Memory"}[d.Index]
}

func (d *OverviewMetricDetail) Reader() *textview.Viewer {
	if d.Sources {
		return &d.readers[1]
	}
	return &d.readers[0]
}

func (d *OverviewMetricDetail) EnterLabel() string {
	r := d.Reader()
	if r.Searching {
		return "Finish search"
	}
	if r.Query != "" {
		return "Next match"
	}
	if !d.Sources && !r.Selecting {
		return "Source details"
	}
	return ""
}

func (s *OverviewState) OpenMetric() bool {
	if s.Pane != 0 || s.Cursor[0] < 0 || s.Cursor[0] > 1 {
		return false
	}
	s.MetricDetail = &OverviewMetricDetail{Index: s.Cursor[0], readers: [2]textview.Viewer{{Wrap: true}, {Wrap: true}}}
	return true
}

func (s *OverviewState) renderMetric(name string, width, height int, now time.Time) string {
	d := s.MetricDetail
	title, action, back := d.Title()+" · All namespaces", d.EnterLabel(), "Overview"
	if d.Sources {
		title, back = d.Title()+" · Source details", "Chart"
	}
	controls := ui.MouseAction(ui.Muted.Render("Esc "+back), "esc")
	if action != "" {
		controls = ui.MouseAction(ui.TitleStyle.Render("Enter "+action), "enter") + "  ·  " + controls
	}
	if width < 40 {
		title = d.Title() + " · Cluster"
		if d.Sources {
			title = d.Title() + " · Sources"
		} else if action != "" {
			controls = ui.MouseAction(ui.TitleStyle.Render("Enter "+action), "enter")
		}
	}
	heading := ui.OverlayTitleStyle.Render(title)
	if width >= 54 {
		heading = overviewPair(heading, ui.Muted.Render(ui.Clipped(name, width/3)), width)
	}
	heading += "\n" + controls
	top := ui.MeasureViewport(heading, width, height)
	separator := "\n\n"
	if height < 18 {
		separator = "\n"
	}
	bodyHeight := max(1, height-len(top.Rows)-len(separator)+1)
	var content []string
	if d.Sources {
		content = s.metricSources(d.Index, now)
	} else {
		content = s.metricContent(d.Index, width, bodyHeight, now)
	}
	rows := ui.MeasureViewport(strings.Join(content, "\n"), width, bodyHeight).Rows
	lines := make([]textview.Line, 0, len(rows))
	for _, line := range rows {
		lines = append(lines, textview.Line{Text: ansi.Strip(line), Styled: line})
	}
	r := d.Reader()
	r.SetLines(lines)
	return ui.StyledView(top.View(0)+separator+r.View(width, bodyHeight, ui.WorkspaceTheme()), width, height)
}

func (s *OverviewState) metricContent(index, width, height int, now time.Time) []string {
	k := s.Data.Kubernetes
	metric, stale := s.observedMetric(index, now)
	style := ui.ChartCPU
	if index == 1 {
		style = ui.ChartMemory
	}
	value := s.tiles(now)[index].value
	if metric.Used == nil {
		style = ui.ChangedStyle
	}
	lines := []string{style.Bold(true).Render(value), ui.Muted.Render(overviewNumber(metric.Used, index == 1) + " / " + overviewNumber(metric.Allocatable, index == 1) + " allocatable")}
	coverage := fmt.Sprintf("Coverage: %d/%d Nodes", k.MetricsObserved, k.MetricsExpected)
	if !k.MetricsAt.IsZero() {
		coverage += " · source age " + overviewAge(k.MetricsAt, now)
	}
	lines = append(lines, ui.Muted.Render(coverage))
	if stale {
		lines = append(lines, ui.ChangedStyle.Render("STALE · waiting for fresh Node metrics"))
	}
	if k.MetricsError != "" {
		lines = append(lines, ui.ChangedStyle.Render(k.MetricsError))
	}
	lines = append(lines, "")
	facts := s.metricBudget(index, width, now)
	warnings := s.metricWarnings(index, now)
	remainingContent := strings.Join(lines, "\n") + "\n" + strings.Join(facts, "\n") + "\n" + strings.Join(warnings, "\n")
	remaining := ui.MeasureViewport(remainingContent, width, height)
	plotHeight := min(12, max(2, height-len(remaining.Rows)-5))
	lines = append(lines, s.metricHistory(index, width, plotHeight+2, now)...)
	lines = append(lines, "", ui.SectionHeadingStyle.Render("Usage and declared budgets"))
	lines = append(lines, facts...)
	lines = append(lines, "")
	return append(lines, warnings...)
}

func (s *OverviewState) metricBudget(index, width int, now time.Time) []string {
	metric, _ := s.observedMetric(index, now)
	read := s.Data.Kubernetes.Reads["pods"]
	unit := "Cores"
	if index == 1 {
		unit = "GiB"
	}
	var lines []string
	if width >= 54 {
		lines = append(lines, ui.Muted.Render(fmt.Sprintf("%-17s %14s %20s", "", unit, "% of allocatable")))
	}
	for i, n := range []*float64{metric.Used, metric.Allocatable, metric.Requests, metric.Limits} {
		label := []string{"Usage", "Allocatable", "Requests", "Declared limits"}[i]
		value, percent := "Unavailable", ""
		if i >= 2 && (read.ReadAt.IsZero() || read.Error != "" || read.Partial) {
			n = nil
			if read.Partial {
				value = "Partial"
			}
		}
		if n != nil {
			if index == 0 {
				value = fmt.Sprintf("%.2f", *n)
			} else {
				value = fmt.Sprintf("%.1f", *n/(1<<30))
			}
			if metric.Allocatable != nil && *metric.Allocatable > 0 {
				percent = fmt.Sprintf("%.0f%%", 100**n / *metric.Allocatable)
			}
		}
		if width >= 54 {
			lines = append(lines, fmt.Sprintf("%-17s %14s %20s", label, value, percent))
		} else {
			if n != nil {
				value += " cores"
				if index == 1 {
					value = fmt.Sprintf("%.1f GiB", *n/(1<<30))
				}
			}
			if percent != "" {
				value += " · " + percent
			}
			if len(label)+len(value)+2 <= width {
				lines = append(lines, overviewPair(ui.Muted.Render(label), value, width))
			} else {
				lines = append(lines, ui.Muted.Render(label), "  "+value)
			}
		}
	}
	return lines
}

func (s *OverviewState) metricWarnings(index int, now time.Time) []string {
	k := s.Data.Kubernetes
	metric, _ := s.observedMetric(index, now)
	read := k.Reads["pods"]
	var lines []string
	if read.Error != "" {
		lines = append(lines, ui.ChangedStyle.Render("Pod budgets unavailable: "+read.Error))
	} else if read.ReadAt.IsZero() {
		lines = append(lines, ui.ChangedStyle.Render("Pod budgets not collected yet"))
	} else {
		if read.Partial {
			lines = append(lines, ui.ChangedStyle.Render("Partial Pod inventory · budget totals unavailable"))
		}
		if now.Sub(read.ReadAt) > 45*time.Second || s.Retained {
			lines = append(lines, ui.ChangedStyle.Render("STALE · Pod budgets from "+overviewAge(read.ReadAt, now)+" ago"))
		}
		if metric.RequestsMissing > 0 {
			lines = append(lines, ui.ChangedStyle.Render(fmt.Sprintf("%d observed active Pods have missing %s requests.", metric.RequestsMissing, []string{"CPU", "memory"}[index])))
		}
		if metric.LimitsMissing > 0 {
			lines = append(lines, ui.ChangedStyle.Render(fmt.Sprintf("%d observed active Pods have missing %s limits.", metric.LimitsMissing, []string{"CPU", "memory"}[index])))
		}
	}
	lines = append(lines, ui.Muted.Render("Budgets include active unscheduled Pods."), ui.Muted.Render("Declared limits are not a cluster ceiling; unspecified limits may be unbounded."))
	return lines
}

func (s *OverviewState) metricSources(index int, now time.Time) []string {
	k := s.Data.Kubernetes
	lines := []string{
		ui.SectionHeadingStyle.Render("Measurement"),
		"Source: metrics.k8s.io Node observations",
		"Scope: selected cluster, all namespaces; readable objects only",
		fmt.Sprintf("Coverage: %d/%d observed Nodes", k.MetricsObserved, k.MetricsExpected),
		"Fetched: " + overviewEvidenceTime(k.MetricsReadAt),
		"Oldest source: " + overviewEvidenceTime(k.MetricsAt),
		"Newest source: " + overviewEvidenceTime(k.MetricsEnd),
		"Allocatable: capacity reported by the same observed Node set",
	}
	if k.MetricsError != "" {
		lines = append(lines, ui.ChangedStyle.Render(k.MetricsError))
	}
	lines = append(lines, "", ui.SectionHeadingStyle.Render("Declared budgets"),
		fmt.Sprintf("Active Pods observed: %d", k.ActivePods),
		"Source: active Pod specifications, including unscheduled Pods",
		"Collected: "+overviewEvidenceTime(k.Reads["pods"].ReadAt))
	lines = append(lines, s.metricWarnings(index, now)...)
	lines = append(lines, "", ui.SectionHeadingStyle.Render("Chart"),
		"Usage as a percentage of allocatable capacity.",
		"The labeled chart scale is a display bound, not cluster capacity.",
		"Up to 15 minutes of real samples in this session; gaps over 30 seconds stay blank.")
	return lines
}

package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	"image"
	"math"
	"strings"
	"time"
)

type overviewTile struct {
	title, value string
	lines        []string
}

func overviewNumber(value *float64, memory bool) string {
	if value == nil {
		return "Unavailable"
	}
	if memory {
		return fmt.Sprintf("%.1f GiB", *value/(1<<30))
	}
	return fmt.Sprintf("%.2f cores", *value)
}

func overviewAmount(value *float64, memory bool) string {
	if value == nil {
		return "?"
	}
	if memory {
		return fmt.Sprintf("%.1fGi", *value/(1<<30))
	}
	return fmt.Sprintf("%.2fc", *value)
}

func (s *OverviewState) observedMetric(index int, now time.Time) (kubernetes.OverviewMetric, bool) {
	k := s.Data.Kubernetes
	metric := k.CPU
	if index == 1 {
		metric = k.Memory
	}
	stale := !k.MetricsAt.IsZero() && now.Sub(k.MetricsAt) > kubernetes.UsageMaxAge
	if stale || k.MetricsAt.IsZero() {
		metric.Used = nil
	}
	return metric, stale
}

func (s *OverviewState) tiles(now time.Time) []overviewTile {
	k := s.Data.Kubernetes
	tiles := []overviewTile{}
	for i := 0; i < 2; i++ {
		metric, stale := s.observedMetric(i, now)
		title := "CPU"
		if i == 1 {
			title = "Memory"
		}
		used := metric.Used
		value := overviewNumber(used, i == 1)
		if used != nil && metric.Allocatable != nil && *metric.Allocatable > 0 {
			value = fmt.Sprintf("%.0f%% used", 100**used / *metric.Allocatable)
		}
		lines := []string{"Usage " + overviewNumber(used, i == 1), "Allocatable " + overviewNumber(metric.Allocatable, i == 1), "Requests " + overviewNumber(metric.Requests, i == 1), "Limits " + overviewNumber(metric.Limits, i == 1)}
		if k.Reads["pods"].Partial {
			lines[2], lines[3] = "Requests: partial", "Limits: partial"
		}
		if stale {
			value = "STALE"
			lines[0] = "Usage stale"
		}
		tiles = append(tiles, overviewTile{title, value, lines})
	}
	count := func(key string) string {
		r := k.Reads[key]
		if r.Error != "" || r.ReadAt.IsZero() {
			return "Unavailable"
		}
		suffix := ""
		if r.Partial {
			suffix = "+ partial"
		}
		return fmt.Sprint(r.Count) + suffix
	}
	tiles = append(tiles,
		overviewTile{"Nodes", count("nodes"), []string{fmt.Sprintf("%d Ready", k.NodesReady), fmt.Sprintf("%d unknown", k.NodesUnknown)}},
		overviewTile{"Pods", count("pods") + " total", []string{fmt.Sprintf("%d running", k.PodsRunning), fmt.Sprintf("%d pending · %d failed", k.PodsPending, k.PodsFailed), fmt.Sprintf("%d done · %d unknown", k.PodsSucceeded, k.PodsUnknown), fmt.Sprintf("%d restarts (total)", k.Restarts)}},
		overviewTile{"Autoscaling", count("horizontalpodautoscalers.autoscaling") + " HPAs", []string{fmt.Sprintf("Replicas %d → %d", k.HPACurrent, k.HPADesired), fmt.Sprintf("%d up · %d down", k.HPAUp, k.HPADown), fmt.Sprintf("%d stable · %d unknown", k.HPAStable, k.HPAUnknown), "Last scale " + overviewAge(k.HPALastScale, now)}},
	)
	for i, key := range map[int]string{2: "nodes", 3: "pods", 4: "horizontalpodautoscalers.autoscaling"} {
		r := k.Reads[key]
		if r.Error != "" || r.ReadAt.IsZero() {
			tiles[i].value = "Unavailable"
			tiles[i].lines = []string{"Read unavailable", "Enter to open list"}
		}
	}
	if r := k.Reads["nodes"]; r.Error == "" && !r.ReadAt.IsZero() {
		if !r.Partial {
			tiles[2].value = fmt.Sprintf("%d / %d ready", k.NodesReady, r.Count)
		}
		tiles[2].lines = []string{fmt.Sprintf("%d not ready", max(0, r.Count-k.NodesReady-k.NodesUnknown)), fmt.Sprintf("%d unknown", k.NodesUnknown), fmt.Sprintf("%d scheduling off · %d under pressure", k.NodesCordoned, k.NodesPressure)}
	}
	if k.NodesPressureUnknown > 0 {
		tiles[2].lines = append(tiles[2].lines, fmt.Sprintf("Pressure unknown on %d Nodes", k.NodesPressureUnknown))
	}
	if k.HPAUnknown > 0 {
		tiles[4].lines[0] = "Replica totals partial"
	}
	if r := k.Reads["horizontalpodautoscalers.autoscaling"]; k.HPALastScale.IsZero() && r.Error == "" && !r.ReadAt.IsZero() {
		tiles[4].lines[len(tiles[4].lines)-1] = "Last scale: unknown"
	}
	if r := k.Reads["horizontalpodautoscalers.autoscaling"]; r.Error == "" && !r.ReadAt.IsZero() && !r.Partial && r.Count == 0 {
		tiles[4].lines = []string{"No autoscalers", "in readable scope"}
	}
	return tiles
}

func overviewBar(value, total float64, width int, style lipgloss.Style) string {
	if total <= 0 {
		return ui.Muted.Render("No capacity reported")
	}
	n := min(width, max(0, int(math.Round(value/total*float64(width)))))
	if value > 0 && n == 0 {
		n = 1
	}
	return style.Render(strings.Repeat("━", n)) + ui.Muted.Render(strings.Repeat("─", max(0, width-n)))
}

func (s *OverviewState) metricHistory(index, width, height int, now time.Time) []string {
	metric, stale := s.observedMetric(index, now)
	style := ui.ChartCPU
	if index == 1 {
		style = ui.ChartMemory
	}
	plotHeight := max(1, height-2)
	plot := make([]string, plotHeight)
	label := "Collecting history"
	if metric.Used == nil {
		label = "Usage unavailable"
	}
	if stale {
		label = "Source sample stale"
	}
	note := "Actual usage · awaiting samples"
	if metric.Used == nil {
		note = "Metrics unavailable · Enter details"
		if s.Data.Kubernetes.MetricsError != "" {
			note = s.Data.Kubernetes.MetricsError
		}
	}
	if stale {
		note = "Sample age " + overviewAge(s.Data.Kubernetes.MetricsAt, now)
	}
	points := s.Samples[index]
	window := time.Minute
	if len(points) > 0 {
		elapsed := now.Sub(points[0].Time)
		window = min(15*time.Minute, max(time.Minute, (elapsed+time.Minute-1)/time.Minute*time.Minute))
	}
	if len(points) > 0 && metric.Used != nil {
		maximum := s.Scales[index]
		if maximum <= 0 {
			maximum = 1
		}
		plot = ui.TimeChart(points, ui.ChartOptions{Now: now, Window: window, Gap: 30 * time.Second, Width: width, Height: plotHeight, Maximum: maximum, Line: style, Bars: true})
		duration := points[len(points)-1].Time.Sub(points[0].Time)
		span := fmt.Sprintf("%ds", int(duration.Seconds()))
		if duration >= time.Minute {
			span = fmt.Sprintf("%dm", int(duration.Minutes()))
		}
		note = fmt.Sprintf("0–%g%% · %s sampled", maximum, span)
		if len(points) == 1 {
			note = fmt.Sprintf("0–%g%% · 1 sample", maximum)
		}
	} else {
		plot[plotHeight/2] = ui.Muted.Render(ui.Clipped(label, width))
	}
	axis := overviewPair(fmt.Sprintf("-%dm", int(window/time.Minute)), "now", width)
	return append(plot, ui.Muted.Render(axis), ui.Muted.Render(ui.Clipped(note, width)))
}

func (s *OverviewState) metricFacts(index, width int, now time.Time) []string {
	k := s.Data.Kubernetes
	metric, stale := s.observedMetric(index, now)
	style := ui.ChartCPU
	if index == 1 {
		style = ui.ChartMemory
	}
	value := "Unavailable"
	if stale {
		value = "STALE"
	}
	if metric.Used != nil {
		value = overviewNumber(metric.Used, index == 1)
		if metric.Allocatable != nil && *metric.Allocatable > 0 {
			value = fmt.Sprintf("%.0f%% used", 100**metric.Used / *metric.Allocatable)
		}
	}
	if metric.Used == nil {
		style = ui.ChangedStyle
	}
	lines := []string{style.Bold(true).Render(value), ui.Muted.Render(overviewAmount(metric.Used, index == 1) + " / " + overviewAmount(metric.Allocatable, index == 1))}
	for i, n := range []*float64{metric.Requests, metric.Limits} {
		label, missing := "Requests", metric.RequestsMissing
		if i == 1 {
			label, missing = "Limits", metric.LimitsMissing
		}
		if width < 24 {
			label = []string{"Req", "Lim"}[i]
		}
		value := overviewAmount(n, index == 1)
		if k.Reads["pods"].Partial {
			value = "partial"
		} else if n != nil && metric.Allocatable != nil && *metric.Allocatable > 0 {
			value += fmt.Sprintf(" %.0f%%", 100**n / *metric.Allocatable)
		}
		if missing > 0 {
			value += "*"
		}
		lines = append(lines, overviewPair(ui.Muted.Render(label), value, width))
	}
	return lines
}

func (s *OverviewState) metricTileSized(index, width, height int, now time.Time) []string {
	facts := s.metricFacts(index, width, now)
	if height < 8 {
		return facts
	}
	history := s.metricHistory(index, width, height-4, now)
	lines := append([]string{}, facts[:2]...)
	lines = append(lines, history[:len(history)-1]...)
	lines = append(lines, facts[2:]...)
	return append(lines, history[len(history)-1])
}

func (s *OverviewState) pulseSummary(width int, focused bool, now time.Time) string {
	tiles := s.tiles(now)
	l := overviewPulseLayout{columns: 5, rows: 1, summary: true}
	cells := []string{}
	for i, tile := range tiles {
		w := l.tileBounds(i, width, 3).Dx()
		value := tile.value
		if i == 2 {
			value = strings.TrimSuffix(value, " ready")
		}
		if i == 3 {
			value = strings.TrimSuffix(value, " total")
		}
		if i == 4 {
			value = strings.TrimSuffix(value, " HPAs")
		}
		style := ui.TitleStyle
		if i == 0 {
			style = ui.ChartCPU.Bold(true)
		}
		if i == 1 {
			style = ui.ChartMemory.Bold(true)
		}
		if value == "Unavailable" || value == "STALE" {
			style = ui.ChangedStyle.Bold(true)
		}
		cells = append(cells, ui.MouseTarget(overviewBox(tile.title, style.Render(value), w, 3, focused && s.Pane == 0 && s.Cursor[0] == i), fmt.Sprintf("overview-tile/%d", i)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, interleaveOverview(cells)...)
}

func (s *OverviewState) pulseDetail(index, width, height int, focused bool, now time.Time) string {
	innerWidth, innerHeight := width-4, height-2
	leftWidth := min(34, innerWidth/2)
	var left, right []string
	if index < 2 {
		left = s.metricFacts(index, leftWidth, now)[1:]
		metric, _ := s.observedMetric(index, now)
		if metric.RequestsMissing > 0 || metric.LimitsMissing > 0 {
			left = append(left, ui.Muted.Render("* unspecified · Enter details"))
		}
		right = s.metricHistory(index, innerWidth-leftWidth-2, innerHeight, now)
	} else {
		left = append(left, s.tiles(now)[index].lines...)
		if change := s.observationDeltaLine(index, now); change != "" {
			left = append(left, change)
		}
		cut := min(innerHeight, len(left))
		right, left = left[cut:], left[:cut]
	}
	lines := make([]string, innerHeight)
	for i := range lines {
		a, b := "", ""
		if i < len(left) {
			a = ui.Clipped(left[i], leftWidth)
		}
		if i < len(right) {
			b = right[i]
		}
		lines[i] = a + strings.Repeat(" ", max(0, leftWidth-lipgloss.Width(a))+2) + b
	}
	title := s.tiles(now)[index].title + " · detail"
	if index < 2 {
		metric, _ := s.observedMetric(index, now)
		if metric.RequestsMissing > 0 || metric.LimitsMissing > 0 {
			title += " · * unspecified budgets"
		}
	}
	return ui.MouseTarget(overviewBox(title, strings.Join(lines, "\n"), width, height, focused), fmt.Sprintf("overview-tile/%d", index))
}

type overviewPulseLayout struct {
	columns, rows, start int
	summary              bool
}

func (s *OverviewState) pulseLayout(width, height int) overviewPulseLayout {
	l := overviewPulseLayout{columns: min(5, max(1, (width+1)/24)), rows: 1}
	if width >= 64 && width < 120 && !s.Expanded {
		l.columns, l.summary = 5, true
	} else if s.Expanded && height >= 24 && l.columns < 5 {
		l.rows, l.columns = 2, min(l.columns, 3)
	}
	count := min(5, l.columns*l.rows)
	l.start = (min(s.Cursor[0], 4) / count) * count
	return l
}

func (l overviewPulseLayout) tileBounds(index, width, height int) image.Rectangle {
	col, row := (index-l.start)%l.columns, (index-l.start)/l.columns
	w := (width - l.columns + 1) / l.columns
	x := col * (w + 1)
	if col == l.columns-1 {
		w = width - x
	}
	h := min(18, max(3, (height-1)/l.rows))
	if l.summary {
		h = 3
	}
	return image.Rect(x, 1+row*h, x+w, 1+(row+1)*h)
}

func (s *OverviewState) renderPulse(width, height int, focused bool, now time.Time) string {
	tiles := s.tiles(now)
	l := s.pulseLayout(width, height)
	if l.summary {
		return ui.StyledView(ui.SectionHeadingStyle.Render("◉ Cluster Pulse")+"\n"+s.pulseSummary(width, focused, now)+"\n"+s.pulseDetail(min(s.Cursor[0], 4), width, height-4, focused, now), width, height)
	}
	heading := ui.SectionHeadingStyle.Render("◉ Cluster Pulse")
	lines := []string{heading}
	for row := 0; row < l.rows && l.start+row*l.columns < 5; row++ {
		cells := []string{}
		for col := 0; col < l.columns; col++ {
			index := l.start + row*l.columns + col
			if index >= 5 {
				break
			}
			bounds := l.tileBounds(index, width, height)
			w, tileHeight := bounds.Dx(), bounds.Dy()
			body := s.pulseBody(index, w-4, tileHeight-2, now)
			title := tiles[index].title
			if index < 2 {
				metric, _ := s.observedMetric(index, now)
				if metric.RequestsMissing > 0 || metric.LimitsMissing > 0 {
					title += " · * unspecified"
				}
			}
			cells = append(cells, ui.MouseTarget(overviewBox(title, strings.Join(body, "\n"), w, tileHeight, focused && index == min(s.Cursor[0], 4)), fmt.Sprintf("overview-tile/%d", index)))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, interleaveOverview(cells)...))
	}
	return ui.StyledView(strings.Join(lines, "\n"), width, height)
}

func (s *OverviewState) pulseBody(index, width, height int, now time.Time) []string {
	if index < 2 {
		return s.metricTileSized(index, width, height, now)
	}
	tiles := s.tiles(now)
	t := tiles[index]
	body := append([]string{ui.TitleStyle.Render(t.value)}, t.lines...)
	if t.value == "Unavailable" {
		body[0] = ui.ChangedStyle.Bold(true).Render(t.value)
		return body
	}
	k := s.Data.Kubernetes
	switch index {
	case 2:
		r := k.Reads["nodes"]
		blocks := overviewBar(float64(k.NodesReady), float64(r.Count), width, ui.AddedStyle)
		if r.Count > 0 && r.Count <= width {
			blocks = ui.AddedStyle.Render(strings.Repeat("■", k.NodesReady)) + ui.RemovedStyle.Render(strings.Repeat("□", max(0, r.Count-k.NodesReady-k.NodesUnknown))) + ui.Muted.Render(strings.Repeat("?", k.NodesUnknown))
		}
		body = []string{body[0], ui.Muted.Render("Ready / total"), blocks, "", fmt.Sprintf("%d not ready · %d ?", max(0, r.Count-k.NodesReady-k.NodesUnknown), k.NodesUnknown), fmt.Sprintf("%d scheduling off", k.NodesCordoned), fmt.Sprintf("%d under pressure", k.NodesPressure)}
		if k.NodesPressureUnknown > 0 {
			body[len(body)-1] = fmt.Sprintf("%d pressure · %d ?", k.NodesPressure, k.NodesPressureUnknown)
		}
	case 3:
		r := k.Reads["pods"]
		bar, previous, cumulative := "", 0, 0
		counts := []int{k.PodsRunning, k.PodsPending, k.PodsFailed, k.PodsSucceeded, k.PodsUnknown}
		styles := []lipgloss.Style{ui.AddedStyle, ui.ChangedStyle, ui.RemovedStyle, ui.InfoStyle, ui.Muted}
		labels := []string{"Running", "Pending", "Failed", "Done", "Unknown"}
		for i, n := range counts {
			cumulative += n
			end := int(math.Round(float64(cumulative) * float64(width) / float64(max(1, r.Count))))
			bar += styles[i].Render(strings.Repeat("━", max(0, end-previous)))
			previous = end
		}
		if r.Count == 0 {
			bar = ui.Muted.Render("Empty inventory")
		}
		body = []string{body[0], bar}
		for i, n := range counts {
			if i == 4 && n == 0 {
				continue
			}
			percent := fmt.Sprintf("%.0f%%", 100*float64(n)/float64(max(1, r.Count)))
			if r.Partial {
				percent = "partial"
			}
			body = append(body, overviewPair(styles[i].Render(fmt.Sprintf("%d %s", n, strings.ToLower(labels[i]))), ui.Muted.Render(percent), width))
		}
		body = append(body, ui.Muted.Render(fmt.Sprintf("%d restarts (total)", k.Restarts)))
		if change := s.observationDeltaLine(index, now); change != "" {
			body = append(body, change)
		}
	case 4:
		r := k.Reads["horizontalpodautoscalers.autoscaling"]
		if r.Count == 0 {
			return body
		}
		body = []string{body[0], fmt.Sprintf("Replicas %d → %d", k.HPACurrent, k.HPADesired), "", ui.InfoStyle.Render(fmt.Sprintf("↑ %d scaling up", k.HPAUp)), ui.InfoStyle.Render(fmt.Sprintf("↓ %d scaling down", k.HPADown)), ui.AddedStyle.Render(fmt.Sprintf("= %d stable", k.HPAStable)), ui.Muted.Render("Last scale " + overviewAge(k.HPALastScale, now))}
		if change := s.observationDeltaLine(index, now); change != "" {
			body[2] = change
		}
		if k.HPAUnknown > 0 {
			body[1] = "Replica totals partial"
			body = append(body, fmt.Sprintf("? %d unknown", k.HPAUnknown))
		}
	}
	if len(body) > height && index == 3 && height >= 7 {
		body = append(body[:1], body[2:]...)
	}
	return body
}

func (s *OverviewState) observationDeltaLine(index int, now time.Time) string {
	if s.Retained || s.Error != "" {
		return ""
	}
	d := s.Delta
	if index == 3 && d.Restarts != nil && *d.Restarts > 0 && overviewDeltaCurrent(d.RestartsUntil, now) {
		return ui.ChangedStyle.Bold(true).Render(fmt.Sprintf("+%d restarts / %s", *d.Restarts, overviewInterval(d.RestartsSince, d.RestartsUntil)))
	}
	if index == 4 && d.Replicas != nil && *d.Replicas != 0 && overviewDeltaCurrent(d.ReplicasUntil, now) {
		return ui.TitleStyle.Render(fmt.Sprintf("%+d replicas / %s", *d.Replicas, overviewInterval(d.ReplicasSince, d.ReplicasUntil)))
	}
	return ""
}

package monitor

import (
	"fmt"
	"image"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
)

func overviewBox(title, body string, width, height int, focused bool) string {
	if width < 4 || height < 3 {
		return ui.StyledView(body, width, height)
	}
	color, heading, marker := ui.BorderColor, ui.SectionHeadingStyle, " "
	if focused {
		color, heading, marker = ui.FocusedBorder, ui.OverlayTitleStyle, "▸"
	}
	edge := lipgloss.NewStyle().Foreground(color)
	top := ui.Clipped(marker+" "+ui.OneLine(title)+" ", width-2)
	lines := []string{edge.Render("╭") + heading.Render(top) + edge.Render(strings.Repeat("─", max(0, width-2-lipgloss.Width(top)))+"╮")}
	rows := strings.Split(body, "\n")
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(rows) {
			line = ui.Clipped(rows[i], width-4)
		}
		lines = append(lines, edge.Render("│")+" "+line+strings.Repeat(" ", max(0, width-3-lipgloss.Width(line)))+edge.Render("│"))
	}
	lines = append(lines, edge.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(lines, "\n")
}

type overviewLayout struct {
	pulse, lower, columns int
	compact               bool
}

func (l overviewLayout) paneBounds(pane, width int) image.Rectangle {
	if l.compact || l.columns == 1 {
		return image.Rect(0, 0, width, l.lower)
	}
	w := (width - l.columns + 1) / l.columns
	x, y := (pane-1)*(w+1), 0
	if l.columns == 2 && pane == 3 {
		x = w + 1
	}
	if pane == l.columns || l.columns == 2 && pane == 3 {
		w = width - (l.columns-1)*(w+1)
	}
	h := l.lower
	if l.columns == 2 && pane > 1 {
		snapshot := min(8, max(6, l.lower/2))
		h = l.lower - snapshot - 1
		if pane == 3 {
			y = l.lower - snapshot
			h = snapshot
		}
	}
	return image.Rect(x, y, x+w, y+h)
}

func (s *OverviewState) layout(width, height int) overviewLayout {
	available := max(0, height-2)
	if s.Input != nil || s.Query != "" {
		available--
	}
	l := overviewLayout{compact: width < 64 || height < 28 || s.Expanded}
	if l.compact {
		l.lower = max(3, available-2)
		return l
	}
	l.pulse = min(16, max(14, available/3))
	if width < 120 {
		l.pulse = min(14, max(12, available/3))
	}
	l.lower = available - l.pulse - 1
	l.columns = 1
	if width >= 120 {
		l.columns = 3
	} else if width >= 78 {
		l.columns = 2
	}
	return l
}

func (s *OverviewState) Render(name string, width, height int, focused bool, now time.Time) string {
	if s.MetricDetail != nil {
		return s.renderMetric(name, width, height, now)
	}
	if width < 8 || height < 4 {
		return ui.StyledView("Overview · resize for details", width, height)
	}
	state := "Loading"
	if s.Loaded {
		state = "Observed " + s.Data.Kubernetes.ReadAt.Local().Format("15:04:05")
	}
	if s.Loaded && now.Sub(s.Data.Kubernetes.ReadAt) > 45*time.Second {
		state = "STALE · " + state
	}
	if s.partial() {
		state = "PARTIAL · " + state
	}
	if s.Loading {
		state += " · refreshing"
	}
	if s.Error != "" {
		state = "Unavailable · g Retry"
		if s.Retained {
			state = "STALE · refresh failed"
		}
	}
	health, style := s.healthStatus()
	if s.Loaded && (now.Sub(s.Data.Kubernetes.ReadAt) > 45*time.Second || s.Retained) {
		health, style = "? Stale health", ui.ChangedStyle
	}
	coverage := "All namespaces"
	if len(s.notices()) > 0 {
		label := "coverage notices"
		if len(s.notices()) == 1 {
			label = "coverage notice"
		}
		coverage += fmt.Sprintf(" · %d %s", len(s.notices()), label)
	}
	header := overviewPair(ui.OverlayTitleStyle.Render("◉ "+overviewIdentity(name, width/2)), style.Render(health), width) + "\n" + overviewPair(ui.Muted.Render(coverage), ui.Muted.Render(state), width)
	if s.Input != nil || s.Query != "" {
		query := s.Query
		if s.Input != nil {
			query = s.Input.Display(focused)
		}
		header += "\n" + ui.Clipped("/ "+query+" · Esc clear", width)
	}
	l := s.layout(width, height)
	if l.compact {
		return ui.StyledView(header+"\n"+s.healthStrip(width, now)+"\n"+s.paneLabel(width)+"\n"+s.renderPane(s.Pane, width, l.lower, focused, now), width, height)
	}
	pulse := s.renderPulse(width, l.pulse, focused && s.Pane == 0, now)
	panes := []int{1, 2, 3}
	if l.columns == 1 {
		pane := max(1, s.Pane)
		panes = []int{pane}
	}
	if l.columns == 2 {
		leftBounds, attention, snapshot := l.paneBounds(1, width), l.paneBounds(2, width), l.paneBounds(3, width)
		left := s.renderPane(1, leftBounds.Dx(), leftBounds.Dy(), focused, now)
		right := s.renderPane(2, attention.Dx(), attention.Dy(), focused, now) + "\n\n" + s.renderPane(3, snapshot.Dx(), snapshot.Dy(), focused, now)
		return ui.StyledView(header+"\n"+pulse+"\n"+lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right), width, height)
	}
	blocks := []string{}
	for _, p := range panes {
		bounds := l.paneBounds(p, width)
		blocks = append(blocks, s.renderPane(p, bounds.Dx(), bounds.Dy(), focused, now))
	}
	return ui.StyledView(header+"\n"+pulse+"\n"+lipgloss.JoinHorizontal(lipgloss.Top, interleaveOverview(blocks)...), width, height)
}

func (s *OverviewState) paneLabel(width int) string {
	names := []string{"Pulse", "Deployments", "Attention", "Snapshot"}
	parts := []string{}
	if s.Pane > 0 {
		parts = append(parts, "↑ "+names[s.Pane-1])
	}
	if s.Pane < 3 {
		parts = append(parts, "↓ "+names[s.Pane+1])
	}
	return ui.Muted.Render(ui.Clipped(strings.Join(parts, " · ")+" · at section edge", width))
}

func overviewPair(left, right string, width int) string {
	if lipgloss.Width(right)+2 >= width {
		return ui.Clipped(right, width)
	}
	left = ui.Clipped(left, max(0, width-lipgloss.Width(right)-1))
	return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

func overviewIdentity(value string, width int) string {
	value = ui.OneLine(value)
	if lipgloss.Width(value) <= width {
		return value
	}
	if width < 7 {
		return ui.Clipped(value, width)
	}
	tail := min(12, width/2)
	return ansi.Truncate(value, width-tail-1, "") + "…" + ansi.TruncateLeft(value, lipgloss.Width(value)-tail, "")
}

func overviewAge(at, now time.Time) string {
	if at.IsZero() {
		return "time unknown"
	}
	d := now.Sub(at)
	if d < 0 {
		return "clock skew"
	}
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func overviewStatus(row OverviewRow) (string, lipgloss.Style) {
	status := row.Status
	if row.Severity == 2 {
		return "! Error", ui.RemovedStyle.Bold(true)
	}
	if row.Severity == 1 {
		return "! Warning", ui.ChangedStyle.Bold(true)
	}
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "fail") || strings.Contains(lower, "degraded") || strings.Contains(lower, "error"):
		return "× " + status, ui.RemovedStyle
	case strings.Contains(lower, "unknown") || strings.Contains(lower, "previous") || status == "":
		return "? " + strings.TrimSpace(status), ui.Muted
	case strings.Contains(lower, "suspend") || strings.Contains(lower, "progress") || strings.Contains(lower, "pending") || strings.Contains(lower, "outofsync") || strings.Contains(lower, "not ready"):
		return "~ " + status, ui.ChangedStyle
	case lower == "succeeded" || lower == "available" || lower == "deployed" || lower == "ready" || lower == "synced / healthy":
		return "✓ " + status, ui.AddedStyle
	default:
		return "? " + status, ui.Muted
	}
}

type overviewPaneContent struct {
	rows          []OverviewRow
	lines         []string
	starts, ends  []int
	width, height int
	intro         string
}

func (s *OverviewState) paneContent(pane, width, height int, focused bool, now time.Time) overviewPaneContent {
	p := overviewPaneContent{rows: s.Filtered(pane), width: max(1, width-4), height: height - 3}
	// Narrow sections use the outer main frame instead of nesting another box.
	if width < 28 {
		p.width, p.height = max(1, width), height-2
	}
	switch pane {
	case 1:
		if width >= 28 {
			p.intro = overviewPair("TIME  WORKLOAD / CHANGE", "RESULT", p.width)
		}
	case 2:
		if width >= 28 {
			p.intro = overviewPair("CAUSE · × error / ! warning", "AGE", p.width)
		}
		if s.Group != "" {
			p.intro = "Affected resources · Esc group"
		}
	case 3:
		if s.InventoryTool != "" {
			p.intro = overviewBrand(s.InventoryTool) + " · " + overviewCounterLabel(s.InventoryCounter) + " · " + s.inventoryCoverage()
		}
	}
	if p.intro != "" {
		p.height--
	}
	p.height = max(1, p.height)
	for i, row := range p.rows {
		p.starts = append(p.starts, len(p.lines))
		lines := s.rowLines(row, pane, p.width, focused && s.Pane == pane && s.Cursor[pane] == i, now)
		if pane == 3 && s.InventoryTool != "" {
			lines = s.rowLines(row, 4, p.width, focused && s.Pane == 3 && s.Cursor[3] == i, now)
		} else if pane == 3 && height < 14 && (len(p.rows) > 1 || height < 9) {
			lines = overviewCompactIntegration(row, p.width, focused && s.Pane == 3 && s.Cursor[3] == i, s.Counter, now)
		}
		if p.height < 3 {
			lines = []string{overviewIdentity(row.Title, p.width)}
		}
		lines = ui.MeasureViewport(strings.Join(lines, "\n"), p.width, height).Rows
		p.lines = append(p.lines, lines...)
		p.ends = append(p.ends, len(p.lines))
		if pane == 1 || pane == 2 && height >= 18 {
			p.lines = append(p.lines, lipgloss.NewStyle().Foreground(ui.BorderColor).Render(strings.Repeat("─", p.width)))
		}
	}
	return p
}

func (s *OverviewState) paneOffset(p overviewPaneContent, pane int) int {
	if len(p.rows) == 0 {
		return 0
	}
	selected := min(max(0, s.Cursor[pane]), len(p.rows)-1)
	offset := min(max(0, s.Scroll[pane]), max(0, len(p.lines)-p.height))
	if p.starts[selected] < offset {
		offset = p.starts[selected]
	}
	end := min(p.ends[selected], p.starts[selected]+p.height)
	if end > offset+p.height {
		offset = end - p.height
	}
	for _, start := range p.starts {
		if start >= offset && start <= p.starts[selected] {
			offset = start
			break
		}
	}
	return max(0, offset)
}

func (s *OverviewState) Resize(width, height int) {
	s.Width, s.Height = width, height
	if width < 8 || height < 4 {
		return
	}
	l := s.layout(width, height)
	for pane := 1; pane < 4; pane++ {
		bounds := l.paneBounds(pane, width)
		p := s.paneContent(pane, bounds.Dx(), bounds.Dy(), false, time.Now())
		s.Scroll[pane] = s.paneOffset(p, pane)
	}
}

func (s *OverviewState) renderPane(pane, width, height int, focused bool, now time.Time) string {
	if pane == 0 {
		return s.renderPulse(width, height, focused, now)
	}
	p := s.paneContent(pane, width, height, focused, now)
	offset := s.paneOffset(p, pane)
	end := min(len(p.lines), offset+p.height)
	for i, start := range p.starts {
		if start >= end {
			break
		}
		if start >= offset && p.ends[i] > end && i != s.Cursor[pane] {
			end = start
			break
		}
	}
	body := ""
	if p.intro != "" {
		body = ui.Muted.Render(ui.Clipped(p.intro, p.width)) + "\n"
	}
	footer := ""
	if len(p.rows) == 0 {
		empty := "No matching entries"
		if s.Query == "" && s.Input == nil {
			empty = "No timestamped changes observed"
			if pane == 2 {
				empty = "No warnings in readable results"
			}
			if pane == 3 && s.InventoryTool != "" {
				empty = "No matching objects in readable results"
				if s.inventoryCoverage() == "Unavailable" {
					empty = "Source unavailable · see Availability"
				}
			}
			if !s.Loaded {
				empty = "Loading observations…"
			}
		}
		p.lines = []string{ui.Muted.Render(empty)}
	} else {
		first, last := 0, 0
		for i := range p.rows {
			if p.ends[i] > offset && first == 0 {
				first = i + 1
			}
			if p.starts[i] < end {
				last = i + 1
			}
		}
		footer = fmt.Sprintf("%d–%d of %d", first, last, len(p.rows))
		if end < len(p.lines) {
			footer += " · ↓ more"
		}
		if pane == 3 {
			count := 0
			var oldest time.Time
			for _, r := range p.rows {
				if r.Target.Tool != "" {
					count++
					if !r.At.IsZero() && (oldest.IsZero() || r.At.Before(oldest)) {
						oldest = r.At
					}
				}
			}
			label := "integrations"
			if count == 1 {
				label = "integration"
			}
			footer = fmt.Sprintf("%d %s", count, label)
			if height >= 14 && !oldest.IsZero() {
				footer += " · oldest read " + overviewAge(oldest, now)
			}
			if offset+p.height < len(p.lines) {
				footer += " · ↓ more"
			}
			if s.InventoryTool != "" {
				footer = fmt.Sprintf("%d observed objects", len(p.rows))
			}
			if focused && s.Pane == 3 && s.InventoryTool == "" && s.Counter > 0 {
				row := p.rows[min(s.Cursor[3], len(p.rows)-1)]
				if s.Counter <= len(row.Counters) {
					c := row.Counters[s.Counter-1]
					footer = overviewCounterText(c) + " · Enter " + s.EnterLabel()
				}
			}
		}
	}
	visible := p.lines
	if len(p.rows) > 0 {
		visible = p.lines[offset:end]
	}
	for len(visible) < p.height {
		visible = append(visible, "")
	}
	body += strings.Join(visible, "\n") + "\n" + ui.Muted.Render(ui.Clipped(footer, p.width))
	title := OverviewPanes[pane]
	if lipgloss.Width(title)+4 > width {
		title = []string{"Pulse", "Deployments", "Attention", "Snapshot"}[pane]
	}
	if width < 28 {
		style, marker := ui.SectionHeadingStyle, " "
		if focused && s.Pane == pane {
			style, marker = ui.OverlayTitleStyle, "▸"
		}
		return ui.StyledView(style.Render(marker+" "+title)+"\n"+body, width, height)
	}
	if width >= 36 {
		title = []string{"", "↗ Recent Deployments", "! Needs Attention", "◈ Infrastructure Snapshot"}[pane]
	}
	return overviewBox(title, body, width, height, focused && s.Pane == pane)
}

func interleaveOverview(cells []string) []string {
	result := []string{}
	for i, cell := range cells {
		if i > 0 {
			result = append(result, " ")
		}
		result = append(result, cell)
	}
	return result
}

func (s *OverviewState) healthStatus() (string, lipgloss.Style) {
	if !s.Loaded {
		return "Loading health", ui.Muted
	}
	errors, warnings := 0, 0
	for _, row := range s.Rows[2] {
		n := max(1, len(row.Members))
		if row.Severity == 2 {
			errors += n
		} else {
			warnings += n
		}
	}
	if errors > 0 {
		return fmt.Sprintf("! %d errors · %d warnings · %d groups", errors, warnings, len(s.Rows[2])), ui.RemovedStyle
	}
	if warnings > 0 {
		return fmt.Sprintf("! %d warnings", warnings), ui.ChangedStyle
	}
	if s.Error != "" {
		return "? Health unavailable", ui.ChangedStyle
	}
	if s.partial() {
		return "? Partial health", ui.ChangedStyle
	}
	if !s.healthComplete() {
		return "? Health incomplete", ui.ChangedStyle
	}
	return "✓ No observed warnings", ui.AddedStyle
}

func (s *OverviewState) healthStrip(width int, now time.Time) string {
	values := []string{}
	for i, name := range []string{"CPU", "MEM"} {
		metric, _ := s.observedMetric(i, now)
		value := "?"
		if metric.Used != nil && metric.Allocatable != nil && *metric.Allocatable > 0 {
			value = fmt.Sprintf("%.0f%%", 100**metric.Used / *metric.Allocatable)
		}
		values = append(values, name+" "+value)
	}
	nodes := "?"
	if r := s.Data.Kubernetes.Reads["nodes"]; !r.ReadAt.IsZero() && r.Error == "" && !r.Partial {
		nodes = fmt.Sprintf("%d/%d", s.Data.Kubernetes.NodesReady, r.Count)
	}
	values = append(values, "Nodes "+nodes)
	for _, item := range []struct{ name, key string }{{"Pods", "pods"}, {"HPA", "horizontalpodautoscalers.autoscaling"}} {
		value := "?"
		if r := s.Data.Kubernetes.Reads[item.key]; !r.ReadAt.IsZero() && r.Error == "" && !r.Partial {
			value = fmt.Sprint(r.Count)
		}
		values = append(values, item.name+" "+value)
	}
	return ui.OverlayTitleStyle.Render(ui.Clipped(strings.Join(values, " · "), width))
}

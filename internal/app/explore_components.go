package app

import (
	"fmt"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) resourceCountSummary(rows []kubernetes.Row) string {
	if c := m.explorer().Inventory; c != nil {
		if c.Summary == "" {
			c.Summary = m.prepareResourceCountSummary(rows)
		}
		return c.Summary
	}
	return m.prepareResourceCountSummary(rows)
}

func (m *model) prepareResourceCountSummary(rows []kubernetes.Row) string {
	e := m.explorer()
	text := fmt.Sprintf("%d %s", len(rows), monitor.ResourceTitle(e.Resource))
	if e.Resource.Namespaced {
		ns := e.Preference.Namespace
		if ns == "" {
			ns = "all namespaces"
		}
		text += " · " + ui.OneLine(ns)
	}
	if e.Resource.Key() == "pods" {
		unready, pending := 0, 0
		for _, row := range rows {
			ready, total, _, status := kubernetes.PodState(row.Object.Object)
			if status == "Pending" || status == "ContainerCreating" || status == "PodInitializing" {
				pending++
			} else if ready < total && status != "Succeeded" && status != "Completed" {
				unready++
			}
		}
		if unready > 0 {
			text += fmt.Sprintf(" · %d not ready", unready)
		}
		if pending > 0 {
			text += fmt.Sprintf(" · %d pending", pending)
		}
	}
	return text
}

func (m *model) compactResourceList(columns []string, rows []kubernetes.Row, before []string, width, height int) string {
	e := m.explorer()
	cursor := 0
	for i, row := range rows {
		if kubernetes.Identity(row.Object) == e.Selected {
			cursor = i
			break
		}
	}
	available := max(0, height-len(before)-1)
	body := ui.CollectionWindow(len(rows), cursor, available, func(i int) []string {
		row := rows[i]
		active := kubernetes.Identity(row.Object) == e.Selected
		name := row.Object.GetName()
		if e.Resource.Namespaced && e.Preference.Namespace == "" {
			name = row.Object.GetNamespace() + "/" + name
		}
		prefix := "  "
		if active {
			prefix = "› "
		}
		item := []string{ui.TitleStyle.Render(ui.Clipped(prefix+ui.OneLine(name), width))}
		if e.Resource.Key() == "pods" {
			ready, total, restarts, state := kubernetes.PodState(row.Object.Object)
			item = append(item, strings.Split(ansi.Wrap(fmt.Sprintf("  %d/%d ", ready, total)+monitor.StatusValue(state)+fmt.Sprintf("  ↻%d", restarts), width, ""), "\n")...)
			cpu, memory := slices.Index(columns, "CPU"), slices.Index(columns, "Memory")
			if cpu >= 0 && memory >= 0 {
				item = append(item, ui.Muted.Render(ui.Clipped("  CPU "+row.Cells[cpu]+" · Memory "+row.Cells[memory], width)))
			}
		} else {
			var values []string
			for i, column := range columns {
				if column != "Name" && column != "Namespace" && i < len(row.Cells) {
					values = append(values, ui.Muted.Render(column+" ")+monitor.SummaryValue(column, row.Cells[i]))
				}
			}
			wrapped := strings.Split(ansi.Wrap(strings.Join(values, " · "), max(1, width-2), ""), "\n")
			for _, line := range wrapped {
				item = append(item, "  "+line)
			}
		}
		for i, line := range item {
			if active {
				line = lipgloss.NewStyle().Background(ui.Highlight).Width(width).Render(line)
			}
			item[i] = ui.MouseTarget(line, "resource/"+mouseResourceID(row.Object))
		}
		return item
	})
	return strings.Join(before, "\n") + "\n" + body + "\n" + ui.Muted.Render(ui.Clipped(m.resourceCountSummary(rows), width))
}

func (m *model) resourceSummary(width int) string {
	e := m.explorer()
	if e.Detail == nil || width <= 0 {
		return ""
	}
	var sections []string
	content := kubernetes.Summary(e.Resource, *e.Detail)
	if len(e.Supplement) > 0 {
		var combined []kubernetes.DetailSection
		if len(content) > 0 {
			combined = append(combined, content[0])
		}
		for _, section := range e.Supplement {
			if section.Title != "Assigned Pods" {
				combined = append(combined, section)
			}
		}
		if len(content) > 1 {
			combined = append(combined, content[1:]...)
		}
		for _, section := range e.Supplement {
			if section.Title == "Assigned Pods" {
				combined = append(combined, section)
			}
		}
		content = combined
	}
	for index, section := range content {
		if len(section.Fields) == 0 && len(section.Rows) == 0 && len(section.Bars) == 0 {
			continue
		}
		lines := []string{monitor.SummaryHeading(section.Title, width)}
		lines = append(lines, monitor.RenderSummaryFields(section.Fields, width)...)
		if len(section.Rows) > 0 {
			if len(section.Fields) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, monitor.RenderDetailTable(section, width, e.Container)...)
		}
		lines = append(lines, monitor.RenderDetailBars(section.Bars, width)...)
		sections = append(sections, strings.Join(lines, "\n"))
		if index == 0 {
			for _, chart := range kubernetes.StatusCharts(e.Resource, *e.Detail) {
				sections = append(sections, monitor.RenderChartSection(chart, width))
			}
		}
		if e.Resource.Key() == "pods" && section.Title == "Containers" || e.Resource.Key() != "pods" && index == 0 {
			if usage := m.usageSummary(width); usage != "" {
				sections = append(sections, usage)
			}
		}
	}
	if e.SupplementNote != "" {
		sections = append([]string{ui.ChangedStyle.Render(ansi.Wrap(ui.Plain(e.SupplementNote), width, ""))}, sections...)
	}
	return strings.Join(sections, "\n\n")
}

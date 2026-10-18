package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) relatedContent(width int) (string, int, int) {
	e := m.explorer()
	if len(e.References) == 0 {
		text := "No related resources found."
		if e.DetailRun != nil && !e.DetailLoaded {
			text = m.loading("Loading relationships…")
		}
		if e.DetailText != "" {
			text += "\n\n" + e.DetailText
		}
		return text, 0, 0
	}
	columns := []string{"Kind", "Name", "Namespace", "Relationship"}
	section := kubernetes.DetailSection{Columns: columns}
	for _, ref := range e.References {
		ns, name := ref.Namespace, ref.Name
		if ns == "" {
			ns = "cluster"
			if ref.Resource.Namespaced {
				ns = "all namespaces"
			}
		}
		if ref.List {
			name = "Open list"
		}
		section.Rows = append(section.Rows, []string{ref.Resource.Kind, name, ns, ref.Relation})
	}
	widths, tabular := monitor.DetailTableWidths(section, width)
	tabular = tabular && width >= 72
	// Keep relationship conditions readable when four columns become too narrow.
	tabular = tabular && width >= 64
	var lines []string
	selectedLine, selectedEnd, group := 0, 0, -1
	for i, ref := range e.References {
		next := monitor.RelatedGroup(ref)
		if next != group {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, monitor.SummaryHeading(monitor.RelatedGroups[next], width))
			if next == 3 {
				lines = append(lines, ui.Muted.Render(ui.Clipped("Label matches do not establish ownership.", width)))
			}
			if tabular {
				lines = append(lines, monitor.RenderDetailTableHeader(columns, widths)...)
			}
			group = next
		}
		rowStart := len(lines)
		active := i == e.RelatedCursor
		if active {
			selectedLine = len(lines)
		}
		if tabular {
			lines = append(lines, monitor.RenderDetailTableRow(columns, widths, section.Rows[i], false, active)...)
		} else {
			row := section.Rows[i]
			prefix := "  "
			if active {
				prefix = "› "
			}
			text := prefix + row[0] + " / " + row[1] + "\n  " + row[2] + " · " + row[3]
			text = ansi.Wrap(text, max(1, width), "")
			if active {
				text = ui.Selected.Render(text)
			}
			lines = append(lines, strings.Split(text, "\n")...)
		}
		for j := rowStart; j < len(lines); j++ {
			lines[j] = ui.MouseTarget(lines[j], "related/"+ref.Resource.Key()+"/"+ref.Namespace+"/"+ref.Name+"/"+ref.Relation)
		}
		if active {
			selectedEnd = len(lines)
		}
	}
	if e.DetailText != "" {
		lines = append(lines, "", monitor.SummaryHeading("Unavailable relationships", width), ansi.Wrap(ui.Plain(e.DetailText), max(1, width), ""))
	}
	return strings.Join(lines, "\n"), selectedLine, selectedEnd
}

func (m *model) relatedViewport(width, height int) (ui.Viewport, int, string) {
	text, selectedLine, selectedEnd := m.relatedContent(width)
	e := m.explore
	hint := ""
	if len(e.References) > 0 {
		hint = "↑↓ Select · Enter Open resource · / Find"
		if e.DetailText != "" {
			hint = "↑↓ Select · Enter Open · End Read notes · / Find"
		}
		height--
	}
	layout := ui.MeasureViewport(text, width, height)
	offset := max(0, selectedEnd-max(1, height))
	offset = max(min(offset, selectedLine), e.Scroll)
	return layout, offset, hint
}

func (m *model) relatedBody(width, height int) string {
	if m.activePanelSearch(1) != nil {
		text, _, _ := m.relatedContent(width)
		return text
	}
	layout, offset, hint := m.relatedViewport(width, height)
	text := layout.View(offset)
	if hint != "" {
		text += "\n" + ui.Muted.Render(ui.Clipped(hint, width))
	}
	return text
}

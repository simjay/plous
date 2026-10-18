package monitor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/ui"
)

func HelmCellStyle(column, value string) lipgloss.Style {
	if column == "Updated" || column == "Template" {
		return ui.Muted
	}
	if column != "Status" {
		return lipgloss.NewStyle()
	}
	switch {
	case strings.Contains(value, "failed"), strings.Contains(value, "unavailable"):
		return ui.RemovedStyle
	case strings.Contains(value, "pending"), strings.Contains(value, "uninstalling"), strings.Contains(value, "unknown"):
		return ui.ChangedStyle
	case strings.Contains(value, "deployed"):
		return ui.AddedStyle
	default:
		return ui.Muted
	}
}

func HelmTable(columns []string, rows [][]string, cursor int, summary, empty string, width, height int, order ...string) string {
	if height <= 0 {
		return ""
	}
	if len(rows) == 0 {
		return ui.StyledTextView(ui.Muted.Render(empty), width, height, 0)
	}
	indices := make([]int, len(columns))
	for i := range columns {
		indices[i] = i
	}
	indices, widths, compact := CollectionTableColumns(columns, rows, indices, width, false)
	before := ""
	if !compact && height >= 4 {
		headings := append([]string(nil), columns...)
		if len(order) == 2 {
			for i, column := range columns {
				if column == order[0] {
					headings[i] += order[1]
				}
			}
		}
		before = ui.CollectionTableRow(columns, headings, indices, widths, width, true, false, HelmCellStyle) + "\n" + ui.Muted.Render(strings.Repeat("─", width)) + "\n"
	}
	available := max(1, height-strings.Count(before, "\n")-1)
	body := ui.CollectionWindow(len(rows), cursor, available, func(i int) []string {
		row := rows[i]
		active := i == cursor
		if !compact {
			return []string{ui.MouseChoice(ui.CollectionTableRow(columns, row, indices, widths, width, false, active, HelmCellStyle), i, strings.Join(row, "/"))}
		}
		name := row[0]
		for j, column := range columns {
			if column == "Namespace" && j < len(row) {
				name = row[j] + "/" + name
			}
		}
		prefix := "  "
		if active {
			prefix = "› "
		}
		lines := []string{ui.TitleStyle.Render(ui.Clipped(prefix+ui.OneLine(name), width))}
		var values []string
		for j := 1; j < len(columns); j++ {
			if columns[j] == "Namespace" || j >= len(row) || row[j] == "" {
				continue
			}
			values = append(values, ui.Muted.Render(columns[j]+" ")+HelmCellStyle(columns[j], row[j]).Render(ui.OneLine(row[j])))
		}
		// A short panel prioritizes the selected identity over wrapped metadata.
		if height > 4 && len(values) > 0 {
			for _, line := range strings.Split(ansi.Wrap(strings.Join(values, " · "), max(1, width-2), ""), "\n") {
				lines = append(lines, "  "+line)
			}
		}
		for j, line := range lines {
			if active {
				line = lipgloss.NewStyle().Background(ui.Highlight).Width(width).Render(line)
			}
			lines[j] = ui.MouseChoice(line, i, strings.Join(row, "/"))
		}
		return lines
	})
	return ui.StyledView(before+body+"\n"+ui.Muted.Render(ui.Clipped(summary, width)), width, height)
}

func (h *HelmState) CollectionData() (columns []string, rows [][]string, summary string) {
	releases := h.ReleaseRows()

	columns = []string{"Name", "Namespace", "Status", "Chart", "Version", "Revision", "App version", "Updated"}
	for _, r := range releases {
		status, chart := r.Status, r.Chart
		if r.Error != "" {
			chart = "Metadata unavailable"
		}
		rows = append(rows, []string{r.Name, r.Namespace, status, chart, r.Version, strconv.Itoa(r.Revision), r.AppVersion, r.Updated})
	}
	summary = fmt.Sprintf("%d releases", len(rows))
	if h.NamespaceScope() != "" {
		columns = append(columns[:1], columns[2:]...)
		for i, row := range rows {
			rows[i] = append(row[:1], row[2:]...)
		}
	}
	namespace := h.NamespaceScope()
	if namespace == "" {
		namespace = "all namespaces"
	}
	return columns, rows, summary + " · " + namespace
}

type HelmChartGroup struct {
	Key      string
	Releases []helmtool.Release
}

func HelmChartName(r helmtool.Release) string {
	if r.Error != "" || r.Chart == "" {
		return "Metadata unavailable / " + r.Key()
	}
	return r.Chart
}

func (h *HelmState) ChartGroups() []HelmChartGroup {
	rows := h.ReleaseRows()
	if h.Inventory.Groups != nil {
		return h.Inventory.Groups
	}
	groups := map[string][]helmtool.Release{}
	for _, r := range rows {
		name := HelmChartName(r)
		groups[name] = append(groups[name], r)
	}
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	if h.Preference.Sort == "Chart" && h.Preference.Descending {
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
	}
	var out []HelmChartGroup
	for _, name := range names {
		out = append(out, HelmChartGroup{Key: name, Releases: groups[name]})
	}
	h.Inventory.Groups = out
	return out
}

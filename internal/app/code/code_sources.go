package code

import (
	"fmt"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
)

func CodePane(title, body string, width, height int, active bool) string {
	if width < 1 || height < 1 {
		return ""
	}
	style := ui.Muted
	if active {
		style = ui.TitleStyle
		title = "▸ " + title
	}
	lines := []string{style.Render(ui.Clipped(title, width)), ui.Muted.Render(strings.Repeat("─", width))}
	lines = append(lines, strings.Split(body, "\n")...)
	for n := range lines {
		lines[n] = ui.Clipped(lines[n], width)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func CodeColumns(left, right string, lw, rw, height int) string {
	divider := strings.TrimSuffix(strings.Repeat(ui.Muted.Render(" │ ")+"\n", max(1, height)), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(lw).Render(left), divider, lipgloss.NewStyle().Width(rw).Render(right))
}

func (s *CodeState) SourceIndex(id string) int {
	return slices.IndexFunc(s.Entries(), func(e source.Entry) bool { return e.ID == id })
}

func (s *CodeState) Dependencies(id string) []string {
	return s.Inventory.EntryDependencies(id)
}

func (s *CodeState) SourceStatus(e source.Entry) string {
	if e.Component {
		return "Component"
	}
	if !e.Renderable() {
		return "Files only"
	}
	if s.Busy && s.RunningEntry == e.ID && (s.Running == "preview" || s.Running == "render") {
		return "Rendering…"
	}
	return s.Status(e.ID)
}

func (s *CodeState) SourceLink(id, prefix string, width int) string {
	e := s.Inventory.Entry(id)
	if e == nil {
		return ""
	}
	row := prefix + hierarchyTail(e.Path, max(1, width-ansi.StringWidth(prefix)))
	if e.ID == s.Preference.Selected {
		row = ui.ShortcutStyle.Render(row)
	}
	if index := slices.IndexFunc(s.NavigationEntries(), func(e source.Entry) bool { return e.ID == id }); index >= 0 {
		row = ui.MouseTarget(row, fmt.Sprintf("code-source/%d", index))
	}
	return row
}

func (s *CodeState) SourceInspector(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	e := s.Inventory.Entry(s.Preference.Selected)
	if e == nil {
		return "Choose a source node."
	}
	lines := []string{ui.ShortcutStyle.Render(ansi.Hardwrap(ui.OneLine(e.File), width, false)), hierarchyRole(s.Inventory.EntryRoles(e.ID)), CodeEntryKind(e.Kind) + " · " + s.SourceStatus(*e), ""}
	appendGroup := func(title string, values []string) {
		lines = append(lines, ui.TitleStyle.Render(title))
		if len(values) == 0 {
			values = []string{ui.Muted.Render("None in this snapshot")}
		}
		lines = append(lines, values...)
		lines = append(lines, "")
	}
	charts := s.Inventory.EntryHelmCharts(e.ID)
	if len(charts) > 0 {
		var values []string
		for _, chart := range charts {
			name := "Helm " + chart.Name
			if chart.Version != "" {
				name += " " + chart.Version
			}
			values = append(values, ansi.Wrap(ui.OneLine(name), width, ""))
			location := chart.Path
			if location == "" {
				location = "No captured local chart"
			}
			locationLabel := "Expected location: "
			if chart.Status == "captured" {
				locationLabel = "Location: "
			}
			values = append(values, ansi.Wrap(locationLabel+ui.OneLine(location), width, ""), ansi.Wrap("Source: "+ui.OneLine(chart.Status), width, ""))
			if chart.Repository != "" {
				values = append(values, ansi.Wrap("Repository: "+ui.OneLine(chart.Repository), width, ""))
			}
			if chart.DeclaredVersion != "" && chart.DeclaredVersion != chart.Version {
				values = append(values, ansi.Wrap("Declared version: "+ui.OneLine(chart.DeclaredVersion), width, ""))
			}
			if chart.Inherited {
				if owner := s.Inventory.Entry(chart.DeclaredBy); owner != nil {
					values = append(values, ansi.Wrap("Inherited from "+ui.OneLine(owner.Path), width, ""))
				}
			}
			if chart.Error != "" {
				values = append(values, ui.ChangedStyle.Render(ansi.Wrap(ui.OneLine(chart.Error), width, "")))
			}
		}
		appendGroup("HELM CHARTS", values)
	}
	includes := []string{}
	for _, ref := range e.References {
		arrow := "← "
		if ref.Relation == "components" {
			arrow = "⇠ "
		}
		label := arrow + ref.Relation + ": " + ref.Path
		if ref.Error != "" {
			label += " · " + ref.Error
		}
		includes = append(includes, ansi.Wrap(ui.OneLine(source.DisplayText(label)), width, ""))
	}
	appendGroup("INCLUDES", includes)
	changes := []string{}
	if s.Inventory.Kustomize != nil {
		if k := s.Inventory.Kustomize.Entry(e.Path); k != nil && strings.HasPrefix(e.ID, "kustomize:") {
			for _, field := range []string{"namespace", "namePrefix", "nameSuffix", "patches", "patchesStrategicMerge", "patchesJson6902", "transformers", "images", "replicas", "labels", "commonLabels", "commonAnnotations", "replacements", "configMapGenerator", "secretGenerator", "helmCharts"} {
				if value, ok := k.Configuration[field]; ok {
					switch v := value.(type) {
					case []any:
						changes = append(changes, fmt.Sprintf("%s: %d", field, len(v)))
					case map[string]any:
						changes = append(changes, fmt.Sprintf("%s: %d keys", field, len(v)))
					default:
						changes = append(changes, ansi.Wrap(field+": "+ui.OneLine(fmt.Sprint(value)), width, ""))
					}
				}
			}
		}
	}
	appendGroup("THIS NODE ADDS", changes)
	consumers := []string{}
	for _, id := range s.Inventory.EntryConsumers(e.ID) {
		consumers = append(consumers, s.SourceLink(id, "→ ", width))
	}
	appendGroup("USED BY", consumers)
	affected := []string{}
	consuming := s.hierarchyConsumers()
	for _, entry := range s.NavigationEntries() {
		if entry.ID != e.ID && consuming[entry.ID] && len(s.Inventory.EntryConsumers(entry.ID)) == 0 && entry.Renderable() {
			affected = append(affected, s.SourceLink(entry.ID, "→ ", width))
		}
	}
	appendGroup("TARGET CANDIDATES", affected)
	findings := []string{}
	chain := s.hierarchyChain()
	for _, f := range s.AllFindings() {
		if chain[f.Target] {
			label := f.Severity + ": " + f.Message
			if f.Target != e.ID {
				if entry := s.Inventory.Entry(f.Target); entry != nil {
					label = entry.Path + " · " + label
				}
			}
			findings = append(findings, ansi.Wrap(ui.OneLine(source.DisplayText(label)), width, ""))
		}
	}
	appendGroup("FINDINGS IN THIS CHAIN", findings)
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	start := min(s.DetailOffset, max(0, len(lines)-height))
	s.DetailOffset = max(0, start)
	return strings.Join(lines[s.DetailOffset:min(len(lines), s.DetailOffset+height)], "\n")
}

func (s *CodeState) SourcesView(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	if len(s.Entries()) == 0 {
		message := "No recognized configuration detected. Browse the source in Files."
		if s.Filter != "" {
			message = "No entries match this filter. Clear it with Esc or Backspace."
		}
		return CodePane("Configuration map", ansi.Wrap(message, width, ""), width, height, true)
	}
	inspectorWidth := min(40, max(28, width/3))
	mapWidth := width - inspectorWidth - 3
	if width >= 78 {
		return CodeColumns(CodePane("Reference graph", s.SourceGraph(mapWidth, max(1, height-2)), mapWidth, height, s.InspectorArea == 0), CodePane("Selected · → Details", s.SourceInspector(inspectorWidth, max(1, height-2)), inspectorWidth, height, s.InspectorArea == 1), mapWidth, inspectorWidth, height)
	}
	if s.InspectorArea == 1 {
		return CodePane("Selected · ← Graph", s.SourceInspector(width, max(1, height-2)), width, height, true)
	}
	return CodePane("Graph · → Details", s.CompactGraph(width, max(1, height-2)), width, height, true)
}

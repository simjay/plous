package code

import (
	"path"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (s *CodeState) KustomizeGraphScope() map[string]bool {
	if s.NodeGraph == "" || s.Inventory == nil {
		return nil
	}
	roles := s.Inventory.EntryRoles(s.NodeGraph)
	if slices.Contains(roles, "overlay") || !slices.Contains(roles, "base") && !slices.Contains(roles, "component") {
		return hierarchyWalk(s.NodeGraph, s.Inventory.EntryDependencies)
	}
	return hierarchyWalk(s.NodeGraph, s.Inventory.EntryConsumers)
}

func (s *CodeState) OpenKustomizeOverview(id string) bool {
	if !s.SelectKustomizeNode(id) {
		return false
	}
	s.PushLocation()
	s.NodeGraph, s.BrowserDetail, s.Filter, s.DetailOffset = "", true, "", 0
	s.Preference.Selected = id
	s.RestoreCursor()
	s.Notice = ""
	return true
}

func (s *CodeState) OpenKustomizeGraph() {
	item, ok := s.SelectedCatalogItem()
	if !ok || s.View != "code-kustomize" {
		return
	}
	if !s.BrowserDetail {
		s.OpenKustomizeOverview(item.EntryID)
	}
	s.PushLocation()
	s.NodeGraph, s.BrowserDetail, s.Filter, s.DetailOffset = item.EntryID, false, "", 0
	s.RestoreCursor()
}

func (s *CodeState) KustomizeActions() []textview.ActionGroup {
	if s.KustomizeDirectory() {
		return []textview.ActionGroup{{Actions: []textview.Action{{Key: "Enter", Label: "Open group"}, {Key: "/", Label: "Find group"}, {Key: "g", Label: "Refresh"}}}}
	}
	actions := []textview.Action{{Key: "Enter", Label: "Overview"}, {Key: "/", Label: "Find node"}}
	if s.BrowserDetail {
		actions = []textview.Action{{Key: "Esc", Label: "Back"}, {Key: "G", Label: "Graph", Mouse: "code-browser/graph"}}
		item, ok := s.SelectedCatalogItem()
		if ok {
			if len(s.RenderCandidates()) > 0 {
				label := "Render"
				if s.PreviewFailure(item.EntryID) != "" {
					label = "Retry render"
				}
				actions = append(actions, textview.Action{Key: "r", Label: label, Mouse: "code-browser/render"})
			}
			if s.KustomizeBaseOnly() {
				actions = append(actions, textview.Action{Label: "Render Base Only", Mouse: "code-browser/base"})
			}
			if s.Builds[item.EntryID] != nil {
				actions = append(actions, textview.Action{Label: "View output", Mouse: "code-browser/saved-output"})
			}
		}
		actions = append(actions, textview.Action{Key: "d", Label: "Open source"})
	} else if s.NodeGraph != "" {
		actions = append([]textview.Action{{Key: "Esc", Label: "Overview"}}, actions...)
	} else {
		back := textview.Action{Key: "Esc", Label: "Groups", Mouse: "code-browser/groups"}
		if s.Filter != "" {
			back = textview.Action{Key: "Esc", Label: "Clear filter"}
		}
		actions = append([]textview.Action{back}, actions...)
	}
	return []textview.ActionGroup{{Actions: append(actions, textview.Action{Key: "a", Label: "Actions"})}}
}

func (s *CodeState) KustomizeName(item catalog.Item) string {
	if entry := s.Inventory.Entry(item.EntryID); entry != nil {
		return s.kustomizeGraph(item).label(*entry)
	}
	return path.Base(item.Name)
}

func (s *CodeState) KustomizeOverview(item catalog.Item, width int) string {
	entry := s.Inventory.Entry(item.EntryID)
	if entry == nil {
		return "Configuration is no longer in this source. Refresh to inspect available nodes."
	}
	width = max(1, min(width, 110))
	relative := func(value string) string { return source.DisplayText(strings.TrimPrefix(value, item.Group+"/")) }
	identity := ui.TitleStyle.Render(source.DisplayText(s.KustomizeName(item))) + "  " + ui.OverlayTitleStyle.Render(item.Kind)
	file := ui.Muted.Render("File: ") + ui.MouseTarget(ui.ShortcutStyle.Render(relative(entry.File)), "code-browser/files")
	lines := []string{ansi.Wrap(identity, width, ""), ansi.Wrap(file, width, ""), ""}
	if failure := s.FailureView(entry.ID, width); failure != "" {
		lines = append(lines, failure, "")
	} else if !entry.Component {
		title, description, tone := "Not rendered yet", "Render this configuration to inspect its Kubernetes YAML.", ui.Muted
		switch state := s.Status(entry.ID); state {
		case "not rendered":
		case "stale":
			title, description, tone = "Output is out of date", "Inputs have changed. Render again to update the output.", ui.ChangedStyle
		default:
			title, description, tone = "Rendered · "+state, "", ui.AddedStyle
		}
		lines = append(lines, ui.DetailCallout(title, description, "", width, tone), "")
	}
	var includes, changes, consumers []string
	for _, ref := range entry.References {
		if ref.Entry {
			includes = append(includes, relative(ref.Path))
		}
	}
	for _, chart := range s.Inventory.EntryHelmCharts(entry.ID) {
		includes = append(includes, "Helm chart: "+source.DisplayText(strings.TrimSpace(chart.Name+" "+chart.Version))+" · "+source.DisplayText(chart.Status), relative(chart.Path))
	}
	if s.Inventory.Kustomize != nil {
		if k := s.Inventory.Kustomize.Entry(entry.Path); k != nil {
			for _, change := range k.ChangeDescriptions() {
				changes = append(changes, source.DisplayText(change))
			}
		}
	}
	for _, id := range s.Inventory.EntryConsumers(entry.ID) {
		if e := s.Inventory.Entry(id); e != nil {
			consumers = append(consumers, relative(e.Path))
		}
	}
	section := func(title string, items []string, w int) string {
		rows := []string{ui.DetailHeading(title, w)}
		for _, item := range items {
			rows = append(rows, ansi.Wrap(item, w, ""))
		}
		return strings.Join(rows, "\n")
	}
	if width >= 84 && len(includes) > 0 && len(changes) > 0 {
		left := (width - 4) / 2
		a := lipgloss.NewStyle().Width(left).Render(section("Includes", includes, left))
		b := section("Changes here", changes, width-left-4)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, a, "    ", b), "")
	} else {
		if len(includes) > 0 {
			lines = append(lines, section("Includes", includes, width), "")
		}
		if len(changes) > 0 {
			lines = append(lines, section("Changes here", changes, width), "")
		}
	}
	if len(consumers) > 0 {
		lines = append(lines, section("Used by", consumers, width), "")
	}
	if entry.Component {
		lines = append(lines, ui.Muted.Render(ansi.Wrap("Render a consuming configuration to include this component in its output.", width, "")))
	}
	if s.PreviewFailure(entry.ID) == "" {
		for _, finding := range s.Inventory.Findings {
			if finding.Target == entry.ID && finding.Severity == "error" {
				lines = append(lines, ui.DetailCallout("Configuration needs attention", "Open Configuration details in Actions to inspect its references.", "", width, ui.ChangedStyle))
				break
			}
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

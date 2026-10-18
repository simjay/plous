package code

import (
	"fmt"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	catalog "github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func BrowserTool(id string) string {
	if id == "files" {
		return id
	}
	return "code-" + id
}
func IsBrowser(tool string) bool {
	return slices.Contains([]string{"code-helm", "code-kustomize", "code-argo", "code-flux"}, tool)
}
func BrowserLabel(tool string) string {
	for _, view := range CodeViews {
		if BrowserTool(view.Id) == tool {
			return view.Label
		}
	}
	return "Files"
}
func (s *CodeState) CatalogItems() []catalog.Item {
	if s.Inventory == nil {
		return nil
	}
	var items []catalog.Item
	scope := s.KustomizeGraphScope()
	for _, item := range s.Inventory.Catalog {
		if (s.View != "code-kustomize" || s.Group == "" || item.Group == s.Group) && (s.NodeGraph == "" || scope[item.EntryID]) && item.Technology == strings.TrimPrefix(s.View, "code-") && ui.SearchContains(item.Name+" "+item.File+" "+item.Kind+" "+item.Group+" "+item.Location, s.Filter) {
			items = append(items, item)
		}
	}
	if s.View == "code-kustomize" {
		ranks := map[string]int{"Base + Overlay": 1, "Base": 2, "Component": 3}
		slices.SortStableFunc(items, func(a, b catalog.Item) int {
			if a.Group != b.Group {
				return strings.Compare(a.Group, b.Group)
			}
			return ranks[a.Kind] - ranks[b.Kind]
		})
	}
	return items
}
func (s *CodeState) SelectedCatalogItem() (catalog.Item, bool) {
	if s.KustomizeDirectory() {
		return catalog.Item{}, false
	}
	items := s.CatalogItems()
	if s.Cursor < 0 || s.Cursor >= len(items) {
		return catalog.Item{}, false
	}
	return items[s.Cursor], true
}
func (s *CodeState) CatalogView(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	if s.KustomizeDirectory() {
		return s.KustomizeDirectoryView(width, height)
	}
	items := s.CatalogItems()
	if len(items) == 0 {
		detail := "No " + BrowserLabel(s.View) + " configuration found. All source files remain available in Files."
		if s.Filter != "" {
			detail = "No configurations match this filter. Esc clears it."
		}
		return ui.EmptyState("No Configurations", detail, "", height)
	}
	if s.BrowserDetail {
		item, _ := s.SelectedCatalogItem()
		detail := s.CatalogDetail(item, width)
		if s.View == "code-kustomize" {
			detail = s.KustomizeOverview(item, width)
		}
		body := strings.Split(detail, "\n")
		start := min(s.DetailOffset, max(0, len(body)-height))
		return strings.Join(body[start:min(len(body), start+height)], "\n")
	}
	if s.View == "code-kustomize" {
		return s.KustomizeGraphView(width, height)
	}
	var rows []string
	selectedStart, selectedEnd, groupStart := 0, 0, 0
	group := ""
	for n, item := range items {
		if n == 0 || item.Group != group {
			group = item.Group
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			groupStart = len(rows)
			heading := source.DisplayText(group)
			if heading == "." {
				heading = "Source root"
			}
			rows = append(rows, strings.Split(ui.SectionHeadingStyle.Render(ansi.Wrap(heading, width, "")), "\n")...)
		}
		card := ui.MouseTarget(catalogCard(item, n == s.Cursor, width), fmt.Sprintf("code-row/%d", n))
		lines := strings.Split(card, "\n")
		if n == s.Cursor {
			selectedStart, selectedEnd = len(rows), len(rows)+len(lines)
			if selectedEnd-groupStart <= height {
				selectedStart = groupStart
			}
		}
		rows = append(rows, lines...)
	}
	top := min(selectedStart, max(0, selectedEnd-height))
	return strings.Join(rows[top:min(len(rows), top+height)], "\n")
}

func (s *CodeState) CatalogActions() []textview.ActionGroup {
	if s.View == "code-kustomize" {
		return s.KustomizeActions()
	}
	actions := []textview.Action{{Key: "Enter", Label: "Details"}, {Key: "d", Label: "Open in Files"}}
	if len(s.RenderCandidates()) > 0 {
		actions = append(actions, textview.Action{Key: "r", Label: "Render", Mouse: "code-browser/render"})
	}
	actions = append(actions, textview.Action{Key: "/", Label: "Find"}, textview.Action{Key: "a", Label: "Actions"})
	if s.BrowserDetail {
		actions = actions[1:]
		actions = append([]textview.Action{{Key: "Esc", Label: "Back"}}, actions...)
	}
	return []textview.ActionGroup{{Actions: actions}}
}

func (s *CodeState) RenderChain() map[string]bool {
	result := map[string]bool{}
	if s.Inventory == nil {
		return result
	}
	var visit func(string)
	visit = func(id string) {
		if result[id] {
			return
		}
		result[id] = true
		for _, next := range s.Inventory.EntryDependencies(id) {
			visit(next)
		}
	}
	visit(s.RenderRoot)
	return result
}
func (s *CodeState) RenderControls(width int) string {
	if s.ReturnView == "code-kustomize" {
		return ""
	}
	actions := []textview.Action{
		{Label: "Back to " + BrowserLabel(s.ReturnView), Mouse: "code-browser/back"},
		{Label: "Graph", Mouse: "code-browser/graph", Focused: s.Preference.Section == 0},
		{Label: "Output", Mouse: "code-browser/output", Focused: s.Preference.Section == 1},
	}
	return textview.ActionBar([]textview.ActionGroup{{Actions: actions}}, width, 2, ui.WorkspaceTheme())
}
func (s *CodeState) FileControls(width int) string {
	root := textview.ActionChip(textview.Action{Label: "Repository root", Mouse: "code-browser/root"}, 24, ui.WorkspaceTheme())
	if s.Config.Repository == "" {
		root = textview.ActionChip(textview.Action{Label: "Source root", Mouse: "code-browser/root"}, 20, ui.WorkspaceTheme())
	}
	if s.FileRoot != "" {
		root += " / " + ui.Clipped(s.FileRoot, max(1, width-32)) + " " + textview.ActionChip(textview.Action{Label: "Parent", Mouse: "code-browser/parent"}, 12, ui.WorkspaceTheme())
	}
	return ui.Clipped(root, width) + "\n" + ansi.Wrap(technologyLegend(), width, "")
}

// Narrow graphs show one node and its labeled edges; they remain graphs rather
// than introducing a second filesystem-like tree navigation model.
func (s *CodeState) CompactGraph(width, height int) string {
	return s.compactGraph(width, height, s.renderGraph())
}
func (s *CodeState) compactGraph(width, height int, graph sourceGraph) string {
	if graph.forward {
		return s.compactKustomizeGraph(width, height, graph)
	}
	entry := s.Inventory.Entry(graph.selected)
	if entry == nil {
		return "Select a configuration."
	}
	lines := []string{}
	card := func(id string) string {
		e := s.Inventory.Entry(id)
		if e == nil {
			return ""
		}
		name := hierarchyTail(graph.label(*e), max(1, width-6))
		if id == graph.selected {
			name = "› " + name
		}
		text := name + "\n" + ui.Clipped(hierarchyRole(s.Inventory.EntryRoles(id)), max(1, width-4))
		return ui.MouseTarget(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(width).MaxWidth(width).Render(text), graph.target(id))
	}
	lines = append(lines, card(entry.ID))
	for _, id := range graph.dependencies(s, entry.ID) {
		dep := s.Inventory.Entry(id)
		if dep != nil {
			lines = append(lines, ui.Muted.Render(ui.Clipped(graph.label(*entry)+" → "+graph.label(*dep), width)), card(id))
		}
	}
	if len(graph.dependencies(s, entry.ID)) == 0 {
		lines = append(lines, ui.Muted.Render("No included configurations."))
	}
	lines = append(lines, ui.Muted.Render("↑↓ Select nodes · → Details"))
	return textview.Fit(strings.Join(lines, "\n"), width, height)
}

func (s *CodeState) MoveCatalog(key string) {
	if s.moveKustomizeEdge(key) {
		return
	}
	items := s.CatalogItems()
	if len(items) == 0 {
		s.Cursor = 0
		return
	}
	if key == "left" {
		key = "up"
	} else if key == "right" {
		key = "down"
	}
	s.Cursor = ui.MoveCursor(s.Cursor, len(items), key)
}

func (s *CodeState) RenderCandidates() []source.Entry {
	if s.Inventory == nil || !IsBrowser(s.View) {
		return nil
	}
	var targets []source.Entry
	item, ok := s.SelectedCatalogItem()
	if !ok {
		return nil
	}
	if s.View == "code-kustomize" {
		e := s.Inventory.Entry(item.EntryID)
		if e == nil || !e.Renderable() || s.KustomizeBaseOnly() {
			return nil
		}
		return []source.Entry{*e}
	}
	if e := s.Inventory.Entry(item.EntryID); e != nil && e.Renderable() {
		targets = s.Inventory.RenderTargets(e.Path, true)
		if item.External {
			targets = []source.Entry{*e}
		}
	} else if !item.External {
		targets = s.Inventory.RenderTargets(item.File, false)
	}
	// Controller declarations can be inspected, but rendering them is not chart rendering.
	if item.External || item.Technology == "argo" || item.Technology == "flux" {
		filtered := targets[:0]
		for _, e := range targets {
			if e.Kind != "plain" {
				filtered = append(filtered, e)
			}
		}
		targets = filtered
	}
	return targets
}

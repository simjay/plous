package code

import (
	"path"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
)

func (s *CodeState) KustomizeGroups() []string {
	var groups []string
	if s.Inventory != nil {
		for _, item := range s.Inventory.Catalog {
			if item.Technology == "kustomize" && !slices.Contains(groups, item.Group) {
				groups = append(groups, item.Group)
			}
		}
	}
	return groups
}

func (s *CodeState) SelectKustomizeNode(id string) bool {
	if s.View != "code-kustomize" || s.Inventory == nil {
		return false
	}
	member := slices.IndexFunc(s.Inventory.Catalog, func(item catalog.Item) bool {
		return item.Technology == "kustomize" && item.EntryID == id
	})
	if member < 0 {
		return false
	}
	if group := s.Inventory.Catalog[member].Group; group != s.Group {
		s.Group = group
		s.SyncListScope()
	}
	index := slices.IndexFunc(s.CatalogItems(), func(item catalog.Item) bool { return item.EntryID == id })
	if index < 0 {
		s.Filter = ""
		index = slices.IndexFunc(s.CatalogItems(), func(item catalog.Item) bool { return item.EntryID == id })
	}
	if index < 0 {
		return false
	}
	s.Cursor, s.BrowserDetail, s.DetailOffset = index, false, 0
	s.Err = ""
	s.RememberSelection()
	return true
}

func (s *CodeState) ChooseKustomizeGroup(group string) {
	if !slices.Contains(s.KustomizeGroups(), group) {
		return
	}
	s.Group, s.NodeGraph, s.BrowserDetail = group, "", false
	s.Err, s.Notice = "", ""
	s.Preference.KustomizeGroup = group
	s.SyncListScope()
	s.Filter = ""
	s.RestoreCursor()
	if item, ok := s.SelectedCatalogItem(); ok && item.ID == s.Preference.BrowserItem {
		return
	}
	for _, item := range s.CatalogItems() {
		if item.Group == group {
			s.SelectKustomizeNode(item.EntryID)
			return
		}
	}
}

func (s *CodeState) KustomizeBaseOnly() bool {
	item, ok := s.SelectedCatalogItem()
	if !ok || s.View != "code-kustomize" {
		return false
	}
	roles := s.Inventory.EntryRoles(item.EntryID)
	return slices.Contains(roles, "base") && !slices.Contains(roles, "overlay") && !slices.Contains(roles, "component")
}

func (s *CodeState) kustomizeGraph(item catalog.Item) sourceGraph {
	g := sourceGraph{selected: item.EntryID, forward: true, target: func(id string) string { return "code-kustomize/" + id }}
	scope := s.KustomizeGraphScope()
	for _, member := range s.Inventory.Catalog {
		if member.Technology == "kustomize" && member.Group == item.Group && (s.NodeGraph == "" || scope[member.EntryID]) {
			if entry := s.Inventory.Entry(member.EntryID); entry != nil {
				g.entries = append(g.entries, *entry)
			}
		}
	}
	short := func(e source.Entry) string {
		name := path.Base(e.Path)
		if name == "base" {
			name = path.Base(path.Dir(e.Path))
		}
		return name
	}
	counts := map[string]int{}
	for _, e := range g.entries {
		counts[short(e)]++
	}
	g.label = func(e source.Entry) string {
		name := short(e)
		if counts[name] > 1 {
			name = strings.TrimPrefix(e.Path, item.Group+"/")
		}
		return source.DisplayText(name)
	}
	return g
}

func (s *CodeState) KustomizeGraphView(width, height int) string {
	item, ok := s.SelectedCatalogItem()
	if !ok {
		return ""
	}
	g := s.kustomizeGraph(item)
	name := path.Base(item.Group)
	if item.Group == "." {
		name = "Source root"
	}
	heading := ui.SectionHeadingStyle.Render(ui.Clipped(source.DisplayText(name), max(1, width-4)))
	header := []string{ui.Clipped(heading, width), ui.Muted.Render(hierarchyTail(source.DisplayText(item.Group), width)), ""}
	if s.NodeGraph != "" {
		anchor := s.Inventory.Entry(s.NodeGraph)
		header = []string{ui.SectionHeadingStyle.Render(ui.Clipped("Graph · "+path.Base(anchor.Path), width)), ui.Muted.Render(hierarchyTail(source.DisplayText(anchor.Path), width)), ""}
	}
	graphHeight := max(1, height-len(header))
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(strings.Join(header, "\n") + "\n" + s.sourceGraph(width, graphHeight, g))
}

func (s *CodeState) moveKustomizeEdge(key string) bool {
	if s.View != "code-kustomize" || key != "left" && key != "right" {
		return false
	}
	item, ok := s.SelectedCatalogItem()
	if !ok {
		return true
	}
	ids := s.Inventory.EntryDependencies(item.EntryID)
	if key == "left" {
		ids = s.Inventory.EntryConsumers(item.EntryID)
	}
	for _, id := range ids {
		if s.SelectKustomizeNode(id) {
			break
		}
	}
	return true
}

func (s *CodeState) compactKustomizeGraph(width, height int, g sourceGraph) string {
	entry := s.Inventory.Entry(g.selected)
	lines := []string{ui.MouseTarget(ui.ShortcutStyle.Background(ui.Highlight).Width(width).Render(ui.Clipped("› "+g.label(*entry), width)), g.target(entry.ID))}
	add := func(ids []string, incoming bool) {
		for _, id := range ids {
			if other := s.Inventory.Entry(id); other != nil {
				label := "  → " + g.label(*other)
				if incoming {
					label = "  ← " + g.label(*other)
				}
				lines = append(lines, ui.MouseTarget(ui.TitleStyle.Render(ui.Clipped(label, width)), g.target(id)))
			}
		}
	}
	if ids := g.dependencies(s, entry.ID); len(ids) > 0 {
		lines = append(lines, ui.Muted.Render("Includes"))
		add(ids, false)
	}
	var consumers []string
	for _, id := range s.Inventory.EntryConsumers(entry.ID) {
		if slices.ContainsFunc(g.entries, func(e source.Entry) bool { return e.ID == id }) {
			consumers = append(consumers, id)
		}
	}
	if len(consumers) > 0 {
		lines = append(lines, ui.Muted.Render("Used by"))
		add(consumers, true)
	}
	if len(lines) > height {
		lines = append(lines[:max(1, height-1)], ui.Muted.Render("↑↓ Select remaining nodes"))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

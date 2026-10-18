package code

import (
	"path"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
)

type KustomizeGroup struct {
	Path, Name, Summary string
}

func (s *CodeState) KustomizeDirectory() bool {
	return s.View == "code-kustomize" && s.Group == "" && !s.BrowserDetail && s.NodeGraph == ""
}

func (s *CodeState) KustomizeGroupItems() []KustomizeGroup {
	var groups []KustomizeGroup
	for _, group := range s.KustomizeGroups() {
		name := path.Base(group)
		if group == "." {
			name = "Source root"
		}
		if !ui.SearchContains(name+" "+group, s.Filter) {
			continue
		}
		counts := map[string]int{}
		for _, item := range s.Inventory.Catalog {
			if item.Technology == "kustomize" && item.Group == group {
				kind := map[string]string{"Overlay": "overlay", "Base + Overlay": "overlay", "Base": "base", "Component": "component"}[item.Kind]
				if kind == "" {
					kind = "configuration"
				}
				counts[kind]++
			}
		}
		var summary []string
		for _, kind := range []string{"overlay", "base", "component", "configuration"} {
			if counts[kind] > 0 {
				summary = append(summary, CodeCountLabel(counts[kind], kind, kind+"s"))
			}
		}
		groups = append(groups, KustomizeGroup{group, name, strings.Join(summary, " · ")})
	}
	return groups
}

func (s *CodeState) OpenKustomizeGroup(group string) bool {
	if !s.KustomizeDirectory() || !slices.Contains(s.KustomizeGroups(), group) {
		return false
	}
	s.Preference.KustomizeGroup = group
	s.RestoreCursor()
	s.PushLocation()
	s.ChooseKustomizeGroup(group)
	return true
}

func (s *CodeState) ShowKustomizeGroups() {
	for !s.KustomizeDirectory() && len(s.Details) > 0 {
		s.CloseDetail()
	}
	if !s.KustomizeDirectory() {
		s.Group, s.NodeGraph, s.BrowserDetail = "", "", false
		s.SyncListScope()
		s.RestoreCursor()
	}
}

func (s *CodeState) KustomizeDirectoryView(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	groups := s.KustomizeGroupItems()
	if len(groups) == 0 {
		if s.Filter != "" {
			return ui.EmptyState("No matching groups", "Search by group name or source path.", "Backspace Clear filter", height)
		}
		return ui.EmptyState("No Kustomize groups", "No Kustomize configuration found in this source.", "g Refresh source", height)
	}
	header := ""
	if height >= 6 {
		header = ui.SectionHeadingStyle.Render(CodeCountLabel(len(groups), "group", "groups")) + "\n"
	}
	if height >= 12 {
		header += ansi.Wrap("Choose a group to explore its graph.", width, "") + "\n"
	}
	if header != "" {
		header += "\n"
	}
	available := max(1, height-strings.Count(header, "\n"))
	body := ui.CollectionWindow(len(groups), s.Cursor, available, func(index int) []string {
		group := groups[index]
		active := index == s.Cursor
		name := "  " + source.DisplayText(group.Name)
		title, detail, summary := ui.OverlayTitleStyle, ui.Muted, ui.CanvasStyle
		if active {
			name = "› " + source.DisplayText(group.Name)
			title = ui.ShortcutStyle.Background(ui.Highlight)
			detail, summary = detail.Background(ui.Highlight), summary.Background(ui.Highlight)
		}
		lines := []string{title.Width(width).Render(ui.Clipped(name, width))}
		location := "Path: " + source.DisplayText(group.Path)
		if group.Path == "." {
			location = "Top-level configuration"
		}
		for _, line := range strings.Split(ansi.Wrap(location, max(1, width-2), ""), "\n") {
			lines = append(lines, detail.Width(width).Render(ui.Clipped("  "+line, width)))
		}
		if height >= 6 {
			for _, line := range strings.Split(ansi.Wrap(group.Summary, max(1, width-2), ""), "\n") {
				lines = append(lines, summary.Width(width).Render(ui.Clipped("  "+line, width)))
			}
		}
		for n := range lines {
			lines[n] = ui.MouseTarget(lines[n], "code-kustomize-group/"+group.Path)
		}
		return append(lines, "")
	})
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(header + body)
}

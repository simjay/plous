package code

import (
	docedit "github.com/simjay/plous/internal/ui/editor"
	helpview "github.com/simjay/plous/internal/ui/help"
	"slices"
)

func (s *CodeState) Help() helpview.Section {
	if s == nil {
		return helpview.New("Code", helpview.Command("refresh", "g", "Load source"))
	}
	// Menu inspection can populate field and comparison caches. Keep those
	// caches and cursor adjustments detached from the open document and view.
	snapshot := *s
	return snapshot.help()
}

func (s *CodeState) help() helpview.Section {
	if s.Discard {
		return helpview.Confirmation("Code · Discard file edits")
	}
	if s.Input != nil {
		h := helpview.Input("Code · "+s.InputKind, "Confirm value", "Cancel input")
		h.Note(s.InputHelp())
		return h
	}
	if s.Menu != "" {
		h := helpview.Picker("Code · "+s.MenuTitle(), "Open selected item")
		h.Add("Navigation", helpview.Key("page", "PgUp / PgDn", "Jump ten rows"), helpview.Key("ends", "Home / End", "First / last"))
		if slices.Contains([]string{"actions", "more-tools", "comparison-options", "entry", "document", "helm-inputs"}, s.Menu) {
			for _, item := range s.MenuItems() {
				h.Add("", helpview.Key("menu-"+item, "Enter", item))
			}
		}
		return h
	}
	if s.Review != nil {
		target, title := "", "Rendered comparison (read only)"
		back := "Back to rendered output"
		if s.File != nil {
			target, title = "local file", "Optional file review"
			back = "Back to editor; keep draft"
		}
		return s.Review.Help(title, target, back)
	}
	if s.Document != nil {
		target, title := "", "Code document (read only)"
		if !s.Document.ReadOnly {
			target, title = "local file", "Code · Plous Editor"
		}
		h := s.Document.Help(title, target, "back")
		if s.Document.Diagnostic != "" && !s.Document.Searching && (s.Document.Controls || s.Document.Mode == docedit.Normal) {
			h.Add("Error", helpview.Key("error", "!", "Read full save error"))
		}
		if s.Document.ReadOnly && !s.Document.Searching {
			h.Add("File", helpview.Command("actions", ":a", "File Actions"), helpview.Key("close", ":q", "Back"))
			if s.DocumentPath != "" && s.DocumentKind != "file" {
				h.Add("Source", helpview.Command("describe", ":d", "Open captured source"))
			}
		}
		return h
	}
	if s.Busy && s.Running != "preview" && !s.BrowsingDuringDiscovery() {
		return helpview.New("Code · Working", helpview.Key("cancel", "Esc", "Cancel; keep previous results"))
	}
	if s.Inventory == nil {
		return helpview.New("Code · Source unavailable", helpview.Key("retry", "Enter", "Retry loading source"), helpview.Command("actions", "a", "Actions"), helpview.Key("back", "Esc", "Focus sidebar"))
	}
	h := helpview.New("Code · " + s.ViewLabel())
	items := s.MenuItems()
	switch {
	case s.View == "files" || s.View == "code":
		h.Add("Files", helpview.Key("open", "Enter", "Toggle folder / open file"), helpview.Key("fold", "←→", "Fold / expand; parent / child"), helpview.Key("filter", "/", "Find files"))
	case s.View == "code-kustomize":
		if s.KustomizeDirectory() {
			h.Title = "Code · Kustomize groups"
			h.Add("Groups", helpview.Key("open", "Enter", "Open group graph"), helpview.Key("filter", "/", "Find groups by name or path"))
			h.Note("Each group contains connected Kustomize configurations. Click a group or press Enter to explore its graph. Browsing does not render.")
		} else if s.BrowserDetail {
			h.Title = "Code · Node overview"
			h.Add("Node", helpview.Command("graph", "G", "Graph"), helpview.Command("describe", "d", "Open source"))
			if len(s.RenderCandidates()) > 0 {
				h.Add("Node", helpview.Command("render", "r", "Render this node"))
			}
			h.Add("Navigation", helpview.Key("scroll", "↑↓", "Scroll overview"))
			h.Add("Navigation", helpview.Key("find", "/", "Find text in overview"))
		} else {
			h.Title = "Code · Kustomize graph"
			h.Add("Graph", helpview.Key("open", "Enter", "Open node overview"), helpview.Key("nodes", "↑↓", "Select nodes"), helpview.Key("edges", "←→", "Follow consumer / included node"), helpview.Key("filter", "/", "Find nodes"))
			h.Note("Click a node to open its overview. Overlays highlight included configurations; bases and components highlight consumers. Crossed lines (╳) do not join. Dashed borders identify components.")
		}
	case IsBrowser(s.View):
		if !s.BrowserDetail {
			h.Add("Browse", helpview.Key("open", "Enter", "Open configuration details"))
		} else {
			h.Title += " · Details"
		}
		h.Add("Browse", helpview.Command("describe", "d", "Open in Files"), helpview.Key("filter", "/", "Find configurations"))
		if !s.BrowserDetail {
			h.Add("Browse", helpview.Key("cards", "↑↓←→", "Select configuration / scroll details"))
		}
		if len(s.RenderCandidates()) > 0 {
			entry := helpview.Command("render", "r", "Render selected configuration")
			h.Add("Render", entry)
		}
	case s.View == "manifests":
		switch s.Preference.Section {
		case 0:
			h.Title = "Code · Configuration graph"
			h.Add("Graph", helpview.Key("open", "Enter", "Render selected node / open output"), helpview.Key("area", "←→", "Graph / node details"), helpview.Command("describe", "d", "Open source"))
		case 1:
			h.Title = "Code · Rendered resources"
			if s.ComparisonActive() {
				h.Title = "Code · Output comparison"
			}
			h.Add("Inspect", helpview.Key("open", "Enter", "Inspect resource / field history"), helpview.Key("area", "←→", "Resources / YAML / field history"), helpview.Command("yaml", "y", "Open full YAML"))
			if s.ResourceOrigin() != "" {
				h.Add("Source", helpview.Command("describe", "d", "Open verified source"))
			}
		case 2:
			h.Title = "Code · Field history"
			h.Add("Field", helpview.Key("open", "Enter", "Inspect field history"), helpview.Command("describe", "d", "Open captured field source"))
		case 3:
			h.Title = "Code · Findings"
			h.Add("Finding", helpview.Key("open", "Enter", "Inspect finding"), helpview.Command("describe", "d", "Open finding source"))
		}
		h.Add("Filter", helpview.Key("filter", "/", "Filter current view"))
		if s.Inventory != nil {
			if e := s.Inventory.Entry(s.Preference.Selected); e != nil && e.Renderable() {
				h.Add("Render", helpview.Command("render", "r", "Render saved inputs"))
				if len(s.ComparisonEntries()) > 0 {
					h.Add("Compare", helpview.Command("review", "v", "Compare entry points"))
				}
			}
		}
	default:
		h.Add("Browse", helpview.Key("open", "Enter", "Open configuration files"))
	}
	if IsBrowser(s.View) && s.BrowserDetail {
		h.Add("Navigation", helpview.Scroll()...)
	} else {
		h.Add("Navigation", helpview.List()...)
	}
	h.Add("View", helpview.Key("clear-filter", "Backspace", "Clear applied filter"), helpview.Key("back", "Esc", "Clear filter / back"), helpview.Command("actions", "a", "Actions"), helpview.Command("refresh", "g", "Refresh source"))
	if s.Config.Repository != "" {
		h.Add("Source", helpview.Command("revision", "F", "Choose Git revision"))
	}
	for _, label := range items {
		if !slices.Contains([]string{"Refresh Source", "Render", "Open in Files"}, label) {
			h.Add("Actions menu", helpview.Key("menu-"+label, "a →", label))
		}
	}
	for _, group := range []struct{ label, menu string }{{"More Tools", "more-tools"}, {"Comparison Options", "comparison-options"}} {
		if slices.Contains(items, group.label) {
			s.Menu = group.menu
			for _, label := range s.MenuItems() {
				h.Add("Actions → "+group.label, helpview.Key("menu-"+label, "Enter", label))
			}
		}
	}
	return h
}

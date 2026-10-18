package monitor

import (
	helpview "github.com/simjay/plous/internal/ui/help"
)

func (h *HelmState) Help() helpview.Section {
	if h == nil {
		return helpview.New("Helm", helpview.Command("refresh", "g", "Refresh connection"))
	}
	if h.Discard {
		return helpview.Confirmation("Helm · Discard values draft")
	}
	if h.Comparison != nil {
		if h.Document != nil && !h.Document.ReadOnly {
			return h.Comparison.Help("Helm · Values review", "draft values", "Back to values editor")
		}
		return h.Comparison.Help("Helm comparison (read only)", "", "Back to release / preview")
	}
	if h.Document != nil {
		target := ""
		title := "Helm document (read only)"
		if !h.Document.ReadOnly {
			target = "draft values"
			title = "Helm · Values editor"
		}
		s := h.Document.Help(title, target, "back")
		if h.Document.Controls && !h.Document.ReadOnly {
			for i := range s.Groups {
				if s.Groups[i].Title == "Document actions" {
					s.Groups[i].Entries = []helpview.Entry{helpview.Key("read", "Enter", "Return to document"), helpview.Key("close", "Esc", "Request Close")}
				}
			}
		}
		return s
	}
	if h.Namespaces != nil {
		return h.Namespaces.Help("Helm · Namespace")
	}
	if h.Input != nil && h.InputKind != "picker" {
		return helpview.Input("Helm · "+h.InputKind, "Apply value / filter", "Cancel input")
	}
	if h.Menu != nil {
		s := helpview.Picker("Helm · "+h.MenuKind, "Open selected item")
		s.Add("Navigation", helpview.Key("page", "PgUp / PgDn", "Jump ten rows"), helpview.Key("ends", "Home / End", "First / last"))
		if h.MenuKind == "actions" {
			for _, item := range h.Menu {
				s.Add("", helpview.Key("menu-"+item, "Enter", item))
			}
		}
		if h.Input != nil {
			s.Note("Type to filter choices.")
		}
		return s
	}
	if h.Confirm {
		return helpview.Confirmation("Helm · Confirm rollback")
	}
	if h.Busy {
		return helpview.New("Helm · Working", helpview.Key("cancel", "Esc", "Cancel request; check release before retrying"))
	}
	if h.Client == nil {
		s := helpview.New("Helm · Connection", helpview.Command("refresh", "g", "Retry connection"), helpview.Command("namespace", "n", "Namespace"))
		if h.Err != "" {
			s.Add("Error", helpview.Key("error", "!", "Read full error"))
		}
		return s
	}
	if h.Find != nil && h.Find.Reader.Searching {
		return helpview.Search("Helm · Find", "Finish find")
	}
	if h.WorkflowVisible() && h.Draft != nil && h.Plan == nil {
		s := h.Draft.Form.Help()
		s.Title = "Helm · Chart inputs"
		for gi := range s.Groups {
			for ei := range s.Groups[gi].Entries {
				e := &s.Groups[gi].Entries[ei]
				if e.ID == "back" {
					e.Label = "Back to release; keep inputs"
					if h.Draft.Form.Index < len(h.Draft.Form.Fields) {
						e.Label = "Focus actions; keep inputs"
					}
				}
			}
		}
		s.Note("Preview chart changes or edit draft values. No changes are applied by previewing.")
		return s
	}
	title := "Helm · " + h.Preference.Section
	s := helpview.New(title)
	switch {
	case h.WorkflowVisible() && h.Plan != nil:
		s.Title = "Helm · Preview"
		s.Add("Preview", helpview.Command("review", "v", "Review resource differences"), helpview.Key("open", "Enter", "Review resource differences"), helpview.Key("back", "Esc", "Return to release; keep preview"))
		if h.Plan.Operation == "rollback" {
			s.Add("Rollback", helpview.Command("rollback", "B", "Confirm prepared rollback"))
		}
		s.Add("Read", helpview.Find()...)
	case h.Detail != nil:
		s.Title = "Helm · " + HelmDetailSections[h.Tab]
		s.Add("Inspect", helpview.Command("release", "J", "Choose release"), helpview.Command("revision", "F", "Choose revision"), helpview.Key("section", "←→", "Change section", "[ / ]"), helpview.Key("open", "Enter", "Open selected configuration / document"), helpview.Key("back", "Esc", "Back to revision timeline"))
		if h.Detail.Version < h.LatestRevision() {
			s.Add("Revision", helpview.Command("rollback", "B", "Preview rollback"))
		}
		s.Add("Read", helpview.Find()...)
	case h.Timeline != nil:
		s.Title = "Helm · Release timeline"
		s.Add("Revision", helpview.Key("open", "Enter", "Inspect exact revision"), helpview.Command("review", "v", "Review recorded changes"), helpview.Key("back", "Esc", "Back to releases"))
		if r := h.Timeline.Selected(); r > 0 && r < h.LatestRevision() {
			s.Add("Revision", helpview.Command("rollback", "B", "Preview rollback"))
		}
		s.Add("Read", helpview.Find()...)
	default:
		s.Add("Browse", helpview.Key("open", "Enter", "Open selected release"), helpview.Key("filter", "/", "Filter releases"), helpview.Key("clear-filter", "Backspace", "Clear text filter"), helpview.Key("sort", "S", "Sort"), helpview.Command("namespace", "n", "Namespace"), helpview.Key("back", "Esc", "Focus sidebar"))
		if h.Preference.Section == "Charts" {
			s.Add("Cards", helpview.Key("cards", "←→", "Select chart card"))
		}
	}
	switch {
	case h.WorkflowVisible() && h.Plan != nil || h.Detail != nil:
		label := "Scroll"
		if h.Detail != nil && h.Tab == 5 {
			label = "Select configuration"
		}
		s.Add("Navigation", helpview.Key("scroll", "↑↓", label), helpview.Key("page", "PgUp / PgDn", "Move ten lines"), helpview.Key("ends", "Home / End", "First / last"))
	case h.Timeline != nil:
		s.Add("Navigation", helpview.Key("select", "↑↓", "Select revision", "j / k"), helpview.Key("page", "PgUp / PgDn", "Scroll revision summary"), helpview.Key("ends", "Home / End", "First / last revision"))
	default:
		s.Add("Navigation", helpview.List()...)
	}
	s.Add("Actions", helpview.Command("refresh", "g", "Refresh selected view"), helpview.Command("actions", "a", "Actions"))
	for _, label := range h.ActionItems() {
		s.Add("", helpview.Key("menu-"+label, "a →", label))
	}
	if h.Err != "" {
		s.Add("Error", helpview.Key("error", "!", "Read full error"))
	}
	if h.Find != nil {
		s.Note("Esc clears find before returning.")
	}
	return s
}

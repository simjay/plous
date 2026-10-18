package monitor

import (
	docedit "github.com/simjay/plous/internal/ui/editor"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (e *ExploreState) Help() helpview.Section {
	if e == nil {
		return helpview.New("Kubernetes", helpview.Command("connections", ":K", "Choose connection"))
	}
	if e.Picker == "namespace" {
		return e.Namespaces.Help("Kubernetes · Namespace")
	}
	if e.Input != nil && e.Picker == "" {
		return helpview.Input("Kubernetes · "+e.InputKind, "Apply value / filter", "Keep applied filter")
	}
	if e.Picker != "" {
		title := map[string]string{"connections": "Connections", "containers": "Container", "sort": "Sort", "logrange": "Log time range", "namespace": "Namespace"}[e.Picker]
		if title == "" {
			title = e.Picker
		}
		open := "Apply selection"
		if e.Picker == "sort" {
			open = "Sort by column / reverse current sort"
		}
		s := helpview.Picker("Kubernetes · "+title, open)
		s.Add("Navigation", helpview.Key("page", "PgUp / PgDn", "Jump ten rows"))
		if e.Input != nil {
			s.Note("Type to filter or enter a value.")
		}
		return s
	}
	if e.Action != nil {
		return e.Action.Help()
	}
	if t := e.Terminal; t != nil {
		if t.Captured {
			return helpview.New("Exec · Captured input", helpview.Key("release", "Ctrl+]", "Review output"), helpview.Key("quit", "Ctrl+C", "Save and exit Plous"))
		}
		var reader *textview.Viewer
		if t.Review != nil {
			reader = &t.Review.Viewer.Reader
		}
		s := reader.Help("Exec · Output review", "Close session")
		if reader != nil && (reader.Searching || reader.Selecting) {
			return s
		}
		if t.Done {
			if t.Launch != nil {
				s.Add("Session", helpview.Key("edit", "e", "Edit command / choose shell"), helpview.Key("retry", "R", "Retry same command"))
			}
		} else {
			s.Add("Session", helpview.Key("resume", "r", "Resume terminal input"), helpview.Key("interrupt", "t", "Interrupt remote process"))
		}
		s.Add("Session", helpview.Key("close", "x", "Close session"))
		return s
	}
	if e.Usage != nil && e.Usage.Charts.Inspect {
		s := helpview.New("Usage charts", helpview.Command("charts", "I", "Back"), helpview.Key("back", "Esc", "Leave charts"))
		if e.Detail != nil {
			s.Add("Samples", helpview.Key("sample", "← / →", "Inspect actual samples"), helpview.Command("chartrange", "W", "Switch 5 / 15 minutes"), helpview.Command("chartlatest", "P", "Follow latest"))
		} else {
			s.Note("Rankings are read-only. Return to the list to select a resource.")
		}
		s.Add("Read", helpview.Scroll()...)
		return s
	}
	if e.Detail != nil && e.Tab == 4 {
		s := e.LogViewer.Help()
		if e.LogViewer == nil || !e.LogViewer.Reader.Searching {
			s.Add("Log controls", helpview.Key("reload", "g", "Reload logs; clears buffer"), helpview.Key("container", "o", "Choose container"), helpview.Key("previous", "p", "Previous instance"), helpview.Key("range", "u", "Time range"), helpview.Key("timestamps", "t", "Timestamps"), helpview.Key("wrap", "w", "Wrap lines"))
		}
		return s
	}
	if e.Detail != nil && e.Tab == 1 {
		var d *docedit.Model
		if e.YamlDocument != nil {
			d = e.YamlDocument.Document
		}
		if d != nil && d.Searching {
			return d.Help("YAML (read only)", "", "back")
		}
		s := helpview.New("YAML (read only)", helpview.Key("back", "Esc", "Clear search / selection, then back"))
		s.Add("Read", helpview.Find()...)
		s.Add("Text", helpview.Key("select", "z", "Select text"), helpview.Key("copy", "y", "Copy selection / line"))
		s.Add("Navigation", helpview.Key("move", "↑↓←→", "Move cursor", "j / k"), helpview.Key("page", "PgUp / PgDn", "Move a page"), helpview.Key("document", "Home / End", "Document start / end"))
		s.Add("View", helpview.Command("linenumbers", ":L", "Line numbers"), helpview.Command("fullscreen", ":f", "Fullscreen / restore layout"))
		for _, action := range ActionsFor(e.Resource, e.Detail) {
			if action.Id == "edit" {
				s.Add("Resource", helpview.Command(action.Command, action.Alias, action.Label))
			}
		}
		if e.Resource.Key() == "secrets" {
			s.Add("Secret", helpview.Key("reveal", "v", "Reveal / hide Secret values"))
		}
		return s
	}
	if e.Detail != nil {
		section := "Describe"
		if e.Tab == 2 {
			section = "Events"
		} else if e.Tab == 3 {
			section = "Related"
		}
		s := helpview.New("Kubernetes · "+ResourceTitle(e.Resource)+" · "+section, helpview.Command("describe", "d", "Describe"), helpview.Command("yaml", "y", "YAML"), helpview.Key("events", "e", "Events"), helpview.Key("related", "r", "Related"), helpview.Key("section", "[ / ]", "Previous / next section"), helpview.Key("back", "Esc", "Back"))
		if e.Tab == 3 {
			s.Add("Related", helpview.Key("open", "Enter", "Open selected resource / list"))
		}
		if e.Tab == 0 && e.Resource.Key() == "pods" {
			s.Add("Container", helpview.Key("container", "o", "Choose container"), helpview.Key("previous", "p", "Previous instance logs"))
		}
		var actions []helpview.Entry
		for _, a := range ActionsFor(e.Resource, e.Detail) {
			actions = append(actions, helpview.Command(a.Command, a.Alias, a.Label))
		}
		if e.Usage != nil && e.Tab == 0 {
			s.Add("Charts", helpview.Command("charts", "I", "Inspect charts"))
		}
		s.Add("Resource actions", append(actions, helpview.Command("actions", "a", "More actions"))...)
		s.Add("Read", helpview.Find()...)
		if e.Tab == 3 {
			s.Add("Navigation", helpview.List()...)
		} else {
			s.Add("Navigation", helpview.Scroll()...)
		}
		s.Add("View", helpview.Command("refresh", ":g", "Refresh object and section"), helpview.Command("namespace", ":N", "Namespace"))
		return s
	}
	s := helpview.New("Resource table · "+ResourceTitle(e.Resource), helpview.Command("describe", "Enter", "Describe"), helpview.Command("yaml", "y", "Open YAML"))
	s.Groups[0].Entries[0].Alternatives = []string{"d"}
	if e.Usage != nil {
		s.Add("Charts", helpview.Command("charts", "I", "Top consumers"))
	}
	s.Add("Filter and sort", helpview.Key("filter", "/", "Text filter"), helpview.Key("clear-filter", "Backspace", "Clear text filter"), helpview.Key("selector", "L", "Label selector"), helpview.Key("sort", "S", "Sort"), helpview.Command("namespace", "n", "Namespaces"), helpview.Command("refresh", "g", "Refresh"))
	s.Add("Navigation", helpview.List()...)
	s.Add("Browse", helpview.Key("back", "Esc", "Focus sidebar"), helpview.Command("connections", ":K", "Connection picker"), helpview.Key("resource-command", ":pods", "Go to resource type", ":po / :deploy / :svc"))
	return s
}

func (a *ResourceActionState) Help() helpview.Section {
	if a.Busy {
		return helpview.New("Resource action · Working", helpview.Key("cancel", "Esc", "Cancel local request; outcome may be unknown"))
	}
	if a.Discard {
		return helpview.Confirmation("Discard draft")
	}
	if a.Diagnostic != nil {
		return a.Diagnostic.Help("Error details", "Back to editor; keep draft")
	}
	if a.Stage == "review" && a.Review != nil {
		return a.Review.Help("Optional review", "Kubernetes", "Back to editor; keeps draft")
	}
	if a.Stage == "edit" && a.Document != nil {
		s := a.Document.Help("Plous Editor", "Kubernetes", "back")
		if a.Err != "" && !a.Document.Searching && (a.Document.Controls || a.Document.Mode == docedit.Normal) {
			s.Add("Error", helpview.Key("error", "!", "Full save / validation error"))
		}
		return s
	}
	if a.ValueDocument != nil {
		s := a.ValueDocument.Help("Value (read only)", "", "hide")
		for i := range s.Groups {
			for j := range s.Groups[i].Entries {
				e := &s.Groups[i].Entries[j]
				if e.ID == "document" {
					e.Alternatives = []string{"Home / End"}
				} else if e.ID == "line" {
					e.Key, e.Alternatives = "0 / $", nil
				}
			}
		}
		if !a.ValueDocument.Searching && a.ValueDocument.Mode != docedit.Select {
			if a.ValueDocument.Controls {
				s = helpview.New("Value (read only) · Actions", helpview.Key("back", "Esc", "Hide value"), helpview.Key("read", "a", "Return to value"), helpview.Key("choose", "←→", "Choose action"), helpview.Key("activate", "Enter", "Activate focused action"))
			} else {
				s.Add("Value actions", helpview.Key("actions", "a", "Focus actions"))
			}
			for _, item := range ValueControls(a.Resource) {
				s.Add("", helpview.Key("value-"+item, "a →", item))
			}
		}
		return s
	}
	if a.Stage == "done" && a.Progress != nil {
		s := helpview.New("Operation progress", helpview.Key("retry", "g", "Check status again"), helpview.Key("back", "Enter / Esc", "Return to resource"))
		s.Add("Read", helpview.Scroll()...)
		s.Note("Closing stops checks, not the accepted operation.")
		return s
	}
	if a.Stage == "confirm" {
		return helpview.Confirmation("Resource action · Confirm")
	}
	if a.Stage == "scale" || a.Stage == "export" {
		finish := "Preview replica change"
		if a.Stage == "export" {
			finish = "Export to entered path"
		}
		return helpview.Input("Resource action · "+a.Stage, finish, "Back")
	}
	if a.Stage == "execcommand" {
		if a.ExecEditing {
			return helpview.Input("Exec · Command", "Finish command", "Leave field; keep command")
		}
		return helpview.Picker("Exec · Shell / command", "Choose shell / edit command / Start")
	}
	s := helpview.Picker("Resource action · "+a.Stage, "Next / activate")
	if a.Stage == "menu" {
		for _, item := range a.Items {
			entry := helpview.Command(item.Command, item.Alias, item.Label)
			entry.Reason = a.Reasons[item.Id]
			s.Add("", entry)
		}
	}
	return s
}

func (p *NamespacePicker) Help(title string) helpview.Section {
	if p != nil && p.Manual {
		s := helpview.Input(title+" · Enter name", "Apply namespace", "Return to namespace list")
		s.Note("Use * for all namespaces.")
		return s
	}
	s := helpview.Picker(title, "Apply namespace / open selected action")
	s.Add("Navigation", helpview.Key("page", "PgUp / PgDn", "Jump ten rows"), helpview.Key("ends", "Home / End", "First / last"))
	s.Note("Type to filter. Choose Enter namespace manually to use a name outside the list.")
	return s
}

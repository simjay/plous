package settings

import helpview "github.com/simjay/plous/internal/ui/help"

func (f *Form) Help() helpview.Section {
	if f == nil {
		return helpview.New("Settings", helpview.Key("select", "↑↓", "Select"), helpview.Key("open", "Enter", "Open setting"), helpview.Key("back", "Esc", "Return to previous view"))
	}
	title := f.Title
	if title == "" {
		title = "Form"
	}
	if f.Kind == "kube" {
		if f.Index == 0 {
			h := helpview.Input("Add cluster contexts · File path", "Focus Refresh contexts", "Return without adding")
			h.Add("Fields", helpview.Key("select", "↑↓", "Select field / action"))
			h.Note("Editing the path clears checks. Refresh contexts before adding.")
			return h
		}
		h := helpview.New("Add cluster contexts", helpview.Key("select", "↑↓", "Select field / context / action"), helpview.Key("activate", "Enter", "Check context / activate action"), helpview.Key("check", "Space", "Check selected context"), helpview.Key("name", "→", "Name selected connection"), helpview.Key("back", "Esc", "Return without adding"))
		h.Note("Editing the path clears checks. Refresh contexts before adding.")
		return h
	}
	if f.Directory != nil {
		if f.Directory.Editing {
			return helpview.Input("Choose Local Directory · Path", "Open path", "Cancel typing")
		}
		if f.Directory.Loading {
			return helpview.New("Choose Local Directory · Loading", helpview.Key("back", "Esc", "Return to form"))
		}
		return helpview.New("Choose Local Directory", helpview.Key("select", "↑↓", "Select folder"), helpview.Key("open", "Enter", "Open folder / activate action"), helpview.Key("parent", "← / Backspace", "Parent directory"), helpview.Key("path", "/", "Enter directory path"), helpview.Key("back", "Esc", "Cancel typing, then return to form"))
	}
	h := helpview.New(title, helpview.Key("select", "↑↓", "Select field / action"), helpview.Key("activate", "Enter", "Next field / activate action"), helpview.Key("back", "Esc", "Cancel to preceding screen"))
	for _, field := range f.Fields {
		if len(field.Options) > 0 {
			h.Add("Options", helpview.Key("option", "←→", "Choose option", "Space"))
			break
		}
	}
	for _, action := range f.Actions() {
		h.Add("Form actions", helpview.Key("form-"+action, "Enter", action))
	}
	if f.Kind == "mainagent" {
		h.Note("Choose an agent to preview its setup. Save applies the agent choice; provider forms return here.")
	} else {
		h.Note("Text and paste stay literal in fields. Commands work from action buttons.")
	}
	return h
}

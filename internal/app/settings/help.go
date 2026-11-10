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
		h.Note("Choose an agent to preview its setup. External agents have separate entry commands. Save applies the choice and commands; provider forms return here.")
	} else {
		h.Note("Text and paste stay literal in fields. Commands work from action buttons.")
	}
	return h
}

package diff

import helpview "github.com/simjay/plous/internal/ui/help"

func (m *Model) Help(title, target, back string) helpview.Section {
	if m != nil && m.ErrorActive {
		return m.ErrorReader.Help("Comparison error", "Back to comparison")
	}
	if m != nil && m.Reader.Searching {
		return helpview.Search(title+" · Find", "Finish search")
	}
	s := helpview.New(title, helpview.Key("back", "Esc", back))
	if m != nil && m.Reader.Selecting {
		s = m.Reader.Help(title, back)
	}
	if target != "" {
		s.Add("Save", helpview.Key("save", ":w", "Save to "+target), helpview.Key("save-close", ":wq", "Save and close after success"))
		s.Note("Enter while reading never saves.")
	}
	if m != nil && m.Reader.Selecting {
		return s
	}
	if m != nil && m.Error != "" {
		s.Add("Error", helpview.Key("error", "!", "Read full error"))
	}
	if m != nil && m.Actions {
		s.Add("Actions", helpview.Key("read", "a", "Return to comparison"), helpview.Key("activate", "Enter", "Activate focused action"))
		if m.ConfirmLabel != "" {
			s.Add("Actions", helpview.Key("choose", "←→", "Choose action"))
		}
		return s
	}
	s.Add("Changes", helpview.Key("change", "[ / ]", "Previous / next change"))
	s.Add("Read", helpview.Find()...)
	s.Add("Text", helpview.Key("select", "v", "Select text"), helpview.Key("copy", "y", "Copy selection / line"), helpview.Key("horizontal", "←→", "Move text cursor"), helpview.Key("actions", "a", "Focus actions"))
	s.Add("Navigation", helpview.Scroll()...)
	return s
}

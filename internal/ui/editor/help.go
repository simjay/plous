package editor

import helpview "github.com/simjay/plous/internal/ui/help"

func (m *Model) Help(title, target, back string) helpview.Section {
	if m != nil && m.Searching {
		return helpview.Search(title+" · Find", "Finish search")
	}
	readonly := m == nil || m.ReadOnly
	s := helpview.New(title)
	if readonly {
		s.Add("Read", helpview.Find()...)
		s.Add("Text", helpview.Key("select", "v / V", "Select characters / lines"), helpview.Key("copy", "y", "Copy selection / line"))
		s.Add("Navigation", helpview.Key("move", "↑↓←→", "Move cursor", "j / k"), helpview.Key("page", "PgUp / PgDn", "Move a page"), helpview.Key("document", "gg / G", "Document start / end"), helpview.Key("line", "Home / End", "Line start / end", "0 / $"))
		s.Add("View", helpview.Command("linenumbers", ":L", "Line numbers"), helpview.Key("back", "Esc", "Clear search / selection, then "+back))
		return s
	}
	if m.Controls {
		s.Add("Document actions", helpview.Key("choose", "←→", "Choose action"), helpview.Key("activate", "Enter", "Activate action"), helpview.Key("back", "Esc", "Request Close"))
	}
	if m.Mode == Insert && !m.Controls {
		s.Add("Insert", helpview.Key("normal", "Esc", "Return to Normal"), helpview.Key("indent", "Tab", "Indent two spaces"), helpview.Key("newline", "Enter", "New line with indentation"), helpview.Key("delete", "Backspace / Delete", "Delete character"))
		s.Note("Text, paste, and colon stay literal. Press Esc before a command.")
		return s
	}
	s.Add("Save and close", helpview.Key("save", ":w", "Save to "+target), helpview.Key("save-close", ":wq", "Save and close after success"), helpview.Command("review", ":v", "Optional review without saving"), helpview.Key("close", ":q", "Close; ask before discarding edits"), helpview.Key("discard", ":q!", "Discard draft and close"))
	if m.Controls {
		return s
	}
	escape := "Focus actions"
	if m.Mode == Select {
		escape = "Leave selection"
	} else if m.Query != "" {
		escape = "Clear search"
	}
	s.Add("Save and close", helpview.Key("back", "Esc", escape))
	s.Add("Edit", helpview.Key("insert", "i", "Insert at cursor"), helpview.Key("append", "a", "Append after cursor"), helpview.Key("new-line", "o", "New line below"))
	if m.Mode == Select {
		s.Add("Edit", helpview.Key("select", "v / V", "Leave selection"), helpview.Key("delete", "x", "Delete selection"), helpview.Key("cut", "d", "Cut selection"), helpview.Key("copy", "y", "Copy selection"))
	} else {
		s.Add("Edit", helpview.Key("select", "v / V", "Select characters / lines"), helpview.Key("delete", "x", "Delete character"), helpview.Key("cut", "dd", "Cut whole line"), helpview.Key("copy", "yy", "Copy line"))
	}
	s.Add("Edit", helpview.Key("paste", "p", "Paste copied / cut text"), helpview.Key("undo", "u", "Undo", ":u / :undo"), helpview.Key("redo", "U", "Redo", ":U / :redo"), helpview.Key("indent", "> / <", "Indent / outdent"))
	s.Add("Movement", helpview.Key("move", "↑↓←→", "Move cursor", "h j k l"), helpview.Key("word", "w / b / e", "Next / previous word / word end"), helpview.Key("line", "Home / End", "Line start / end", "0 / $"), helpview.Key("document", "gg / G", "Document start / end"), helpview.Key("page", "PgUp / PgDn", "Move a page"))
	s.Add("Find", helpview.Find()...)
	s.Add("View", helpview.Command("linenumbers", ":L", "Line numbers"), helpview.Command("fullscreen", ":f", "Fullscreen / restore layout"), helpview.Key("panels", "Alt+← / Alt+→", "Switch panels in every mode"))
	s.Note("Document keys own app prefix letters. Paste is one undoable edit.")
	return s
}

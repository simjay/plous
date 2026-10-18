// Package help holds presentation data shared by feature controllers and readers.
package help

import "strings"

type Entry struct {
	ID, Key, Label, Command string
	Alternatives            []string
	Reason                  string
}

type Group struct {
	Title   string
	Entries []Entry
	Note    string
}

type Section struct {
	Title  string
	Groups []Group
}

func Key(id, key, label string, alternatives ...string) Entry {
	return Entry{ID: id, Key: key, Label: label, Alternatives: alternatives}
}

func Command(name, key, label string) Entry {
	return Entry{ID: name, Key: key, Label: label, Command: name}
}

func New(title string, entries ...Entry) Section {
	return Section{Title: title, Groups: []Group{{Entries: entries}}}
}

func (s *Section) Add(title string, entries ...Entry) {
	if len(entries) == 0 {
		return
	}
	if n := len(s.Groups); n > 0 && s.Groups[n-1].Title == title && s.Groups[n-1].Note == "" {
		s.Groups[n-1].Entries = append(s.Groups[n-1].Entries, entries...)
		return
	}
	s.Groups = append(s.Groups, Group{Title: title, Entries: entries})
}

func (s *Section) Note(text string) {
	if text != "" {
		s.Groups = append(s.Groups, Group{Note: text})
	}
}

// Text is the unstyled reference, including alternatives that follow the main controls.
func (s Section) Text() string {
	lines := []string{s.Title}
	for _, g := range s.Groups {
		if g.Title != "" {
			lines = append(lines, "", g.Title)
		}
		for _, e := range g.Entries {
			line := strings.TrimSpace(e.Key + " " + e.Label)
			if e.Reason != "" {
				line += " (" + e.Reason + ")"
			}
			lines = append(lines, line)
		}
		if g.Note != "" {
			lines = append(lines, g.Note)
		}
	}
	var alternatives []string
	for _, g := range s.Groups {
		for _, e := range g.Entries {
			if len(e.Alternatives) > 0 {
				alternatives = append(alternatives, strings.Join(e.Alternatives, " / ")+"  "+e.Label)
			}
		}
	}
	if len(alternatives) > 0 {
		lines = append(lines, "", "Alternative keys")
		lines = append(lines, alternatives...)
	}
	return strings.Join(lines, "\n")
}

func List() []Entry {
	return []Entry{Key("select", "↑↓", "Select", "j / k"), Key("page", "PgUp / PgDn", "Move a page"), Key("ends", "Home / End", "First / last")}
}

func Scroll() []Entry {
	return []Entry{Key("scroll", "↑↓", "Scroll", "j / k"), Key("page", "PgUp / PgDn", "Scroll a page"), Key("ends", "Home / End", "First / last")}
}

func Find() []Entry {
	return []Entry{Key("find", "/", "Find text"), Key("matches", "n / N", "Next / previous match")}
}

func Picker(title, open string) Section {
	s := New(title, Key("open", "Enter", open), Key("back", "Esc", "Cancel / return"))
	s.Add("Navigation", Key("select", "↑↓", "Select"))
	return s
}

func Input(title, finish, back string) Section {
	s := New(title, Key("finish", "Enter", finish), Key("back", "Esc", back))
	s.Note("Text and paste stay literal in this field.")
	return s
}

func Search(title, finish string) Section {
	s := New(title, Key("finish", "Enter", finish), Key("back", "Esc", "Cancel search"))
	s.Note("Type or paste to search. Navigation shortcuts remain literal.")
	return s
}

func Confirmation(title string) Section {
	return New(title, Key("choose", "←→", "Choose"), Key("confirm", "Enter", "Activate selected choice"), Key("cancel", "Esc", "Keep draft / return"))
}

package app

import (
	tea "charm.land/bubbletea/v2"
	appcode "github.com/simjay/plous/internal/app/code"

	"github.com/simjay/plous/internal/source"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
	"strings"
)

func (m *model) openCodeDocumentSource() {
	s := m.coder()
	captured := s.DocumentSource
	if s.DocumentPath == "" || captured == nil || !captured.Source.Has(s.DocumentPath) {
		m.status = "No captured source file is available for this document."
		return
	}
	if s.DocumentKind == "file" {
		return
	}
	name := s.DocumentPath
	preview := *s
	preview.Inventory = captured
	s.PushDocument()
	s.ShowDocument(name, appcode.CodeFilePreview(&preview, name))
	s.DocumentSource = captured
	s.DocumentKind, s.DocumentPath = "file", name
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		s.Document.Syntax = "yaml"
	}
}

func (m *model) codeDocumentCommand(command string) bool {
	if !m.codeVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil {
		return false
	}
	s := m.coder()
	if s.Document == nil && s.Review == nil {
		return false
	}
	switch command {
	case "a", "actions":
		if s.Document != nil && s.Document.ReadOnly {
			s.Menu = "document"
			s.Choice = 0
		} else {
			return false
		}
	case "d", "describe":
		if s.Document != nil && s.Document.ReadOnly && s.DocumentPath != "" {
			m.openCodeDocumentSource()
		} else {
			return false
		}

	case "q", "q!":
		if command == "q" && s.Document != nil && s.Document.Dirty() {
			s.Discard = true
			s.DiscardChoice = 0
		} else {
			s.CloseDocument()
		}
	case "w", "wq":
		m.saveCodeDocument(command == "wq")
	case "v", "review":
		if s.File != nil && s.Document != nil {
			s.Review = diffview.New(string(s.File.Baseline), s.Document.Text(), s.File.Directory+"/"+s.File.Path, s.Document.Revision, s.Document.Dirty())
			s.Review.Name = s.File.Path
			s.Review.ConfirmKey = ":w"
			s.Review.ConfirmLabel = "Save file"
			s.Review.Notice = "Review only. :w saves this exact draft to the local file."
		}
	case "u", "undo", "U", "redo":
		if s.Document != nil && s.File != nil {
			if command == "u" || command == "undo" {
				s.Document.Undo()
			} else {
				s.Document.Redo()
			}
			if s.Review != nil {
				s.Review.Invalidate()
			}
		}
	default:
		return false
	}
	return true
}

func (m *model) saveCodeDocument(closeAfter bool) {
	s := m.coder()
	d := s.Document
	if d == nil || s.File == nil || d.ReadOnly {
		m.status = "This document is read-only."
		return
	}
	if !d.Dirty() {
		if closeAfter {
			s.CloseDocument()
		} else {
			d.SavedStatus = "No unsaved changes"
		}
		return
	}
	if s.Review != nil && !s.Review.Matches(string(s.File.Baseline), d.Text(), s.File.Directory+"/"+s.File.Path, d.Revision) {
		d.Diagnostic = "Draft changed after review. Return to editing and review again."
		s.Review.SetError(d.Diagnostic)
		return
	}
	if err := source.ValidateDocument(s.File.Path, []byte(d.Text())); err != nil {
		d.Diagnostic = err.Error()
		if s.Review != nil {
			s.Review.SetError(err.Error())
		}
		return
	}
	if err := s.File.Save([]byte(d.Text())); err != nil {
		d.Diagnostic = err.Error()
		if s.Review != nil {
			s.Review.SetError(err.Error())
		}
		return
	}
	d.MarkSaved(d.Text())
	d.SavedStatus = "Saved to local file"
	d.Diagnostic = ""
	s.Review = nil
	s.Notice = "Saved. Previous renders are stale until the updated preview completes."
	if s.Busy {
		s.Stop()
	}
	if entry := s.Inventory.Entry(s.Preference.Selected); entry != nil && entry.Renderable() && s.Builds[entry.ID] != nil {
		if s.SavedBaseline == nil {
			s.SavedBaseline = s.Builds[entry.ID]
		}
		s.Pending = "render"
	}
	updated, err := s.Inventory.Source.RefreshFile(s.File.Path)
	if err != nil {
		s.Notice = "Saved; refresh discovery: " + err.Error()
		return
	}
	inventory := *s.Inventory
	inventory.Source, inventory.Discovering = updated, true
	s.Inventory = &inventory
	s.Rescan, s.ScanCancelled, s.ScanFailed = true, false, false
	s.DocumentSource = s.Inventory
	for n := range s.Documents {
		frame := &s.Documents[n]
		if frame.Kind != "file" && frame.Path != "" {
			frame.Path = ""
			frame.Document.Target += " · stale source snapshot"
		}
	}
	s.Failures = map[string]string{}
	s.FailureSource = s.Inventory.Source.Identity
	if closeAfter {
		s.CloseDocument()
	}
}

func (m *model) codeKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.codeVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false, nil
	}
	s, key := m.coder(), msg.String()
	if s.Input != nil {
		switch key {
		case "esc":
			s.ExportResult = nil
			s.Input = nil
			s.Menu, s.Choice = s.InputMenu, s.InputChoice
		case "enter":
			m.submitCodeInput()
		default:
			s.Input.Key(msg, false)
		}
		m.shortcutPrefix = ""
		return true, nil
	}
	if s.Discard {
		if key == ":" {
			return false, nil
		}
		switch key {
		case "left", "right", "up", "down":
			s.DiscardChoice = 1 - s.DiscardChoice
		case "esc":
			s.Discard = false
		case "enter":
			s.Discard = false
			if s.DiscardChoice == 1 {
				s.CloseDocument()
			}
		}
		return true, nil
	}
	if s.Menu != "" {
		return false, nil
	}
	if s.Document == nil && s.Review == nil {
		return false, nil
	}
	typing := s.Document != nil && s.Review == nil && !s.Document.ReadOnly && !s.Document.Controls
	searching := s.Document != nil && s.Document.Searching || s.Review != nil && (s.Review.Reader.Searching || s.Review.ErrorActive && s.Review.ErrorReader.Searching)
	insert := typing && s.Document.Mode == docedit.Insert
	if !searching {
		if key == "tab" && !typing {
			return false, nil
		}
		if key == ":" && !insert {
			if s.Document != nil {
				s.Document.ClearPrefix()
			}
			return false, nil
		}
		if !typing && m.updateShortcut(msg) {
			return true, nil
		}
	}
	m.shortcutPrefix = ""
	if s.Review != nil {
		decision, copy := s.Review.Key(key, msg.Text)
		switch decision.Action {
		case "back", "close":
			s.Review = nil
		case "confirm":
			if decision.ID == s.Review.ID {
				m.saveCodeDocument(false)
			}
		}
		return true, m.copyWorkspaceText(copy)
	}
	d := s.Document
	if key == "!" && d.Diagnostic != "" && !d.Searching && (d.Controls || d.Mode == docedit.Normal) {
		message := d.Diagnostic
		s.PushDocument()
		s.ShowDocument("Save error", message)
		s.DocumentKind = "diagnostic"
		return true, nil
	}
	if d.Controls {
		switch key {
		case "left":
			s.DocumentAction = (s.DocumentAction + 5) % 6
		case "right":
			s.DocumentAction = (s.DocumentAction + 1) % 6
		case "enter":
			m.activateCodeDocumentAction(s.DocumentAction)
		case "esc":
			m.codeDocumentCommand("q")
		}
		return true, nil
	}
	event := d.Key(key, msg.Text)
	if event.Action == "back" {
		m.codeDocumentCommand("q")
	}
	if event.Copy != "" {
		s.Yank = event.Copy
	}
	if event.Action == "paste" && !d.ReadOnly && s.Yank != "" {
		d.Paste(s.Yank)
	}
	return true, m.copyWorkspaceText(event.Copy)
}

func (m *model) activateCodeDocumentAction(index int) {
	s := m.coder()
	if s.Document == nil || s.Document.ReadOnly || s.Discard || s.Review != nil || s.Menu != "" {
		return
	}
	if index == 0 {
		s.Document.Controls, s.Document.Mode = false, docedit.Insert
		return
	}
	if index >= 1 && index < 6 {
		m.codeDocumentCommand([]string{"", "w", "v", "u", "U", "q"}[index])
	}
}

func (m *model) codeDocumentActions() []textview.ActionGroup {
	s := m.coder()
	d := s.Document
	if d.ReadOnly {
		actions := []textview.Action{{Key: ":q", Label: "Back"}, {Key: "/", Label: "Find"}, {Key: "y", Label: "Copy"}}
		if s.DocumentKind == "file" && s.Config.Repository == "" {
			actions = append([]textview.Action{{Key: ":a", Label: "File Actions"}}, actions...)
		} else if s.DocumentPath != "" && s.DocumentKind != "file" {
			actions = append([]textview.Action{{Key: ":d", Label: "Open Source"}}, actions...)
		}
		return []textview.ActionGroup{{Label: "Read", Actions: actions}, m.documentDisplayActions(d)}
	}
	if !d.Controls && d.Mode != docedit.Normal {
		return []textview.ActionGroup{{Label: "Editing", Actions: []textview.Action{{Key: "Esc", Label: "Normal mode"}}}}
	}
	labels := []string{"Insert", "Save", "Review", "Undo", "Redo", "Close"}
	keys := []string{"i", ":w", ":v", "u", "U", ":q"}
	buttons := []textview.Action{}
	for i, label := range labels {
		button := textview.Action{Key: keys[i], Label: label, Mouse: "codedocumentaction/" + strconv.Itoa(i), Primary: i == 1}
		if d.Controls {
			if !strings.HasPrefix(button.Key, ":") {
				button.Key = ""
			}
			button.Focused = i == s.DocumentAction
			if button.Focused {
				button.Key = "Enter"
			}
		}
		buttons = append(buttons, button)
	}
	if d.Diagnostic != "" {
		buttons = append(buttons, textview.Action{Key: "!", Label: "Error details"})
	}
	return []textview.ActionGroup{{Label: "File", Actions: buttons}, m.documentDisplayActions(d)}
}

func (m *model) codePaste(text string) bool {
	if !m.codeVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	s := m.coder()
	if s.Menu != "" || s.Discard {
		return true
	}
	if s.Input != nil {
		s.Input.Insert(text, false)
	} else if s.Review != nil {
		if s.Review.ErrorActive {
			s.Review.ErrorReader.Paste(text)
		} else {
			s.Review.Reader.Paste(text)
		}
	} else if s.Document != nil && !s.Discard && !s.Document.Controls {
		s.Document.Paste(text)
	}
	return true
}

func (m *model) codeHint() string {
	s := m.coder()
	if s.Input != nil {
		return "Enter Confirm · Esc Back · Alt+←/→ Panels"
	}
	if s.Discard {
		return "←→ Choose · Enter Confirm · Esc Keep Editing"
	}
	if s.Menu != "" {
		return "↑↓ Choose · Enter Open · Esc Back · hh Help"
	}
	if s.Review != nil {
		if s.Review.Reader.Searching || s.Review.ErrorActive && s.Review.ErrorReader.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		if s.File != nil {
			return ":w Save file · Esc " + s.Review.EscapeLabel() + " · :h Help"
		}
		return "[ ] Change · / Find · Esc " + s.Review.EscapeLabel() + " · :h Help"
	}
	if d := s.Document; d != nil {
		if d.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		if d.Mode == docedit.Insert {
			return "Tab Indent · Esc Normal mode"
		}
		if d.ReadOnly {
			return "/ Find · y Copy · Esc " + documentBackHint(d, "Back") + " · :h Help"
		}
		if d.Controls {
			return "←→ Choose action · Enter Activate · Esc Close editor · :h Help"
		}
		return "i Insert · :w Save · Esc " + documentBackHint(d, "Actions") + " · :h Help"
	}
	if s.Busy && s.Running != "preview" && !s.BrowsingDuringDiscovery() {
		return "Esc Cancel source operation"
	}
	if s.Filter != "" {
		if s.KustomizeDirectory() {
			return "↑↓ Groups · Enter Open group · Backspace Clear filter · Esc Clear filter · hh Help"
		}
		if s.View == "code-kustomize" && !s.BrowserDetail {
			return "↑↓ Matches · ←→ Follow reference · Enter Overview · Esc Clear filter"
		}
		action := "Open"
		if m.tool == "manifests" && s.Preference.Section == 0 && s.Inventory != nil {
			if e := s.Inventory.Entry(s.Preference.Selected); e != nil && e.Renderable() {
				action = "Render"
			}
		}
		return "Enter " + action + " · / Filter · Backspace Clear filter · Esc Clear filter · hh Help"
	}
	if s.Inventory == nil {
		return "Enter Retry · a Actions · Esc Sidebar · hh Help"
	}
	if m.tool == "manifests" {
		if s.Preference.Section == 0 {
			action := "Render"
			if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
				if e.Component {
					action = "Relationships"
				} else if !e.Renderable() {
					action = "Files"
				}
			}
			back := "Sidebar"
			if s.ReturnView != "" {
				back = s.DetailBackLabel()
			}
			return "↑↓ Select node · ←→ Map/details · Enter " + action + " · / Filter · Esc " + back + " · hh Help"
		}
		back := "Configuration Map"
		if s.ReturnView != "" {
			back = s.DetailBackLabel()
		}
		if s.Preference.Section != 1 {
			back = "Back"
		}
		if s.Preference.Section == 3 {
			return "Enter Details · ←→ Area · ↑↓ Select/scroll · a Actions · Esc " + back + " · hh Help"
		}
		return "←→ Area · ↑↓ Select/scroll · a Actions · Esc " + back + " · hh Help"
	}

	if m.tool == "files" || m.tool == "code" {
		return "Enter Open · ←→ Folders · / Filter · Esc Sidebar · hh Help"
	}
	if s.KustomizeDirectory() {
		return "↑↓ Groups · Enter Open group · / Find group · Esc Sidebar · hh Help"
	}
	if s.View == "code-kustomize" && !s.BrowserDetail {
		return "↑↓ Nodes · ←→ Follow reference · Enter Overview · a Actions · hh Help"
	}
	if s.View == "code-kustomize" && s.BrowserDetail {
		if len(s.RenderCandidates()) > 0 {
			return "G Graph · r Render · d Open source · Esc Back · hh Help"
		}
		return "↑↓ Scroll · G Graph · d Open source · Esc Back · hh Help"
	}
	if len(s.RenderCandidates()) > 0 {
		return "Enter Details · r Render · d Open in Files · / Find · Esc Back · hh Help"
	}
	return "Enter Details · d Open in Files · / Find · Esc Back · hh Help"
}

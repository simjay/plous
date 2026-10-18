package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
	"strings"
)

func (m *model) documentActive() bool {
	return m.explore != nil && m.explore.Action != nil && (m.explore.Action.Stage == "edit" || m.explore.Action.Stage == "review")
}

func (m *model) closeDocument() {
	a := m.explore.Action
	if a.Document != nil && a.Document.Dirty() {
		a.Discard, a.DocumentAction = true, 0
		return
	}
	m.stopResourceAction()
}

func (m *model) validateDocument() {
	a := m.explore.Action
	if a.Busy || a.Pending != "" {
		return
	}
	a.Diagnostic, a.Err, a.ConfirmedID = nil, "", ""
	a.DocumentErrorTitle = ""
	if a.Document != nil {
		a.Document.Notice, a.Document.Diagnostic = "", ""
	}
	a.CloseAfterApply, a.SaveRequested = false, false
	a.Review, a.Proposed = nil, nil
	a.Stage = "edit"
	a.Pending = "validate"
}

func (m *model) saveDocument(closeAfter bool) {
	a := m.explore.Action
	if a.Busy || a.Pending != "" || a.Document == nil {
		return
	}
	if !a.Document.Dirty() {
		if closeAfter {
			m.stopResourceAction()
		} else {
			a.Document.SavedStatus = "No unsaved changes"
		}
		return
	}
	if a.Stage == "review" && a.Review != nil {
		m.confirmDocument(a.Review.ID)
	} else {
		m.validateDocument()
		a.SaveRequested = true
		a.SaveRevision = a.Document.Revision
	}
	if a.SaveRequested {
		a.Discard = false
		a.CloseAfterApply = closeAfter
	}
}

func (m *model) confirmDocument(id string) {
	a := m.explore.Action
	if a.Busy || a.Pending != "" || a.Review == nil || a.Document == nil {
		return
	}
	if id != a.Review.ID || a.Review.Stale || a.Proposed == nil || a.ValidatedRevision != a.Document.Revision {
		a.FailDocument("Save stopped", "The draft changed after review. Check it in the editor before saving.")
		return
	}
	a.ConfirmedID, a.Pending = id, "apply"
	a.SaveRequested, a.SaveRevision = true, a.Document.Revision
}

// Document commands apply only to the focused editable workflow.
func (m *model) editorLocalCommand(command string) bool {
	if !m.documentActive() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil {
		return false
	}
	a := m.explore.Action
	switch command {
	case "w", "wq":
		m.saveDocument(command == "wq")
	case "q", "q!":
		if !a.Busy {
			if command == "q!" {
				m.stopResourceAction()
			} else {
				m.closeDocument()
			}
		}
	case "v", "review":
		if a.Stage == "edit" {
			m.validateDocument()
		}
	case "u", "undo", "U", "redo":
		if a.Stage == "edit" && !a.Busy && a.Document != nil {
			if command == "u" || command == "undo" {
				a.Document.Undo()
			} else {
				a.Document.Redo()
			}
			if a.Review != nil {
				a.Review.Invalidate()
			}
		}
	default:
		return false
	}
	return true
}

func (m *model) updateDocumentKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	key := msg.String()
	if a.Document == nil {
		a.InitDocument()
	}
	if a.Discard {
		switch key {
		case "left", "right":
			a.DocumentAction = 1 - a.DocumentAction
		case "esc":
			a.Discard = false
		case "enter":
			if a.DocumentAction == 1 {
				m.stopResourceAction()
			} else {
				a.Discard = false
			}
		}
		return nil
	}
	if a.Diagnostic != nil {
		handled, copy := a.Diagnostic.Key(key, msg.Text)
		if key == "esc" && !handled {
			a.Diagnostic = nil
		}
		return m.copyWorkspaceText(copy)
	}
	if a.Stage == "review" && a.Review != nil {
		decision, copy := a.Review.Key(key, msg.Text)
		switch decision.Action {
		case "back", "close":
			a.Stage = "edit"
			a.CloseAfterApply = false
		case "confirm":
			m.confirmDocument(decision.ID)
		}
		return m.copyWorkspaceText(copy)
	}
	d := a.Document
	if key == "!" && a.Err != "" && !d.Searching && (d.Controls || d.Mode == docedit.Normal) {
		a.ShowDiagnostic(a.Err)
		return nil
	}
	if d.Controls {
		switch key {
		case "left":
			a.DocumentAction = (a.DocumentAction + 5) % 6
		case "right":
			a.DocumentAction = (a.DocumentAction + 1) % 6
		case "esc":
			m.closeDocument()
		case "enter":
			switch a.DocumentAction {
			case 0:
				d.Controls = false
				d.Mode = docedit.Insert
			case 1:
				m.saveDocument(false)
			case 2:
				m.validateDocument()
			case 3:
				d.Undo()
			case 4:
				d.Redo()
			case 5:
				m.closeDocument()
			}
		}
		return nil
	}
	revision := d.Revision
	event := d.Key(key, msg.Text)
	if event.Copy != "" {
		a.Yank = event.Copy
	}
	switch event.Action {
	case "paste":
		if a.Yank != "" {
			d.Paste(a.Yank)
		}
	}
	if revision != d.Revision && a.Review != nil {
		a.Review.Invalidate()
	}
	return m.copyWorkspaceText(event.Copy)
}

func (m *model) documentBody(width, height int) string {
	a := m.explore.Action
	if a.Document == nil {
		a.InitDocument()
	}
	if a.Discard {
		actions := []textview.Action{{Mouse: "confirm/0", Label: "Keep Editing", Focused: a.DocumentAction == 0}, {Mouse: "confirm/1", Label: "Discard Changes", Focused: a.DocumentAction == 1}}
		actions[a.DocumentAction].Key = "Enter"
		return ui.TitleStyle.Render("Unsaved document") + "\n" + ui.Clipped(a.Document.Name, width) + "\n\n" + textview.ActionBar([]textview.ActionGroup{{Actions: actions}}, width, max(1, height-5), ui.WorkspaceTheme()) + "\n\n" + ui.Muted.Render("←→ choose · Enter confirm · Esc keep editing")
	}
	if a.Diagnostic != nil {
		controls := []textview.ActionGroup{{Label: "Read", Actions: []textview.Action{{Key: "Esc", Label: m.documentEscapeLabel()}, {Key: ":h", Label: "Help"}, {Key: "/", Label: "Search"}, {Key: "v", Label: "Select"}, {Key: "y", Label: "Copy"}}}}
		title := a.DocumentErrorTitle
		if title == "" {
			title = "Error details"
		}
		header := ui.RemovedStyle.Render(title) + "\n" + ui.Clipped(a.Document.Name, width) + "\n" + ui.Muted.Render("Esc returns to your draft.")
		header += "\n" + textview.ActionBar(controls, width, max(1, min(3, height-lipgloss.Height(header)-3)), ui.WorkspaceTheme())
		return textview.Fit(header+"\n"+a.Diagnostic.View(width, max(0, height-lipgloss.Height(header)-1), ui.WorkspaceTheme()), width, height)
	}
	if a.Stage == "review" && a.Review != nil {
		a.Review.CanConfirm = !a.Busy && a.Pending == "" && !a.Review.Stale && a.Document.Dirty()
		a.Review.Notice = "Review only. Nothing saved.\n:w saves to Kubernetes. Esc returns to editing."
		switch {
		case a.Busy || a.Pending == "apply":
			a.Review.Notice = "Saving to Kubernetes. Waiting for confirmation."
		case a.Review.Stale:
			a.Review.Notice = "Draft changed. Return to the editor before saving."
		case a.Review.Added+a.Review.Removed == 0:
			a.Review.Notice = "No manifest differences. Esc returns to editing."
		}
		a.Review.EscapeOverride = m.documentEscapeLabel()
		a.Review.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{{Key: ":h", Label: "Help"}, m.expandAction()}}}
		return a.Review.View(width, height, ui.WorkspaceTheme())
	}
	labels := []string{"Insert", "Save", "Review changes", "Undo", "Redo", "Close"}
	keys := []string{"i", ":w", ":v", "u", "U", ":q"}
	buttons := []textview.Action{}
	for i, label := range labels {
		button := textview.Action{Mouse: "documentaction/" + strconv.Itoa(i), Key: keys[i], Label: label, Primary: i == 1}
		if a.Document.Controls {
			if !strings.HasPrefix(button.Key, ":") {
				button.Key = ""
			}
			button.Focused = a.DocumentAction == i
			if button.Focused {
				button.Key = "Enter"
			}
		}
		buttons = append(buttons, button)
	}
	actions := []textview.ActionGroup{{Label: "Document", Actions: buttons}}
	if !a.Document.Controls && a.Document.Mode != docedit.Normal {
		actions = []textview.ActionGroup{{Label: "Editing", Actions: []textview.Action{{Key: "Esc", Label: "Normal mode"}}}}
	}
	if a.Busy {
		label := "Validating"
		if a.SaveRequested {
			label = "Saving"
		}
		actions = []textview.ActionGroup{{Label: label, Actions: []textview.Action{{Key: "Esc", Label: "Cancel"}}}}
	}
	a.Document.Actions = actions
	if !a.Busy && a.Document.Mode == docedit.Normal {
		display := m.documentDisplayActions(a.Document)
		display.Actions = append([]textview.Action{{Key: ":h", Label: "Editor help"}}, display.Actions...)
		a.Document.Actions = append(a.Document.Actions, textview.ActionGroup{Label: "Close", Actions: []textview.Action{{Key: ":wq", Label: "Save and close"}, {Key: ":q!", Label: "Discard"}}}, display)
	}
	return a.Document.View(width, height, ui.WorkspaceTheme())
}

func (m *model) documentHint() string {
	a := m.explore.Action
	if a.Busy {
		return "Esc Cancel request"
	}
	if a.Discard {
		return "←→ Choose · Enter Confirm · Esc Keep Editing"
	}
	if a.Diagnostic != nil {
		if a.Diagnostic.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · Esc " + m.documentEscapeLabel() + " · :h Help"
	}
	if a.Stage == "review" {
		if a.Review != nil && (a.Review.Reader.Searching || a.Review.ErrorActive && a.Review.ErrorReader.Searching) {
			return "Enter Finish search · Esc Cancel search"
		}
		return ":w Save · Esc " + m.documentEscapeLabel() + " · :h Help"
	}
	if a.Document == nil {
		return "i Insert · :w Save · Esc Actions · :h Help"
	}
	if a.Document.Controls {
		return ":w Save · ←→ Choose action · Enter Activate · Esc Close editor · :h Help"
	}
	if a.Document.Searching {
		return "Enter Finish search · Esc Cancel search"
	}
	if a.Document.Mode == docedit.Insert {
		return "Tab Indent · Esc Normal mode"
	}
	if a.Document.Mode == docedit.Select {
		return "↑↓←→ Select text · y Copy · Esc Normal mode · :h Help"
	}
	return "i Insert · :w Save · Esc " + documentBackHint(a.Document, "Actions") + " · :h Help"
}

func (m *model) documentEscapeLabel() string {
	a := m.explore.Action
	if a.Busy {
		return "Cancel request"
	}
	if a.Diagnostic != nil {
		return a.Diagnostic.EscapeLabel("Back to editor")
	}
	if a.Review != nil && a.Stage == "review" {
		label := a.Review.EscapeLabel()
		if label == a.Review.BackLabel && m.viewExpanded() {
			return "Restore layout"
		}
		return label
	}
	return "Back to editor"
}

// Local editors consume text before the global command and shortcut router.
func (m *model) workspaceKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() && !(m.resourceTerminalVisible() && m.explore.Terminal.Captured) {
		return false, nil
	}
	key := msg.String()
	if m.resourceTerminalVisible() {
		t := m.explore.Terminal
		local := t.Captured || t.Review != nil && t.Review.Viewer.Reader.Searching
		if !local {
			if key == ":" || key == "tab" {
				return false, nil
			}
			if m.updateShortcut(msg) {
				m.persist()
				return true, nil
			}
		}
		m.shortcutPrefix = ""
		return true, m.updateResourceTerminalKey(msg)
	}
	if m.documentActive() {
		a := m.explore.Action
		if a.Busy {
			return true, m.updateResourceActionKey(msg)
		}
		if a.Document == nil {
			a.InitDocument()
		}
		d := a.Document
		typing := !a.Discard && a.Diagnostic == nil && a.Stage == "edit" && !d.Controls
		search := d.Searching && typing || a.Diagnostic != nil && a.Diagnostic.Searching || a.Stage == "review" && a.Review != nil && (a.Review.Reader.Searching || a.Review.ErrorActive && a.Review.ErrorReader.Searching)
		if !search && !a.Discard {
			if key == "tab" && !typing {
				return false, nil
			}
			if key == ":" && !(typing && d.Mode == docedit.Insert) {
				d.ClearPrefix()
				return false, nil
			}
		}
		if !typing && !search && !a.Discard && m.updateShortcut(msg) {
			m.persist()
			return true, nil
		}
		m.shortcutPrefix = ""
		return true, m.updateDocumentKey(msg)
	}
	if handled, cmd := m.readOnlyWorkspaceKey(msg); handled {
		return true, cmd
	}
	return m.logWorkspaceKey(msg)
}

func (m *model) workspacePaste(text string) bool {
	if m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	if m.activityReviewPaste(text) || m.yamlDocumentPaste(text) {
		return true
	}
	if m.valueDocumentActive() {
		m.explore.Action.ValueDocument.Paste(text)
		return true
	}
	if m.documentActive() {
		a := m.explore.Action
		if a.Busy || a.Discard {
			return true
		}
		if a.Diagnostic != nil {
			a.Diagnostic.Paste(text)
			return true
		}
		if a.Stage == "review" && a.Review != nil {
			if a.Review.ErrorActive {
				a.Review.ErrorReader.Paste(text)
			} else {
				a.Review.Reader.Paste(text)
			}
			return true
		}
		if a.Document == nil {
			a.InitDocument()
		}
		if !a.Document.Controls {
			a.Document.Paste(text)
			if a.Review != nil {
				a.Review.Invalidate()
			}
		}
		return true
	}
	if m.resourceTerminalVisible() && !m.explore.Terminal.Captured {
		m.explore.Terminal.Review.Viewer.Reader.Paste(text)
		return true
	}
	if m.logWorkspaceActive() {
		m.podLogViewer().Reader.Paste(text)
		return true
	}
	return false
}

func (m *model) clearDocumentPrefix() (changed bool) {
	if m.codeActive() {
		if d := m.coder().Document; d != nil {
			changed = d.Prefix() != ""
			d.ClearPrefix()
		}
	}

	if m.documentActive() && m.explore.Action.Document != nil {
		changed = changed || m.explore.Action.Document.Prefix() != ""
		m.explore.Action.Document.ClearPrefix()
	}
	return changed
}

package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/textview"
)

func workspaceTheme() textview.Theme {
	t := textview.DefaultTheme()
	t.Text, t.Muted, t.Accent = canvasStyle, muted, lipgloss.NewStyle().Foreground(accent).Bold(true)
	t.Added, t.Removed = addedStyle, removedStyle
	t.Selection = lipgloss.NewStyle().Background(highlight)
	return t
}

func (a *resourceActionState) initDocument() {
	a.document = docedit.New(kubernetes.YAML(*a.object, true))
	a.document.Syntax = "yaml"
	a.document.Name = a.resource.Kind + " / " + a.object.GetName()
	a.document.Target = a.context + " · " + a.object.GetNamespace() + "\nUID " + string(a.object.GetUID())
}
func (a *resourceActionState) showDiagnostic(message string) {
	a.diagnostic = &textview.Viewer{Wrap: true}
	rows := []textview.Line{}
	for _, line := range strings.Split(message, "\n") {
		rows = append(rows, textview.Line{Text: line})
	}
	a.diagnostic.SetLines(rows)
}

func (a *resourceActionState) failDocument(title, message string) {
	a.err, a.documentErrorTitle = message, title
	a.pending, a.runningJob, a.confirmedID = "", "", ""
	a.saveRequested, a.closeAfterApply = false, false
	a.diagnostic = nil
	if a.review != nil {
		a.review.Invalidate()
	}
	if a.document != nil {
		a.stage = "edit"
		a.document.Controls = false
		a.document.Diagnostic = title + " · ! Error details\n" + message
	}
}
func (m *model) documentActive() bool {
	return m.explore != nil && m.explore.action != nil && (m.explore.action.stage == "edit" || m.explore.action.stage == "review")
}
func (m *model) closeDocument() {
	a := m.explore.action
	if a.document != nil && a.document.Dirty() {
		a.discard, a.documentAction = true, 0
		return
	}
	m.stopResourceAction()
}
func (m *model) validateDocument() {
	a := m.explore.action
	if a.busy || a.pending != "" {
		return
	}
	a.diagnostic, a.err, a.confirmedID = nil, "", ""
	a.documentErrorTitle = ""
	if a.document != nil {
		a.document.Notice, a.document.Diagnostic = "", ""
	}
	a.closeAfterApply, a.saveRequested = false, false
	a.review, a.proposed = nil, nil
	a.stage = "edit"
	a.pending = "validate"
}

func (m *model) saveDocument(closeAfter bool) {
	a := m.explore.action
	if a.busy || a.pending != "" || a.document == nil {
		return
	}
	if !a.document.Dirty() {
		if closeAfter {
			m.stopResourceAction()
		} else {
			a.document.SavedStatus = "No unsaved changes"
		}
		return
	}
	if a.stage == "review" && a.review != nil {
		m.confirmDocument(a.review.ID)
	} else {
		m.validateDocument()
		a.saveRequested = true
		a.saveRevision = a.document.Revision
	}
	if a.saveRequested {
		a.discard = false
		a.closeAfterApply = closeAfter
	}
}

func (m *model) confirmDocument(id string) {
	a := m.explore.action
	if a.busy || a.pending != "" || a.review == nil || a.document == nil {
		return
	}
	if id != a.review.ID || a.review.Stale || a.proposed == nil || a.validatedRevision != a.document.Revision {
		a.failDocument("Save stopped", "The draft changed after review. Check it in the editor before saving.")
		return
	}
	a.confirmedID, a.pending = id, "apply"
	a.saveRequested, a.saveRevision = true, a.document.Revision
}

// Document commands apply only to the focused editable workflow.
func (m *model) editorLocalCommand(command string) bool {
	if !m.documentActive() || m.focus != 1 || m.help != nil || m.settingsOverlay != nil {
		return false
	}
	a := m.explore.action
	switch command {
	case "w", "wq":
		m.saveDocument(command == "wq")
	case "q", "q!":
		if !a.busy {
			if command == "q!" {
				m.stopResourceAction()
			} else {
				m.closeDocument()
			}
		}
	case "v", "review":
		if a.stage == "edit" {
			m.validateDocument()
		}
	case "u", "undo", "U", "redo":
		if a.stage == "edit" && !a.busy && a.document != nil {
			if command == "u" || command == "undo" {
				a.document.Undo()
			} else {
				a.document.Redo()
			}
			if a.review != nil {
				a.review.Invalidate()
			}
		}
	default:
		return false
	}
	return true
}

func (m *model) updateDocumentKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.action
	key := msg.String()
	if a.document == nil {
		a.initDocument()
	}
	if a.discard {
		switch key {
		case "left", "right":
			a.documentAction = 1 - a.documentAction
		case "esc":
			a.discard = false
		case "enter":
			if a.documentAction == 1 {
				m.stopResourceAction()
			} else {
				a.discard = false
			}
		}
		return nil
	}
	if a.diagnostic != nil {
		handled, copy := a.diagnostic.Key(key, msg.Text)
		if key == "esc" && !handled {
			a.diagnostic = nil
		}
		return m.copyWorkspaceText(copy)
	}
	if a.stage == "review" && a.review != nil {
		decision, copy := a.review.Key(key, msg.Text)
		switch decision.Action {
		case "back", "close":
			a.stage = "edit"
			a.closeAfterApply = false
		case "confirm":
			m.confirmDocument(decision.ID)
		}
		return m.copyWorkspaceText(copy)
	}
	d := a.document
	if key == "!" && a.err != "" && !d.Searching && (d.Controls || d.Mode == docedit.Normal) {
		a.showDiagnostic(a.err)
		return nil
	}
	if d.Controls {
		switch key {
		case "left":
			a.documentAction = (a.documentAction + 5) % 6
		case "right":
			a.documentAction = (a.documentAction + 1) % 6
		case "esc":
			m.closeDocument()
		case "enter":
			switch a.documentAction {
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
		a.yank = event.Copy
	}
	switch event.Action {
	case "paste":
		if a.yank != "" {
			d.Paste(a.yank)
		}
	}
	if revision != d.Revision && a.review != nil {
		a.review.Invalidate()
	}
	return m.copyWorkspaceText(event.Copy)
}
func (m *model) copyWorkspaceText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	m.status = "Copied to the terminal clipboard. Clipboard support depends on your terminal."
	return tea.SetClipboard(text)
}

func (m *model) documentBody(width, height int) string {
	a := m.explore.action
	if a.document == nil {
		a.initDocument()
	}
	if a.discard {
		actions := []textview.Action{{Label: "Keep editing", Focused: a.documentAction == 0}, {Label: "Discard changes", Focused: a.documentAction == 1}}
		actions[a.documentAction].Key = "Enter"
		return titleStyle.Render("Unsaved document") + "\n" + clipped(a.document.Name, width) + "\n\n" + textview.ActionBar([]textview.ActionGroup{{Actions: actions}}, width, max(1, height-5), workspaceTheme()) + "\n\n" + muted.Render("←→ choose · Enter confirm · Esc keep editing")
	}
	if a.diagnostic != nil {
		controls := []textview.ActionGroup{{Label: "Read", Actions: []textview.Action{{Key: "Esc", Label: m.documentEscapeLabel()}, {Key: ":h", Label: "Help"}, {Key: "/", Label: "Search"}, {Key: "v", Label: "Select"}, {Key: "y", Label: "Copy"}}}}
		title := a.documentErrorTitle
		if title == "" {
			title = "Error details"
		}
		header := removedStyle.Render(title) + "\n" + clipped(a.document.Name, width) + "\n" + muted.Render("Esc returns to your draft.")
		header += "\n" + textview.ActionBar(controls, width, max(1, min(3, height-lipgloss.Height(header)-3)), workspaceTheme())
		return textview.Fit(header+"\n"+a.diagnostic.View(width, max(0, height-lipgloss.Height(header)-1), workspaceTheme()), width, height)
	}
	if a.stage == "review" && a.review != nil {
		a.review.CanConfirm = !a.busy && a.pending == "" && !a.review.Stale && a.document.Dirty()
		a.review.Notice = "Review only. Nothing saved.\n:w saves to Kubernetes. Esc returns to editing."
		switch {
		case a.busy || a.pending == "apply":
			a.review.Notice = "Saving to Kubernetes. Waiting for confirmation."
		case a.review.Stale:
			a.review.Notice = "Draft changed. Return to the editor before saving."
		case a.review.Added+a.review.Removed == 0:
			a.review.Notice = "No manifest differences. Esc returns to editing."
		}
		a.review.EscapeOverride = m.documentEscapeLabel()
		a.review.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{{Key: ":h", Label: "Help"}, m.expandAction()}}}
		return a.review.View(width, height, workspaceTheme())
	}
	labels := []string{"Insert", "Save", "Review changes", "Undo", "Redo", "Close"}
	keys := []string{"i", ":w", ":v", "u", "U", ":q"}
	buttons := []textview.Action{}
	for i, label := range labels {
		button := textview.Action{Key: keys[i], Label: label, Primary: i == 1}
		if a.document.Controls {
			if !strings.HasPrefix(button.Key, ":") {
				button.Key = ""
			}
			button.Focused = a.documentAction == i
			if button.Focused {
				button.Key = "Enter"
			}
		}
		buttons = append(buttons, button)
	}
	actions := []textview.ActionGroup{{Label: "Document", Actions: buttons}}
	if !a.document.Controls && a.document.Mode != docedit.Normal {
		actions = []textview.ActionGroup{{Label: "Editing", Actions: []textview.Action{{Key: "Esc", Label: "Normal mode"}}}}
	}
	if a.busy {
		label := "Validating"
		if a.saveRequested {
			label = "Saving"
		}
		actions = []textview.ActionGroup{{Label: label, Actions: []textview.Action{{Key: "Esc", Label: "Cancel"}}}}
	}
	a.document.Actions = actions
	if !a.busy && a.document.Mode == docedit.Normal {
		display := m.documentDisplayActions(a.document)
		display.Actions = append([]textview.Action{{Key: ":h", Label: "Editor help"}}, display.Actions...)
		a.document.Actions = append(a.document.Actions, textview.ActionGroup{Label: "Close", Actions: []textview.Action{{Key: ":wq", Label: "Save and close"}, {Key: ":q!", Label: "Discard"}}}, display)
	}
	return a.document.View(width, height, workspaceTheme())
}
func (m *model) documentHint() string {
	a := m.explore.action
	if a.busy {
		return "Esc Cancel request"
	}
	if a.discard {
		return "←→ Choose · Enter Confirm · Esc Keep editing"
	}
	if a.diagnostic != nil {
		if a.diagnostic.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · Esc " + m.documentEscapeLabel() + " · :h Help"
	}
	if a.stage == "review" {
		if a.review != nil && (a.review.Reader.Searching || a.review.ErrorActive && a.review.ErrorReader.Searching) {
			return "Enter Finish search · Esc Cancel search"
		}
		return ":w Save · Esc " + m.documentEscapeLabel() + " · :h Help"
	}
	if a.document == nil {
		return "i Insert · :w Save · Esc Actions · :h Help"
	}
	if a.document.Controls {
		return ":w Save · ←→ Choose action · Enter Activate · Esc Close editor · :h Help"
	}
	if a.document.Searching {
		return "Enter Finish search · Esc Cancel search"
	}
	if a.document.Mode == docedit.Insert {
		return "Tab Indent · Esc Normal mode"
	}
	if a.document.Mode == docedit.Select {
		return "↑↓←→ Select text · y Copy · Esc Normal mode · :h Help"
	}
	return "i Insert · :w Save · Esc " + documentBackHint(a.document, "Actions") + " · :h Help"
}

func (m *model) documentEscapeLabel() string {
	a := m.explore.action
	if a.busy {
		return "Cancel request"
	}
	if a.diagnostic != nil {
		return a.diagnostic.EscapeLabel("Back to editor")
	}
	if a.review != nil && a.stage == "review" {
		label := a.review.EscapeLabel()
		if label == a.review.BackLabel && m.viewExpanded() {
			return "Restore layout"
		}
		return label
	}
	return "Back to editor"
}

// Local editors consume text before the global command and shortcut router.
func (m *model) workspaceKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.focus != 1 || m.help != nil || m.settingsOverlay != nil || m.command != nil || m.tooSmall() && !(m.resourceTerminalVisible() && m.explore.terminal.captured) {
		return false, nil
	}
	key := msg.String()
	if m.resourceTerminalVisible() {
		t := m.explore.terminal
		local := t.captured || t.review != nil && t.review.viewer.Reader.Searching
		if !local {
			if key == "tab" || key == "shift+tab" || key == ":" {
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
		a := m.explore.action
		if a.busy {
			return true, m.updateResourceActionKey(msg)
		}
		if a.document == nil {
			a.initDocument()
		}
		d := a.document
		typing := !a.discard && a.diagnostic == nil && a.stage == "edit" && !d.Controls
		search := d.Searching && typing || a.diagnostic != nil && a.diagnostic.Searching || a.stage == "review" && a.review != nil && (a.review.Reader.Searching || a.review.ErrorActive && a.review.ErrorReader.Searching)
		if !search && !a.discard {
			if key == "shift+tab" || key == "tab" && !(typing && d.Mode == docedit.Insert) {
				d.ClearPrefix()
				return false, nil
			}
			if key == ":" && !(typing && d.Mode == docedit.Insert) {
				d.ClearPrefix()
				return false, nil
			}
		}
		if !typing && !search && !a.discard && m.updateShortcut(msg) {
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
	if m.focus != 1 || m.help != nil || m.settingsOverlay != nil || m.command != nil || m.tooSmall() {
		return false
	}
	if m.activityReviewPaste(text) || m.yamlDocumentPaste(text) {
		return true
	}
	if m.valueDocumentActive() {
		m.explore.action.valueDocument.Paste(text)
		return true
	}
	if m.documentActive() {
		a := m.explore.action
		if a.busy || a.discard {
			return true
		}
		if a.diagnostic != nil {
			a.diagnostic.Paste(text)
			return true
		}
		if a.stage == "review" && a.review != nil {
			if a.review.ErrorActive {
				a.review.ErrorReader.Paste(text)
			} else {
				a.review.Reader.Paste(text)
			}
			return true
		}
		if a.document == nil {
			a.initDocument()
		}
		if !a.document.Controls {
			a.document.Paste(text)
			if a.review != nil {
				a.review.Invalidate()
			}
		}
		return true
	}
	if m.resourceTerminalVisible() && !m.explore.terminal.captured {
		m.explore.terminal.review.viewer.Reader.Paste(text)
		return true
	}
	if m.logWorkspaceActive() {
		m.podLogViewer().Reader.Paste(text)
		return true
	}
	return false
}

func (m *model) clearDocumentPrefix() {
	if m.documentActive() && m.explore.action.document != nil {
		m.explore.action.document.ClearPrefix()
	}
}

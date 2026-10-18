package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"

	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) footerContext() string {
	switch m.focus {
	case 0:
		return "Connections"
	case 2:
		return "AI assistant"
	}
	if m.presentedDocumentVisible() {
		return "Assistant document"
	}
	if m.codeVisible() {
		s := m.coder()
		if s.Review != nil {
			return "Code comparison"
		}
		if s.Document != nil {
			return "Code document"
		}
		return "Code " + s.ViewLabel()
	}

	if m.gitOpsVisible() {
		return m.gitOpsCurrent().State.Title()
	}
	if m.helmVisible() {
		return "Helm " + m.helmer().Preference.Section
	}
	if m.exploreVisible() && m.explore != nil && m.explore.Resource.Name != "" {
		return monitor.ResourceTitle(m.explore.Resource)
	}
	return modes[m.mode]
}

func (m *model) sidebarHint() string {
	if m.leftHidden {
		return "bb Show connections"
	}
	return "Esc Connections"
}

func documentBackHint(d *docedit.Model, back string) string {
	switch {
	case d.Searching:
		return "Cancel search"
	case d.Prefix() != "":
		return "Cancel pending key"
	case d.Mode != docedit.Normal:
		return "Normal mode"
	case d.Query != "":
		return "Clear search"
	}
	return back
}

func (m *model) footer() string {
	label, message := m.footerContext(), m.hint()
	if m.helpVisible() {
		label = "Help"
	} else if m.chatSourcesVisible() {
		label, message = "Sources", m.chatSourcesHint()
		if m.chatSources.ShowReply {
			label = "Read reply"
		}
	} else if m.settingsOverlay != nil && m.focus == 1 {
		label = "Settings"
		if m.toolSetup != nil {
			label = "Add Connection"
			if m.toolSetup.ContextsOnly {
				label = "Add Context"
			}
		}
		if m.form != nil {
			if m.form.Source != nil {
				label = m.form.Title
			} else if m.form.Kind == "kube" || m.form.Kind == "contextname" {
				label = "Add Context"
			}
		}
	} else if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		label, message = "Actions", "↑↓ Choose · Enter Select · Esc Back · hh Help"
		a := m.explore.Action
		if a.Busy {
			message = "Esc Cancel request"
		} else if a.Stage == "scale" || a.Stage == "export" || a.Stage == "execcommand" && a.ExecEditing {
			message = "Enter Continue · Esc Back"
		} else if a.Stage == "confirm" {
			message = "←→ Choose · Enter Confirm · Esc Back · hh Help"
		} else if a.Stage == "done" {
			message = "Esc Return to resource · hh Help"
			if a.Progress != nil {
				message = monitor.OperationProgressFooter(a.Progress, m.width)
			}
		}
	} else if m.focus == 1 {
		switch {
		case m.presentedDocumentVisible():
			message = "/ Find · y Copy · Esc " + documentBackHint(m.uiDocument.model, "Back") + " · hh Help"
		case m.activityReviewVisible():
			label, message = "Changes", "[ ] Change · / Find · Esc "+m.activityReview.Comparison.EscapeLabel()+" · hh Help"
			if m.activityReviewSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		case m.activityDetailActive() && m.searches[1] == nil:
			message = "d View changes · / Find · Esc Activity · hh Help"
		case m.mode == 2 && m.detail != "" && m.operationDetail != nil && m.searches[1] == nil:
			message = "↑↓ Scroll · / Find · Esc Activity · hh Help"
			if m.activityHasObservations() {
				message = "O Observations · " + message
			}
			if m.activityReleaseUnavailable(m.operationDetail) == "" {
				message = "Enter Open release · " + message
			}
		case m.valueDocumentActive():
			message = "/ Find · y Copy · a Actions · Esc " + documentBackHint(m.explore.Action.ValueDocument, "Hide value") + " · hh Help"
			if m.explore.Action.ValueDocument.Searching {
				message = "Enter Finish search · Esc Cancel search"
			}
		case m.yamlDocumentActive():
			back := documentBackHint(m.syncYAMLDocument().Document, strings.TrimPrefix(m.resourceBackLabel(), "Esc "))
			message = "/ Find · y Copy · E Edit · Esc " + back + " · hh Help"
			if m.yamlDocumentSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		}
	}
	if m.documentActive() && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil {
		label, message = "Editor", m.documentHint()
		a := m.explore.Action
		if a.Stage == "review" {
			label = "Review changes"
		} else if a.Document != nil {
			label += " " + string(a.Document.Mode)
		}
	}
	if m.resourceTerminalVisible() {
		label, message = "Exec", "Alt+←/→ Panels · Ctrl+] Review output · Ctrl+C Exit Plous"
		if !m.explore.Terminal.Captured {
			label = "Exec review"
			message = "r Resume input · End Latest · / Find · Esc Close session · hh Help"
			if m.explore.Terminal.Done {
				message = "e Edit command · R Retry · End Latest · Esc Close session · hh Help"
			}
			if m.explore.Terminal.Review != nil {
				reader := &m.explore.Terminal.Review.Viewer.Reader
				message = strings.Replace(message, "Esc Close session", "Esc "+reader.EscapeLabel("Close session"), 1)
				if reader.Searching {
					message = "Enter Finish search · Esc Cancel search"
				} else if reader.Selecting {
					message = "↑↓←→ Select text · y Copy · Esc Clear selection · hh Help"
				}
			}
		}
	}
	if m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil && !m.commandFocused() {
		if !m.mainViewLocalLayer() {
			controls := strings.Split(message, " · ")
			if m.viewExpanded() {
				hasBack := false
				for i, control := range controls {
					if strings.HasPrefix(control, "Esc ") {
						controls[i], hasBack = "Esc Restore layout", true
					}
				}
				if !hasBack {
					controls = append(controls, "Esc Restore layout")
				}
			}
			// Text components show fullscreen in their action bars. Other main views
			// need a footer hint, without repeating the expanded Escape action.
			if source, _, _, _ := m.expandableView(); source == nil && !m.viewExpanded() && !strings.Contains(message, "ff Fullscreen") {
				index := min(2, len(controls))
				controls = append(controls[:index], append([]string{":f " + m.expandAction().Label}, controls[index:]...)...)
			}
			message = strings.Join(controls, " · ")
		}
	}
	if m.chatSourcesVisible() && !m.helpVisible() {
		label, message = "Sources", m.chatSourcesHint()
		if m.chatSources.ShowReply {
			label = "Read reply"
		}
	}
	if m.commandFocused() {
		label, message = "Command", "Enter Run · Esc Cancel"
	}
	if m.selectedInput() != nil {
		controls := strings.Split(message, " · ")
		for i, control := range controls {
			if strings.HasPrefix(control, "Esc ") {
				controls[i] = "Esc Clear selection"
			}
		}
		message = strings.Join(controls, " · ")
	}
	selected := m.clipboardSurface().hasSelection()
	if selected {
		message = "[ Copy selection ] · " + message
	}
	if m.shortcutPrefix != "" {
		message = m.hint()
	} else if !selected && m.currentRoot && m.currentNotice != "" && m.status == "" && !m.nativeAgentCaptured() && !(m.resourceTerminalVisible() && m.explore.Terminal.Captured) {
		return renderFooter(label, ui.OneLine(m.currentNotice), m.width, true)
	} else if m.status != "" && !m.nativeAgentCaptured() && !(m.resourceTerminalVisible() && m.explore.Terminal.Captured) {
		return renderFooter(label, ui.OneLine(m.status), m.width, true)
	}
	return renderFooter(label, message, m.width, false)
}

func renderFooter(label, message string, width int, notice bool) string {
	prefix := ui.Selected.Render(" "+ui.Clipped(ui.OneLine(label), min(20, max(0, width/4)))) + "  "
	available := max(0, width-ansi.StringWidth(prefix))
	if notice {
		return lipgloss.NewStyle().Width(width).Render(prefix + ui.Muted.Render(ui.Clipped(message, available)))
	}
	var controls []string
	back, help := "", ""
	for _, control := range strings.Split(message, " · ") {
		switch {
		case strings.HasPrefix(control, "Esc ") || control == "bb Show connections":
			back = control
		case control == "hh Help" || control == ":h help" || control == ":h Help":
			help = control
		default:
			controls = append(controls, control)
		}
	}
	var tail []string
	if back != "" {
		tail = append(tail, back)
	}
	if help != "" {
		tail = append(tail, help)
	}
	var shown []string
	limit := 5
	if label == "Overview" {
		limit = 9
	}
	for _, control := range controls {
		candidate := append(append(append([]string{}, shown...), control), tail...)
		if len(shown)+len(tail) < limit && ansi.StringWidth(strings.Join(candidate, " · ")) <= available {
			shown = append(shown, control)
		}
	}
	shown = append(shown, tail...)
	for i, control := range shown {
		if control == "[ Copy selection ]" {
			shown[i] = textview.ActionChip(textview.Action{Label: "Copy selection", Mouse: "clipboard/copy"}, available, ui.WorkspaceTheme())
			continue
		}
		key, action, _ := strings.Cut(control, " ")
		shown[i] = ui.Selected.Render(key) + ui.Muted.Render(" "+action)
		if ui.MouseFooterKey(key) {
			shown[i] = ui.MouseAction(shown[i], key)
		}
	}
	return lipgloss.NewStyle().Width(width).Render(prefix + ui.Clipped(strings.Join(shown, ui.Muted.Render(" · ")), available))
}

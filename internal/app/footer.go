package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/editor"
)

func (m *model) footerContext() string {
	switch m.focus {
	case 0:
		return "Workspaces"
	case 2:
		return "AI assistant"
	}
	if m.exploreVisible() && m.explore != nil && m.explore.resource.Name != "" {
		return resourceTitle(m.explore.resource)
	}
	return modes[m.mode]
}

func (m *model) sidebarHint() string {
	if m.leftHidden {
		return "bb Show workspaces"
	}
	return "Esc Workspaces"
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
	if m.help != nil {
		label = "Help"
	} else if m.settingsOverlay != nil && m.focus == 1 {
		label = "Settings"
		if m.toolSetup != nil {
			label = "Add tool"
			if m.toolSetup.contextsOnly {
				label = "Add context"
			}
		}
	} else if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		label, message = "Actions", "↑↓ Choose · Enter Select · Esc Back · hh Help"
		a := m.explore.action
		if a.busy {
			message = "Esc Cancel request"
		} else if a.stage == "scale" || a.stage == "export" || a.stage == "execcommand" && a.execEditing {
			message = "Enter Continue · Esc Back"
		} else if a.stage == "confirm" {
			message = "←→ Choose · Enter Confirm · Esc Back · hh Help"
		} else if a.stage == "done" {
			message = "Esc Return to resource · hh Help"
			if a.progress != nil {
				message = operationProgressFooter(a.progress, m.width)
			}
		}
	} else if m.focus == 1 {
		switch {
		case m.activityReviewVisible():
			label, message = "Changes", "[ ] Change · / Find · Esc "+m.activityReview.comparison.EscapeLabel()+" · hh Help"
			if m.activityReviewSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		case m.activityDetailActive() && m.searches[1] == nil:
			message = "d View changes · / Find · Esc Activity · hh Help"
		case m.valueDocumentActive():
			message = "/ Find · y Copy · a Actions · Esc " + documentBackHint(m.explore.action.valueDocument, "Hide value") + " · hh Help"
			if m.explore.action.valueDocument.Searching {
				message = "Enter Finish search · Esc Cancel search"
			}
		case m.yamlDocumentActive():
			back := documentBackHint(m.syncYAMLDocument().document, strings.TrimPrefix(m.resourceBackLabel(), "Esc "))
			message = "/ Find · y Copy · E Edit · Esc " + back + " · hh Help"
			if m.yamlDocumentSearching() {
				message = "Enter Finish search · ↑↓ Match · Esc Cancel search"
			}
		}
	}
	if m.documentActive() && m.focus == 1 && m.help == nil && m.settingsOverlay == nil {
		label, message = "Editor", m.documentHint()
		a := m.explore.action
		if a.stage == "review" {
			label = "Review changes"
		} else if a.document != nil {
			label += " " + string(a.document.Mode)
		}
	}
	if m.resourceTerminalVisible() {
		label, message = "Exec", "Ctrl+] Review output · Ctrl+C Exit Plous"
		if !m.explore.terminal.captured {
			label = "Exec review"
			message = "r Resume input · End Latest · / Find · Esc Close session · hh Help"
			if m.explore.terminal.done {
				message = "e Edit command · R Retry · End Latest · Esc Close session · hh Help"
			}
			if m.explore.terminal.review != nil {
				reader := &m.explore.terminal.review.viewer.Reader
				message = strings.Replace(message, "Esc Close session", "Esc "+reader.EscapeLabel("Close session"), 1)
				if reader.Searching {
					message = "Enter Finish search · Esc Cancel search"
				} else if reader.Selecting {
					message = "↑↓←→ Select text · y Copy · Esc Clear selection · hh Help"
				}
			}
		}
	}
	if m.viewExpanded() && m.command == nil {
		_, _, _, local := m.expandableView()
		if !local {
			controls := strings.Split(message, " · ")
			for i, control := range controls {
				if strings.HasPrefix(control, "Esc ") {
					controls[i] = "Esc Restore layout"
				}
			}
			message = strings.Join(controls, " · ")
		}
	}
	if m.command != nil && m.focus == 1 {
		label, message = "Command", "Enter Run · Esc Cancel"
	}
	if m.shortcutPrefix != "" {
		message = m.hint()
	} else if m.status != "" && !(m.resourceTerminalVisible() && m.explore.terminal.captured) {
		return renderFooter(label, oneLine(m.status), m.width, true)
	}
	return renderFooter(label, message, m.width, false)
}

func renderFooter(label, message string, width int, notice bool) string {
	prefix := selected.Render(" "+clipped(oneLine(label), min(20, max(0, width/4)))) + "  "
	available := max(0, width-ansi.StringWidth(prefix))
	if notice {
		return lipgloss.NewStyle().Width(width).Render(prefix + muted.Render(clipped(message, available)))
	}
	var controls []string
	back, help := "", ""
	for _, control := range strings.Split(message, " · ") {
		switch {
		case strings.HasPrefix(control, "Esc "):
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
	for _, control := range controls {
		candidate := append(append(append([]string{}, shown...), control), tail...)
		if len(shown)+len(tail) < 5 && ansi.StringWidth(strings.Join(candidate, " · ")) <= available {
			shown = append(shown, control)
		}
	}
	shown = append(shown, tail...)
	for i, control := range shown {
		key, action, _ := strings.Cut(control, " ")
		shown[i] = selected.Render(key) + muted.Render(" "+action)
	}
	return lipgloss.NewStyle().Width(width).Render(prefix + clipped(strings.Join(shown, muted.Render(" · ")), available))
}

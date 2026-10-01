package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/textview"
)

func (m *model) updateExecLaunch(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.action
	key := msg.String()
	if key == "esc" {
		if a.execEditing {
			a.execEditing = false
			return nil
		}
		if a.previousTerminal != nil {
			m.explore.terminal = a.previousTerminal
			a.previousTerminal = nil
			m.stopResourceAction()
		} else {
			a.stage, a.cursor = "containers", 0
		}
		return nil
	}
	if a.execEditing {
		if key == "enter" {
			a.execEditing = false
			a.execChoice = 3
		} else {
			a.input.key(msg, false)
		}
		return nil
	}
	switch key {
	case "up", "k":
		a.execChoice = max(0, a.execChoice-1)
	case "down", "j":
		a.execChoice = min(4, a.execChoice+1)
	case "enter":
		switch a.execChoice {
		case 0:
			a.input = newEditor("/bin/sh")
			a.execChoice = 3
		case 1:
			a.input = newEditor("/bin/bash")
			a.execChoice = 3
		case 2:
			a.execEditing = true
		case 3:
			return m.startResourceTerminal(a)
		case 4:
			return m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEscape})
		}
	}
	return nil
}
func (m *model) execLaunchBody(width, height int) string {
	a := m.explore.action
	title := titleStyle.Render(clipped("Exec · "+a.pod.GetName()+" / "+a.container, width))
	choices := []string{"sh (/bin/sh)", "bash (/bin/bash)", "Custom command", "Start", "Cancel"}
	buttons := []textview.Action{}
	for i, label := range choices {
		button := textview.Action{Label: label, Focused: !a.execEditing && i == a.execChoice, Primary: i == 3}
		if button.Focused {
			button.Key = "Enter"
		}
		buttons = append(buttons, button)
	}
	fieldBorder := borderColor
	if a.execEditing {
		fieldBorder = focusedBorder
	}
	field := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(fieldBorder).Width(width).MaxWidth(width).Render(muted.Render("Command") + "\n" + clipped(a.input.display(a.execEditing), width-2))
	choicesView := textview.ActionBar([]textview.ActionGroup{{Label: "Shell", Actions: buttons[:3]}}, width, 3, workspaceTheme())
	startView := textview.ActionBar([]textview.ActionGroup{{Label: "Session", Actions: buttons[3:]}}, width, 2, workspaceTheme())
	scope := muted.Render(clipped(a.context+" · "+a.pod.GetNamespace(), width))
	rows := []string{title, scope, choicesView, field, startView}
	if a.err != "" {
		rows = append(rows, changedStyle.Render(clipped(a.err, width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, muted.Render(clipped("Shell availability depends on the image.", width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, muted.Render(clipped("Ctrl+] review · Ctrl+C exits Plous", width)))
	}
	return textview.Fit(strings.Join(rows, "\n"), width, height)
}

func (m *model) renderExecLaunchOverlay(base string) string {
	width, height := min(110, m.width-6), min(18, m.height-2)
	innerWidth, bodyHeight := width-4, height-3
	body := m.execLaunchBody(innerWidth, bodyHeight)
	footer := "↑↓ choose · Enter select/start · Esc back"
	if m.explore.action.execEditing {
		footer = "Enter finish command · Esc leave field"
	}
	content := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body) + "\n" + muted.Render(clipped(footer, innerWidth))
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(surface).Foreground(foreground).Padding(0, 1).Width(width).Height(height).MaxHeight(height).Render(content)
	return centeredOverlay(base, window)
}

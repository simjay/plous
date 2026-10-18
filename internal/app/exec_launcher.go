package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
	"strings"
)

func (m *model) updateExecLaunch(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	key := msg.String()
	if key == "esc" {
		if a.ExecEditing {
			a.ExecEditing = false
			return nil
		}
		if a.PreviousTerminal != nil {
			m.explore.Terminal = a.PreviousTerminal
			a.PreviousTerminal = nil
			m.stopResourceAction()
		} else {
			a.Stage, a.Cursor = "containers", 0
		}
		return nil
	}
	if a.ExecEditing {
		if key == "enter" {
			a.ExecEditing = false
			a.ExecChoice = 3
		} else {
			a.Input.Key(msg, false)
		}
		return nil
	}
	switch key {
	case "up", "k":
		a.ExecChoice = max(0, a.ExecChoice-1)
	case "down", "j":
		a.ExecChoice = min(4, a.ExecChoice+1)
	case "enter":
		switch a.ExecChoice {
		case 0:
			a.Input = textview.NewField("/bin/sh")
			a.ExecChoice = 3
		case 1:
			a.Input = textview.NewField("/bin/bash")
			a.ExecChoice = 3
		case 2:
			a.ExecEditing = true
		case 3:
			return m.startResourceTerminal(a)
		case 4:
			return m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEscape})
		}
	}
	return nil
}

func (m *model) execLaunchBody(width, height int) string {
	a := m.explore.Action
	title := ui.TitleStyle.Render(ui.Clipped("Exec · "+a.Pod.GetName()+" / "+a.Container, width))
	choices := []string{"sh (/bin/sh)", "bash (/bin/bash)", "Custom command", "Start", "Cancel"}
	buttons := []textview.Action{}
	for i, label := range choices {
		button := textview.Action{Mouse: "execaction/" + strconv.Itoa(i), Label: label, Focused: !a.ExecEditing && i == a.ExecChoice, Primary: i == 3}
		if button.Focused {
			button.Key = "Enter"
		}
		buttons = append(buttons, button)
	}
	fieldBorder := ui.BorderColor
	if a.ExecEditing {
		fieldBorder = ui.FocusedBorder
	}
	field := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(fieldBorder).Width(width).MaxWidth(width).Render(ui.Muted.Render("Command") + "\n" + ui.EditorLine(a.Input, width-2, a.ExecEditing, "execfield"))
	choicesView := textview.ActionBar([]textview.ActionGroup{{Label: "Shell", Actions: buttons[:3]}}, width, 3, ui.WorkspaceTheme())
	startView := textview.ActionBar([]textview.ActionGroup{{Label: "Session", Actions: buttons[3:]}}, width, 2, ui.WorkspaceTheme())
	scope := ui.Muted.Render(ui.Clipped(a.Context+" · "+a.Pod.GetNamespace(), width))
	rows := []string{title, scope, choicesView, ui.MouseTarget(field, "execfield"), startView}
	if a.Err != "" {
		rows = append(rows, ui.ChangedStyle.Render(ui.Clipped(a.Err, width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, ui.Muted.Render(ui.Clipped("Shell availability depends on the image.", width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, ui.Muted.Render(ui.Clipped("Ctrl+] review · Ctrl+C exits Plous", width)))
	}
	return textview.Fit(strings.Join(rows, "\n"), width, height)
}

func (m *model) renderExecLaunchOverlay(base string) string {
	width, height := min(110, m.width-6), min(18, m.height-2)
	innerWidth, bodyHeight := width-4, height-3
	body := m.execLaunchBody(innerWidth, bodyHeight)
	footer := "↑↓ choose · Enter select/start · Esc back"
	if m.explore.Action.ExecEditing {
		footer = "Enter finish command · Esc leave field"
	}
	content := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body) + "\n" + ui.Muted.Render(ui.Clipped(footer, innerWidth))
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Accent).Background(ui.Surface).Foreground(ui.Foreground).Padding(0, 1).Width(width).Height(height).MaxHeight(height).Render(content)
	return ui.CenteredOverlay(base, window)
}

package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/ui"
)

const monitorsPage = "monitors"

type monitorStatusMsg struct {
	states []monitor.State
	err    error
}

func monitorTick(root string, delay time.Duration) tea.Cmd {
	if root == "" {
		return nil
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		states, err := monitor.List(root)
		return monitorStatusMsg{states, err}
	})
}

func (m *model) monitorHeader() string {
	open := 0
	running := false
	incomplete := m.monitorListErr
	for _, state := range m.monitorStates {
		if state.Running {
			running = true
			open += state.OpenAlerts()
			incomplete = incomplete || len(state.Coverage) > 0
		}
	}
	if !running && !incomplete {
		return ""
	}
	label := openAlertCount(open)
	style := ui.Muted
	if incomplete {
		label += " · coverage incomplete"
	}
	if open > 0 || incomplete {
		style = ui.TitleStyle
	}
	return ui.MouseTarget(style.Render(label), "monitors") + "  "
}

func openAlertCount(count int) string {
	if count == 1 {
		return "1 open alert"
	}
	return fmt.Sprintf("%d open alerts", count)
}

func (m *model) runningMonitors() []monitor.State {
	var running []monitor.State
	for _, state := range m.monitorStates {
		if state.Running {
			running = append(running, state)
		}
	}
	return running
}

func (m *model) monitorIndex(running []monitor.State) int {
	return max(0, slices.IndexFunc(running, func(state monitor.State) bool {
		return state.Name == m.monitorSelection
	}))
}

func (m *model) monitorsBody(width, height int) string {
	lines := []string{ui.Heading("Monitors")}
	if m.monitorListErr {
		lines = append(lines, "Some monitor records are unavailable.")
	}
	running := m.runningMonitors()
	if len(running) == 0 {
		lines = append(lines, "No running monitors.")
		return ui.StyledView(strings.Join(lines, "\n"), width, height)
	}
	selected := m.monitorIndex(running)
	compact := width < 74
	now := time.Now()
	if compact {
		state := running[selected]
		scope := state.Context + "/" + state.Namespace
		last := "No complete check"
		if !state.LastOK.IsZero() {
			last = "Complete " + state.LastOK.Local().Format("01/02 15:04")
		}
		for _, line := range []string{scope, fmt.Sprintf("Every %s · %d open", monitorInterval(state.Interval), state.OpenAlerts()), last} {
			lines = append(lines, ui.Muted.Render(ui.Clipped(ui.OneLine(line), width)))
		}
		lines = append(lines, monitorScheduleLine(state, now, width, true))
	}
	list := ui.CollectionWindow(len(running), selected, height-len(lines), func(i int) []string {
		state := running[i]
		label := state.Name
		target := "monitor/" + state.Name
		if compact {
			if i != selected && state.Overdue(now) {
				label = "Overdue " + label
			}
			return []string{ui.MouseTarget(ui.ChoiceLine(label, i == selected, width), target)}
		}
		last := "No complete check"
		if !state.LastOK.IsZero() {
			last = "Last complete check " + state.LastOK.Local().Format("Jan 02 15:04")
		}
		label = strings.Join([]string{label, openAlertCount(state.OpenAlerts()), state.Context + "/" + state.Namespace, "every " + monitorInterval(state.Interval), last}, " · ")
		return []string{
			ui.MouseTarget(ui.ChoiceLine(label, i == selected, width), target),
			ui.MouseTarget(monitorScheduleLine(state, now, width, false), target),
		}
	})
	return strings.Join(lines, "\n") + "\n" + list
}

func monitorScheduleLine(state monitor.State, now time.Time, width int, compact bool) string {
	style, label := ui.Muted, "Next check unknown"
	if !state.NextCheck.IsZero() {
		format := "Jan 02 15:04:05"
		if compact {
			format = "15:04:05"
		}
		when := state.NextCheck.Local().Format(format)
		switch {
		case state.Overdue(now):
			style, label = ui.TitleStyle, "Overdue "+when
		case !now.Before(state.NextCheck):
			label = "Check due " + when
		default:
			label = "Next check " + when
		}
	}
	if !compact {
		label = "  " + label
	}
	return style.Render(ui.Clipped(label, width))
}

func monitorInterval(interval time.Duration) string {
	if interval%time.Minute == 0 {
		return fmt.Sprintf("%dm", interval/time.Minute)
	}
	return interval.String()
}

func (m *model) canOpenMonitors() bool {
	if m.settingsOverlay != nil || m.form != nil || m.removeForm != nil || m.presentedDocumentVisible() || m.documentActive() || m.valueDocumentActive() {
		m.status = "Close the active form or document first."
		return false
	}
	return true
}

func (m *model) openMonitors() {
	if m.canOpenMonitors() {
		m.closeHelp()
		m.page, m.focus, m.status = monitorsPage, 1, ""
	}
}

func (m *model) openAlerts(name string) {
	if !m.canOpenMonitors() {
		return
	}
	m.openMode(2)
	if s := m.searches[1]; s != nil && s.kind == "activity" {
		m.restoreActivityPosition(s)
		m.searches[1] = nil
	}
	m.alertMonitor = name
	m.activityUI.Filter = activity.Alerts
	m.setActivityPosition(0)
}

func (m *model) updateMonitors(key string) {
	running := m.runningMonitors()
	index := m.monitorIndex(running)
	switch key {
	case "esc":
		m.page, m.status = "", ""
		return
	case "up":
		index--
	case "down":
		index++
	case "pgup":
		index -= 10
	case "pgdown":
		index += 10
	case "home":
		index = 0
	case "end":
		index = len(running) - 1
	case "enter":
		if len(running) > 0 {
			m.openAlerts(running[index].Name)
		}
		return
	default:
		return
	}
	if len(running) > 0 {
		m.monitorSelection = running[max(0, min(index, len(running)-1))].Name
	}
}

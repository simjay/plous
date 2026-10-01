package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/logview"
	"github.com/simjay/plous/internal/textview"
	"strings"
)

func (m *model) logWorkspaceActive() bool {
	return m.exploreVisible() && m.explore.detail != nil && m.explore.tab == 4 && m.explore.picker == "" && m.explore.action == nil && m.explore.terminal == nil
}
func (m *model) podLogViewer() *logview.Model {
	e := m.explorer()
	if e.logViewer == nil {
		e.logViewer = logview.New(exploreLogBufferLimit, 5000)
		e.logViewer.SetFormatter(podLogFormatter(e.logTimestamps, e.mergedLogs))
	}
	return e.logViewer
}

func (m *model) logWorkspaceKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.logWorkspaceActive() {
		return false, nil
	}
	v := m.podLogViewer()
	key := msg.String()
	if !v.Reader.Searching {
		if key == ":" || key == "tab" || key == "shift+tab" {
			return false, nil
		}
		if m.updateShortcut(msg) {
			m.persist()
			return true, nil
		}
	}
	m.shortcutPrefix = ""
	if !v.Reader.Searching && key == "z" {
		m.viewCommand("zoom")
		return true, nil
	}
	handled, copied := v.Key(key, msg.Text)
	if !handled {
		return false, nil
	}
	return true, m.copyWorkspaceText(copied)
}
func (m *model) podLogHeader(width, height int) logview.Header {
	e := m.explorer()
	state := "Connecting"
	failed := e.logError != "" || e.detailError != "" || !e.detailValid && e.detailRun == nil && e.detailText != ""
	switch {
	case failed:
		state = "Disconnected"
	case e.log != nil && e.logConnected:
		state = "Live"
		if e.previous {
			state = "Loading"
		}
	case e.log == nil && e.logKey != "":
		state = "Ended"
	}
	if e.mergedLogs && !failed {
		status := e.workloadLogStatus
		switch {
		case status.Active > 0:
			state = "Live"
		case status.Finished > 0:
			state = "Watching Pods"
			if e.resource.Key() == "pods" {
				state = "Watching containers"
			}
			if e.previous {
				state = "Ended"
			}
		default:
			state = "Waiting"
		}
		if status.Failed > 0 || status.MembershipError != "" {
			state += " · Partial failure"
		}
	}
	if failed {
		state += " · Error below (End)"
	}
	if !m.podLogViewer().Following {
		state += " · Reading"
	} else {
		state += " · Following"
	}
	source := "Current instance"
	if e.previous {
		source = "Previous instance"
	}
	if width < 48 {
		source = strings.TrimSuffix(source, " instance")
	}
	rangeLabel := "Latest 200 lines"
	for _, entry := range logRanges {
		if entry.seconds == e.logSince {
			rangeLabel = entry.label
		}
	}
	follow := "Pause"
	if !m.podLogViewer().Following {
		follow = "Jump live"
	}
	// Keep the action width stable so pausing cannot rewrap the toolbar.
	follow += strings.Repeat(" ", len("Jump live")-len(follow))
	value := func(enabled bool) string {
		if enabled {
			return "On"
		}
		return "Off"
	}
	expand := m.expandAction()
	expand.Key = "z"
	actions := []textview.ActionGroup{
		{Label: "Stream", Actions: []textview.Action{{Key: "Space", Label: follow}, {Key: "g", Label: "Reload logs"}}},
		{Label: "Source", Actions: []textview.Action{{Key: "o", Label: "Container"}, {Key: "u", Label: "Time range"}, {Key: "p", Label: "Previous instance", Value: value(e.previous)}}},
		{Label: "Display", Actions: []textview.Action{expand, {Key: "t", Label: "Timestamps", Value: value(e.logTimestamps)}, {Key: "w", Label: "Wrap lines", Value: value(m.podLogViewer().Reader.Wrap)}}},
	}
	if width < 64 {
		actions[2].Actions = actions[2].Actions[1:]
		actions[0].Actions = append([]textview.Action{expand}, actions[0].Actions...)
	}
	title := "Plous Log Viewer · Logs"
	if width < 64 {
		title = "Logs · " + e.container
	}
	target := e.detail.GetNamespace() + " / " + e.detail.GetName() + " / " + e.container
	note := source + " · " + rangeLabel
	if e.mergedLogs {
		container := e.container
		if container == "" {
			container = "application containers"
		}
		scope := "All Pods"
		if e.resource.Key() == "pods" {
			scope = "All containers"
			target = e.detail.GetNamespace() + " / " + e.detail.GetName() + " / " + allApplicationContainers
		} else {
			target = e.detail.GetNamespace() + " / " + e.detail.GetName() + " / All Pods · " + container
		}
		if width < 64 {
			title = "Logs · " + scope
		}
		status := e.workloadLogStatus
		note += fmt.Sprintf("\n%d Pods · %d sources", status.Pods, status.Sources)
		note += "\n" + ansi.Wrap(fmt.Sprintf("%d failed · %d waiting · %d live · %d ended", status.Failed, status.Waiting, status.Active, status.Finished), max(1, width), "")
		if status.Skipped > 0 {
			note += fmt.Sprintf("\nPartial coverage: %d sources omitted (limit %d). Choose a Pod or container.", status.Skipped, kubernetes.MaxWorkloadLogSources)
		}
		if width < 48 || height < 22 {
			title = "Logs · " + scope
			position := "Following"
			if !m.podLogViewer().Following {
				position = "Reading"
			}
			state = fmt.Sprintf("%d live · %s", status.Active, position)
			if failed {
				state = "Disconnected · " + position
			}
			if e.previous {
				state = "Previous · " + position
			}
			rangeText := rangeLabel
			if e.logSince == 0 {
				rangeText = "200/source"
			}
			note = source + " · " + rangeText
			note += fmt.Sprintf("\n%d Pods · %d sources", status.Pods, status.Sources)
			note += fmt.Sprintf("\n%d failed · %d omitted", status.Failed, status.Skipped)
			if status.MembershipError != "" {
				state = "Membership unavailable"
			}
		}
	}
	return logview.Header{Title: title, Target: target, State: state, Note: note, Actions: actions}
}
func (m *model) exploreLogBody(width, height int) string {
	e := m.explorer()
	v := m.podLogViewer()
	message := strings.TrimSpace(e.detailError + "\n" + e.logError + "\n" + e.workloadLogStatus.MembershipError)
	if message == "" && !e.detailValid && e.detailRun == nil {
		message = e.detailText
	}
	if message != "" {
		recovery := "g Reload logs after resolving the problem."
		if e.previous {
			recovery += "\np Return to current instance logs."
		}
		message = recovery + "\no Choose another container.\nEsc Return to the resource.\n\n" + message
	}
	if v.Text == "" && message == "" {
		switch {
		case e.detailRun != nil:
			message = m.loading("Loading container…")
		case e.mergedLogs:
			message = "Waiting for matching Pod containers. Membership updates every 3 seconds."
		case e.container == "":
			message = "No containers available."
		case e.log != nil && !e.logConnected:
			message = m.loading("Connecting to logs…")
		case e.log != nil:
			message = "Waiting for output. New lines will appear here."
		default:
			message = "No logs returned for this source and time range."
		}
	}
	v.Message = message
	v.Refresh()
	return v.View(m.podLogHeader(width, height), width, height, workspaceTheme())
}

package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) logWorkspaceActive() bool {
	return m.explore != nil && m.exploreVisible() && m.explore.Detail != nil && m.explore.Tab == 4 && m.explore.Picker == "" && m.explore.Action == nil && m.explore.Terminal == nil
}

func (m *model) podLogViewer() *logview.Model {
	e := m.explorer()
	if e.LogViewer == nil {
		e.LogViewer = logview.New(monitor.ExploreLogBufferLimit, 5000)
		e.LogViewer.SetFormatter(monitor.PodLogFormatter(e.LogTimestamps, e.MergedLogs))
	}
	return e.LogViewer
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
	handled, copied := v.Key(key, msg.Text)
	if !handled {
		return false, nil
	}
	return true, m.copyWorkspaceText(copied)
}

func (m *model) podLogHeader(width, height int) logview.Header {
	e := m.explorer()
	state := "Connecting"
	failed := e.LogError != "" || e.DetailError != "" || !e.DetailValid && e.DetailRun == nil && e.DetailText != ""
	switch {
	case failed:
		state = "Disconnected"
	case e.Log != nil && e.LogConnected:
		state = "Live"
		if e.Previous {
			state = "Loading"
		}
	case e.Log == nil && e.LogKey != "":
		state = "Ended"
	}
	if e.MergedLogs && !failed {
		status := e.WorkloadLogStatus
		switch {
		case status.Active > 0:
			state = "Live"
		case status.Finished > 0:
			state = "Watching Pods"
			if e.Resource.Key() == "pods" {
				state = "Watching containers"
			}
			if e.Previous {
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
	if e.Previous {
		source = "Previous instance"
	}
	if width < 48 {
		source = strings.TrimSuffix(source, " instance")
	}
	rangeLabel := "Latest 200 lines"
	for _, entry := range monitor.LogRanges {
		if entry.Seconds == e.LogSince {
			rangeLabel = entry.Label
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
	expand.Key = "ff"
	actions := []textview.ActionGroup{
		{Label: "Stream", Actions: []textview.Action{{Key: "Space", Label: follow}, {Key: "g", Label: "Reload logs"}}},
		{Label: "Source", Actions: []textview.Action{{Key: "o", Label: "Container"}, {Key: "u", Label: "Time range"}, {Key: "p", Label: "Previous instance", Value: value(e.Previous)}}},
		{Label: "Display", Actions: []textview.Action{expand, {Key: "t", Label: "Timestamps", Value: value(e.LogTimestamps)}, {Key: "w", Label: "Wrap lines", Value: value(m.podLogViewer().Reader.Wrap)}}},
	}
	if width < 64 {
		actions[2].Actions = actions[2].Actions[1:]
		actions[0].Actions = append([]textview.Action{expand}, actions[0].Actions...)
	}
	title := "Plous Log Viewer · Logs"
	if width < 64 {
		title = "Logs · " + e.Container
	}
	target := e.Detail.GetNamespace() + " / " + e.Detail.GetName() + " / " + e.Container
	note := source + " · " + rangeLabel
	if e.MergedLogs {
		container := e.Container
		if container == "" {
			container = "application containers"
		}
		scope := "All Pods"
		if e.Resource.Key() == "pods" {
			scope = "All containers"
			target = e.Detail.GetNamespace() + " / " + e.Detail.GetName() + " / " + monitor.AllApplicationContainers
		} else {
			target = e.Detail.GetNamespace() + " / " + e.Detail.GetName() + " / All Pods · " + container
		}
		if width < 64 {
			title = "Logs · " + scope
		}
		status := e.WorkloadLogStatus
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
			if e.Previous {
				state = "Previous · " + position
			}
			rangeText := rangeLabel
			if e.LogSince == 0 {
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
	message := strings.TrimSpace(e.DetailError + "\n" + e.LogError + "\n" + e.WorkloadLogStatus.MembershipError)
	if message == "" && !e.DetailValid && e.DetailRun == nil {
		message = e.DetailText
	}
	if message != "" {
		recovery := "g Reload logs after resolving the problem."
		if e.Previous {
			recovery += "\np Return to current instance logs."
		}
		message = recovery + "\no Choose another container.\nEsc Return to the resource.\n\n" + message
	}
	if v.Text == "" && message == "" {
		switch {
		case e.DetailRun != nil:
			message = m.loading("Loading container…")
		case e.MergedLogs:
			message = "Waiting for matching Pod containers. Membership updates every 3 seconds."
		case e.Container == "":
			message = "No containers available."
		case e.Log != nil && !e.LogConnected:
			message = m.loading("Connecting to logs…")
		case e.Log != nil:
			message = "Waiting for output. New lines will appear here."
		default:
			message = "No logs returned for this source and time range."
		}
	}
	v.Message = message
	v.Refresh()
	return v.View(m.podLogHeader(width, height), width, height, ui.WorkspaceTheme())
}

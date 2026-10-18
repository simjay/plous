package app

import (
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/ui"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) terminalHeader() logview.Header {
	t := m.explore.Terminal
	state, note := t.TerminalState()
	if !t.Captured {
		if t.Done {
			state += " · Plous controls active"
		} else {
			state = "Plous controls active · " + state
		}
	}
	actions := monitor.ExecInputActions()
	if !t.Captured {
		actions = []textview.ActionGroup{{Label: "Session", Actions: []textview.Action{{Key: "r", Label: "Resume"}, {Key: "t", Label: "Interrupt"}, {Key: "x", Label: "Close"}}}}
		if t.Done {
			actions[0].Actions = []textview.Action{{Key: "e", Label: "Edit command"}, {Key: "R", Label: "Retry"}, {Key: "x", Label: "Close"}}
		}
		actions = append(actions, textview.ActionGroup{Label: "View", Actions: []textview.Action{m.expandAction()}})
	}
	return logview.Header{Title: "Exec · " + ui.OneLine(t.Pod) + " / " + ui.OneLine(t.Container), Target: "Context " + ui.OneLine(t.Context) + " · Namespace " + ui.OneLine(t.Namespace), State: state, Note: "Command " + ui.OneLine(t.Command) + " · " + note, Actions: actions}
}

func (m *model) terminalBody(width, height int) string {
	t := m.explore.Terminal
	header := m.terminalHeader()
	if t.Captured {
		return logview.HeaderView(header, width, height, ui.WorkspaceTheme()) + "\n" + t.Emulator.Render()
	}
	if t.Review == nil {
		t.BeginReview()
	}
	return t.Review.Viewer.View(header, width, height, ui.WorkspaceTheme())
}

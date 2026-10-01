package app

import (
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/logview"
	"github.com/simjay/plous/internal/textview"
)

const terminalScrollbackLimit = 5000

type terminalReview struct {
	viewer  *logview.Model
	version uint64
	alt     bool
	limited bool
	ended   bool
}

// Screen writes and snapshots share a lock because the emulator exposes
// scrollback cells by reference. Reviewing never pauses the remote process.
func (t *resourceTerminal) Write(p []byte) (int, error) {
	t.screenMu.Lock()
	defer t.screenMu.Unlock()
	n, err := t.emulator.Write(p)
	if n > 0 {
		t.outputVersion.Add(1)
	}
	return n, err
}

func (t *resourceTerminal) resize(width, height int) {
	t.screenMu.Lock()
	defer t.screenMu.Unlock()
	t.emulator.Resize(width, height)
}

func (t *resourceTerminal) beginReview() {
	t.captured = false
	t.snapshotReview()
}

func (t *resourceTerminal) snapshotReview() {
	t.screenMu.Lock()
	defer t.screenMu.Unlock()
	width, height := t.emulator.Width(), t.emulator.Height()
	review := &terminalReview{version: t.outputVersion.Load(), alt: t.emulator.IsAltScreen(), ended: t.done}
	rows := []textview.Line{}
	appendRow := func(styled string) { rows = append(rows, textview.Line{Text: ansi.Strip(styled), Styled: styled}) }
	if !review.alt {
		scrollback := t.emulator.Scrollback()
		review.limited = scrollback.Len() >= terminalScrollbackLimit
		for _, row := range scrollback.Lines() {
			appendRow(row.Render())
		}
	}
	for y := range height {
		row := make(uv.Line, width)
		for x := range width {
			if cell := t.emulator.CellAt(x, y); cell != nil {
				row[x] = *cell
			}
		}
		appendRow(row.Render())
	}
	review.viewer = logview.New(2*1024*1024, terminalScrollbackLimit+height+1000)
	review.viewer.Reader.Wrap = false
	if t.err != "" {
		review.viewer.Reader.Wrap = true
		for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1].Text) == "" {
			rows = rows[:len(rows)-1]
		}
		rows = append(rows, textview.Line{Text: "Session error", Styled: changedStyle.Render("Session error"), Metadata: true})
		for _, line := range strings.Split(plain(t.err), "\n") {
			rows = append(rows, textview.Line{Text: line, Styled: changedStyle.Render(line)})
		}
	}
	review.viewer.Snapshot(rows)
	if t.err != "" && len(review.viewer.Reader.Lines) > 0 {
		last := review.viewer.Reader.Lines[len(review.viewer.Reader.Lines)-1]
		review.viewer.Reader.Cursor.Col = utf8.RuneCountInString(last.Text)
	}
	t.review = review
}

func (t *resourceTerminal) reviewKey(key, text string) (bool, string) {
	if t.review == nil {
		t.snapshotReview()
	}
	r := &t.review.viewer.Reader
	if key == "end" && !r.Searching && !r.Selecting {
		t.snapshotReview()
		return true, ""
	}
	return r.Key(key, text)
}

func (t *resourceTerminal) terminalState() (string, string) {
	if t.captured {
		if t.outputVersion.Load() == 0 {
			return "Terminal input active", "Waiting for output"
		}
		return "Terminal input active", "Keys go to the container"
	}
	state := "Review output"
	if t.done {
		state = "Session ended · Review output"
		if t.err != "" {
			state = "Session failed · Review output"
		}
	}
	if t.review == nil {
		return state, ""
	}
	if t.done && !t.review.ended {
		return state, "Final output available · End latest"
	}
	if t.review.version != t.outputVersion.Load() {
		return state, "New output available · End latest"
	}
	if t.review.alt {
		return state, "Alternate screen snapshot"
	}
	if t.review.limited {
		return state, "Last 5,000 history lines retained"
	}
	return state, "Output held still for reading"
}

func execInputActions() []textview.ActionGroup {
	return []textview.ActionGroup{{Label: "Session", Actions: []textview.Action{{Key: "Ctrl+]", Label: "Review"}, {Key: "Ctrl+C", Label: "Exit Plous"}}}}
}

func (m *model) terminalHeader() logview.Header {
	t := m.explore.terminal
	state, note := t.terminalState()
	if !t.captured {
		if t.done {
			state += " · Plous controls active"
		} else {
			state = "Plous controls active · " + state
		}
	}
	actions := execInputActions()
	if !t.captured {
		actions = []textview.ActionGroup{{Label: "Session", Actions: []textview.Action{{Key: "r", Label: "Resume"}, {Key: "t", Label: "Interrupt"}, {Key: "x", Label: "Close"}}}}
		if t.done {
			actions[0].Actions = []textview.Action{{Key: "e", Label: "Edit command"}, {Key: "R", Label: "Retry"}, {Key: "x", Label: "Close"}}
		}
		actions = append(actions, textview.ActionGroup{Label: "View", Actions: []textview.Action{m.expandAction()}})
	}
	return logview.Header{Title: "Exec · " + oneLine(t.pod) + " / " + oneLine(t.container), Target: "Context " + oneLine(t.context) + " · Namespace " + oneLine(t.namespace), State: state, Note: "Command " + oneLine(t.command) + " · " + note, Actions: actions}
}
func (m *model) terminalBody(width, height int) string {
	t := m.explore.terminal
	header := m.terminalHeader()
	if t.captured {
		return logview.HeaderView(header, width, height, workspaceTheme()) + "\n" + t.emulator.Render()
	}
	if t.review == nil {
		t.beginReview()
	}
	return t.review.viewer.View(header, width, height, workspaceTheme())
}

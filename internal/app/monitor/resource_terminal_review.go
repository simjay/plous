package monitor

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"unicode/utf8"
)

const TerminalScrollbackLimit = 5000

type TerminalReview struct {
	Viewer  *logview.Model
	Version uint64
	Alt     bool
	Limited bool
	Ended   bool
}

// Screen writes and snapshots share a lock because the emulator exposes
// scrollback cells by reference. Reviewing never pauses the remote process.
func (t *ResourceTerminal) Write(p []byte) (int, error) {
	t.ScreenMu.Lock()
	defer t.ScreenMu.Unlock()
	n, err := t.Emulator.Write(p)
	if n > 0 {
		t.OutputVersion.Add(1)
	}
	return n, err
}

func (t *ResourceTerminal) Resize(width, height int) {
	t.ScreenMu.Lock()
	defer t.ScreenMu.Unlock()
	t.Emulator.Resize(width, height)
}

func (t *ResourceTerminal) BeginReview() {
	t.Captured = false
	t.SnapshotReview()
}

func (t *ResourceTerminal) SnapshotReview() {
	t.ScreenMu.Lock()
	defer t.ScreenMu.Unlock()
	width, height := t.Emulator.Width(), t.Emulator.Height()
	review := &TerminalReview{Version: t.OutputVersion.Load(), Alt: t.Emulator.IsAltScreen(), Ended: t.Done}
	rows := []textview.Line{}
	appendRow := func(styled string) { rows = append(rows, textview.Line{Text: ansi.Strip(styled), Styled: styled}) }
	if !review.Alt {
		scrollback := t.Emulator.Scrollback()
		review.Limited = scrollback.Len() >= TerminalScrollbackLimit
		for _, row := range scrollback.Lines() {
			appendRow(row.Render())
		}
	}
	for y := range height {
		row := make(uv.Line, width)
		for x := range width {
			if cell := t.Emulator.CellAt(x, y); cell != nil {
				row[x] = *cell
			}
		}
		appendRow(row.Render())
	}
	review.Viewer = logview.New(2*1024*1024, TerminalScrollbackLimit+height+1000)
	review.Viewer.Reader.Wrap = false
	if t.Err != "" {
		review.Viewer.Reader.Wrap = true
		for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1].Text) == "" {
			rows = rows[:len(rows)-1]
		}
		rows = append(rows, textview.Line{Text: "Session error", Styled: ui.ChangedStyle.Render("Session error"), Metadata: true})
		for _, line := range strings.Split(ui.Plain(t.Err), "\n") {
			rows = append(rows, textview.Line{Text: line, Styled: ui.ChangedStyle.Render(line)})
		}
	}
	review.Viewer.Snapshot(rows)
	if t.Err != "" && len(review.Viewer.Reader.Lines) > 0 {
		last := review.Viewer.Reader.Lines[len(review.Viewer.Reader.Lines)-1]
		review.Viewer.Reader.Cursor.Col = utf8.RuneCountInString(last.Text)
	}
	t.Review = review
}

func (t *ResourceTerminal) ReviewKey(key, text string) (bool, string) {
	if t.Review == nil {
		t.SnapshotReview()
	}
	r := &t.Review.Viewer.Reader
	if key == "end" && !r.Searching && !r.Selecting {
		t.SnapshotReview()
		return true, ""
	}
	return r.Key(key, text)
}

func (t *ResourceTerminal) TerminalState() (string, string) {
	if t.Captured {
		if t.OutputVersion.Load() == 0 {
			return "Terminal input active", "Waiting for output"
		}
		return "Terminal input active", "Keys go to the container"
	}
	state := "Review output"
	if t.Done {
		state = "Session ended · Review output"
		if t.Err != "" {
			state = "Session failed · Review output"
		}
	}
	if t.Review == nil {
		return state, ""
	}
	if t.Done && !t.Review.Ended {
		return state, "Final output available · End latest"
	}
	if t.Review.Version != t.OutputVersion.Load() {
		return state, "New output available · End latest"
	}
	if t.Review.Alt {
		return state, "Alternate screen snapshot"
	}
	if t.Review.Limited {
		return state, "Last 5,000 history lines retained"
	}
	return state, "Output held still for reading"
}

func ExecInputActions() []textview.ActionGroup {
	return []textview.ActionGroup{{Label: "Session", Actions: []textview.Action{{Key: "Ctrl+]", Label: "Review"}, {Key: "Ctrl+C", Label: "Exit Plous"}}}}
}

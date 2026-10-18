package chat

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ui/terminal"
)

// Native keeps the installed CLI screen separate from the Plous conversation view.
type Native struct {
	Input              *terminal.Input
	Kind               agent.Kind
	Scope, Label       string
	Screen             *vt.SafeEmulator
	Captured, Finished bool
	Err                string
	SaveErr            error
	Save               func() error
	Saving             bool
	CursorVisible      atomic.Bool
	HasOutput          atomic.Bool
	Cancel             context.CancelFunc
	Done               chan struct{}
	mu                 sync.Mutex
	terminal           *cli.Terminal
	once               sync.Once
	width, height      int
	scrollback         []nativeHistoryRow
	scrollTop          int
}

type nativeHistoryRow struct {
	cells uv.Line
	text  string
}

func NewNative(ctx context.Context, kind agent.Kind, scope, label string, width, height int) (*Native, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	n := &Native{Kind: kind, Scope: scope, Label: label, Screen: vt.NewSafeEmulator(width, height), Captured: true, Cancel: cancel, Done: make(chan struct{}), width: width, height: height}
	n.CursorVisible.Store(true)
	n.Screen.SetCallbacks(vt.Callbacks{CursorVisibility: func(v bool) { n.CursorVisible.Store(v) }})
	n.Screen.SetScrollbackSize(5000)
	// Drain replies before setup completes so input cannot block on startup.
	n.Input = &terminal.Input{Ctx: ctx, Chunks: make(chan []byte, 128)}
	go func() {
		if _, err := io.Copy(n.Input, n.Screen); err != nil {
			n.Close()
		}
	}()
	return n, ctx
}

func (n *Native) SetTerminal(t *cli.Terminal) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.terminal = t
	_ = t.Resize(n.width, n.height)
}

func (n *Native) Resize(width, height int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.width == width && n.height == height {
		return
	}
	n.width, n.height = width, height
	n.Screen.Resize(width, height)
	n.scrollTop = min(n.scrollTop, max(0, len(n.scrollback)-height))
	if n.terminal != nil {
		_ = n.terminal.Resize(width, height)
	}
}

func (n *Native) Close() {
	n.once.Do(func() {
		n.Cancel()
		// Close only the input pipe; Emulator.Close mutates an unguarded flag.
		if pipe, ok := n.Screen.InputPipe().(io.Closer); ok {
			_ = pipe.Close()
		}
	})
}

func (n *Native) Write(p []byte) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	count, err := n.Screen.Write(p)
	if count > 0 {
		n.HasOutput.Store(true)
	}
	return count, err
}

// Hold a screen snapshot while reading so redraws, output and history eviction
// cannot move the text under the reader. Native conversation storage stays private.
// Boundary events are handled without changing the view or reaching CLI input.
func (n *Native) Scroll(delta int) (handled, changed bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.scrollback == nil {
		if n.Screen.IsAltScreen() {
			return false, false
		}
		if delta >= 0 || n.Screen.ScrollbackLen() == 0 {
			return true, false
		}
		// vt already clones history cells on insertion. Detach the row list before
		// its buffer rotates; only live screen cells need another copy.
		history := n.Screen.Scrollback().Lines()
		n.scrollback = make([]nativeHistoryRow, len(history)+n.height)
		for i, row := range history {
			n.scrollback[i].cells = row
		}
		n.scrollTop = len(history)
		for y := range n.height {
			row := make(uv.Line, n.width)
			for x := range n.width {
				if cell := n.Screen.CellAt(x, y); cell != nil {
					row[x] = *cell
				}
			}
			n.scrollback[len(history)+y].cells = row
		}
	}
	if delta == 0 {
		return true, false
	}
	last := max(0, len(n.scrollback)-n.height)
	top := max(0, min(last, n.scrollTop+delta))
	changed = top != n.scrollTop
	n.scrollTop = top
	if n.scrollTop == last {
		n.scrollback = nil
		changed = true
	}
	return true, changed
}

func (n *Native) Reading() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.scrollback != nil
}

func (n *Native) Follow() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.scrollback, n.scrollTop = nil, 0
}

func (n *Native) Render() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.scrollback != nil {
		var out strings.Builder
		for i := n.scrollTop; i < min(len(n.scrollback), n.scrollTop+n.height); i++ {
			row := &n.scrollback[i]
			if row.text == "" {
				row.text = row.cells.Render()
			}
			if i > n.scrollTop {
				out.WriteByte('\n')
			}
			out.WriteString(row.text)
		}
		return out.String()
	}
	return n.Screen.Render()
}

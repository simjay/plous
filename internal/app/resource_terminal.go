package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/logview"
	"k8s.io/client-go/tools/remotecommand"
)

type terminalSizes struct {
	ctx   context.Context
	sizes chan remotecommand.TerminalSize
}

func (q *terminalSizes) Next() *remotecommand.TerminalSize {
	select {
	case size := <-q.sizes:
		return &size
	case <-q.ctx.Done():
		return nil
	}
}
func (q *terminalSizes) resize(width, height int) {
	value := remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
	select {
	case q.sizes <- value:
	default:
		select {
		case <-q.sizes:
		default:
		}
		select {
		case q.sizes <- value:
		default:
		}
	}
}

// Drain terminal replies before the connection completes so keyboard input and
// screen rendering cannot block on network setup or a stopped remote reader.
type terminalInput struct {
	ctx     context.Context
	chunks  chan []byte
	pending []byte
}

func (b *terminalInput) Write(p []byte) (int, error) {
	copy := append([]byte(nil), p...)
	select {
	case <-b.ctx.Done():
		return 0, io.EOF
	case b.chunks <- copy:
		return len(p), nil
	default:
		return 0, fmt.Errorf("Terminal input buffer is full.")
	}
}
func (b *terminalInput) Read(p []byte) (int, error) {
	if len(b.pending) == 0 {
		select {
		case <-b.ctx.Done():
			return 0, io.EOF
		case b.pending = <-b.chunks:
		}
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

type resourceTerminal struct {
	emulator                           *vt.SafeEmulator
	input                              *terminalInput
	sizes                              *terminalSizes
	cancel                             context.CancelFunc
	once                               sync.Once
	context, namespace, pod, container string
	captured, done                     bool
	err                                string
	cursorVisible                      atomic.Bool
	command                            string
	screenMu                           sync.Mutex
	outputVersion                      atomic.Uint64
	review                             *terminalReview
	launch                             *resourceActionState
}
type resourceTerminalResult struct {
	terminal *resourceTerminal
	err      error
}
type resourceTerminalTick struct{ terminal *resourceTerminal }

func (t *resourceTerminal) close() {
	t.once.Do(func() {
		t.cancel()
		// SafeEmulator.Close writes an unguarded flag. Closing its input pipe
		// unblocks reads and replies without racing an in-flight screen write.
		if pipe, ok := t.emulator.InputPipe().(io.Closer); ok {
			_ = pipe.Close()
		}
	})
}
func (m *model) stopResourceTerminal() {
	if m.explore != nil && m.explore.terminal != nil {
		m.explore.terminal.close()
		m.explore.terminal = nil
	}
}
func (m *model) resourceTerminalVisible() bool {
	return m.explore != nil && m.explore.terminal != nil && m.exploreVisible() && m.focus == 1 && m.help == nil && m.command == nil && m.settingsOverlay == nil
}
func (m *model) terminalDimensions() (int, int) {
	width, height := max(1, m.panelWidths()[1]-4), max(1, m.height-6)
	header := logview.Header{Title: "Exec", Note: "Command", Actions: execInputActions()}
	return width, max(1, height-lipgloss.Height(logview.HeaderView(header, width, height, workspaceTheme())))
}
func (m *model) resizeResourceTerminal() {
	if m.explore == nil || m.explore.terminal == nil {
		return
	}
	t := m.explore.terminal
	width, height := m.terminalDimensions()
	if t.emulator.Width() == width && t.emulator.Height() == height {
		return
	}
	t.resize(width, height)
	if t.sizes != nil {
		t.sizes.resize(width, height)
	}
}
func terminalTick(t *resourceTerminal) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return resourceTerminalTick{t} })
}

func (m *model) startResourceTerminal(a *resourceActionState) tea.Cmd {
	command, err := splitExecCommand(a.input.text)
	if err != nil {
		a.err = err.Error()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	width, height := m.terminalDimensions()
	t := &resourceTerminal{emulator: vt.NewSafeEmulator(width, height), cancel: cancel, captured: true, context: a.context, namespace: a.pod.GetNamespace(), pod: a.pod.GetName(), container: a.container, command: a.input.text}
	t.sizes = &terminalSizes{ctx: ctx, sizes: make(chan remotecommand.TerminalSize, 1)}
	t.sizes.resize(width, height)
	t.input = &terminalInput{ctx: ctx, chunks: make(chan []byte, 128)}
	t.emulator.SetForegroundColor(foreground)
	t.emulator.SetBackgroundColor(background)
	t.emulator.SetScrollbackSize(terminalScrollbackLimit)
	t.cursorVisible.Store(true)
	t.emulator.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { t.cursorVisible.Store(visible) }})
	a.previousTerminal = nil
	launch := *a
	launch.cancel, launch.busy, launch.pending = nil, false, ""
	launch.pod, launch.object = a.pod.DeepCopy(), a.object.DeepCopy()
	t.launch = &launch
	client, pod, container := a.client, a.pod.DeepCopy(), a.container
	m.stopResourceAction()
	m.explore.terminal = t
	return tea.Batch(terminalTick(t), func() tea.Msg {
		go func() {
			_, copyErr := io.Copy(t.input, t.emulator)
			if copyErr != nil && ctx.Err() == nil {
				t.close()
			}
		}()
		err := client.Exec(ctx, kubernetes.ExecOptions{Namespace: pod.GetNamespace(), Pod: pod.GetName(), UID: pod.GetUID(), Container: container, Command: command, Stdin: t.input, Stdout: t, Stderr: t, TTY: true, SizeQueue: t.sizes})
		t.close()
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		return resourceTerminalResult{t, err}
	})
}

func (m *model) updateResourceTerminalKey(msg tea.KeyPressMsg) tea.Cmd {
	t := m.explore.terminal
	key := msg.String()
	if t.captured {
		if key == "ctrl+]" {
			t.beginReview()
			return nil
		}
		// The emulator's legacy key encoder drops shifted printable keys.
		// Text already includes the keyboard's case and layout translation.
		if msg.Text != "" && msg.Mod&^(tea.ModShift|tea.ModCapsLock|tea.ModNumLock) == 0 {
			t.emulator.SendText(msg.Text)
		} else {
			t.emulator.SendKey(uv.KeyPressEvent(msg))
		}
		return nil
	}
	if t.review == nil {
		t.beginReview()
	}
	reader := &t.review.viewer.Reader
	if reader.Searching || reader.Selecting || key == "esc" && reader.Query != "" {
		_, copied := reader.Key(key, msg.Text)
		return m.copyWorkspaceText(copied)
	}
	switch key {
	case "esc", "x":
		m.stopResourceTerminal()
		return nil
	case "r", "t":
		if !t.done {
			if key == "t" {
				t.emulator.SendText("\x03")
			}
			t.captured, t.review = true, nil
		}
		return nil
	case "e":
		if t.done && t.launch != nil {
			a := t.launch
			a.stage, a.err, a.execEditing = "execcommand", "", true
			a.execChoice = 2
			m.explore.action = a
			// Keep the completed output until a new command is explicitly started.
			m.explore.terminal = nil
			a.previousTerminal = t
		}
		return nil
	case "R":
		if t.done && t.launch != nil {
			return m.startResourceTerminal(t.launch)
		}
		return nil
	}
	handled, copied := t.reviewKey(key, msg.Text)
	if handled {
		return m.copyWorkspaceText(copied)
	}
	return nil
}

// Parse arguments without a local shell. Metacharacters remain literal unless
// the user explicitly runs a shell with -c in the container.
func splitExecCommand(text string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range text {
		if escaped {
			if quote == '"' && !strings.ContainsRune("$`\"\\\n", r) {
				word.WriteRune('\\')
			}
			if r == '\n' {
				escaped = false
				continue
			}
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("Close the quote or complete the escaped character.")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("Enter a command to run in the container.")
	}
	return args, nil
}

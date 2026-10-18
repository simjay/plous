package app

import (
	"context"
	"errors"
	"io"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	logview "github.com/simjay/plous/internal/ui/logs"
	"github.com/simjay/plous/internal/ui/terminal"
	"k8s.io/client-go/tools/remotecommand"
)

func (m *model) stopResourceTerminal() { m.explore.StopTerminal() }

func (m *model) resourceTerminalVisible() bool {
	return m.explore != nil && m.explore.Terminal != nil && m.exploreVisible() && m.focus == 1 && !m.helpVisible() && m.command == nil && m.settingsOverlay == nil
}

func (m *model) terminalDimensions() (int, int) {
	width, height := max(1, m.panelWidths()[1]-4), max(1, m.height-6)
	header := logview.Header{Title: "Exec", Note: "Command", Actions: monitor.ExecInputActions()}
	return width, max(1, height-lipgloss.Height(logview.HeaderView(header, width, height, ui.WorkspaceTheme())))
}

func (m *model) resizeResourceTerminal() {
	if m.explore == nil || m.explore.Terminal == nil || !m.panelVisible(1) {
		return
	}
	t := m.explore.Terminal
	width, height := m.terminalDimensions()
	if t.Emulator.Width() == width && t.Emulator.Height() == height {
		return
	}
	t.Resize(width, height)
	if t.Sizes != nil {
		t.Sizes.Resize(width, height)
	}
}

func (m *model) startResourceTerminal(a *monitor.ResourceActionState) tea.Cmd {
	command, err := monitor.SplitExecCommand(a.Input.Text)
	if err != nil {
		a.Err = err.Error()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	width, height := m.terminalDimensions()
	t := &monitor.ResourceTerminal{Emulator: vt.NewSafeEmulator(width, height), Cancel: cancel, Captured: true, Context: a.Context, Namespace: a.Pod.GetNamespace(), Pod: a.Pod.GetName(), Container: a.Container, Command: a.Input.Text}
	t.Sizes = &monitor.TerminalSizes{Ctx: ctx, Sizes: make(chan remotecommand.TerminalSize, 1)}
	t.Sizes.Resize(width, height)
	t.Input = &monitor.TerminalInput{Ctx: ctx, Chunks: make(chan []byte, 128)}
	t.Emulator.SetForegroundColor(ui.Foreground)
	t.Emulator.SetBackgroundColor(ui.Background)
	t.Emulator.SetScrollbackSize(monitor.TerminalScrollbackLimit)
	t.CursorVisible.Store(true)
	t.Emulator.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { t.CursorVisible.Store(visible) }})
	a.PreviousTerminal = nil
	launch := *a
	launch.Cancel, launch.Busy, launch.Pending = nil, false, ""
	launch.Pod, launch.Object = a.Pod.DeepCopy(), a.Object.DeepCopy()
	t.Launch = &launch
	client, pod, container := a.Client, a.Pod.DeepCopy(), a.Container
	m.stopResourceAction()
	m.explore.Terminal = t
	return tea.Batch(monitor.TerminalTick(t), func() tea.Msg {
		go func() {
			_, copyErr := io.Copy(t.Input, t.Emulator)
			if copyErr != nil && ctx.Err() == nil {
				t.Close()
			}
		}()
		err := client.Exec(ctx, kubernetes.ExecOptions{Namespace: pod.GetNamespace(), Pod: pod.GetName(), UID: pod.GetUID(), Container: container, Command: command, Stdin: t.Input, Stdout: t, Stderr: t, TTY: true, SizeQueue: t.Sizes})
		t.Close()
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		return monitor.ResourceTerminalResult{Terminal: t, Err: err}
	})
}

func (m *model) updateResourceTerminalKey(msg tea.KeyPressMsg) tea.Cmd {
	t := m.explore.Terminal
	key := msg.String()
	if t.Captured {
		if key == "ctrl+]" {
			t.BeginReview()
			return nil
		}
		terminal.SendKey(t.Emulator, uv.KeyPressEvent(msg))
		return nil
	}
	if t.Review == nil {
		t.BeginReview()
	}
	reader := &t.Review.Viewer.Reader
	if reader.Searching || reader.Selecting || key == "esc" && reader.Query != "" {
		_, copied := reader.Key(key, msg.Text)
		return m.copyWorkspaceText(copied)
	}
	switch key {
	case "esc", "x":
		m.stopResourceTerminal()
		return nil
	case "r", "t":
		if !t.Done {
			if key == "t" {
				t.Emulator.SendText("\x03")
			}
			t.Captured, t.Review = true, nil
		}
		return nil
	case "e":
		if t.Done && t.Launch != nil {
			a := t.Launch
			a.Stage, a.Err, a.ExecEditing = "execcommand", "", true
			a.ExecChoice = 2
			m.explore.Action = a
			// Keep the completed output until a new command is explicitly started.
			m.explore.Terminal = nil
			a.PreviousTerminal = t
		}
		return nil
	case "R":
		if t.Done && t.Launch != nil {
			return m.startResourceTerminal(t.Launch)
		}
		return nil
	}
	handled, copied := t.ReviewKey(key, msg.Text)
	if handled {
		return m.copyWorkspaceText(copied)
	}
	return nil
}

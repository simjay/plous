package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	ai "github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/ui/terminal"
	textview "github.com/simjay/plous/internal/ui/text"
)

type nativeAgentResult struct {
	session *appchat.Native
	err     error
	saveErr error
	save    func() error
}
type nativeAgentTick struct{ session *appchat.Native }

type nativeScrollFrame struct{ session *appchat.Native }

// Only consecutive native wheel events reuse these frames. Every other update
// invalidates the panel cache before it can change focus, layout or content.
type nativeScrollState struct {
	pending               *nativeScrollFrame
	view                  *tea.View
	panels                [2]string
	deferView, onlyNative bool
}

func (m *model) queueNativeScroll() tea.Cmd {
	s := &m.nativeScroll
	if !s.onlyNative {
		s.panels = [2]string{}
	}
	s.onlyNative, s.deferView = true, true
	if s.pending != nil {
		return nil
	}
	frame := &nativeScrollFrame{session: m.nativeAgent}
	s.pending = frame
	return tea.Tick(time.Second/60, func(time.Time) tea.Msg { return frame })
}

func (m *model) nativeScrollWheel(msg tea.MouseWheelMsg) bool {
	n := m.nativeAgent
	if n == nil || !m.externalAgentSelected() || m.helpVisible() || m.chatSourcesVisible() {
		return false
	}
	if msg.Button != tea.MouseWheelUp && msg.Button != tea.MouseWheelDown {
		return false
	}
	if !m.mouseAllowed(msg) {
		return false
	}
	span, hit := m.mouse.hit(msg.X, msg.Y)
	return hit && span.Scope == "chat" && (n.Reading() || !n.Screen.IsAltScreen())
}

func (m *model) externalAgentSelected() bool { return m.settings.MainAgent.Effective() != agent.Plous }
func (m *model) nativeAgentCaptured() bool {
	return m.externalAgentSelected() && m.nativeAgent != nil && m.nativeChange == nil && m.nativeAgent.Captured && !m.nativeAgent.Finished && m.focus == 2 && m.panelVisible(2) && !m.helpVisible() && !m.chatSourcesVisible() && !m.commandFocused() && !m.tooSmall()
}
func (m *model) nativeAgentDimensions() (int, int) {
	return max(1, m.panelWidths()[2]-4), max(1, m.height-6)
}
func (m *model) resizeNativeAgent() {
	if m.nativeAgent != nil && m.panelVisible(2) && !m.tooSmall() {
		w, h := m.nativeAgentDimensions()
		m.nativeAgent.Resize(w, h)
	}
}
func nativeTick(n *appchat.Native) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return nativeAgentTick{n} })
}

func (m *model) syncNativeAgent() tea.Cmd {
	if !m.nativeRequested || m.nativeChange != nil {
		return nil
	}
	// Wait for the initial target and usable panel dimensions before capturing scope.
	if (m.nativeAgent == nil || m.nativeAgent.Finished) && (m.tooSmall() || !m.panelVisible(2) || m.currentRequest != nil && m.currentRequest.Startup || m.currentLoading != nil && m.currentLoading.Startup) {
		return nil
	}
	m.nativeRequested = false
	if !m.externalAgentSelected() || m.quitting {
		return nil
	}
	if n := m.nativeAgent; n != nil && n.SaveErr != nil {
		m.status = "Retry the evidence save with :X before opening another session."
		return nil
	}
	if n := m.nativeAgent; n != nil && !n.Finished {
		n.Captured = true
		return nil
	}
	if m.run != nil {
		m.status = "Stop the Plous response before opening an external agent."
		return nil
	}
	if m.currentDirectory == nil {
		m.nativeError = m.currentDirectoryError
		m.status = m.nativeError
		return nil
	}
	directory := m.currentDirectory.Connections[0].Settings["path"]
	path, err := cli.Executable(m.settings.MainAgent)
	if err != nil {
		m.status = err.Error()
		m.nativeError = err.Error()
		return nil
	}
	if err = m.ensureConversationScope(); err != nil {
		m.status = err.Error()
		m.nativeError = err.Error()
		return nil
	}
	root, err := m.store.DataRoot()
	if err != nil {
		m.status = err.Error()
		return nil
	}
	w, h := m.nativeAgentDimensions()
	n, ctx := appchat.NewNative(context.Background(), m.settings.MainAgent, m.conversationScope(), m.assistantScopeLabel(), w, h)
	n.Screen.SetForegroundColor(ui.Foreground)
	n.Screen.SetBackgroundColor(ui.Background)
	m.nativeAgent = n
	m.nativeError = ""
	connections := m.assistantConnections()
	m.captureSourceReads(connections)
	o := ai.Options{UI: m.uiTools(connections), Agent: n.Kind, Store: m.store, History: m.history, WorkspaceID: n.Scope, ScopeContext: m.contextText(), Connections: connections}
	fresh := m.nativeFresh
	m.nativeFresh = false
	return tea.Batch(nativeTick(n), func() tea.Msg {
		defer close(n.Done)
		defer n.Close()
		fail := func(err error) tea.Msg { return nativeAgentResult{session: n, err: err} }
		stateDirectory := filepath.Join(root, "native-agents", string(n.Kind), fmt.Sprintf("%x", sha256.Sum256([]byte(directory+"\x00"+n.Scope))))
		if err := os.MkdirAll(stateDirectory, 0700); err != nil {
			return fail(err)
		}
		marker := filepath.Join(stateDirectory, "started")
		_, previous := os.Stat(marker)
		tools, err := ai.OpenExternalTools(ctx, o)
		if err != nil {
			return fail(err)
		}
		instructions := ai.Instructions + "\nThis native terminal session uses only Plous tools. Its captured scope remains fixed until the session closes. Code execution is unavailable. Call the directly exposed Plous tools.\nCaptured connection and page:\n" + o.ScopeContext
		t, err := cli.StartTerminal(tools.Context, cli.TerminalOptions{Kind: n.Kind, Path: path, Directory: directory, StateDirectory: stateDirectory, Instructions: instructions, ID: tools.ID, Resume: !fresh && previous == nil, Tools: tools, Width: w, Height: h, Input: n.Input, Output: n})
		if err == nil {
			n.SetTerminal(t)
			_ = os.WriteFile(marker, []byte("Native CLI owns session persistence.\n"), 0600)
			err = t.Wait()
		}
		cause := err
		save := func() error { return tools.Finish(cause) }
		saveErr := save()
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		return nativeAgentResult{session: n, err: err, saveErr: saveErr, save: save}
	})
}

func (m *model) nativeAgentKey(msg tea.KeyPressMsg) tea.Cmd {
	n := m.nativeAgent
	key := msg.String()
	if n != nil {
		_, height := m.nativeAgentDimensions()
		switch {
		case key == "pgup":
			if handled, _ := n.Scroll(-max(1, height-1)); handled {
				return nil
			}
		case key == "pgdown":
			if handled, _ := n.Scroll(max(1, height-1)); handled {
				return nil
			}
		case n.Reading() && (key == "esc" || key == "end"):
			n.Follow()
			return nil
		case !m.nativeAgentCaptured() && (key == "up" || key == "k"):
			n.Scroll(-1)
			return nil
		case !m.nativeAgentCaptured() && (key == "down" || key == "j"):
			n.Scroll(1)
			return nil
		}
	}
	if m.nativeAgentCaptured() {
		m.shortcutPrefix = ""
		n.Follow()
		terminal.SendKey(n.Screen, uv.KeyPressEvent(msg))
		return nil
	}
	if msg.String() == "enter" {
		m.focusChat(true)
	}
	return nil
}

func (m *model) nativeAgentWheel(point tea.Mouse) tea.Cmd {
	n := m.nativeAgent
	if n == nil || (point.Button != tea.MouseWheelUp && point.Button != tea.MouseWheelDown) {
		return nil
	}
	delta := 1
	if point.Button == tea.MouseWheelUp {
		delta = -1
	}
	if handled, changed := n.Scroll(delta); handled {
		if changed {
			return m.queueNativeScroll()
		}
		m.nativeScroll.deferView = true
		return nil
	}
	if n.Finished {
		return nil
	}
	// Alternate-screen applications own their viewport. Send actual mouse input
	// only when requested by the CLI, never arrow keys that recall old prompts.
	widths := m.panelWidths()
	w, h := m.nativeAgentDimensions()
	point.X -= widths[0] + widths[1] + 2
	point.Y -= 4
	if point.X >= 0 && point.X < w && point.Y >= 0 && point.Y < h {
		n.Screen.SendMouse(uv.MouseWheelEvent(point))
	}
	return nil
}

func (m *model) nativeAgentBody(width, height int) string {
	if n := m.nativeAgent; n != nil {
		if change := m.nativeChange; change != nil && change.session == n {
			return ui.TextView("Closing "+n.Kind.Label()+"…\nSaving tool evidence before switching to "+change.kind.Label()+".", width, height, 0)
		}
		if n.Finished {
			message := "Session ended. cc Resume · :C New session"
			if n.Err != "" {
				message = "Session ended: " + ui.Plain(n.Err) + "\ncc Retry · :C New session"
			}
			if n.SaveErr != nil {
				message = "Evidence save failed: " + ui.Plain(n.SaveErr.Error()) + "\n:X Retry save"
			}
			return textview.Fit(message+"\n"+n.Render(), width, height)
		}
		if !n.HasOutput.Load() {
			return ui.TextView("Starting "+n.Kind.Label()+"…", width, height, 0)
		}
		return textview.Fit(n.Render(), width, height)
	}
	message := m.settings.MainAgent.Label() + " native terminal"
	if m.nativeError != "" {
		message += "\n\n" + ui.Plain(m.nativeError)
	} else {
		message += "\n\nUses your installed CLI and sign-in.\n\ncc Open terminal"
	}
	return ui.TextView(message, width, height, 0)
}

func (m *model) stopNativeAgent() {
	if m.nativeAgent != nil {
		m.nativeAgent.Close()
	}
}
func (m *model) drainNativeAgent() {
	if n := m.nativeAgent; n != nil {
		n.Close()
		<-n.Done
	}
}

func (m *model) retryNativeSave() {
	n := m.nativeAgent
	if n == nil || n.SaveErr == nil || n.Saving || n.Save == nil {
		return
	}
	n.Saving = true
	m.enqueuePersistence(&persistenceJob{write: func() any { return n.Save() }, apply: func(value any) tea.Cmd {
		n.Saving = false
		n.SaveErr, _ = value.(error)
		if n.SaveErr != nil {
			m.status = "Evidence save failed: " + n.SaveErr.Error()
			if m.quitting {
				m.persistenceErr = n.SaveErr
			}
		} else {
			m.status = "Native tool evidence saved."
		}
		return nil
	}})
}

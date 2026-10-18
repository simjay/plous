package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
	appchat "github.com/simjay/plous/internal/app/chat"
)

func nativeModel(t *testing.T) (*model, *appchat.Native) {
	t.Helper()
	m, _ := testModel(t)
	m.width, m.height = 160, 36
	m.settings.MainAgent = agent.ClaudeCode
	m.focus = 2
	w, h := m.nativeAgentDimensions()
	n, _ := appchat.NewNative(t.Context(), agent.ClaudeCode, "captured-scope", "Captured cluster", w, h)
	m.nativeAgent = n
	t.Cleanup(n.Close)
	return m, n
}

func TestNativeTerminalOwnsInputAndPreservesPlousDraft(t *testing.T) {
	m, n := nativeModel(t)
	m.draft().Insert("Plous draft", true)
	typeText(m, ":s hh ff /model")
	press(t, m, "tab")
	press(t, m, "esc")
	m.Update(tea.PasteMsg{Content: "literal :quit"})
	if m.focus != 2 || m.form != nil || m.command != nil || m.help != nil || !m.nativeAgentCaptured() || m.draft().Text != "Plous draft" {
		t.Fatal("native input escaped into Plous controls")
	}
	got := ""
	deadline := time.After(time.Second)
	for !strings.Contains(got, "literal :quit") {
		select {
		case p := <-n.Input.Chunks:
			got += string(p)
		case <-deadline:
			t.Fatalf("terminal input missing: %q", got)
		}
	}
	if !strings.Contains(got, ":s hh ff /model\t\x1b") {
		t.Fatalf("keys were rewritten: %q", got)
	}
	m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if m.nativeAgentCaptured() {
		t.Fatal("release did not return ownership")
	}
	press(t, m, "alt+left")
	if m.focus != 0 {
		t.Fatal("Alt+Left did not move panels")
	}
	m.chatCommand("chat")
	m.syncNativeAgent()
	if !m.nativeAgentCaptured() {
		t.Fatal("chat did not recapture the same terminal")
	}
	if m.nativeAgent != n || n.Scope != "captured-scope" {
		t.Fatal("navigation retargeted native tools")
	}
}

func TestNativeCtrlCForwardsInterruptWithoutClosingSession(t *testing.T) {
	for _, kind := range []agent.Kind{agent.ClaudeCode, agent.Codex} {
		for _, reading := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reading=%t", kind, reading), func(t *testing.T) {
				m, n := nativeModel(t)
				m.settings.MainAgent, n.Kind = kind, kind
				m.draft().Insert("retained Plous draft", true)
				if reading {
					_, _ = n.Write([]byte(strings.Repeat("Native output\r\n", 50)))
					n.Scroll(-5)
					if !n.Reading() {
						t.Fatal("fixture did not enter scrollback")
					}
				}
				press(t, m, "ctrl+c")
				select {
				case input := <-n.Input.Chunks:
					if string(input) != "\x03" {
						t.Fatalf("native interrupt bytes = %q", input)
					}
				case <-time.After(time.Second):
					t.Fatal("Ctrl+C did not reach the native terminal")
				}
				if m.quitting || m.nativeAgent != n || n.Finished || n.Input.Ctx.Err() != nil || n.Reading() || !m.nativeAgentCaptured() {
					t.Fatal("interrupt closed the session or changed input ownership")
				}
				if m.draft().Text != "retained Plous draft" || !strings.Contains(m.hint(), "Ctrl+C Interrupt agent") {
					t.Fatal("interrupt lost the Plous draft or showed the wrong control hint")
				}
				m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
				if m.focus != 1 {
					t.Fatal("Ctrl+] did not return to main")
				}
				press(t, m, "ctrl+c")
				if !m.quitting || n.Input.Ctx.Err() != context.Canceled {
					t.Fatal("Ctrl+C outside native input did not close Plous and its native session")
				}
			})
		}
	}
}

func TestNativeCtrlCExitsOutsideCapturedInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*model)
	}{
		{"tree", func(m *model) { m.focus = 0 }},
		{"main", func(m *model) { m.focus = 1 }},
		{"help", func(m *model) { m.openHelp() }},
		{"command", func(m *model) { m.beginCommand() }},
		{"settings", func(m *model) { m.openSettings() }},
		{"small terminal", func(m *model) { m.Update(tea.WindowSizeMsg{Width: 60, Height: 10}) }},
		{"finished session", func(m *model) { m.nativeAgent.Finished = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, n := nativeModel(t)
			tc.setup(m)
			if m.nativeAgentCaptured() {
				t.Fatal("fixture still owns native input")
			}
			press(t, m, "ctrl+c")
			if !m.quitting || n.Input.Ctx.Err() != context.Canceled {
				t.Fatal("Ctrl+C did not request Plous shutdown")
			}
		})
	}
}

func TestNativeTerminalFitsPanelAndCursorAfterResize(t *testing.T) {
	m, n := nativeModel(t)
	_, _ = n.Write([]byte("\x1b[2J\x1b[HNative screen\x1b[3;5H@"))
	for _, size := range [][2]int{{160, 36}, {200, 44}, {100, 20}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		w, h := m.nativeAgentDimensions()
		if n.Screen.Width() != w || n.Screen.Height() != h {
			t.Fatal("PTY screen not resized")
		}
		view := m.View()
		rows := strings.Split(ansi.Strip(view.Content), "\n")
		if len(rows) != size[1] {
			t.Fatalf("frame height=%d want=%d", len(rows), size[1])
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatal("native terminal escaped panel")
			}
		}
		if view.Cursor == nil {
			t.Fatal("native cursor hidden")
		}
		cursor := view.Cursor
		if cursor.Y >= len(rows) || !strings.Contains(rows[cursor.Y], "@") {
			t.Fatalf("cursor outside native screen: %+v", cursor)
		}
		body := ansi.Strip(m.chatBody(w, h))
		for _, old := range []string{"Write a message", "/model", "New/History", "CLI-managed"} {
			if strings.Contains(body, old) {
				t.Fatal("Plous recreated native controls", old)
			}
		}
	}
}

func TestNativeSessionMustCloseBeforeAgentChangeAndExitCancelsIt(t *testing.T) {
	m, n := nativeModel(t)
	closed := make(chan struct{})
	original := n.Cancel
	n.Cancel = func() { original(); close(closed) }
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	press(t, m, "right")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent != agent.ClaudeCode || m.form == nil {
		t.Fatal("replaced active terminal")
	}
	m.stopViews()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("exit leaked terminal")
	}
	if n.Input.Ctx.Err() != context.Canceled {
		t.Fatal("exit did not revoke queued input")
	}
}

func TestNativeExitWaitsForEvidenceAndRetriesFailedSave(t *testing.T) {
	m, n := nativeModel(t)
	m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	press(t, m, "ctrl+c")
	m.drainPersistence()
	if cmd := m.syncQuit(); cmd != nil || !m.quitting || n.Input.Ctx.Err() != context.Canceled {
		t.Fatal("exit did not wait for the native session to close")
	}
	failure := errors.New("fixture storage unavailable")
	retries := 0
	m.Update(nativeAgentResult{session: n, saveErr: failure, save: func() error {
		retries++
		return nil
	}})
	m.drainPersistence()
	if cmd := m.syncQuit(); cmd != nil || m.quitting || !n.Finished || n.SaveErr != failure {
		t.Fatal("failed evidence save did not keep the app open")
	}
	m.focusChat(true)
	if cmd := m.syncNativeAgent(); cmd != nil || m.nativeAgent != n {
		t.Fatal("a new session replaced unsaved evidence")
	}
	press(t, m, "ctrl+c")
	m.drainPersistence()
	// Retrying evidence precedes the final application session snapshot.
	m.syncQuit()
	m.drainPersistence()
	cmd := m.syncQuit()
	if cmd == nil {
		t.Fatalf("saved evidence still blocked exit: %s", m.status)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("saved evidence did not allow exit")
	}
	if retries != 1 || n.SaveErr != nil {
		t.Fatal("exit did not retry the failed evidence save")
	}
}

func TestNativeStartupWaitsForTargetAndPanel(t *testing.T) {
	for _, kind := range []agent.Kind{agent.ClaudeCode, agent.Codex} {
		for _, hasContext := range []bool{true, false} {
			t.Run(string(kind)+fmt.Sprint(hasContext), func(t *testing.T) {
				original, _ := testModel(t)
				settings := original.settings
				settings.MainAgent = kind
				if !original.saveAISettings(settings) {
					t.Fatal(original.status)
				}
				m, err := newModel(original.store, original.history, "test")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { m.quitting = true; m.drainPersistence() })
				if !m.nativeRequested {
					t.Fatal("saved external agent was not requested at startup")
				}
				if hasContext {
					m.currentPaths = currentFixture(t)
				}
				bin := t.TempDir()
				name := "claude"
				if kind == agent.Codex {
					name = "codex"
				}
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", bin)
				if m.syncNativeAgent() != nil || m.nativeAgent != nil {
					t.Fatal("started before size and context")
				}
				m.width, m.height = 160, 36
				if m.syncNativeAgent() != nil {
					t.Fatal("started before context resolution")
				}
				resolve := m.syncCurrent()
				if m.syncNativeAgent() != nil {
					t.Fatal("started while context resolution was pending")
				}
				m.applyCurrent(resolve().(currentResult))
				m.rightHidden = true
				if m.syncNativeAgent() != nil {
					t.Fatal("started in hidden panel")
				}
				m.rightHidden = false
				cmd := m.syncNativeAgent()
				if cmd == nil || m.nativeAgent == nil {
					t.Fatalf("did not start: %s", m.status)
				}
				n := m.nativeAgent
				t.Cleanup(n.Close)
				if n.Scope != m.conversationScope() || hasContext && n.Scope == "" {
					t.Fatal("captured the wrong startup scope")
				}
				if m.focus != 1 || m.mode != 0 {
					t.Fatal("startup moved focus or tab")
				}
				if m.syncNativeAgent() != nil || m.nativeAgent != n {
					t.Fatal("started twice")
				}
				n.Finished = true
				if m.syncNativeAgent() != nil {
					t.Fatal("automatically restarted a closed session")
				}
			})
		}
	}
}

func TestNativeMouseFocusKeepsSessionAndResumesInput(t *testing.T) {
	m, n := nativeModel(t)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("native capture disabled panel mouse controls")
	}
	for _, scope := range []string{"main", "tree"} {
		clickMouse(t, m, scope, "")
		if m.nativeAgentCaptured() || m.nativeAgent != n || n.Finished || !n.Captured {
			t.Fatal("click did not leave the native panel with its session intact")
		}
		clickMouse(t, m, "chat", "")
		if m.focus != 2 || !m.nativeAgentCaptured() || m.nativeAgent != n {
			t.Fatal("click did not return input to the same native terminal")
		}
	}
	m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	clickMouse(t, m, "chat", "")
	if !m.nativeAgentCaptured() {
		t.Fatal("click did not resume native input after the focus toggle")
	}
	typeText(m, "draft after clicks")
	got := ""
	deadline := time.After(time.Second)
	for got != "draft after clicks" {
		select {
		case chunk := <-n.Input.Chunks:
			got += string(chunk)
		case <-deadline:
			t.Fatalf("mouse escaped into native input or typing was lost: %q", got)
		}
	}
	m.settings.MouseDisabled = true
	clickMouse(t, m, "main", "")
	if m.focus != 2 || m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("native panel ignored the saved mouse preference")
	}
}

func nativeWheel(t *testing.T, m *model, button tea.MouseButton) {
	t.Helper()
	m.View()
	widths := m.panelWidths()
	m.Update(tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: button})
	if frame := m.nativeScroll.pending; frame != nil {
		m.Update(frame)
	}
}

func TestNativeWheelBatchesFramesAndKeepsOneRowSteps(t *testing.T) {
	m, n := nativeModel(t)
	for i := range 90 {
		_, _ = fmt.Fprintf(n, "Reply %03d\r\n", i)
	}
	before := m.View().Content
	widths := m.panelWidths()
	wheel := tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: tea.MouseWheelUp}
	_, cmd := m.Update(wheel)
	frame := m.nativeScroll.pending
	if cmd == nil || frame == nil || m.View().Content != before {
		t.Fatal("wheel did not defer drawing to one frame")
	}
	first := strings.Split(ansi.Strip(n.Render()), "\n")[0]
	if first != "Reply 060" {
		t.Fatalf("wheel did not move one row: %q", first)
	}
	for range 4 {
		_, cmd = m.Update(wheel)
		if cmd != nil || m.nativeScroll.pending != frame || m.View().Content != before {
			t.Fatal("wheel burst scheduled extra frames or repainted early")
		}
	}
	m.Update(nativeAgentTick{session: n})
	if m.nativeScroll.pending != frame || m.View().Content != before {
		t.Fatal("background terminal tick interrupted the scroll frame")
	}
	m.Update(frame)
	if m.View().Content == before || !strings.HasPrefix(ansi.Strip(n.Render()), "Reply 056\n") {
		t.Fatal("frame did not show every accumulated row")
	}
	// Consecutive scroll frames retain the unchanged sidebar and main panel.
	panels := m.nativeScroll.panels
	m.Update(wheel)
	m.Update(m.nativeScroll.pending)
	m.View()
	if panels != m.nativeScroll.panels || !m.nativeScroll.onlyNative {
		t.Fatal("scrolling invalidated unchanged panels")
	}
	// A newer input owns the view; a delayed frame cannot restore scrollback.
	m.Update(wheel)
	stale := m.nativeScroll.pending
	typeText(m, "x")
	live := m.View().Content
	if n.Reading() || m.nativeScroll.pending != nil || m.nativeScroll.onlyNative {
		t.Fatal("typing did not immediately restore live input")
	}
	m.Update(stale)
	if m.View().Content != live || n.Reading() {
		t.Fatal("late scroll frame overwrote typing")
	}
}

func TestNativeScrollFramesInvalidateOnResizeAndPanelTravel(t *testing.T) {
	m, n := nativeModel(t)
	_, _ = n.Write([]byte(strings.Repeat("older output\r\n", 90)))
	m.View()
	widths := m.panelWidths()
	m.Update(tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: tea.MouseWheelUp})
	stale := m.nativeScroll.pending
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	resized := m.View().Content
	if m.nativeScroll.pending != nil || m.nativeScroll.onlyNative {
		t.Fatal("resize kept stale scroll rendering")
	}
	m.Update(stale)
	if m.View().Content != resized {
		t.Fatal("late frame replaced resized geometry")
	}
	press(t, m, "alt+left")
	m.View()
	if m.focus != 1 || m.nativeScroll.onlyNative || !n.Reading() {
		t.Fatal("panel travel lost reading position or kept cached focus")
	}
}

func TestNativeWheelBoundaryBurstsKeepInputResponsive(t *testing.T) {
	for _, name := range []string{"top", "bottom", "empty-up", "empty-down"} {
		t.Run(name, func(t *testing.T) {
			m, n := nativeModel(t)
			empty := strings.HasPrefix(name, "empty")
			if !empty {
				for i := range 90 {
					_, _ = fmt.Fprintf(n, "Reply %03d\r\n", i)
				}
			}
			button := tea.MouseWheelDown
			if name == "top" || name == "empty-up" {
				button = tea.MouseWheelUp
			}
			if name == "top" {
				n.Scroll(-10000)
			}
			before := m.View().Content
			view := m.nativeScroll.view
			widths := m.panelWidths()
			wheel := tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: button}
			for range 1000 {
				_, cmd := m.Update(wheel)
				if cmd != nil || m.nativeScroll.pending != nil {
					t.Fatal("scrolling past the boundary scheduled work")
				}
				if m.View().Content != before || m.nativeScroll.view != view {
					t.Fatal("scrolling past the boundary redrew the screen")
				}
			}
			_, _ = n.Write([]byte("fresh output\r\n"))
			m.Update(nativeAgentTick{session: n})
			if name == "top" {
				if m.View().Content != before {
					t.Fatal("incoming output moved the held history")
				}
			} else if !strings.Contains(m.View().Content, "fresh output") {
				t.Fatal("boundary scrolling froze live output")
			}
			if !empty {
				before = n.Render()
				if button == tea.MouseWheelUp {
					wheel.Button = tea.MouseWheelDown
				} else {
					wheel.Button = tea.MouseWheelUp
				}
				m.Update(wheel)
				if m.nativeScroll.pending == nil || n.Render() == before {
					t.Fatal("reversing direction did not move immediately")
				}
				m.Update(m.nativeScroll.pending)
				m.View()
			}
			typeText(m, "x")
			press(t, m, "alt+left")
			if n.Reading() || m.focus != 1 {
				t.Fatal("boundary scrolling blocked typing or panel travel")
			}
			select {
			case input := <-n.Input.Chunks:
				if string(input) != "x" {
					t.Fatalf("wheel events reached the native prompt: %q", input)
				}
			case <-time.After(time.Second):
				t.Fatal("typing was blocked after boundary scrolling")
			}
		})
	}
}

func TestNativeWheelBoundaryKeepsLastPendingFrame(t *testing.T) {
	for _, name := range []string{"top", "bottom"} {
		t.Run(name, func(t *testing.T) {
			m, n := nativeModel(t)
			for i := range 90 {
				_, _ = fmt.Fprintf(n, "Reply %03d\r\n", i)
			}
			button := tea.MouseWheelDown
			n.Scroll(-1)
			if name == "top" {
				n.Scroll(-10000)
				n.Scroll(1)
				button = tea.MouseWheelUp
			}
			before := m.View().Content
			widths := m.panelWidths()
			wheel := tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: button}
			m.Update(wheel)
			frame := m.nativeScroll.pending
			if frame == nil {
				t.Fatal("the last movable row did not schedule a frame")
			}
			for range 1000 {
				_, cmd := m.Update(wheel)
				if cmd != nil || m.nativeScroll.pending != frame || m.View().Content != before {
					t.Fatal("boundary events replaced or interrupted the last frame")
				}
			}
			m.Update(frame)
			if m.View().Content == before {
				t.Fatal("the last movement was never displayed")
			}
		})
	}
}

func TestNativeWheelBoundaryClearsShortcutHint(t *testing.T) {
	m, n := nativeModel(t)
	n.Captured = false
	typeText(m, "i")
	if !strings.Contains(ansi.Strip(m.View().Content), "i → i resize") {
		t.Fatal("missing pending shortcut hint")
	}
	widths := m.panelWidths()
	wheel := tea.MouseWheelMsg{X: widths[0] + widths[1] + 5, Y: 7, Button: tea.MouseWheelDown}
	_, cmd := m.Update(wheel)
	if cmd != nil || m.nativeScroll.pending != nil || m.shortcutPrefix != "" || strings.Contains(ansi.Strip(m.View().Content), "i → i resize") {
		t.Fatal("boundary wheel did not clear the shortcut hint immediately")
	}
	view := m.nativeScroll.view
	m.Update(wheel)
	m.View()
	if m.nativeScroll.view != view {
		t.Fatal("another boundary event redrew the cleared hint")
	}
}

func TestNativeWheelReadsOutputWithoutChangingInputOrFocus(t *testing.T) {
	m, n := nativeModel(t)
	for i := range 90 {
		_, _ = fmt.Fprintf(n, "Reply %03d\r\n", i)
	}
	m.draft().Insert("separate Plous draft", true)
	m.focus = 1
	nativeWheel(t, m, tea.MouseWheelUp)
	if !n.Reading() || m.focus != 1 || !n.Captured || m.draft().Text != "separate Plous draft" {
		t.Fatal("wheel lost history, moved focus, or changed the draft")
	}
	before := n.Render()
	_, _ = n.Write([]byte("new streaming output\r\n"))
	if n.Render() != before {
		t.Fatal("incoming output moved the history viewport")
	}
	clickMouse(t, m, "chat", "")
	if m.View().Cursor != nil {
		t.Fatal("live prompt cursor was drawn over old output")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if n.Render() == before || !n.Reading() {
		t.Fatal("Page Up did not scroll history")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if n.Reading() || m.View().Cursor == nil {
		t.Fatal("End did not restore the live screen and cursor")
	}
	nativeWheel(t, m, tea.MouseWheelUp)
	m.Update(tea.PasteMsg{Content: "retained draft"})
	if n.Reading() {
		t.Fatal("paste did not restore live output")
	}
	nativeWheel(t, m, tea.MouseWheelUp)
	typeText(m, "!")
	if n.Reading() {
		t.Fatal("typing did not restore live output")
	}
	got := ""
	deadline := time.After(time.Second)
	for got != "retained draft!" {
		select {
		case chunk := <-n.Input.Chunks:
			got += string(chunk)
		case <-deadline:
			t.Fatalf("scrolling reached the native command input: %q", got)
		}
	}
	// Actual arrows still belong to the CLI's prompt while input is captured.
	press(t, m, "up")
	select {
	case chunk := <-n.Input.Chunks:
		if string(chunk) != "\x1b[A" {
			t.Fatalf("native prompt arrow changed: %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("native prompt arrow was swallowed")
	}
}

func TestNativeWheelForwardsOnlyRequestedAlternateScreenMouse(t *testing.T) {
	m, n := nativeModel(t)
	_, _ = n.Write([]byte("\x1b[?1049hfullscreen CLI"))
	nativeWheel(t, m, tea.MouseWheelUp)
	_, _ = n.Write([]byte("\x1b[?1002h\x1b[?1006h"))
	m.focus = 1
	nativeWheel(t, m, tea.MouseWheelDown)
	select {
	case chunk := <-n.Input.Chunks:
		if string(chunk) != "\x1b[<65;4;4M" {
			t.Fatalf("wheel used arrows, wrong coordinates, or unrequested input: %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("CLI did not receive its wheel event")
	}
	if m.focus != 1 || n.Reading() {
		t.Fatal("native scrolling moved focus or exposed normal history")
	}
}

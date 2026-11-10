package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

func TestPanelToggleAliasesFromEveryPanel(t *testing.T) {
	for _, command := range []string{"b", "sidebar"} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("%s/from%d", command, focus), func(t *testing.T) {
				m, _ := testModel(t)
				focusPanel(t, m, focus)
				typeText(m, ":"+command)
				press(t, m, "enter")
				left := command == "b" || command == "sidebar"
				if m.leftHidden != left || m.rightHidden == left {
					t.Fatal("toggle did not hide the requested panel")
				}
				wantFocus := focus
				if left && focus == 0 || !left && focus == 2 {
					wantFocus = 1
				}
				if m.focus != wantFocus {
					t.Fatalf("focus = %d, want %d", m.focus, wantFocus)
				}
				typeText(m, ":"+command)
				press(t, m, "enter")
				if m.leftHidden || m.rightHidden || m.focus != wantFocus {
					t.Fatal("showing a panel changed focus or failed to reveal it")
				}
			})
		}
	}
}

func TestHiddenPanelsAreSkippedByFocusAndRestoredAfterRestart(t *testing.T) {
	for _, hidden := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		m, _ := testModel(t)
		if hidden[0] {
			mainCommand(t, m, "b")
		}
		if hidden[1] {
			hideAssistant(t, m)
		}
		if err := m.saveSession(); err != nil {
			t.Fatal(err)
		}
		restored, err := newModel(m.store, m.history, "test-version")
		if err != nil || restored.leftHidden != hidden[0] || restored.rightHidden != hidden[1] {
			t.Fatalf("visibility did not survive restart: %v", err)
		}
		visible := []int{}
		for panel := 0; panel < 3; panel++ {
			if panel == 1 || panel == 0 && !hidden[0] || panel == 2 && !hidden[1] {
				visible = append(visible, panel)
			}
		}
		for _, size := range [][2]int{{120, 30}, {60, 10}} {
			restored.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for i, panel := range visible {
				restored.focus = panel
				press(t, restored, "alt+right")
				if restored.focus != visible[(i+1)%len(visible)] {
					t.Fatal("Alt+Right did not skip hidden panels")
				}
				press(t, restored, "alt+left")
				if restored.focus != panel {
					t.Fatal("Alt+Left did not skip hidden panels")
				}
			}
		}
	}
	for _, data := range []string{
		`{"version":1,"focus":0,"left_hidden":true}`,
		`{"version":1,"focus":2,"right_hidden":true}`,
	} {
		m, _ := testModel(t)
		if err := m.store.WriteSession([]byte(data)); err != nil {
			t.Fatal(err)
		}
		restored, err := newModel(m.store, m.history, "test-version")
		if err != nil || restored.focus != 1 {
			t.Fatalf("restored focus points at a hidden panel: %v", err)
		}
	}
}

func TestCollapsedPanelLayoutsFitAndKeepCommandBox(t *testing.T) {
	m, _ := testModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}, {320, 60}, {480, 60}, {640, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, hidden := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			m.leftHidden, m.rightHidden = hidden[0], hidden[1]
			typeText(m, ":help")
			lines := strings.Split(ansi.Strip(m.render()), "\n")
			if len(lines) != size[1] {
				t.Fatalf("wrong height at %v, hidden %v", size, hidden)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatalf("row overflow at %v: %q", size, line)
				}
			}
			panels := 1
			for _, hide := range hidden {
				if !hide {
					panels++
				}
			}
			if strings.Count(lines[1], "╭") != panels+2 || strings.Count(lines[len(lines)-2], "╰") != panels {
				t.Fatal("hidden panels still occupy space")
			}
			if strings.Contains(lines[2], "Connections") == hidden[0] || strings.Contains(lines[2], "AI assistant") == hidden[1] {
				t.Fatal("unexpected visible panel title")
			}
			if !strings.Contains(lines[len(lines)-4], ":help▏") || !strings.Contains(lines[len(lines)-1], "Enter Run") {
				t.Fatal("command box and single footer must stay at the bottom")
			}
			press(t, m, "esc")
		}
	}
}

func TestResponsivePanelWidthsPreserveCompactLayoutAndGrowSides(t *testing.T) {
	m, _ := testModel(t)
	for _, tc := range []struct {
		width int
		want  [3]int
	}{
		{80, [3]int{24, 32, 24}},
		{120, [3]int{30, 54, 36}},
		{160, [3]int{32, 88, 40}},
		{200, [3]int{32, 118, 50}},
		{256, [3]int{32, 160, 64}},
		{320, [3]int{40, 200, 80}},
		{480, [3]int{60, 300, 120}},
		{640, [3]int{80, 400, 160}},
	} {
		m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 40})
		if got := m.panelWidths(); got != tc.want {
			t.Fatalf("at %d columns, widths = %v, want %v", tc.width, got, tc.want)
		}
	}

	previous := [3]int{}
	for width := 80; width <= 640; width++ {
		m.width = width
		got := m.panelWidths()
		if got[0]+got[1]+got[2] != width {
			t.Fatalf("at %d columns, panels do not fill the terminal: %v", width, got)
		}
		for _, panel := range []int{0, 2} {
			if got[panel] < previous[panel] {
				t.Fatalf("at %d columns, panel %d shrank from %d to %d", width, panel, previous[panel], got[panel])
			}
		}
		previous = got
	}
}

func TestResponsivePanelTogglesGiveFreedWidthToMain(t *testing.T) {
	m, _ := testModel(t)
	for _, width := range []int{120, 160, 200, 320, 480} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		original := m.panelWidths()
		mainCommand(t, m, "b")
		if got := m.panelWidths(); got != [3]int{0, original[0] + original[1], original[2]} {
			t.Fatalf("hiding sidebar at %d columns changed the assistant width: %v", width, got)
		}
		hideAssistant(t, m)
		if got := m.panelWidths(); got != [3]int{0, width, 0} {
			t.Fatalf("hiding both sides at %d columns left unused space: %v", width, got)
		}
		mainCommand(t, m, "b")
		if got := m.panelWidths(); got != [3]int{original[0], original[1] + original[2], 0} {
			t.Fatalf("showing sidebar at %d columns changed its width: %v", width, got)
		}
		mainCommand(t, m, "i")
		if got := m.panelWidths(); got != original {
			t.Fatalf("restoring both sides at %d columns changed the layout: %v", width, got)
		}
	}
}

func TestHelpRemainsAccessibleWithHiddenPanels(t *testing.T) {
	m, _ := testModel(t)
	focusPanel(t, m, 0)
	typeText(m, ":h")
	press(t, m, "enter")
	mainCommand(t, m, "b")
	hideAssistant(t, m)
	for section := 0; section < 3; section++ {
		for m.help.section != section {
			press(t, m, "right")
		}
	}
	view := ansi.Strip(m.render())
	for _, text := range []string{"Workspaces", "AI chat", "Global", "bb Sidebar", "ii AI panel"} {
		if !strings.Contains(view, text) {
			t.Fatalf("help lost %q when panels were hidden", text)
		}
	}
	press(t, m, "esc")
	if m.help != nil || m.focus != 1 || !m.leftHidden || !m.rightHidden {
		t.Fatal("closing help did not respect visibility or repair previous focus")
	}
}

func TestPanelTogglesPreserveInputAndChatCommandsRevealChat(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	startChat(t, m)
	typeText(m, ":b :i :sidebar :aiinteraction")
	m.Update(tea.PasteMsg{Content: " :b"})
	draft := m.draft().Text
	if m.leftHidden || m.rightHidden || draft != ":b :i :sidebar :aiinteraction :b" {
		t.Fatal("chat input executed a toggle")
	}
	press(t, m, "esc")
	for _, command := range []string{"c", "chat", "C", "newchat", "H", "openchat"} {
		hideAssistant(t, m)
		mainCommand(t, m, command)
		if m.rightHidden || m.focus != 2 || m.draft().Text != draft {
			t.Fatalf(":%s failed to reveal chat and preserve the draft", command)
		}
		press(t, m, "esc")
	}
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, ":b :i")
	f := m.form
	if m.leftHidden || m.rightHidden || f.Fields[0].Value.Text != "Payments:b :i" {
		t.Fatal("form text executed a toggle")
	}
	press(t, m, "enter")
	for range 2 {
		typeText(m, ":i")
		press(t, m, "enter")
	}
	typeText(m, ":b")
	press(t, m, "enter")
	if m.form != f || m.current().Name != "Payments" || m.focus != 1 || !m.leftHidden || !m.rightHidden {
		t.Fatal("toggling panels changed or saved an unfinished form")
	}
}

func TestHidingChatKeepsReceivingAndSavingResponse(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Payments")
	m.closeSettings()
	conversation, err := m.history.NewConversation(id, "Test reply")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn(id, conversation.ID, history.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	c := m.chat()
	c.Conversation, c.Turns = conversation, []history.Turn{turn}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	run := &appchat.ChatRun{Chat: c, Cancel: cancel}
	m.run = run
	hideAssistant(t, m)
	m.Update(appchat.ChatEvent{Run: run, Text: "Arrived while hidden"})
	if ctx.Err() != nil || m.run != run || !m.rightHidden {
		t.Fatal("hiding chat stopped the response or a response reopened chat")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	turns, err := m.history.Turns(id, conversation.ID)
	if err != nil || turns[0].Response.Text != "Arrived while hidden" {
		t.Fatalf("hidden response was not saved: %v", err)
	}
	mainCommand(t, m, "aiinteraction")
	if m.chat() != c || !strings.Contains(ansi.Strip(m.render()), "Arrived while hidden") {
		t.Fatal("restored chat lost its response")
	}
}

func hideAssistant(t *testing.T, m *model) {
	t.Helper()
	for range 3 {
		if m.rightHidden {
			return
		}
		typeText(m, ":i")
		press(t, m, "enter")
	}
	t.Fatal("assistant did not reach hidden stage")
}

func TestAssistantSizeCycleFromEveryPanel(t *testing.T) {
	for _, command := range []string{"ii", ":i", ":aiinteraction"} {
		for focus := range 3 {
			t.Run(fmt.Sprintf("%s/%d", command, focus), func(t *testing.T) {
				m, _ := testModel(t)
				m.width, m.height, m.focus = 160, 36, focus
				m.draft().Insert("Retained draft", true)
				for stage, want := range [][3]int{{32, 48, 80}, {32, 128, 0}, {32, 88, 40}} {
					typeText(m, command)
					if strings.HasPrefix(command, ":") {
						press(t, m, "enter")
					}
					if got := m.panelWidths(); got != want {
						t.Fatalf("stage %d: widths=%v want=%v", stage, got, want)
					}
					if !m.panelVisible(m.focus) || m.draft().Text != "Retained draft" {
						t.Fatal("sizing lost focus or draft")
					}
					if err := m.saveSession(); err != nil {
						t.Fatal(err)
					}
					restored, err := newModel(m.store, m.history, "test")
					if err != nil {
						t.Fatal(err)
					}
					restored.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
					if restored.assistantSize != m.assistantSize || restored.rightHidden != m.rightHidden {
						t.Fatal("size did not survive restart")
					}
				}
			})
		}
	}
}

func TestAssistantExpansionRevealsCompactOverview(t *testing.T) {
	m, _ := overviewModel(t)
	m.mode, m.focus, m.width = 0, 1, 100
	typeText(m, "ii")
	if !m.panelVisible(2) || m.panelWidths()[1] < 24 {
		t.Fatal("expansion left the assistant hidden on compact Overview")
	}
	for _, width := range []int{80, 100, 160, 200} {
		for _, size := range []assistantSize{assistantNormal, assistantDouble} {
			m.focus, m.assistantSize = 2, size
			m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			for _, row := range strings.Split(ansi.Strip(m.render()), "\n") {
				if ansi.StringWidth(row) != width {
					t.Fatalf("Overview %d/%s overflow", width, size)
				}
			}
		}
	}
}

func TestHidingDoubleAssistantRestoresOverview(t *testing.T) {
	for _, command := range []string{"ii", ":i"} {
		for _, width := range []int{60, 80, 100, 104, 120, 160, 200, 220, 240} {
			t.Run(fmt.Sprintf("%s/%d", command, width), func(t *testing.T) {
				m, v := overviewModel(t)
				m.width, m.height = width, 36
				v.State.Pane, v.State.Cursor[2] = 2, 1
				m.draft().Insert("Retained draft", true)
				typeText(m, "ii")
				if m.assistantSize != assistantDouble {
					t.Fatal("assistant did not expand")
				}
				typeText(m, command)
				if strings.HasPrefix(command, ":") {
					press(t, m, "enter")
				}
				if !m.rightHidden || m.panelVisible(2) || !m.panelVisible(1) || m.focus != 1 {
					t.Fatalf("hiding assistant did not restore main: focus=%d widths=%v", m.focus, m.panelWidths())
				}
				view := ansi.Strip(m.render())
				if width >= 104 && !strings.Contains(view, "Current Workspace") {
					t.Fatalf("hiding assistant collapsed a tree that fits: widths=%v", m.panelWidths())
				}
				rows := strings.Split(view, "\n")
				if len(rows) != m.height {
					t.Fatalf("frame height=%d want=%d", len(rows), m.height)
				}
				for _, row := range rows {
					if ansi.StringWidth(row) != width {
						t.Fatalf("frame row width=%d want=%d", ansi.StringWidth(row), width)
					}
				}
				typeText(m, "ii")
				if m.rightHidden || m.assistantSize != assistantNormal || m.draft().Text != "Retained draft" || v.State.Pane != 2 || v.State.Cursor[2] != 1 {
					t.Fatal("restoring chat lost its draft, size or Overview selection")
				}
			})
		}
	}
}

func TestAssistantSizesFitTerminalAndNativeScreen(t *testing.T) {
	m, n := nativeModel(t)
	n.Captured = false
	_, _ = n.Write([]byte("Native draft stays here"))
	for _, width := range []int{80, 100, 120, 160, 200, 320} {
		for _, size := range []assistantSize{assistantNormal, assistantDouble} {
			m.mode = 1
			m.focus, m.assistantSize = 2, size
			m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			widths := m.panelWidths()
			if widths[0]+widths[1]+widths[2] != width || widths[1] < 0 {
				t.Fatal(widths)
			}
			rows := strings.Split(ansi.Strip(m.render()), "\n")
			if len(rows) != 36 {
				t.Fatal("wrong frame height", len(rows))
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != width {
					t.Fatalf("%d/%s: row width %d", width, size, ansi.StringWidth(row))
				}
			}
			if n.Screen.Width() != widths[2]-4 || n.Finished {
				t.Fatal("native screen not resized in place")
			}
		}
	}
}

func TestNativeFooterKeepsToggleAndSizingPrefix(t *testing.T) {
	m, n := nativeModel(t)
	m.status = "Unrelated status"
	if !strings.Contains(ansi.Strip(m.footer()), "Ctrl+] Return") {
		t.Fatal("status hid toggle hint")
	}
	m.status, m.currentNotice, m.currentRoot = "", "No context", true
	if !strings.Contains(ansi.Strip(m.footer()), "Ctrl+] Return") {
		t.Fatal("notice hid toggle hint")
	}
	n.Captured = false
	typeText(m, "i")
	if !strings.Contains(ansi.Strip(m.footer()), "i → i resize") {
		t.Fatal("native footer hid sizing prefix")
	}
}

func TestPanelTravelKeepsEditorInputAndUndo(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		text, cursor, revision := d.Text(), d.Cursor, d.Revision
		press(t, m, "alt+right")
		if m.focus != 2 || d.Mode != mode || d.Text() != text || d.Cursor != cursor || d.Revision != revision {
			t.Fatal("panel travel changed the editor")
		}
		press(t, m, "alt+left")
		if m.focus != 1 || m.explore.Action != action || d.Mode != mode {
			t.Fatal("return lost the editor input owner")
		}
	}
	m, action := documentModel(t)
	d := action.Document
	before := d.Text()
	typeText(m, "iX")
	press(t, m, "tab")
	if m.focus != 1 || !strings.Contains(d.Text(), "X  ") {
		t.Fatal("Tab no longer indents the document")
	}
	press(t, m, "alt+left")
	press(t, m, "alt+right")
	d.Undo()
	d.Undo()
	if d.Text() != before {
		t.Fatal("panel travel changed undo history")
	}
}

func TestPanelTravelParksSettingsHelpAndSources(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	m.openHelp()
	h := m.help
	press(t, m, "alt+right")
	if m.focus != 2 || m.helpVisible() || m.form != f || m.current().Name != "Saved" {
		t.Fatal("travel discarded or submitted an overlay")
	}
	m.chatEditing = true
	typeText(m, "draft")
	if m.draft().Text != "draft" || m.activeInteraction().owner != interactionChatInput {
		t.Fatal("parked Help captured chat input")
	}
	press(t, m, "alt+left")
	if !m.helpVisible() || m.help != h || m.form != f {
		t.Fatal("return did not restore Help above the form")
	}
	press(t, m, "esc")
	if m.help != nil || m.form != f || f.Fields[0].Value.Text != "Saved unfinished" {
		t.Fatal("Help did not restore the unfinished form")
	}
	press(t, m, "alt+left")
	press(t, m, "alt+right")
	if m.focus != 1 || m.form != f || m.current().Name != "Saved" {
		t.Fatal("Settings travel applied or lost form edits")
	}

	sources := sourceChatModel(t, 1)
	press(t, sources, "enter")
	s := sources.chatSources
	s.Scroll = 2
	sources.openHelp()
	press(t, sources, "alt+left")
	if sources.focus != 1 || sources.helpVisible() || sources.chatSourcesVisible() || sources.activeInteraction().owner == interactionSources {
		t.Fatal("parked Sources or Help still owns main")
	}
	if strings.Contains(ansi.Strip(sources.render()), "Global · Navigation") {
		t.Fatal("parked Help still covers main")
	}
	typeText(sources, "/needle")
	press(t, sources, "alt+right")
	if !sources.helpVisible() || sources.chatSources != s || s.Scroll != 2 {
		t.Fatal("return lost the overlay stack")
	}
	press(t, sources, "esc")
	if !sources.chatSourcesVisible() || sources.chatSources != s {
		t.Fatal("Help did not uncover Sources")
	}
	// Commands borrow main; travelling and cancelling must restore their origin.
	typeText(sources, ":h")
	press(t, sources, "alt+left")
	press(t, sources, "alt+right")
	if !sources.commandFocused() || !sources.chatSourcesVisible() || sources.command.Text != "h" {
		t.Fatal("travel lost the command inside Sources")
	}
	press(t, sources, "esc")
	if sources.focus != 2 || !sources.chatSourcesVisible() || sources.chatSources != s {
		t.Fatal("command cancel failed to restore Sources")
	}
}

func TestPanelTravelBypassesNativeCaptureAndKeepsTerminalTabsLocal(t *testing.T) {
	m, native := nativeModel(t)
	for _, size := range [][2]int{{160, 36}, {60, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "alt+left")
		if m.focus != 1 || !native.Captured || native.Finished {
			t.Fatal("travel released or closed the native session")
		}
		press(t, m, "alt+right")
		if m.focus != 2 || m.nativeAgent != native {
			t.Fatal("return replaced the native session")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	press(t, m, "tab")
	press(t, m, "shift+tab")
	typeText(m, "still typing")
	got := ""
	deadline := time.After(time.Second)
	for !strings.HasSuffix(got, "still typing") {
		select {
		case chunk := <-native.Input.Chunks:
			got += string(chunk)
		case <-deadline:
			t.Fatalf("missing native input: %q", got)
		}
	}
	if got != "\t\x1b[Zstill typing" || m.focus != 2 || !m.nativeAgentCaptured() {
		t.Fatalf("native terminal received panel shortcuts or lost Tab: %q", got)
	}

}

func TestPanelTravelLeavesExecCapturedAndRestoresMouse(t *testing.T) {
	m := exploreModel(t)
	terminal := reviewTerminal(t, 40, 10)
	m.explore.Terminal = terminal
	terminal.Captured = true
	press(t, m, "alt+left")
	if m.focus != 0 || m.mainTerminalCaptured() || !terminal.Captured || m.View().MouseMode == 0 {
		t.Fatal("background Exec retained application input ownership")
	}
	press(t, m, "alt+right")
	if m.focus != 1 || !m.mainTerminalCaptured() || m.explore.Terminal != terminal || m.View().MouseMode != 0 {
		t.Fatal("Exec capture did not resume on return")
	}
}

func TestPanelTravelRejectsPreviousOverlayMouseTargets(t *testing.T) {
	m, _ := testModel(t)
	m.chatEditing = true
	m.openHelp()
	old := mousePoint(t, m, "footer", "action/Esc")
	press(t, m, "alt+right")
	m.updateMouse(tea.MouseClickMsg{X: old.X, Y: old.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseReleaseMsg{X: old.X, Y: old.Y, Button: tea.MouseLeft})
	if m.focus != 2 || !m.chatEditing || m.help == nil {
		t.Fatal("a stale overlay target consumed input in the new panel")
	}
	clickMouse(t, m, "footer", "action/Esc")
	if m.chatEditing {
		t.Fatal("freshly rendered chat controls did not regain mouse input")
	}
}

func TestPanelTravelAcceptsMacArrowEncodings(t *testing.T) {
	for _, encoding := range []struct {
		name        string
		left, right rune
		mod         tea.KeyMod
	}{
		{"alt arrows", tea.KeyLeft, tea.KeyRight, tea.ModAlt},
		{"word arrows", 'b', 'f', tea.ModAlt},
		{"meta arrows", tea.KeyLeft, tea.KeyRight, tea.ModMeta},
	} {
		t.Run(encoding.name, func(t *testing.T) {
			m, n := nativeModel(t)
			for _, step := range []struct {
				code  rune
				focus int
			}{
				{encoding.left, 1}, {encoding.left, 0}, {encoding.left, 2},
				{encoding.right, 0}, {encoding.right, 1}, {encoding.right, 2},
			} {
				key := tea.KeyPressMsg{Code: step.code, Mod: encoding.mod}
				m.Update(key)
				if m.focus != step.focus || m.nativeAgent != n || n.Finished || !n.Captured {
					t.Fatalf("%s did not move to panel %d", key, step.focus)
				}
			}
			typeText(m, "still typing")
			got := ""
			deadline := time.After(time.Second)
			for !strings.HasSuffix(got, "still typing") {
				select {
				case chunk := <-n.Input.Chunks:
					got += string(chunk)
				case <-deadline:
					t.Fatalf("native input missing: %q", got)
				}
			}
			if got != "still typing" {
				t.Fatalf("panel shortcuts reached native input: %q", got)
			}
		})
	}
}

func TestAgentToggleReturnsToOriginAndRevealsHiddenPanel(t *testing.T) {
	for _, origin := range []int{0, 1} {
		for _, width := range []int{60, 100, 160} {
			m, _ := overviewModel(t)
			m.width, m.height, m.focus = width, 36, origin
			m.rightHidden = true
			m.draft().Insert("retained draft", true)
			m.shortcutPrefix = "i"
			press(t, m, "ctrl+]")
			if m.focus != 2 || m.rightHidden || !m.panelVisible(2) || m.shortcutPrefix != "" {
				t.Fatal("toggle did not reveal and focus the agent")
			}
			press(t, m, "ctrl+]")
			if m.focus != origin || m.draft().Text != "retained draft" || m.rightHidden {
				t.Fatal("return lost the origin, draft or panel visibility")
			}
		}
	}
	m, _ := testModel(t)
	m.focus = 0
	press(t, m, "ctrl+]")
	m.leftHidden = true
	press(t, m, "ctrl+]")
	if m.focus != 1 {
		t.Fatal("hidden origin did not fall back to main")
	}
	// Entry by existing navigation also establishes the return panel.
	m.leftHidden, m.focus = false, 0
	press(t, m, "alt+left")
	press(t, m, "ctrl+]")
	if m.focus != 0 {
		t.Fatal("toggle forgot the panel before Alt travel")
	}
}

func TestAgentToggleKeepsInputAndOverlayState(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		text, cursor, revision := d.Text(), d.Cursor, d.Revision
		press(t, m, "ctrl+]")
		press(t, m, "ctrl+]")
		if m.focus != 1 || m.explore.Action != action || d.Mode != mode || d.Text() != text || d.Cursor != cursor || d.Revision != revision {
			t.Fatal("toggle changed the editor")
		}
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	m.openHelp()
	h := m.help
	press(t, m, "ctrl+]")
	if m.focus != 2 || m.helpVisible() || m.form != f {
		t.Fatal("toggle did not park Help above the unfinished form")
	}
	m.chatEditing = true
	typeText(m, "draft")
	press(t, m, "ctrl+]")
	if m.focus != 1 || !m.helpVisible() || m.help != h || m.form != f || f.Fields[0].Value.Text != "Saved unfinished" || m.current().Name != "Saved" {
		t.Fatal("return lost or submitted the form")
	}
	press(t, m, "ctrl+]")
	typeText(m, " continued")
	if m.draft().Text != "draft continued" || !m.chatEditing {
		t.Fatal("toggle lost the chat input owner")
	}

	sources := sourceChatModel(t, 1)
	press(t, sources, "enter")
	s := sources.chatSources
	s.Scroll = 2
	press(t, sources, "ctrl+]")
	if sources.focus != 1 || sources.chatSourcesVisible() {
		t.Fatal("Sources did not park")
	}
	press(t, sources, "ctrl+]")
	if !sources.chatSourcesVisible() || sources.chatSources != s || s.Scroll != 2 {
		t.Fatal("toggle lost Sources")
	}
	typeText(sources, ":h")
	command := sources.command
	press(t, sources, "ctrl+]")
	press(t, sources, "ctrl+]")
	if !sources.commandFocused() || sources.command != command || command.Text != "h" {
		t.Fatal("toggle lost the colon draft")
	}
	press(t, sources, "esc")
	if sources.focus != 2 || !sources.chatSourcesVisible() {
		t.Fatal("colon cancellation lost its origin")
	}
}

func TestAgentToggleBypassesNativeInputAndSmallTerminal(t *testing.T) {
	m, n := nativeModel(t)
	for _, size := range [][2]int{{160, 36}, {60, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "ctrl+]")
		if m.focus != 1 || !n.Captured || n.Finished {
			t.Fatal("toggle did not leave the running terminal")
		}
		m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl, IsRepeat: true})
		if m.focus != 1 {
			t.Fatal("held toggle bounced focus back")
		}
		press(t, m, "ctrl+]")
		if m.focus != 2 || m.nativeAgent != n {
			t.Fatal("toggle did not return to the same session")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	typeText(m, "x")
	select {
	case chunk := <-n.Input.Chunks:
		if string(chunk) != "x" {
			t.Fatalf("toggle was forwarded to the CLI: %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("native input did not resume")
	}
}

func TestAgentToggleRestoresMainFullscreenAndPreservesExecEscape(t *testing.T) {
	m, _ := testModel(t)
	typeText(m, "ff")
	press(t, m, "ctrl+]")
	if m.focus != 2 || m.viewExpansion != nil || !m.panelVisible(2) {
		t.Fatal("fullscreen hid the focused agent")
	}
	press(t, m, "ctrl+]")
	if m.focus != 1 || m.viewExpanded() {
		t.Fatal("return did not restore main navigation")
	}
	m = exploreModel(t)
	tm := reviewTerminal(t, 40, 10)
	m.explore.Terminal, tm.Captured = tm, true
	press(t, m, "ctrl+]")
	if m.focus != 1 || tm.Captured || tm.Review == nil {
		t.Fatal("Exec lost its output-review escape")
	}
	press(t, m, "ctrl+]")
	if m.focus != 2 {
		t.Fatal("toggle unavailable from Exec review")
	}
}

func TestRetiredAssistantMainSizeRestoresAsDouble(t *testing.T) {
	m, _ := testModel(t)
	m.assistantSize = "main"
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	if restored.assistantSize != assistantDouble || restored.focus != 1 || !restored.panelVisible(1) {
		t.Fatal("saved takeover did not restore a usable layout")
	}
}

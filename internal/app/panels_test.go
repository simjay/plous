package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

func TestPanelToggleAliasesFromEveryPanel(t *testing.T) {
	for _, command := range []string{"b", "sidebar", "i", "aiinteraction"} {
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
			mainCommand(t, m, "i")
		}
		if err := m.saveSession(); err != nil {
			t.Fatal(err)
		}
		restored, err := newModel(m.store, "test-version")
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
				press(t, restored, "tab")
				if restored.focus != visible[(i+1)%len(visible)] {
					t.Fatal("Tab did not skip hidden panels")
				}
				press(t, restored, "shift+tab")
				if restored.focus != panel {
					t.Fatal("Shift+Tab did not skip hidden panels")
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
		restored, err := newModel(m.store, "test-version")
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
			if strings.Contains(lines[2], "Workspaces") == hidden[0] || strings.Contains(lines[2], "AI assistant") == hidden[1] {
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
		mainCommand(t, m, "i")
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
	mainCommand(t, m, "i")
	for section := 0; section < 3; section++ {
		for m.help.section != section {
			press(t, m, "right")
		}
	}
	view := ansi.Strip(m.render())
	for _, text := range []string{"1 Workspaces", "2 Views", "3 AI chat", "4 Global", ":b / :sidebar", ":i / :aiinteraction"} {
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
	draft := m.draft().text
	if m.leftHidden || m.rightHidden || draft != ":b :i :sidebar :aiinteraction :b" {
		t.Fatal("chat input executed a toggle")
	}
	press(t, m, "esc")
	for _, command := range []string{"c", "chat", "C", "newchat", "H", "openchat"} {
		mainCommand(t, m, "i")
		mainCommand(t, m, command)
		if m.rightHidden || m.focus != 2 || m.draft().text != draft {
			t.Fatalf(":%s failed to reveal chat and preserve the draft", command)
		}
		press(t, m, "esc")
	}
	openSettingsItem(t, m, "Rename workspace")
	typeText(m, ":b :i")
	f := m.form
	if m.leftHidden || m.rightHidden || f.fields[0].value.text != "Payments:b :i" {
		t.Fatal("form text executed a toggle")
	}
	press(t, m, "enter")
	typeText(m, ":i")
	press(t, m, "enter")
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
	conversation, err := m.store.NewConversation(id, "Test reply")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.store.StartTurn(id, conversation.ID, workspace.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	c := m.chat()
	c.conversation, c.turns = conversation, []workspace.Turn{turn}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	run := &chatRun{chat: c, cancel: cancel}
	m.run = run
	mainCommand(t, m, "i")
	m.Update(chatEvent{run: run, text: "Arrived while hidden"})
	if ctx.Err() != nil || m.run != run || !m.rightHidden {
		t.Fatal("hiding chat stopped the response or a response reopened chat")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	turns, err := m.store.Turns(id, conversation.ID)
	if err != nil || turns[0].Response.Text != "Arrived while hidden" {
		t.Fatalf("hidden response was not saved: %v", err)
	}
	mainCommand(t, m, "aiinteraction")
	if m.chat() != c || !strings.Contains(ansi.Strip(m.render()), "Arrived while hidden") {
		t.Fatal("restored chat lost its response")
	}
}

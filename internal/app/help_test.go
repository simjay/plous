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

func TestHelpOpensGloballyAndRestoresEachPanel(t *testing.T) {
	for focus := 0; focus < 3; focus++ {
		for _, command := range []string{"h", "help"} {
			t.Run(fmt.Sprintf("%d/%s", focus, command), func(t *testing.T) {
				m, _ := testModel(t)
				createWorkspace(t, m, "Payments")
				mainCommand(t, m, "3")
				press(t, m, "enter")
				detail, active := m.detail, m.active
				focusPanel(t, m, focus)
				typeText(m, ":"+command)
				press(t, m, "enter")
				if m.help == nil || m.focus != focus || m.detail != detail {
					t.Fatal("help did not preserve underlying view and focus")
				}
				press(t, m, "tab")
				press(t, m, "down")
				press(t, m, "esc")
				if m.help != nil || m.focus != focus || m.detail != detail || m.active != active || m.mode != 2 {
					t.Fatal("closing help lost the previous view")
				}
			})
		}
	}
}

func TestHelpPreservesFormAndIgnoresPaste(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename workspace")
	typeText(m, " unsaved")
	form := m.form
	press(t, m, "enter")
	typeText(m, ":h")
	press(t, m, "enter")
	m.Update(tea.PasteMsg{Content: ":quit\n"})
	press(t, m, "enter")
	if m.help == nil || m.command != nil || m.form != form {
		t.Fatal("help accepted paste or submitted the form")
	}
	typeText(m, ":s")
	press(t, m, "enter")
	if m.form != form || m.help == nil || !strings.Contains(m.status, "Finish or cancel") {
		t.Fatal("global settings discarded an unfinished form")
	}
	press(t, m, "esc")
	focusPanel(t, m, 1)
	if m.form != form || form.fields[0].value.text != "Payments unsaved" || m.current().Name != "Payments" {
		t.Fatal("help changed form edits or saved them")
	}
}

func TestGlobalCommandsAndChatTypingAreSeparate(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	focusPanel(t, m, 2)
	typeText(m, "ignored")
	m.Update(tea.PasteMsg{Content: ":q\n"})
	if m.draft().text != "" || m.chatEditing || m.command != nil {
		t.Fatal("chat navigation accepted text or paste")
	}
	for _, command := range []string{"c", "chat"} {
		typeText(m, ":"+command)
		press(t, m, "enter")
		if !m.chatEditing || m.focus != 2 {
			t.Fatal("chat command did not enter typing")
		}
		typeText(m, ":help")
		m.Update(tea.PasteMsg{Content: " :settings"})
		press(t, m, "ctrl+n")
		press(t, m, "ctrl+o")
		if m.help != nil || m.command != nil || m.chat().history || !strings.Contains(m.draft().text, ":help :settings") {
			t.Fatal("typing or retired Ctrl shortcuts invoked navigation")
		}
		press(t, m, "esc")
	}
	draft := m.draft().text
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || m.draft().text != draft {
		t.Fatal("Esc followed by help lost the draft")
	}
	press(t, m, "esc")
	for _, command := range []string{"s", "settings"} {
		focusPanel(t, m, 2)
		typeText(m, ":"+command)
		press(t, m, "enter")
		if m.page != "settings" || m.focus != 1 || m.draft().text != draft {
			t.Fatal("global settings failed")
		}
	}
}

func TestChatActionAliasesAndGlobalStop(t *testing.T) {
	for _, short := range []bool{true, false} {
		m, _ := testModel(t)
		id := createWorkspace(t, m, "Payments")
		conversation, err := m.store.NewConversation(id, "Saved chat")
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
		m.run = &chatRun{chat: c, cancel: cancel}
		startChat(t, m)
		typeText(m, "Keep this draft")
		press(t, m, "esc")
		if ctx.Err() != nil {
			t.Fatal("leaving input stopped a response")
		}
		commands := []string{"stopchat", "newchat", "openchat"}
		if short {
			commands = []string{"X", "C", "H"}
		}
		focusPanel(t, m, 0)
		typeText(m, ":"+commands[0])
		press(t, m, "enter")
		if ctx.Err() == nil || m.run != nil {
			t.Fatal("global stop did not cancel the response")
		}
		saved, err := m.store.Turns(id, conversation.ID)
		if err != nil || saved[0].Response.Status != "cancelled" {
			t.Fatal("stop did not persist the response", err)
		}
		typeText(m, ":"+commands[1])
		press(t, m, "enter")
		if m.chat().conversation.ID != "" || !m.chatEditing || m.focus != 2 || m.draft().text != "Keep this draft" {
			t.Fatal("new chat did not preserve the draft and enter input")
		}
		press(t, m, "esc")
		typeText(m, ":"+commands[2])
		press(t, m, "enter")
		if !m.chat().history || m.chatEditing || len(m.chat().choices) != 1 {
			t.Fatal("open chat did not show history")
		}
		press(t, m, "enter")
		if m.chat().conversation.ID != conversation.ID || m.chatEditing {
			t.Fatal("history selection did not reopen the conversation in navigation")
		}
	}
}

func TestHelpFourSectionsFitAndCanBeRead(t *testing.T) {
	m, _ := testModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mainCommand(t, m, "help")
		for _, editingCommand := range []bool{false, true} {
			if editingCommand {
				typeText(m, ":s")
			}
			view := ansi.Strip(m.render())
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Fatalf("help height %d at %v", len(lines), size)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatalf("help row overflow at %v: %q", size, line)
				}
			}
			for _, label := range []string{"1 Workspaces", "2 Views", "3 AI chat", "4 Global", ":settings", ":help", ":quit", ":b / :sidebar", ":i / :aiinteraction", "bb ii ss cc hh", "11 22 33"} {
				if !strings.Contains(view, label) {
					t.Fatalf("missing %q at %v", label, size)
				}
			}
			window := strings.Split(ansi.Strip(m.helpPanels()), "\n")
			width, height := m.helpDimensions()
			if width >= size[0] || height > size[1]-2 || len(window) != height {
				t.Fatalf("Help must leave visible space around its overlay at %v", size)
			}
			if strings.Count(window[len(window)-1], "╰") != 1 || strings.Count(window[0], "╭") != 3 {
				t.Fatal("Help must retain three upper sections and one full-width global box")
			}
			footerLabel := "Help"
			if editingCommand {
				footerLabel = "Command"
			}
			if !strings.Contains(lines[0], "plous") || !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), footerLabel+"  ") {
				t.Fatal("Help must preserve the application header and single footer")
			}
			if editingCommand && !strings.Contains(view, ":s▏") {
				t.Fatal("command editor was hidden by Help")
			}
			if editingCommand {
				press(t, m, "esc")
			}
		}
		for focus := 0; focus < 3; focus++ {
			for m.help.section != focus {
				press(t, m, "right")
			}
			press(t, m, "end")
			if m.help.scroll[focus] < 0 {
				t.Fatal("invalid scroll offset")
			}
			press(t, m, "home")
			if m.help.scroll[focus] != 0 {
				t.Fatal("help cannot return to the first line")
			}
		}
		press(t, m, "esc")
	}
}

func TestHelpOverlayReturnsToSettingsAndPreservesBackground(t *testing.T) {
	m, _ := testModel(t)
	m.openSettings()
	m.beginAppSettings()
	typeText(m, " unsaved")
	m.form.index = len(m.form.fields)
	before := ansi.Strip(m.render())
	form, settings := m.form, m.settingsOverlay
	typeText(m, "hh")
	if m.help == nil || m.form != form || m.settingsOverlay != settings {
		t.Fatal("hh must open Help over Settings without changing its edits")
	}
	view := strings.Split(ansi.Strip(m.render()), "\n")
	background := strings.Split(before, "\n")
	width, height := m.helpDimensions()
	x, y := (m.width-width)/2, (m.height-height)/2
	for row := 0; row < m.height-1; row++ {
		if row < y || row >= y+height {
			if view[row] != background[row] {
				t.Fatalf("Help replaced the underlying view outside the overlay at row %d", row)
			}
		} else if ansi.Cut(view[row], 0, x) != ansi.Cut(background[row], 0, x) || ansi.Cut(view[row], x+width, m.width) != ansi.Cut(background[row], x+width, m.width) {
			t.Fatalf("Help replaced the background beside the overlay at row %d", row)
		}
	}
	press(t, m, "tab")
	press(t, m, "down")
	help := m.help
	typeText(m, "hh")
	if m.help != help {
		t.Fatal("reopening Help must preserve its original return view")
	}
	press(t, m, "esc")
	if m.help != nil || m.form != form || m.settingsOverlay != settings || m.focus != 1 || ansi.Strip(m.render()) != before {
		t.Fatal("closing Help must return to the same Settings overlay and edits")
	}
}

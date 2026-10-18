package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestDoubleKeyActionsFromEveryPanel(t *testing.T) {
	for _, key := range []string{"b", "i", "s", "c", "h", "f", "1", "2", "3"} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("%s%s/from%d", key, key, focus), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				mainCommand(t, m, "1")
				beforeMode, afterMode := 0, 0
				if key >= "1" && key <= "3" {
					afterMode = int(key[0] - '1')
					beforeMode = (afterMode + 1) % len(modes)
					m.mode = beforeMode
				}
				focusPanel(t, m, focus)
				typeText(m, key)
				if m.shortcutPrefix != key || m.focus != focus || m.leftHidden || m.rightHidden || m.chatEditing || m.help != nil || m.page != "" || m.mode != beforeMode || m.viewExpansion != nil {
					t.Fatal("the first key executed an action")
				}
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != m.height || !strings.Contains(lines[len(lines)-1], key+" → "+key) {
					t.Fatal("the pending shortcut must appear in the single footer")
				}
				m.Update(autosaveMsg{})
				typeText(m, key)
				if m.shortcutPrefix != "" || m.command != nil || m.active != id || m.mode != afterMode {
					t.Fatal("completed shortcut changed scope or left a pending command")
				}
				switch key {
				case "f":
					if !m.viewExpanded() || m.focus != 1 || m.panelWidths() != [3]int{0, m.width, 0} {
						t.Fatal("ff did not expand and focus main")
					}
				case "1", "2", "3":
					if m.focus != 1 || m.page != "" {
						t.Fatal("mode shortcut did not focus the main view")
					}
				case "b":
					if !m.leftHidden || m.focus == 0 {
						t.Fatal("bb did not hide the sidebar and repair focus")
					}
				case "i":
					if m.rightHidden || m.assistantSize != assistantDouble || m.focus != focus {
						t.Fatal("ii did not expand AI while preserving focus")
					}
				case "s":
					if m.page != "settings" || m.focus != 1 {
						t.Fatal("ss did not open Settings")
					}
				case "h":
					if m.help == nil || m.focus != focus {
						t.Fatal("hh did not open Help while preserving focus")
					}
				case "c":
					if m.focus != 2 || !m.chatEditing {
						t.Fatal("cc did not enter the composer")
					}
					typeText(m, "bb ii ss cc hh 11 22 33 44")
					if m.draft().Text != "bb ii ss cc hh 11 22 33 44" || m.leftHidden || m.rightHidden {
						t.Fatal("shortcut letters stopped being literal after entering chat")
					}
				}
			})
		}
	}
}

func TestRetiredFourthModeShortcut(t *testing.T) {
	for focus := 0; focus < 3; focus++ {
		m, _ := testModel(t)
		mainCommand(t, m, "2")
		focusPanel(t, m, focus)
		typeText(m, "44")
		if m.mode != 1 || m.focus != focus || m.shortcutPrefix != "" {
			t.Fatal("44 must not change mode or begin a shortcut")
		}
	}
}

func TestDoubleKeyPrefixClearsOnInterveningInput(t *testing.T) {
	for _, event := range []string{"esc", "down", "x", "tab", "shift+tab", "paste", "resize", "blur", "colon"} {
		t.Run(event, func(t *testing.T) {
			m, _ := testModel(t)
			mainCommand(t, m, "s")
			typeText(m, "b")
			switch event {
			case "paste":
				m.Update(tea.PasteMsg{Content: "bb ii ss cc hh 11 22 33 44"})
			case "resize":
				m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
			case "blur":
				m.Update(tea.BlurMsg{})
			case "colon":
				typeText(m, ":")
				press(t, m, "esc")
			case "x":
				typeText(m, "x")
			default:
				press(t, m, event)
			}
			if m.shortcutPrefix != "" || m.page != "settings" {
				t.Fatal("intervening input did not clear the prefix, or Esc also closed Settings")
			}
			typeText(m, "b")
			if m.leftHidden || m.shortcutPrefix != "b" {
				t.Fatal("a key completed a sequence from before the interruption")
			}
		})
	}
	m, _ := testModel(t)
	typeText(m, "bii")
	if m.leftHidden || m.assistantSize != assistantDouble || m.shortcutPrefix != "" {
		t.Fatal("a different shortcut key did not begin a fresh sequence")
	}
	typeText(m, "b")
	m.Update(tea.KeyPressMsg{Code: 'b', Text: "b", IsRepeat: true})
	if m.leftHidden {
		t.Fatal("a reported key repeat completed a shortcut")
	}
	typeText(m, "b")
	if !m.leftHidden {
		t.Fatal("a separate keypress did not complete the shortcut")
	}
}

func TestDoubleKeysStayLiteralInEditors(t *testing.T) {
	for _, editor := range []string{"form", "chat", "command"} {
		t.Run(editor, func(t *testing.T) {
			m, _ := testModel(t)
			createWorkspace(t, m, "Payments")
			mainCommand(t, m, "1")
			switch editor {
			case "form":
				mainCommand(t, m, "new")
			case "chat":
				typeText(m, "cc")
			case "command":
				typeText(m, ":")
			}
			typeText(m, "bb ii ss cc hh 11 22 33 44")
			if m.shortcutPrefix != "" || m.leftHidden || m.rightHidden || m.page != "" || m.help != nil || m.mode != 0 {
				t.Fatal("editor input triggered a navigation action")
			}
			var value string
			switch editor {
			case "form":
				value = m.form.Fields[0].Value.Text
			case "chat":
				value = m.draft().Text
			case "command":
				value = m.command.Text
			}
			if value != "bb ii ss cc hh 11 22 33 44" {
				t.Fatalf("editor changed literal text: %q", value)
			}
		})
	}
}

func TestDoubleKeyCommandsPreserveFormsAndReplacePendingCommands(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unsaved")
	f := m.form
	press(t, m, "enter")
	for _, sequence := range []string{"ss", "11", "22", "33"} {
		typeText(m, sequence)
		if m.form != f || !strings.Contains(m.status, "Finish or cancel") || m.mode != mode {
			t.Fatalf("%s bypassed the unfinished-form guard", sequence)
		}
	}
	typeText(m, "b")
	press(t, m, "esc")
	if m.form != f {
		t.Fatal("cancelling the prefix cancelled the form")
	}
	typeText(m, "iicc")
	if m.form != f || m.current().Name != "Payments" || m.rightHidden || m.focus != 2 || !m.chatEditing {
		t.Fatal("form actions failed to reveal chat while preserving edits")
	}
	press(t, m, "esc")
	focusPanel(t, m, 1)
	press(t, m, "esc")
	press(t, m, "esc")
	typeText(m, ":q")
	focusPanel(t, m, 0)
	typeText(m, "ss")
	if m.command != nil || m.page != "settings" || m.focus != 1 {
		t.Fatal("completed shortcut retained a stale command instead of opening Settings")
	}
}

func TestDoubleKeysInHelpAndHistory(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "help")
	typeText(m, "b")
	press(t, m, "esc")
	if m.help == nil {
		t.Fatal("prefix cancellation closed Help")
	}
	typeText(m, "bbiiii")
	if m.help == nil || !m.leftHidden || !m.rightHidden {
		t.Fatal("help did not apply visibility changes to the underlying layout")
	}
	typeText(m, "cc")
	if m.help != nil || m.rightHidden || m.focus != 2 || !m.chatEditing {
		t.Fatal("cc from Help failed to open chat")
	}
	press(t, m, "esc")
	mainCommand(t, m, "H")
	typeText(m, "cc")
	if m.chat().History || !m.chatEditing {
		t.Fatal("cc from history failed to enter the composer")
	}
}

func TestDoubleKeyPrefixDoesNotSurviveRestartOrSmallTerminal(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	typeText(m, "b")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if restored.shortcutPrefix != "" {
		t.Fatal("restart restored an incomplete shortcut")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	typeText(m, "bb iiss cc hh 11 22 33 44")
	if m.shortcutPrefix != "" || m.leftHidden || m.rightHidden || m.page != "" || m.chatEditing || m.mode != mode {
		t.Fatal("small-terminal fallback executed hidden actions")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	typeText(m, "b")
	assertCtrlCQuit(t, m)
}

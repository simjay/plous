package app

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestNumericModeCommandsFromEveryPanel(t *testing.T) {
	for mode, command := range []string{"1", "2", "3"} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("%s/from%d", command, focus), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				press(t, m, "esc")
				m.mode, m.page, m.detail = (mode+1)%len(modes), "help", "previous detail"
				focusPanel(t, m, focus)
				typeText(m, ":"+command)
				if cmd := press(t, m, "enter"); cmd != nil {
					if _, quit := cmd().(tea.QuitMsg); quit {
						t.Fatal("switching modes must not quit")
					}
				}
				if m.mode != mode || m.page != "" || m.detail != "" || m.active != id || m.focus != 1 || m.command != nil {
					t.Fatalf(":%s did not select %s and preserve workspace scope", command, modes[mode])
				}
			})
		}
	}
}

func TestMainCommandAliases(t *testing.T) {
	for _, tc := range []struct {
		aliases []string
		page    string
		form    bool
	}{{[]string{"n", "new"}, "", true}, {[]string{"s", "settings"}, "settings", false}} {
		for _, alias := range tc.aliases {
			t.Run(alias, func(t *testing.T) {
				m, _ := testModel(t)
				createWorkspace(t, m, "Payments")
				mainCommand(t, m, "1")
				mainCommand(t, m, alias)
				if (m.form != nil) != tc.form || (!tc.form && m.page != tc.page) || m.command != nil {
					t.Fatalf(":%s opened wrong screen: page=%q form=%#v", alias, m.page, m.form)
				}
			})
		}
	}
}

func TestQuitAliasesSaveDraftBeforeExit(t *testing.T) {
	for _, alias := range []string{"q", "quit"} {
		t.Run(alias, func(t *testing.T) {
			m, _ := testModel(t)
			id := createWorkspace(t, m, "Payments")
			startChat(t, m)
			typeText(m, "unsaved chat draft :q")
			mainCommand(t, m, "3")
			mainCommand(t, m, alias)
			m.drainPersistence()
			cmd := m.syncQuit()
			if cmd == nil {
				t.Fatalf(":%s must request exit", alias)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf(":%s did not return a quit message", alias)
			}
			data, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			var saved session
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Drafts[m.conversationScope()] != "unsaved chat draft :q" || saved.Mode != 2 || saved.Workspace != id {
				t.Fatalf(":%s must save the latest draft and navigation before exit: %#v", alias, saved)
			}
		})
	}
}

func TestMainCommandAliasesRequireExactMatches(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	for _, text := range []string{"qu", "over", "resource", "set", "ne", "he", "q extra", "config", "ch", "st", "o", "overview", "res", "resources", "e", "explore", "activity", "4", "44", "left", "right", "side", "ai", "bb", "ii", "ss", "cc", "z", "zoom", "zz", "ff"} {
		if cmd := mainCommand(t, m, text); cmd != nil {
			t.Fatalf("partial or extended command %q must not quit", text)
		}
		if !strings.Contains(m.status, "Unknown command") || m.mode != 0 || m.page != "" || m.form != nil || m.leftHidden || m.rightHidden {
			t.Fatalf("partial or extended command %q must not invoke any action: %q", text, m.status)
		}
	}
}

func TestCommandDraftSurvivesFocusChangesAndCancels(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	startChat(t, m)
	mainCommand(t, m, "1")
	typeText(m, ":")
	press(t, m, "alt+right")
	typeText(m, ":q")
	press(t, m, "alt+right")
	typeText(m, "a")
	if m.command == nil || m.command.Text != "" || m.mode != 0 || m.draft().Text != ":q" {
		t.Fatal("typing in chat/tree must leave the main command draft and mode alone")
	}
	press(t, m, "alt+right")
	typeText(m, "2")
	press(t, m, "enter")
	if m.mode != 1 || m.command != nil {
		t.Fatal("returning to main should resume and execute its command draft")
	}
	typeText(m, ":q")
	if cmd := press(t, m, "esc"); cmd != nil || m.command != nil || m.mode != 1 {
		t.Fatal("Esc must close the command box without executing the command")
	}
	mainCommand(t, m, "n")
	typeText(m, ":q :4 :settings")
	if m.form == nil || m.form.Fields[0].Value.Text != ":q :4 :settings" || m.command != nil || m.mode != 1 {
		t.Fatal("aliases must remain literal text inside forms")
	}
}

func TestChatCommandFromFormActionPreservesEdits(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unsaved")
	f := m.form
	press(t, m, "enter")
	typeText(m, ":c")
	press(t, m, "enter")
	if m.focus != 2 || !m.chatEditing || m.form != f || m.current().Name != "Payments" {
		t.Fatal("chat command from a form action must focus chat without submitting the form")
	}
	if f.Fields[0].Value.Text != "Payments unsaved" {
		t.Fatal("opening chat lost unfinished form edits")
	}
}

func TestFreshChatCommandReplacesPendingCommandFromAnotherPanel(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	for _, origin := range []int{0, 2} {
		mainCommand(t, m, "1")
		typeText(m, ":s")
		focusPanel(t, m, origin)
		typeText(m, ":c")
		press(t, m, "enter")
		if m.focus != 2 || !m.chatEditing || m.command != nil {
			t.Fatal("fresh :c appended to a pending command instead of entering chat")
		}
		press(t, m, "esc")
		mainCommand(t, m, "1")
		typeText(m, ":s")
		focusPanel(t, m, origin)
		typeText(m, ":c")
		press(t, m, "esc")
		if m.focus != origin || m.command != nil {
			t.Fatal("cancelling a fresh command did not restore its originating panel")
		}
	}
}

func TestSmallTerminalAllowsQuitAliasOnly(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	mainCommand(t, m, "n")
	if m.form != nil || len(m.workspaces) != 1 {
		t.Fatal("a small terminal must not create an invisible workspace form")
	}
	mainCommand(t, m, "q")
	m.drainPersistence()
	cmd := m.syncQuit()
	if cmd == nil {
		t.Fatal("the short quit command must still work in a small terminal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("the short quit command did not request exit")
	}
}

func TestCommandBoxStaysAtMainPanelBottom(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			left := min(26, max(18, size[0]/6))
			right := min(48, max(24, size[0]*3/10))
			width, height := size[0]-left-right, size[1]-2
			before := strings.Split(ansi.Strip(m.mainPanel(width, height)), "\n")
			typeText(m, ":q")
			lines := strings.Split(ansi.Strip(m.mainPanel(width, height)), "\n")
			if len(lines) != height || lines[1] != before[1] {
				t.Fatal("opening command input must preserve panel height and mode tabs")
			}
			if !strings.Contains(strings.Join(lines, "\n"), "revision 1") {
				t.Fatal("current view content must remain above the command input")
			}
			for _, row := range []int{height - 4, height - 3, height - 2} {
				if ansi.StringWidth(lines[row]) != width || !strings.HasPrefix(lines[row], "│ ") || !strings.HasSuffix(lines[row], " │") {
					t.Fatalf("command box exceeds main body at row %d: %q", row, lines[row])
				}
			}
			if !strings.HasPrefix(lines[height-4], "│ ╭") || !strings.HasSuffix(lines[height-4], "╮ │") ||
				!strings.Contains(lines[height-3], ":q▏") || !strings.HasPrefix(lines[height-2], "│ ╰") || !strings.HasSuffix(lines[height-2], "╯ │") {
				t.Fatalf("command must use a three-row rounded box directly above panel bottom:\n%s", strings.Join(lines[height-4:], "\n"))
			}
			full := strings.Split(ansi.Strip(m.render()), "\n")
			if len(full) != size[1] || strings.Count(full[len(full)-2], "╰") != 3 || !strings.Contains(full[len(full)-1], "Enter Run") {
				t.Fatal("command box must stay within the main panel and preserve one footer")
			}
			press(t, m, "esc")
		})
	}
}

func TestCommandBoxKeepsWelcomeInPlace(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.closeSettings()
	m.mode = 1
	for _, size := range [][2]int{{80, 16}, {100, 19}, {120, 30}, {200, 60}} {
		for focus := range 3 {
			t.Run(fmt.Sprintf("%dx%d/focus%d", size[0], size[1], focus), func(t *testing.T) {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m.command = nil
				m.focus = focus
				width, height := m.panelWidths()[1], m.height-2
				render := func() string { return ansi.Strip(m.mainPanel(width, height)) }
				before := render()
				if !strings.Contains(before, "█") {
					t.Fatal("expected welcome artwork before opening the command box")
				}
				for _, input := range []string{":", "help"} {
					typeText(m, input)
					if m.command == nil {
						t.Fatal("command box did not open")
					}
					lines := strings.Split(render(), "\n")
					if len(lines) != height || strings.Join(lines[:height-4], "\n") != strings.Join(strings.Split(before, "\n")[:height-4], "\n") {
						t.Fatal("opening or typing a command moved the welcome content")
					}
				}
				press(t, m, "esc")
				if m.focus != focus || render() != before {
					t.Fatal("closing the command box did not restore the same view and focus")
				}
			})
		}
	}
}

func TestCommandBoxScrollsToCursor(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	typeText(m, ":start-"+strings.Repeat("界x", 100)+"-end")
	for _, width := range []int{38, 60, 124} {
		press(t, m, "end")
		lines := strings.Split(ansi.Strip(m.mainPanel(width, 14)), "\n")
		if !strings.Contains(lines[11], "-end▏") || strings.Contains(lines[11], "start-") || ansi.StringWidth(lines[11]) != width {
			t.Fatalf("command input must reveal the end cursor without overflowing: %q", lines[11])
		}
		press(t, m, "home")
		lines = strings.Split(ansi.Strip(m.mainPanel(width, 14)), "\n")
		if !strings.Contains(lines[11], ":▏start-") || strings.Contains(lines[11], "-end") || ansi.StringWidth(lines[11]) != width {
			t.Fatalf("command input must reveal the start cursor without overflowing: %q", lines[11])
		}
	}
}

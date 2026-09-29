package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMainCommandAliases(t *testing.T) {
	for mode, aliases := range [][]string{{"1", "o", "overview"}, {"2", "r", "resources"}, {"3", "d", "deployment"}, {"4", "a", "activity"}} {
		for _, alias := range aliases {
			t.Run(alias, func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				m.mode, m.page, m.detail = (mode+1)%4, "help", "previous detail"
				if cmd := mainCommand(t, m, alias); cmd != nil {
					t.Fatal("switching modes must not quit")
				}
				if m.mode != mode || m.page != "" || m.detail != "" || m.active != id || m.focus != 1 || m.command != nil {
					t.Fatalf(":%s did not select %s and preserve workspace scope", alias, modes[mode])
				}
			})
		}
	}
	for _, tc := range []struct {
		aliases []string
		page    string
		form    bool
	}{{[]string{"n", "new"}, "", true}, {[]string{"s", "settings"}, "settings", false}, {[]string{"h", "help"}, "help", false}} {
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
			focusPanel(t, m, 2)
			typeText(m, "unsaved chat draft :q")
			mainCommand(t, m, "activity")
			cmd := mainCommand(t, m, alias)
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
			if saved.Drafts[id] != "unsaved chat draft :q" || saved.Mode != 3 || saved.Workspace != id {
				t.Fatalf(":%s must save the latest draft and navigation before exit: %#v", alias, saved)
			}
		})
	}
}

func TestMainCommandAliasesRequireExactMatches(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	for _, text := range []string{"qu", "over", "resource", "deploy", "set", "ne", "he", "q extra"} {
		if cmd := mainCommand(t, m, text); cmd != nil {
			t.Fatalf("partial or extended command %q must not quit", text)
		}
		if !strings.Contains(m.status, "Unknown command") || m.mode != 0 || m.page != "" || m.form != nil {
			t.Fatalf("partial or extended command %q must not invoke any action: %q", text, m.status)
		}
	}
}

func TestCommandDraftSurvivesFocusChangesAndCancels(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "o")
	typeText(m, ":res")
	press(t, m, "tab")
	typeText(m, ":q")
	press(t, m, "tab")
	typeText(m, ":a")
	if m.command == nil || m.command.text != "res" || m.mode != 0 || m.draft().text != ":q" {
		t.Fatal("typing in chat/tree must leave the main command draft and mode alone")
	}
	press(t, m, "tab")
	typeText(m, "ources")
	press(t, m, "enter")
	if m.mode != 1 || m.command != nil {
		t.Fatal("returning to main should resume and execute its command draft")
	}
	for _, cancel := range []string{"esc", "ctrl+c"} {
		typeText(m, ":q")
		if cmd := press(t, m, cancel); cmd != nil || m.command != nil || m.mode != 1 {
			t.Fatalf("%s must close the command box without executing the command", cancel)
		}
	}
	mainCommand(t, m, "n")
	typeText(m, ":q :a :settings")
	if m.form == nil || m.form.fields[0].value.text != ":q :a :settings" || m.command != nil || m.mode != 1 {
		t.Fatal("aliases must remain literal text inside forms")
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
	cmd := mainCommand(t, m, "q")
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
			left := min(28, max(18, size[0]/5))
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
			if len(full) != size[1] || strings.Count(full[len(full)-2], "└") != 3 || !strings.Contains(full[len(full)-1], "Tab / Shift+Tab panels") {
				t.Fatal("command box must stay within the main panel and preserve one footer")
			}
			press(t, m, "esc")
		})
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

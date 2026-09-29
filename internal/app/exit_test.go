package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCtrlCQuitsFromEveryPanelAndSavesSession(t *testing.T) {
	for _, small := range []bool{false, true} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("focus=%d/small=%t", focus, small), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				focusPanel(t, m, 2)
				typeText(m, "latest draft\n:q is literal")
				mainCommand(t, m, "activity")
				focusPanel(t, m, focus)
				if small {
					m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
				}
				assertCtrlCQuit(t, m)
				saved := readExitSession(t, m)
				if saved.Workspace != id || saved.Mode != 3 || saved.Focus != focus || saved.Drafts[id] != "latest draft\n:q is literal" {
					t.Fatalf("Ctrl+C must save the latest session before exit: %#v", saved)
				}
			})
		}
	}
}

func TestCtrlCQuitsWithUnfinishedCommand(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "activity")
	typeText(m, ":new")
	assertCtrlCQuit(t, m)
	if m.form != nil || readExitSession(t, m).Mode != 3 {
		t.Fatal("Ctrl+C must exit without executing the unfinished command")
	}
}

func TestCtrlCDoesNotApplyUnfinishedForm(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Payments")
	m.beginRename()
	press(t, m, "ctrl+u")
	typeText(m, "Unsubmitted name")
	focusPanel(t, m, 2)
	typeText(m, "keep this chat draft")
	focusPanel(t, m, 1)
	assertCtrlCQuit(t, m)
	workspaces, err := m.store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "Payments" || workspaces[0].Revision != 1 {
		t.Fatalf("exit must not submit unfinished form edits: %#v", workspaces)
	}
	if readExitSession(t, m).Drafts[id] != "keep this chat draft" {
		t.Fatal("exit from a form must still save the latest chat draft")
	}
}

func TestCtrlCQuitsDuringFirstRun(t *testing.T) {
	for _, editing := range []bool{false, true} {
		t.Run(fmt.Sprintf("editing=%t", editing), func(t *testing.T) {
			m, _ := testModel(t)
			if editing {
				press(t, m, "enter")
				typeText(m, "Unsubmitted workspace")
			}
			assertCtrlCQuit(t, m)
			workspaces, err := m.store.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(workspaces) != 0 || readExitSession(t, m).Workspace != "" {
				t.Fatal("exit during first run must not create an unfinished workspace")
			}
		})
	}
}

func TestCtrlCSaveFailureKeepsAppOpenAndDraftRecoverable(t *testing.T) {
	for _, small := range []bool{false, true} {
		t.Run(fmt.Sprintf("small=%t", small), func(t *testing.T) {
			m, root := testModel(t)
			id := createWorkspace(t, m, "Payments")
			focusPanel(t, m, 2)
			typeText(m, "draft survives storage failure")
			if small {
				m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
			}
			// A directory at the session path causes a real storage failure,
			// independently of platform permission behavior or test privileges.
			path := filepath.Join(root, "session.json")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if cmd := press(t, m, "ctrl+c"); cmd != nil {
				t.Fatal("failed session save must leave the application open")
			}
			if !strings.Contains(ansi.Strip(m.render()), "Cannot save before exit") {
				t.Fatalf("failed exit must show its storage error: %q", m.status)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			assertCtrlCQuit(t, m)
			if readExitSession(t, m).Drafts[id] != "draft survives storage failure" {
				t.Fatal("retrying exit after storage recovery lost the draft")
			}
		})
	}
}

func assertCtrlCQuit(t *testing.T, m *model) {
	t.Helper()
	cmd := press(t, m, "ctrl+c")
	if cmd == nil {
		t.Fatalf("Ctrl+C did not request exit: %s", m.status)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not return a quit message")
	}
}

func readExitSession(t *testing.T, m *model) session {
	t.Helper()
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	return saved
}

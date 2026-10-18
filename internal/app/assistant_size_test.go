package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

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

package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestLayoutFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 12}, {100, 24}, {120, 40}, {200, 60}, {79, 24}, {120, 11}, {10, 3}, {1, 1}, {0, 0}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := model{width: size[0], height: size[1], focus: 1}
			view := m.View()
			if !view.AltScreen {
				t.Fatal("must use the alternate screen")
			}
			if size[0] == 0 {
				if view.Content != "" {
					t.Fatal("must wait for terminal size")
				}
				return
			}
			if w, h := lipgloss.Size(view.Content); w != size[0] || h != size[1] {
				t.Fatalf("layout is %dx%d, terminal is %dx%d", w, h, size[0], size[1])
			}
			if size[0] >= minWidth && size[1] >= minHeight {
				lines := strings.Split(view.Content, "\n")
				for _, title := range []string{"List", "Main", "AI assistant"} {
					if !strings.Contains(lines[2], title) {
						t.Errorf("missing side-by-side title %q", title)
					}
				}
				if !strings.Contains(view.Content, "welcome to plous") {
					t.Error("missing welcome")
				}
			}
		})
	}
}

func TestResizeAndNavigation(t *testing.T) {
	var current tea.Model = model{focus: 1}
	current, _ = current.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, want := range []int{2, 0, 1} {
		current, _ = current.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if current.(model).focus != want {
			t.Fatalf("focus should cycle to %d", want)
		}
	}
	current, _ = current.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if current.(model).focus != 0 {
		t.Fatal("shift+tab should move backward")
	}
	current, _ = current.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	if w, h := lipgloss.Size(current.View().Content); w != 80 || h != 12 {
		t.Fatal("resize did not reflow")
	}
	for _, key := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: 'c', Mod: tea.ModCtrl}} {
		_, cmd := current.Update(key)
		if cmd == nil {
			t.Fatal("quit key returned no command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("quit key did not quit")
		}
	}
}

package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMainPanelTabsFillTopRow(t *testing.T) {
	m, _ := testModel(t)
	for _, width := range []int{38, 60, 124} {
		for mode := range modes {
			t.Run(fmt.Sprintf("width%d/mode%d", width, mode), func(t *testing.T) {
				m.mode = mode
				lines := strings.Split(ansi.Strip(m.mainPanel(width, 14)), "\n")
				if strings.Contains(lines[1], "Main") {
					t.Fatal("the main panel title must be replaced by tabs")
				}
				cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(lines[1], "│"), "│"), "│")
				if len(cells) != len(modes) {
					t.Fatalf("want four tabs on one row, got %q", lines[1])
				}
				shortest, longest := width, 0
				for i, cell := range cells {
					cellWidth := ansi.StringWidth(cell)
					shortest, longest = min(shortest, cellWidth), max(longest, cellWidth)
					shortcut := fmt.Sprintf(":%d", i+1)
					if !strings.HasSuffix(cell, shortcut) || strings.Count(lines[1], shortcut) != 1 {
						t.Errorf("tab %d must have its shortcut aligned at the right: %q", i+1, cell)
					}
					if !strings.HasPrefix(cell, modes[i][:1]) || strings.HasPrefix(cell, " ") {
						t.Errorf("tab %d must start with its mode name: %q", i+1, cell)
					}
					if width >= 60 && !strings.HasPrefix(cell, modes[i]+" ") {
						t.Errorf("full mode name should fit at this width: %q", cell)
					}
				}
				if longest-shortest > 1 || ansi.StringWidth(lines[1]) != width {
					t.Errorf("tabs must evenly fill the panel: %q", lines[1])
				}
			})
		}
	}
}

func TestChatComposerRemainsAtBottom(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	drafts := []string{"", "short draft", "first-line\n" + strings.Repeat("middle\n", 20) + "last-line"}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		bottomRow := -1
		for focus := 0; focus < 3; focus++ {
			for i, draft := range drafts {
				t.Run(fmt.Sprintf("%dx%d/focus%d/draft%d", size[0], size[1], focus, i), func(t *testing.T) {
					m.focus, m.chatEditing = focus, true
					*m.draft() = newEditor(draft)
					lines := strings.Split(ansi.Strip(m.render()), "\n")
					found := -1
					for row, line := range lines {
						if strings.Contains(line, "╰") && strings.Contains(line, "╯") {
							found = row
						}
					}
					if found != size[1]-3 {
						t.Fatalf("composer bottom is at row %d; want immediately above panel bottom at %d", found, size[1]-3)
					}
					if bottomRow >= 0 && found != bottomRow {
						t.Fatal("draft size or focus moved the composer")
					}
					bottomRow = found
					if i == 2 && !strings.Contains(strings.Join(lines, "\n"), "last-line") {
						t.Fatal("the latest draft line must remain visible when the draft scrolls")
					}
				})
			}
		}
	}
}

func TestChatComposerFollowsEditingCursor(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.focus, m.chatEditing = 2, true
	*m.draft() = newEditor("first-line\n" + strings.Repeat("middle\n", 20) + "last-line")
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "home")
		text := ansi.Strip(m.render())
		if !strings.Contains(text, "▏first-line") || strings.Contains(text, "last-line") {
			t.Fatalf("at %dx%d, moving to the draft start must reveal its cursor and first line", size[0], size[1])
		}
		press(t, m, "end")
		text = ansi.Strip(m.render())
		if !strings.Contains(text, "last-line▏") || strings.Contains(text, "first-line") {
			t.Fatalf("at %dx%d, moving to the draft end must reveal its cursor and last line", size[0], size[1])
		}
	}
}

func TestFooterIsOneLineForHintsAndStatus(t *testing.T) {
	m, _ := testModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for focus := 0; focus < 3; focus++ {
			m.focus = focus
			for _, status := range []string{"", "Saved workspace", "Saved workspace\nMore details", strings.Repeat("Saved workspace ", 30)} {
				m.status = status
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatalf("status added a row: got %d rows in a %d-row terminal", len(lines), size[1])
				}
				if strings.Count(lines[len(lines)-2], "└") != 3 || strings.Count(lines[len(lines)-2], "┘") != 3 {
					t.Fatal("all three panel bottoms must share the row just above the footer")
				}
				footer := lines[len(lines)-1]
				if !strings.Contains(footer, "Tab / Shift+Tab panels") || ansi.StringWidth(footer) != size[0] {
					t.Fatalf("footer must stay on a single full-width row: %q", footer)
				}
				if status != "" && !strings.Contains(footer, "Saved workspace") {
					t.Fatal("status should replace the contextual hint in the footer")
				}
			}
		}
	}
}

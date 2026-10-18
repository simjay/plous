package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func TestMainPanelTabOutlinesFollowFocus(t *testing.T) {
	m, _ := testModel(t)
	for mode := range modes {
		m.mode = mode
		for _, focus := range []int{1, 0, 2, 1} {
			m.focus = focus
			color := ui.BorderColor
			if focus == 1 {
				color = ui.FocusedBorder
			}
			edge := lipgloss.NewStyle().Foreground(color)
			panel, _ := textview.MouseLayout(m.mainPanel(60, 14))
			lines := strings.Split(panel, "\n")
			for _, cellWidth := range tabWidths(60) {
				if !strings.Contains(lines[0], edge.Render("╭"+strings.Repeat("─", cellWidth-2)+"╮")) {
					t.Fatalf("mode %d, focus %d: tab outline does not match panel focus", mode, focus)
				}
			}
			if !strings.Contains(lines[1], edge.Render("│")) || !strings.Contains(lines[2], edge.Render("╯")) {
				t.Fatalf("mode %d, focus %d: tab sides or baseline do not match panel focus", mode, focus)
			}
		}
	}
}

func TestPanelTravelKeepsWidths(t *testing.T) {
	for _, overview := range []bool{false, true} {
		m, _ := testModel(t)
		if overview {
			m, _ = overviewModel(t)
		}
		for _, hidden := range [][2]bool{{false, false}, {true, false}, {false, true}} {
			m.leftHidden, m.rightHidden = hidden[0], hidden[1]
			for _, size := range []assistantSize{assistantNormal, assistantDouble} {
				m.assistantSize = size
				for width := 80; width <= 320; width++ {
					m.width, m.focus = width, 1
					want := m.panelWidths()
					for _, backward := range []bool{false, true} {
						for range 3 {
							m.travelPanel(backward)
							if got := m.panelWidths(); got != want {
								t.Fatalf("overview=%t, hidden=%v, size=%s, width=%d, focus=%d: widths changed from %v to %v", overview, hidden, size, width, m.focus, want, got)
							}
							if want[m.focus] < 24 {
								t.Fatalf("focused panel is too narrow: %v", want)
							}
						}
					}
				}
			}
		}
	}
}

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
				cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(lines[1], "│"), "│"), "│ │")
				if len(cells) != len(modes) {
					t.Fatalf("want three tabs on one row, got %q", lines[1])
				}
				shortest, longest := width, 0
				for i, cell := range cells {
					cellWidth := ansi.StringWidth(cell)
					shortest, longest = min(shortest, cellWidth), max(longest, cellWidth)
					shortcut := fmt.Sprintf(":%d", i+1)
					if !strings.HasSuffix(cell, shortcut+" ") || strings.Count(lines[1], shortcut) != 1 {
						t.Errorf("tab %d must have its shortcut aligned at the right: %q", i+1, cell)
					}
					if !strings.HasPrefix(cell, " "+modes[i][:1]) {
						t.Errorf("tab %d must start with its mode name: %q", i+1, cell)
					}
					if width >= 124 && !strings.HasPrefix(cell, " "+modes[i]+" ") {
						t.Errorf("full mode name should fit at this width: %q", cell)
					}
				}
				if longest-shortest > 1 || ansi.StringWidth(lines[1]) != width {
					t.Errorf("tabs must evenly fill the panel: %q", lines[1])
				}
				if strings.Count(lines[0], "╭") != 3 || strings.Count(lines[0], "╮") != 3 {
					t.Fatal("each tab must have its own rounded outline")
				}
				start := 0
				for i, cell := range cells {
					cellWidth := ansi.StringWidth(cell) + 2
					bottom := ansi.Cut(lines[2], start, start+cellWidth)
					if i == mode {
						if bottom != "╯"+strings.Repeat(" ", cellWidth-2)+"╰" {
							t.Fatalf("active tab must open into the content: %q", bottom)
						}
					} else if bottom != "┴"+strings.Repeat("─", cellWidth-2)+"┴" {
						t.Fatalf("inactive tab must join the baseline: %q", bottom)
					}
					start += cellWidth + 1
				}
			})
		}
	}
}

func TestChatComposerRemainsAtBottom(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.closeSettings()
	drafts := []string{"", "short draft", "first-line\n" + strings.Repeat("middle\n", 20) + "last-line"}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}, {320, 60}, {480, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		bottomRow := -1
		for focus := 0; focus < 3; focus++ {
			for i, draft := range drafts {
				t.Run(fmt.Sprintf("%dx%d/focus%d/draft%d", size[0], size[1], focus, i), func(t *testing.T) {
					m.focus, m.chatEditing = focus, true
					*m.draft() = textview.NewField(draft)
					lines := strings.Split(ansi.Strip(m.render()), "\n")
					found := -1
					for row, line := range lines {
						composer := ansi.Cut(line, size[0]-m.panelWidths()[2]+2, size[0]-2)
						if strings.Contains(composer, "╰") && strings.Contains(composer, "╯") {
							found = row
						}
					}
					if found != size[1]-4 {
						t.Fatalf("composer bottom is at row %d; want above context usage at %d", found, size[1]-4)
					}
					if !strings.Contains(lines[found+1], "Context: no model") {
						t.Fatal("context usage must be visible below the composer")
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
	*m.draft() = textview.NewField("first-line\n" + strings.Repeat("middle\n", 20) + "last-line")
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
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}, {320, 60}, {480, 60}, {640, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for focus := 0; focus < 3; focus++ {
			m.focus = focus
			for _, status := range []string{"", "Saved workspace", "Saved workspace\nMore details", strings.Repeat("Saved workspace ", 30)} {
				m.status = status
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatalf("status added a row: got %d rows in a %d-row terminal", len(lines), size[1])
				}
				if strings.Count(lines[len(lines)-2], "╰") != 3 || strings.Count(lines[len(lines)-2], "╯") != 3 {
					t.Fatal("all three panel bottoms must share the row just above the footer")
				}
				footer := lines[len(lines)-1]
				if strings.Contains(footer, "\n") || ansi.StringWidth(footer) != size[0] {
					t.Fatalf("footer must stay on a single full-width row: %q", footer)
				}
				if status != "" && !strings.Contains(footer, "Saved workspace") {
					t.Fatal("status should replace the contextual hint in the footer")
				}
			}
		}
	}
}

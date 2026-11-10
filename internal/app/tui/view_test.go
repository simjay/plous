package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
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

func TestFooterKeepsCompleteContextualControls(t *testing.T) {
	m := exploreModel(t)
	m.status = ""
	for _, width := range []int{80, 120, 240} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m.width = width
			text := ansi.Strip(m.footer())
			if strings.Contains(text, "\n") || ansi.StringWidth(text) != width {
				t.Fatalf("footer exceeds its single row: %q", text)
			}
			for _, want := range []string{"Pods", "Enter Open", "/ Filter", "Esc Connections", "hh Help"} {
				if !strings.Contains(text, want) {
					t.Errorf("footer missing %q: %s", want, text)
				}
			}
			for _, absent := range []string{"[Main]", "Ctrl+", ":new", ":kinds", "…"} {
				if strings.Contains(text, absent) {
					t.Errorf("footer contains %q: %s", absent, text)
				}
			}
			if width >= 120 && !strings.Contains(text, ":f Fullscreen") {
				t.Fatal("wide footer omitted main expansion")
			}
			t.Log(strings.TrimSpace(text))
		})
	}
	typeText(m, "bb")
	m.status = ""
	text := ansi.Strip(m.footer())
	if !strings.Contains(text, "Esc Browse resources") || strings.Contains(text, "Esc Connections") {
		t.Fatalf("footer advertises a hidden panel as the Esc target: %s", text)
	}
	typeText(m, "b")
	if text := ansi.Strip(m.footer()); !strings.Contains(text, "b → b") || !strings.Contains(text, "Esc cancel") {
		t.Fatalf("pending shortcut lost its instructions: %s", text)
	}
}

func TestFooterFollowsLogAndEditorInputStates(t *testing.T) {
	t.Run("logs", func(t *testing.T) {
		m := exploreModel(t)
		m.openExploreSelection(4)
		m.status = ""
		if !strings.Contains(ansi.Strip(m.footer()), "Space Pause") {
			t.Fatal("following logs should offer Pause")
		}
		press(t, m, "space")
		if !strings.Contains(ansi.Strip(m.footer()), "Space Jump live") {
			t.Fatal("paused logs should offer Jump live")
		}
		typeText(m, "/needle")
		text := ansi.Strip(m.footer())
		if !strings.Contains(text, "Enter Finish search") || strings.Contains(text, "hh Help") || strings.Contains(text, "Space Pause") {
			t.Fatalf("log search advertises navigation keys: %s", text)
		}
		press(t, m, "enter")
		if !strings.Contains(ansi.Strip(m.footer()), "Esc Clear search") {
			t.Fatal("finished log search should clear before leaving")
		}
	})
	t.Run("editor", func(t *testing.T) {
		m, a := documentModel(t)
		m.status = ""
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Editor NORMAL") || !strings.Contains(text, ":w Save") || !strings.Contains(text, ":h Help") || strings.Contains(text, "hh Help") {
			t.Fatalf("editor should use its own controls: %s", text)
		}
		typeText(m, "i")
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Editor INSERT") || !strings.Contains(text, "Esc Normal mode") || strings.Contains(text, ":w") || strings.Contains(text, "Help") {
			t.Fatalf("Insert footer advertises commands that insert text: %s", text)
		}
		press(t, m, "esc")
		typeText(m, "/kind")
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Enter Finish search") || strings.Contains(text, ":w") {
			t.Fatalf("editor search advertises Save: %s", text)
		}
		press(t, m, "esc")
		a.Document.Paste("# unsaved\n")
		m.closeDocument()
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Esc Keep Editing") || strings.Contains(text, ":w") {
			t.Fatalf("discard prompt footer is misleading: %s", text)
		}
	})
}

func TestFooterExpansionReturnsBeforeClosingAndSearchKeepsLocalKeys(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(1)
	m.viewCommand("fullscreen")
	if !strings.Contains(ansi.Strip(m.footer()), "Esc Restore layout") {
		t.Fatal("expanded YAML does not explain the next Esc")
	}
	typeText(m, "/kind")
	text := ansi.Strip(m.footer())
	if !strings.Contains(text, "Esc Cancel search") || strings.Contains(text, "Restore layout") || strings.Contains(text, "hh Help") {
		t.Fatalf("search should retain literal input: %s", text)
	}
	press(t, m, "esc")
	if !strings.Contains(ansi.Strip(m.footer()), "Esc Restore layout") {
		t.Fatal("closing search did not restore the expansion hint")
	}
}

func TestLoadingTicksOnlyAnimateAndStopWhenFinished(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Listed = false
	e.Collection = kubernetes.Collection{}
	stream := e.Stream
	m.dirty, m.shortcutPrefix = false, "b"
	if m.syncLoading() == nil || m.syncLoading() != nil {
		t.Fatal("loading must schedule exactly one animation timer")
	}
	before := m.render()
	_, next := m.Update(loadingTickMsg{})
	after := m.render()
	if next == nil || ansi.Strip(before) == ansi.Strip(after) {
		t.Fatal("a pending request must animate without keyboard input")
	}
	if w, h := lipgloss.Size(before); w != lipgloss.Width(after) || h != lipgloss.Height(after) {
		t.Fatal("animation changed the layout size")
	}
	if m.dirty || m.shortcutPrefix != "b" || e.Stream != stream || e.DetailRun != nil || m.focus != 1 {
		t.Fatal("animation changed navigation, storage, or network request state")
	}
	for range len(loadingFrames) {
		label := ansi.Strip(m.loading("Loading Pods…"))
		if ansi.StringWidth(strings.TrimSuffix(label, " Loading Pods…")) != 5 {
			t.Fatal("dot sweep moved the loading label", label)
		}
		m.Update(loadingTickMsg{})
	}
	m.Update(monitor.ExploreUpdate{Stream: stream, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	_, next = m.Update(loadingTickMsg{})
	if next != nil || m.loadingTickPending {
		t.Fatal("completed requests must stop animating, including empty results")
	}
	for _, frame := range loadingFrames {
		if strings.Contains(ansi.Strip(m.render()), frame) {
			t.Fatal("completed request retained a loading indicator")
		}
	}
}

func TestLoadingPausesBehindOverlaysAndDoesNotAnimateErrors(t *testing.T) {
	m := exploreModel(t)
	m.explore.Listed = false
	for _, state := range []string{"help", "settings", "small", "error", "exit"} {
		switch state {
		case "help":
			m.help = &helpState{}
		case "settings":
			m.settingsOverlay = &appsettings.SettingsOverlay{}
		case "small":
			m.width = 60
		case "error":
			m.explore.Err = "Access denied"
		case "exit":
			m.exploreClosed = true
		}
		_, next := m.Update(loadingTickMsg{})
		if next != nil || m.loadingTickPending {
			t.Fatalf("%s kept the loading timer running", state)
		}
		m.help, m.settingsOverlay, m.width, m.explore.Err, m.exploreClosed = nil, nil, 120, "", false
	}
	if m.syncLoading() == nil {
		t.Fatal("visible unfinished work must resume animation")
	}
}

func TestStartupUsesCurrentContextAndPreservesSavedToolState(t *testing.T) {
	fixtures := map[string]func(*testing.T) *model{
		"Kubernetes": exploreModel,
		"Helm":       func(t *testing.T) *model { m, _ := helmModel(t); return m },
	}
	for tool, fixture := range fixtures {
		for mode, name := range modes {
			t.Run(tool+"/"+name, func(t *testing.T) {
				m := fixture(t)
				m.mode, m.focus = mode, 2
				m.draft().Text = "Keep this workspace draft"
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				restored, err := newModel(m.store, m.history, "test")
				if err != nil {
					t.Fatal(err)
				}
				if restored.mode != 0 || restored.focus != 1 || restored.currentConnection() != nil || restored.tool != "" {
					t.Fatal("startup reopened the saved tool or tab")
				}
				if restored.active != "" || !restored.currentRoot || restored.drafts[m.conversationScope()].Text != "Keep this workspace draft" {
					t.Fatal("startup lost the workspace or its draft")
				}
				body := ansi.Strip(restored.mainBody(90, 32))
				if !strings.Contains(body, "Current Workspace") {
					t.Fatal("startup did not show Current Workspace", body)
				}
				if restored.syncExplore() != nil || restored.syncHelm() != nil || restored.syncActivity() != nil {
					t.Fatal("the welcome page started the saved connection or Activity")
				}
				restored.selectScope(m.active, m.tool, m.connection)
				if restored.currentConnection() == nil || restored.connection != m.connection || restored.draft().Text != "Keep this workspace draft" {
					t.Fatal("the saved connection could not be selected from the welcome page")
				}
			})
		}
	}
}

func TestFirstLaunchOffersCurrentContextAndOptionalWorkspace(t *testing.T) {
	m, _ := testModel(t)
	body := ansi.Strip(m.mainBody(90, 32))
	if !strings.Contains(body, "Current Workspace") {
		t.Fatal("first launch did not show Current Workspace", body)
	}
	press(t, m, "enter")
	if m.explore == nil || m.explore.InputKind != "connections" {
		t.Fatal("Enter did not open context selection")
	}
	press(t, m, "esc")
	mainCommand(t, m, "n")
	if m.form == nil || m.form.Kind != "workspace" {
		t.Fatal(":n did not open optional workspace setup")
	}
}

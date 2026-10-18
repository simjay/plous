package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

func TestPanelTravelKeepsEditorInputAndUndo(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		text, cursor, revision := d.Text(), d.Cursor, d.Revision
		press(t, m, "alt+right")
		if m.focus != 2 || d.Mode != mode || d.Text() != text || d.Cursor != cursor || d.Revision != revision {
			t.Fatal("panel travel changed the editor")
		}
		press(t, m, "alt+left")
		if m.focus != 1 || m.explore.Action != action || d.Mode != mode {
			t.Fatal("return lost the editor input owner")
		}
	}
	m, action := documentModel(t)
	d := action.Document
	before := d.Text()
	typeText(m, "iX")
	press(t, m, "tab")
	if m.focus != 1 || !strings.Contains(d.Text(), "X  ") {
		t.Fatal("Tab no longer indents the document")
	}
	press(t, m, "alt+left")
	press(t, m, "alt+right")
	d.Undo()
	d.Undo()
	if d.Text() != before {
		t.Fatal("panel travel changed undo history")
	}
}

func TestPanelTravelParksSettingsHelpAndSources(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	m.openHelp()
	h := m.help
	press(t, m, "alt+right")
	if m.focus != 2 || m.helpVisible() || m.form != f || m.current().Name != "Saved" {
		t.Fatal("travel discarded or submitted an overlay")
	}
	m.chatEditing = true
	typeText(m, "draft")
	if m.draft().Text != "draft" || m.activeInteraction().owner != interactionChatInput {
		t.Fatal("parked Help captured chat input")
	}
	press(t, m, "alt+left")
	if !m.helpVisible() || m.help != h || m.form != f {
		t.Fatal("return did not restore Help above the form")
	}
	press(t, m, "esc")
	if m.help != nil || m.form != f || f.Fields[0].Value.Text != "Saved unfinished" {
		t.Fatal("Help did not restore the unfinished form")
	}
	press(t, m, "alt+left")
	press(t, m, "alt+right")
	if m.focus != 1 || m.form != f || m.current().Name != "Saved" {
		t.Fatal("Settings travel applied or lost form edits")
	}

	sources := sourceChatModel(t, 1)
	press(t, sources, "enter")
	s := sources.chatSources
	s.Scroll = 2
	sources.openHelp()
	press(t, sources, "alt+left")
	if sources.focus != 1 || sources.helpVisible() || sources.chatSourcesVisible() || sources.activeInteraction().owner == interactionSources {
		t.Fatal("parked Sources or Help still owns main")
	}
	if strings.Contains(ansi.Strip(sources.render()), "Global · Navigation") {
		t.Fatal("parked Help still covers main")
	}
	typeText(sources, "/needle")
	press(t, sources, "alt+right")
	if !sources.helpVisible() || sources.chatSources != s || s.Scroll != 2 {
		t.Fatal("return lost the overlay stack")
	}
	press(t, sources, "esc")
	if !sources.chatSourcesVisible() || sources.chatSources != s {
		t.Fatal("Help did not uncover Sources")
	}
	// Commands borrow main; travelling and cancelling must restore their origin.
	typeText(sources, ":h")
	press(t, sources, "alt+left")
	press(t, sources, "alt+right")
	if !sources.commandFocused() || !sources.chatSourcesVisible() || sources.command.Text != "h" {
		t.Fatal("travel lost the command inside Sources")
	}
	press(t, sources, "esc")
	if sources.focus != 2 || !sources.chatSourcesVisible() || sources.chatSources != s {
		t.Fatal("command cancel failed to restore Sources")
	}
}

func TestPanelTravelBypassesNativeCaptureAndKeepsTerminalTabsLocal(t *testing.T) {
	m, native := nativeModel(t)
	for _, size := range [][2]int{{160, 36}, {60, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "alt+left")
		if m.focus != 1 || !native.Captured || native.Finished {
			t.Fatal("travel released or closed the native session")
		}
		press(t, m, "alt+right")
		if m.focus != 2 || m.nativeAgent != native {
			t.Fatal("return replaced the native session")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	press(t, m, "tab")
	press(t, m, "shift+tab")
	typeText(m, "still typing")
	got := ""
	deadline := time.After(time.Second)
	for !strings.HasSuffix(got, "still typing") {
		select {
		case chunk := <-native.Input.Chunks:
			got += string(chunk)
		case <-deadline:
			t.Fatalf("missing native input: %q", got)
		}
	}
	if got != "\t\x1b[Zstill typing" || m.focus != 2 || !m.nativeAgentCaptured() {
		t.Fatalf("native terminal received panel shortcuts or lost Tab: %q", got)
	}

}

func TestPanelTravelLeavesExecCapturedAndRestoresMouse(t *testing.T) {
	m := exploreModel(t)
	terminal := reviewTerminal(t, 40, 10)
	m.explore.Terminal = terminal
	terminal.Captured = true
	press(t, m, "alt+left")
	if m.focus != 0 || m.mainTerminalCaptured() || !terminal.Captured || m.View().MouseMode == 0 {
		t.Fatal("background Exec retained application input ownership")
	}
	press(t, m, "alt+right")
	if m.focus != 1 || !m.mainTerminalCaptured() || m.explore.Terminal != terminal || m.View().MouseMode != 0 {
		t.Fatal("Exec capture did not resume on return")
	}
}

func TestPanelTravelRejectsPreviousOverlayMouseTargets(t *testing.T) {
	m, _ := testModel(t)
	m.chatEditing = true
	m.openHelp()
	old := mousePoint(t, m, "footer", "action/Esc")
	press(t, m, "alt+right")
	m.updateMouse(tea.MouseClickMsg{X: old.X, Y: old.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseReleaseMsg{X: old.X, Y: old.Y, Button: tea.MouseLeft})
	if m.focus != 2 || !m.chatEditing || m.help == nil {
		t.Fatal("a stale overlay target consumed input in the new panel")
	}
	clickMouse(t, m, "footer", "action/Esc")
	if m.chatEditing {
		t.Fatal("freshly rendered chat controls did not regain mouse input")
	}
}

func TestPanelTravelAcceptsMacArrowEncodings(t *testing.T) {
	for _, encoding := range []struct {
		name        string
		left, right rune
		mod         tea.KeyMod
	}{
		{"alt arrows", tea.KeyLeft, tea.KeyRight, tea.ModAlt},
		{"word arrows", 'b', 'f', tea.ModAlt},
		{"meta arrows", tea.KeyLeft, tea.KeyRight, tea.ModMeta},
	} {
		t.Run(encoding.name, func(t *testing.T) {
			m, n := nativeModel(t)
			for _, step := range []struct {
				code  rune
				focus int
			}{
				{encoding.left, 1}, {encoding.left, 0}, {encoding.left, 2},
				{encoding.right, 0}, {encoding.right, 1}, {encoding.right, 2},
			} {
				key := tea.KeyPressMsg{Code: step.code, Mod: encoding.mod}
				m.Update(key)
				if m.focus != step.focus || m.nativeAgent != n || n.Finished || !n.Captured {
					t.Fatalf("%s did not move to panel %d", key, step.focus)
				}
			}
			typeText(m, "still typing")
			got := ""
			deadline := time.After(time.Second)
			for !strings.HasSuffix(got, "still typing") {
				select {
				case chunk := <-n.Input.Chunks:
					got += string(chunk)
				case <-deadline:
					t.Fatalf("native input missing: %q", got)
				}
			}
			if got != "still typing" {
				t.Fatalf("panel shortcuts reached native input: %q", got)
			}
		})
	}
}

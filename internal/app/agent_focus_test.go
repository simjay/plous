package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

func TestAgentToggleReturnsToOriginAndRevealsHiddenPanel(t *testing.T) {
	for _, origin := range []int{0, 1} {
		for _, width := range []int{60, 100, 160} {
			m, _ := overviewModel(t)
			m.width, m.height, m.focus = width, 36, origin
			m.rightHidden = true
			m.draft().Insert("retained draft", true)
			m.shortcutPrefix = "i"
			press(t, m, "ctrl+]")
			if m.focus != 2 || m.rightHidden || !m.panelVisible(2) || m.shortcutPrefix != "" {
				t.Fatal("toggle did not reveal and focus the agent")
			}
			press(t, m, "ctrl+]")
			if m.focus != origin || m.draft().Text != "retained draft" || m.rightHidden {
				t.Fatal("return lost the origin, draft or panel visibility")
			}
		}
	}
	m, _ := testModel(t)
	m.focus = 0
	press(t, m, "ctrl+]")
	m.leftHidden = true
	press(t, m, "ctrl+]")
	if m.focus != 1 {
		t.Fatal("hidden origin did not fall back to main")
	}
	// Entry by existing navigation also establishes the return panel.
	m.leftHidden, m.focus = false, 0
	press(t, m, "alt+left")
	press(t, m, "ctrl+]")
	if m.focus != 0 {
		t.Fatal("toggle forgot the panel before Alt travel")
	}
}

func TestAgentToggleKeepsInputAndOverlayState(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		text, cursor, revision := d.Text(), d.Cursor, d.Revision
		press(t, m, "ctrl+]")
		press(t, m, "ctrl+]")
		if m.focus != 1 || m.explore.Action != action || d.Mode != mode || d.Text() != text || d.Cursor != cursor || d.Revision != revision {
			t.Fatal("toggle changed the editor")
		}
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	m.openHelp()
	h := m.help
	press(t, m, "ctrl+]")
	if m.focus != 2 || m.helpVisible() || m.form != f {
		t.Fatal("toggle did not park Help above the unfinished form")
	}
	m.chatEditing = true
	typeText(m, "draft")
	press(t, m, "ctrl+]")
	if m.focus != 1 || !m.helpVisible() || m.help != h || m.form != f || f.Fields[0].Value.Text != "Saved unfinished" || m.current().Name != "Saved" {
		t.Fatal("return lost or submitted the form")
	}
	press(t, m, "ctrl+]")
	typeText(m, " continued")
	if m.draft().Text != "draft continued" || !m.chatEditing {
		t.Fatal("toggle lost the chat input owner")
	}

	sources := sourceChatModel(t, 1)
	press(t, sources, "enter")
	s := sources.chatSources
	s.Scroll = 2
	press(t, sources, "ctrl+]")
	if sources.focus != 1 || sources.chatSourcesVisible() {
		t.Fatal("Sources did not park")
	}
	press(t, sources, "ctrl+]")
	if !sources.chatSourcesVisible() || sources.chatSources != s || s.Scroll != 2 {
		t.Fatal("toggle lost Sources")
	}
	typeText(sources, ":h")
	command := sources.command
	press(t, sources, "ctrl+]")
	press(t, sources, "ctrl+]")
	if !sources.commandFocused() || sources.command != command || command.Text != "h" {
		t.Fatal("toggle lost the colon draft")
	}
	press(t, sources, "esc")
	if sources.focus != 2 || !sources.chatSourcesVisible() {
		t.Fatal("colon cancellation lost its origin")
	}
}

func TestAgentToggleBypassesNativeInputAndSmallTerminal(t *testing.T) {
	m, n := nativeModel(t)
	for _, size := range [][2]int{{160, 36}, {60, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "ctrl+]")
		if m.focus != 1 || !n.Captured || n.Finished {
			t.Fatal("toggle did not leave the running terminal")
		}
		m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl, IsRepeat: true})
		if m.focus != 1 {
			t.Fatal("held toggle bounced focus back")
		}
		press(t, m, "ctrl+]")
		if m.focus != 2 || m.nativeAgent != n {
			t.Fatal("toggle did not return to the same session")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	typeText(m, "x")
	select {
	case chunk := <-n.Input.Chunks:
		if string(chunk) != "x" {
			t.Fatalf("toggle was forwarded to the CLI: %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("native input did not resume")
	}
}

func TestAgentToggleRestoresMainFullscreenAndPreservesExecEscape(t *testing.T) {
	m, _ := testModel(t)
	typeText(m, "ff")
	press(t, m, "ctrl+]")
	if m.focus != 2 || m.viewExpansion != nil || !m.panelVisible(2) {
		t.Fatal("fullscreen hid the focused agent")
	}
	press(t, m, "ctrl+]")
	if m.focus != 1 || m.viewExpanded() {
		t.Fatal("return did not restore main navigation")
	}
	m = exploreModel(t)
	tm := reviewTerminal(t, 40, 10)
	m.explore.Terminal, tm.Captured = tm, true
	press(t, m, "ctrl+]")
	if m.focus != 1 || tm.Captured || tm.Review == nil {
		t.Fatal("Exec lost its output-review escape")
	}
	press(t, m, "ctrl+]")
	if m.focus != 2 {
		t.Fatal("toggle unavailable from Exec review")
	}
}

func TestRetiredAssistantMainSizeRestoresAsDouble(t *testing.T) {
	m, _ := testModel(t)
	m.assistantSize = "main"
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.Update(tea.WindowSizeMsg{Width: 160, Height: 36})
	if restored.assistantSize != assistantDouble || restored.focus != 1 || !restored.panelVisible(1) {
		t.Fatal("saved takeover did not restore a usable layout")
	}
}

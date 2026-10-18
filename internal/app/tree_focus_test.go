package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

func TestTabTogglesOnlyTreeAndMain(t *testing.T) {
	for name, setup := range map[string]func(*testing.T) *model{
		"workspace": func(t *testing.T) *model { m, _ := testModel(t); return m },
		"overview":  func(t *testing.T) *model { m, _ := overviewModel(t); return m },
		"resources": exploreModel,
		"yaml":      yamlViewModel,
		"code":      func(t *testing.T) *model { m, _ := codeModel(t); return m },
		"helm":      func(t *testing.T) *model { m, _ := helmModel(t); return m },
		"gitops":    func(t *testing.T) *model { m, _ := gitOpsModel(t); return m },
	} {
		t.Run(name, func(t *testing.T) {
			m := setup(t)
			m.focus, m.leftHidden = 1, true
			m.shortcutPrefix = "i"
			m.draft().Text = "agent draft"
			press(t, m, "tab")
			if m.focus != 0 || m.leftHidden || !m.panelVisible(0) || m.shortcutPrefix != "" {
				t.Fatal("Tab did not reveal and focus tree")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab, IsRepeat: true})
			press(t, m, "shift+tab")
			if m.focus != 0 {
				t.Fatal("repeat or Shift+Tab moved panel focus")
			}
			press(t, m, "tab")
			if m.focus != 1 || m.draft().Text != "agent draft" {
				t.Fatal("Tab did not return to main with the agent draft intact")
			}
			press(t, m, "ctrl+]")
			press(t, m, "tab")
			if m.focus != 2 {
				t.Fatal("Tab moved out of the agent")
			}
			press(t, m, "ctrl+]")
			if m.focus != 1 {
				t.Fatal("Ctrl+] lost its return panel")
			}
		})
	}
}

func TestTabRestoresMainFullscreen(t *testing.T) {
	m, _ := testModel(t)
	typeText(m, "ff")
	press(t, m, "tab")
	if m.viewExpansion != nil || m.focus != 0 || !m.panelVisible(0) {
		t.Fatal("Tab did not restore the layout and focus tree")
	}
	press(t, m, "tab")
	if m.focus != 1 {
		t.Fatal("Tab did not return to main")
	}
}

func TestTabStaysLocalInInputAndOverlays(t *testing.T) {
	for _, mode := range []docedit.Mode{docedit.Normal, docedit.Insert, docedit.Select} {
		m, action := documentModel(t)
		d := action.Document
		d.Mode = mode
		before := d.Text()
		press(t, m, "tab")
		if m.focus != 1 || d.Mode != mode || mode == docedit.Insert && d.Text() == before {
			t.Fatal("Tab escaped the editable document or lost indentation")
		}
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Saved")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unfinished")
	f := m.form
	for _, action := range []bool{false, true} {
		if action {
			f.Index = len(f.Fields)
		}
		press(t, m, "tab")
		if m.focus != 1 || m.form != f || m.current().Name != "Saved" || f.Fields[0].Value.Text != "Saved unfinished" {
			t.Fatal("Tab left or submitted the form")
		}
	}
	m.openHelp()
	press(t, m, "tab")
	if !m.helpVisible() || m.focus != 1 {
		t.Fatal("Tab escaped Help")
	}

	m, _ = testModel(t)
	for _, panel := range []int{0, 1} {
		m.focus = panel
		typeText(m, "/needle")
		press(t, m, "tab")
		if m.focus != panel || m.activePanelSearch(panel) == nil {
			t.Fatal("Tab escaped search input")
		}
		press(t, m, "esc")
	}
	typeText(m, ":s")
	command := m.command
	press(t, m, "tab")
	if !m.commandFocused() || m.command != command || command.Text != "s" {
		t.Fatal("Tab escaped command input")
	}
}

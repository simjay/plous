package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/editor"
)

func TestReadOnlyActivityRoutesLiteralSearchAndGlobalNavigation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	mainCommand(t, m, "3")
	press(t, m, "enter")
	m.cursor = 3
	summary := m.detail
	typeText(m, "d")
	if !m.activityReviewVisible() {
		t.Fatal("visible View changes shortcut did not open shared comparison")
	}
	r := m.activityReview.comparison
	typeText(m, "/bbii33:q")
	m.Update(tea.PasteMsg{Content: "\n:settings"})
	if r.Reader.Query != "bbii33:q:settings" || m.mode != 2 || m.leftHidden || m.rightHidden || m.command != nil || m.settingsOverlay != nil {
		t.Fatal("comparison search escaped into application navigation")
	}
	press(t, m, "esc")
	typeText(m, "b")
	if !strings.Contains(ansi.Strip(m.footer()), "b → b") {
		t.Fatal("comparison hid the pending global shortcut")
	}
	press(t, m, "esc")
	if !m.activityReviewVisible() {
		t.Fatal("clearing a pending prefix also left comparison")
	}
	typeText(m, "v")
	press(t, m, "tab")
	if m.focus != 2 || !r.Reader.Selecting {
		t.Fatal("panel change lost comparison selection")
	}
	press(t, m, "shift+tab")
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if !m.activityReviewVisible() || !r.Reader.Selecting {
		t.Fatal("Help lost comparison reading state")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if !m.activityDetailActive() || m.detail != summary || m.cursor != 3 {
		t.Fatal("comparison did not return to the exact summary position")
	}
	mainCommand(t, m, "v")
	if !m.activityReviewVisible() {
		t.Fatal("named View changes command did not reopen comparison")
	}
	m.execute("applychange")
	if r.CanConfirm || m.explore != nil && m.explore.action != nil {
		t.Fatal("Activity exposed a resource write operation")
	}
}

func TestReadOnlyYAMLRoutesSearchSelectionAndExplicitEdit(t *testing.T) {
	m := yamlViewModel(t)
	m.syncExploreDetail()
	run := m.explore.detailRun
	run.cancel()
	m.Update(exploreDetailResult{run: run, object: m.explore.detail.DeepCopy()})
	m.exploreDetailBody(84, 28)
	d := m.explore.yamlDocument
	original := d.text
	typeText(m, "/bbii33:q")
	m.Update(tea.PasteMsg{Content: "\n:settings"})
	if d.document.Query != "bbii33:q:settings" || m.mode != 1 || m.leftHidden || m.rightHidden || m.command != nil {
		t.Fatal("YAML search escaped into application navigation")
	}
	press(t, m, "esc")
	typeText(m, "z")
	press(t, m, "tab")
	press(t, m, "shift+tab")
	if d.document.Mode != docedit.Select || m.explore.yamlDocument != d {
		t.Fatal("panel focus lost YAML selection")
	}
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if d.document.Mode != docedit.Select || m.explore.yamlDocument != d {
		t.Fatal("Help replaced the YAML reader")
	}
	press(t, m, "esc")
	m.Update(tea.PasteMsg{Content: "kind: Secret\n"})
	typeText(m, "ii")
	if !m.rightHidden || d.text != original || m.explore.action != nil {
		t.Fatal("read-only navigation changed the document or blocked a global shortcut")
	}
	_, copied := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if copied == nil || !strings.Contains(ansi.Strip(m.footer()), "terminal clipboard") {
		t.Fatal("YAML copy feedback was hidden")
	}
	typeText(m, "E")
	if m.explore.action == nil || !m.explore.action.requested || m.explore.action.items[m.explore.action.cursor].id != "edit" {
		t.Fatal("explicit Edit did not enter the resource edit workflow")
	}
}

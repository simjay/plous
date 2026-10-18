package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"strings"
	"testing"
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
	r := m.activityReview.Comparison
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
	press(t, m, "alt+right")
	if m.focus != 2 || !r.Reader.Selecting {
		t.Fatal("panel change lost comparison selection")
	}
	press(t, m, "alt+left")
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
	if r.CanConfirm || m.explore != nil && m.explore.Action != nil {
		t.Fatal("Activity exposed a resource write operation")
	}
}

func TestReadOnlyYAMLRoutesSearchSelectionAndExplicitEdit(t *testing.T) {
	m := yamlViewModel(t)
	m.syncExploreDetail()
	run := m.explore.DetailRun
	run.Cancel()
	m.Update(monitor.ExploreDetailResult{Run: run, Object: m.explore.Detail.DeepCopy()})
	m.exploreDetailBody(84, 28)
	d := m.explore.YamlDocument
	original := d.Text
	typeText(m, "/bbii33:q")
	m.Update(tea.PasteMsg{Content: "\n:settings"})
	if d.Document.Query != "bbii33:q:settings" || m.mode != 1 || m.leftHidden || m.rightHidden || m.command != nil {
		t.Fatal("YAML search escaped into application navigation")
	}
	press(t, m, "esc")
	typeText(m, "z")
	press(t, m, "alt+right")
	press(t, m, "alt+left")
	if d.Document.Mode != docedit.Select || m.explore.YamlDocument != d {
		t.Fatal("panel focus lost YAML selection")
	}
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if d.Document.Mode != docedit.Select || m.explore.YamlDocument != d {
		t.Fatal("Help replaced the YAML reader")
	}
	press(t, m, "esc")
	m.Update(tea.PasteMsg{Content: "kind: Secret\n"})
	typeText(m, "ii")
	if m.assistantSize != assistantDouble || d.Text != original || m.explore.Action != nil {
		t.Fatal("read-only navigation changed the document or blocked a global shortcut")
	}
	_, copied := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if copied == nil || !strings.Contains(ansi.Strip(m.footer()), "terminal clipboard") {
		t.Fatal("YAML copy feedback was hidden")
	}
	typeText(m, "E")
	if m.explore.Action == nil || !m.explore.Action.Requested || m.explore.Action.Items[m.explore.Action.Cursor].Id != "edit" {
		t.Fatal("explicit Edit did not enter the resource edit workflow")
	}
}

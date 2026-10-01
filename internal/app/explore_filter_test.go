package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestExploreBackspaceClearsOnlyAppliedTextFilter(t *testing.T) {
	for _, query := range []string{"checkout", "no-match"} {
		t.Run(query, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.preference.Selector = "app=checkout"
			e.preference.Sort, e.preference.Descending = 2, true
			e.stream.key += e.preference.Selector
			stream := e.stream
			want := e.preference
			typeText(m, "/"+query)
			press(t, m, "enter")
			selected := e.selected
			for _, width := range []int{24, 45, 80} {
				view := ansi.Strip(m.exploreTableBody(width, 15))
				if !strings.Contains(view, "Backspace clear") {
					t.Fatalf("missing clear hint at width %d: %s", width, view)
				}
			}
			press(t, m, "backspace")
			_, rows := m.exploreRows()
			if e.preference != want || len(rows) != 2 || e.selected != selected || e.stream != stream || m.focus != 1 {
				t.Fatalf("clearing changed browsing state: %#v, selection %q, rows %d", e.preference, e.selected, len(rows))
			}
			if saved := m.explorePreferences[m.active+"/"+m.connection]; saved != want {
				t.Fatalf("cleared filter was not remembered: %#v", saved)
			}
			if strings.Contains(ansi.Strip(m.exploreTableBody(80, 15)), "Backspace clear") {
				t.Fatal("empty filter still shows a clear action")
			}
			press(t, m, "backspace")
			if e.selected != selected || e.preference != want {
				t.Fatal("Backspace without a filter changed browsing state")
			}
		})
	}
}

func TestExploreBackspaceEditsFilterWithoutClearingCommittedQuery(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	typeText(m, "/checkout")
	press(t, m, "enter")
	typeText(m, "/")
	press(t, m, "backspace")
	if e.input == nil || e.input.text != "checkou" || e.preference.Filter != "checkout" {
		t.Fatal("Backspace in the editor cleared the applied filter")
	}
	if strings.Contains(ansi.Strip(m.exploreBody(80, 20)), "Backspace clear") {
		t.Fatal("editor advertises navigation-only clear action")
	}
	press(t, m, "esc")
	if e.preference.Filter != "checkout" || e.input != nil {
		t.Fatal("Esc did not cancel only the unfinished edit")
	}
	press(t, m, "esc")
	if m.focus != 0 || e.preference.Filter != "checkout" {
		t.Fatal("root Esc cleared the committed filter")
	}
}

func TestExploreBackspaceDoesNotClearFilterFromOtherControls(t *testing.T) {
	for _, control := range []string{"sidebar", "tree search", "chat", "command", "selector", "kinds", "detail", "help", "settings"} {
		t.Run(control, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.preference.Filter = "checkout"
			var input *editor
			switch control {
			case "sidebar", "tree search":
				m.focus = 0
				if control == "tree search" {
					typeText(m, "/pods")
					input = &m.searches[0].input
				}
			case "chat":
				mainCommand(t, m, "chat")
				typeText(m, "draft")
				input = m.draft()
			case "command":
				typeText(m, ":pods")
				input = m.command
			case "selector":
				typeText(m, "fapp=checkout")
				input = e.input
			case "kinds":
				mainCommand(t, m, "kinds")
				typeText(m, "pods")
				input = e.input
			case "detail":
				press(t, m, "enter")
			case "help", "settings":
				mainCommand(t, m, control)
			}
			want := ""
			if input != nil {
				want = input.text[:len(input.text)-1]
			}
			press(t, m, "backspace")
			if e.preference.Filter != "checkout" {
				t.Fatal("Backspace escaped its focused control and cleared the table filter")
			}
			if input != nil && input.text != want {
				t.Fatalf("Backspace did not edit normally: %q, want %q", input.text, want)
			}
		})
	}
}

func TestResourcePickerUsesCommandsInsteadOfTableO(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "o")
	if m.explore.picker != "" || m.explore.input != nil || m.explore.preference.Resource != "pods" {
		t.Fatal("retired table shortcut still opens a picker")
	}
	for _, command := range []string{"k", "kinds"} {
		for _, focus := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/panel%d", command, focus), func(t *testing.T) {
				m := exploreModel(t)
				m.focus = focus
				m.leftHidden = focus != 0
				mainCommand(t, m, command)
				if m.focus != 1 || m.explore.picker != "kinds" || m.explore.input == nil {
					t.Fatal("resource picker command is unavailable")
				}
				typeText(m, "deployments")
				press(t, m, "enter")
				if m.explore.preference.Resource != "deployments.apps" {
					t.Fatal("resource picker failed to open the searched type")
				}
			})
		}
	}
}

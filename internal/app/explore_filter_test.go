package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func TestBackspaceControlByteDoesNotBecomeNavigation(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "/checkout")
	press(t, m, "enter")
	key := tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}
	m.Update(key)
	if m.explore.Preference.Filter != "checkout" || m.focus != 1 || m.help != nil {
		t.Fatal("Ctrl+H changed navigation or cleared the applied filter")
	}
	typeText(m, "/")
	m.Update(key)
	if m.explore.Input == nil || m.explore.Input.Text != "checkou" || m.explore.Preference.Filter != "checkout" {
		t.Fatal("BS-style Backspace did not stay inside the filter editor")
	}
}

func TestExploreBackspaceClearsOnlyAppliedTextFilter(t *testing.T) {
	for _, query := range []string{"checkout", "no-match"} {
		t.Run(query, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.Preference.Selector = "app=checkout"
			e.Preference.Sort, e.Preference.Descending = 2, true
			e.Stream.Key += e.Preference.Selector
			stream := e.Stream
			want := e.Preference
			typeText(m, "/"+query)
			press(t, m, "enter")
			selected := e.Selected
			for _, width := range []int{24, 45, 80} {
				view := ansi.Strip(m.exploreTableBody(width, 15))
				if !strings.Contains(view, "Backspace clear") {
					t.Fatalf("missing clear hint at width %d: %s", width, view)
				}
			}
			press(t, m, "backspace")
			_, rows := m.exploreRows()
			if e.Preference != want || len(rows) != 2 || e.Selected != selected || e.Stream != stream || m.focus != 1 {
				t.Fatalf("clearing changed browsing state: %#v, selection %q, rows %d", e.Preference, e.Selected, len(rows))
			}
			if saved := m.explorePreferences[m.active+"/"+m.connection]; saved != want {
				t.Fatalf("cleared filter was not remembered: %#v", saved)
			}
			if strings.Contains(ansi.Strip(m.exploreTableBody(80, 15)), "Backspace clear") {
				t.Fatal("empty filter still shows a clear action")
			}
			press(t, m, "backspace")
			if e.Selected != selected || e.Preference != want {
				t.Fatal("Backspace without a filter changed browsing state")
			}
		})
	}
}

func TestTableFullscreenAndLabelSelectorDoNotConflict(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "f")
	if m.shortcutPrefix != "f" || m.explore.Input != nil || m.viewExpanded() {
		t.Fatal("fullscreen prefix opened a filter or changed the layout")
	}
	typeText(m, "fLapp=ff")
	if !m.viewExpanded() || m.explore.InputKind != "selector" || m.explore.Input == nil || m.explore.Input.Text != "app=ff" {
		t.Fatal("label selector is unreachable or interprets fullscreen keys")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || m.explore.Input != nil {
		t.Fatal("closing the selector restored the layout")
	}
	typeText(m, "ff")
	if m.viewExpanded() {
		t.Fatal("fullscreen shortcut did not restore the layout")
	}
}

func TestExploreBackspaceEditsFilterWithoutClearingCommittedQuery(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	typeText(m, "/checkout")
	press(t, m, "enter")
	typeText(m, "/")
	press(t, m, "backspace")
	if e.Input == nil || e.Input.Text != "checkou" || e.Preference.Filter != "checkout" {
		t.Fatal("Backspace in the editor cleared the applied filter")
	}
	if strings.Contains(ansi.Strip(m.exploreBody(80, 20)), "Backspace clear") {
		t.Fatal("editor advertises navigation-only clear action")
	}
	press(t, m, "esc")
	if e.Preference.Filter != "checkout" || e.Input != nil {
		t.Fatal("Esc did not cancel only the unfinished edit")
	}
	press(t, m, "esc")
	if m.focus != 0 || e.Preference.Filter != "checkout" {
		t.Fatal("root Esc cleared the committed filter")
	}
}

func TestExploreBackspaceDoesNotClearFilterFromOtherControls(t *testing.T) {
	for _, control := range []string{"sidebar", "tree search", "chat", "command", "selector", "connections", "detail", "help", "settings"} {
		t.Run(control, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.Preference.Filter = "checkout"
			var input *textview.Field
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
				typeText(m, "Lapp=checkout")
				input = e.Input
			case "connections":
				mainCommand(t, m, "connections")
				typeText(m, "pods")
				input = e.Input
			case "detail":
				press(t, m, "enter")
			case "help", "settings":
				mainCommand(t, m, control)
			}
			want := ""
			if input != nil {
				want = input.Text[:len(input.Text)-1]
			}
			press(t, m, "backspace")
			if e.Preference.Filter != "checkout" {
				t.Fatal("Backspace escaped its focused control and cleared the table filter")
			}
			if input != nil && input.Text != want {
				t.Fatalf("Backspace did not edit normally: %q, want %q", input.Text, want)
			}
		})
	}
}

func TestResourceHeadingAndEscapeRevealSidebar(t *testing.T) {
	for _, input := range []string{"keyboard", "mouse"} {
		for _, hidden := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/hidden=%t", input, hidden), func(t *testing.T) {
				m := exploreModel(t)
				m.width, m.height = 160, 40
				m.leftHidden = hidden
				m.explore.Preference.Filter = "checkout"
				before, connection := m.explore.Preference, m.connection
				if input == "mouse" {
					clickMouse(t, m, "main", "resource-types/")
				} else {
					press(t, m, "esc")
				}
				if m.leftHidden || m.focus != 0 || m.connection != connection || m.explore.Preference != before {
					t.Fatal("resource browsing must reveal the sidebar without changing the connection or filters")
				}
				typeText(m, "/deployments")
				press(t, m, "enter")
				if m.explore.Preference.Resource != "deployments.apps" {
					t.Fatal("sidebar search did not open the selected resource")
				}
			})
		}
	}
}

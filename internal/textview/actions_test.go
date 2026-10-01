package textview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestActionBarWrapsControlsAndKeepsFocusedActionVisible(t *testing.T) {
	groups := []ActionGroup{
		{Label: "Source", Actions: []Action{{Key: "o", Label: "Container"}, {Key: "u", Label: "Range"}, {Key: "p", Label: "Previous", Value: "Off"}}},
		{Label: "Display", Actions: []Action{{Key: "w", Label: "Wrap", Value: "On"}, {Key: "Enter", Label: "Apply", Focused: true, Primary: true}}},
	}
	for _, width := range []int{24, 28, 48, 84, 196} {
		for _, height := range []int{1, 2, 4, 12} {
			view := ansi.Strip(ActionBar(groups, width, height, DefaultTheme()))
			rows := strings.Split(view, "\n")
			if len(rows) > height {
				t.Fatalf("%dx%d toolbar overflows: %s", width, height, view)
			}
			for _, row := range rows {
				if ansi.StringWidth(row) > width {
					t.Fatalf("%dx%d row overflows: %s", width, height, row)
				}
			}
			if !strings.Contains(view, "[› Enter Apply ]") {
				t.Fatalf("%dx%d hides the Enter target: %s", width, height, view)
			}
		}
	}
	view := ansi.Strip(ActionBar(groups, 84, 12, DefaultTheme()))
	for _, text := range []string{"[ o Container ]", "[ p Previous Off ]", "[ w Wrap On ]"} {
		if !strings.Contains(view, text) {
			t.Fatalf("control lost its boundary, key, or value: %s", view)
		}
	}
}

func TestDisabledActionAndUnicodeRemainReadableWithoutColor(t *testing.T) {
	view := ansi.Strip(ActionChip(Action{Key: "Enter", Label: "Apply", Disabled: true, Focused: true}, 28, DefaultTheme()))
	if !strings.Contains(view, "Unavailable") || !strings.HasSuffix(view, " ]") {
		t.Fatalf("disabled state lost: %s", view)
	}
	for _, width := range []int{1, 8, 20, 28, 48} {
		view = ActionChip(Action{Key: "e", Label: strings.Repeat("界é", 30)}, width, DefaultTheme())
		if ansi.StringWidth(view) > width {
			t.Fatalf("Unicode action exceeded %d columns", width)
		}
		if width >= 20 && !strings.HasSuffix(ansi.Strip(view), " ]") {
			t.Fatal("truncation removed the control boundary")
		}
	}
}

func TestActionBarKeepsPrimaryActionVisibleUntilAnotherActionHasFocus(t *testing.T) {
	groups := []ActionGroup{{Label: "Decision", Actions: []Action{
		{Key: "Esc", Label: "Back to editor"},
		{Key: ":w", Label: "Save", Primary: true},
		{Key: "/", Label: "Search"},
	}}}
	view := ansi.Strip(ActionBar(groups, 28, 1, DefaultTheme()))
	if !strings.Contains(view, ":w Save") {
		t.Fatal("a compact toolbar hid the primary action", view)
	}
	groups[0].Actions[0].Focused = true
	view = ansi.Strip(ActionBar(groups, 28, 1, DefaultTheme()))
	if !strings.Contains(view, "Esc Back to editor") {
		t.Fatal("the primary action displaced the actual focused action", view)
	}
}

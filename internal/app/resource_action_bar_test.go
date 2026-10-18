package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestDetailActionKeysPreserveSectionsAndGlobalPrefixes(t *testing.T) {
	for key, operation := range map[string]string{"x": "exec", "S": "scale", "R": "restart", "E": "edit", "D": "delete"} {
		t.Run(key, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(0)
			m.explore.Resource = kubecommon.Resource{Name: "deployments", Group: "apps"}
			m.explore.DetailValid = true
			m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
			a := m.explore.Action
			if a == nil || !a.Requested || a.Items[a.Cursor].Id != operation {
				t.Fatalf("%s did not open %s", key, operation)
			}
		})
	}
	m := exploreModel(t)
	m.openExploreSelection(0)
	for key, tab := range map[string]int{"d": 0, "y": 1, "e": 2, "r": 3} {
		typeText(m, key)
		if m.explore.Tab != tab || m.explore.Action != nil {
			t.Fatalf("%s lost its detail-section binding", key)
		}
	}
	typeText(m, "s")
	if m.shortcutPrefix != "s" || m.explore.Action != nil {
		t.Fatal("resource actions consumed the Settings prefix")
	}
}

func TestSingleLetterActionCommandsKeepOnlyCurrentAliases(t *testing.T) {
	for _, item := range monitor.ResourceActionDefinitions {
		if actionCommand(item.Alias) != item.Id || actionCommand(item.Command) != item.Id || len(item.Alias) != 1 {
			t.Fatalf("invalid short action binding: %#v", item)
		}
		if global, _ := navigationShortcut(item.Alias); global != "" || canonicalCommand(item.Alias) != item.Alias {
			t.Fatalf("action alias collides with global: %s", item.Alias)
		}
	}
	for _, old := range []string{"ac", "lg", "ex", "sl", "rr", "ed", "dl", "ci", "cv", "xy", "un"} {
		if actionCommand(old) != "" {
			t.Fatalf("retired command %s is still active", old)
		}
	}
}

func TestDirectActionEscapeReturnsToOriginWithoutAnExtraMenu(t *testing.T) {
	for _, stage := range []string{"scale", "export", "pods", "confirm"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(1)
			m.explore.DetailValid = true
			m.explore.Scroll = 12
			m.openResourceActions("delete")
			a := m.explore.Action
			a.Pending, a.Stage, a.Operation = "", stage, "delete"
			m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.explore.Action != nil || m.explore.Detail == nil || m.explore.Tab != 1 || m.explore.Scroll != 12 {
				t.Fatal("direct action Escape lost the originating detail")
			}
		})
	}
}

func TestDetailActionBarShowsCommonOperationsAndFitsNarrowViews(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.Resource = kubecommon.Resource{Name: "deployments", Group: "apps"}
	text := ansi.Strip(m.resourceActionBar(110))
	for _, label := range []string{"[l] Logs", "[x] Exec", "[S] Scale", "[R] Restart", "[E] Edit", "[D] Delete", "[a] More"} {
		if !strings.Contains(text, label) {
			t.Fatalf("common action hidden: %s in %s", label, text)
		}
	}
	for _, width := range []int{20, 32, 52, 80, 110} {
		text := m.resourceActionBar(width)
		if len(strings.Split(text, "\n")) > 2 || !strings.Contains(ansi.Strip(text), "More") {
			t.Fatalf("invalid narrow action bar %d: %s", width, text)
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("action bar overflows %d columns", width)
			}
		}
	}
	m.explore.Resource = kubecommon.Resource{Name: "services"}
	text = ansi.Strip(m.resourceActionBar(110))
	for _, label := range []string{"Logs", "Exec", "Scale", "Restart"} {
		if strings.Contains(text, label) {
			t.Fatalf("Service gained unsupported %s", label)
		}
	}
}

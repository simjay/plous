package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestYAMLBackRestoresOriginSectionAndPosition(t *testing.T) {
	for _, tab := range []int{0, 2, 3} {
		for _, entry := range []string{"key", "command", "sections"} {
			t.Run(fmt.Sprintf("%d/%s", tab, entry), func(t *testing.T) {
				m := exploreModel(t)
				m.openExploreSelection(tab)
				e := m.explore
				e.detailValid, e.scroll, e.relatedCursor = true, 17, 2
				uid, preference, selection := e.detail.GetUID(), e.preference, e.selected
				switch entry {
				case "key":
					typeText(m, "y")
				case "command":
					mainCommand(t, m, "yaml")
				case "sections":
					if tab == 0 {
						typeText(m, "]")
					} else if tab == 2 {
						typeText(m, "[")
					} else {
						m.openExploreSelection(1)
					}
				}
				if e.tab != 1 || e.yamlOrigin == nil {
					t.Fatal("YAML did not remember its origin")
				}
				typeText(m, "/kind")
				press(t, m, "esc")
				if e.tab != 1 || e.yamlOrigin == nil {
					t.Fatal("closing search also left YAML")
				}
				typeText(m, "z")
				press(t, m, "esc")
				if e.tab != 1 {
					t.Fatal("closing selection also left YAML")
				}
				// Returning must keep refreshed data rather than restoring an older object snapshot.
				e.detail.SetResourceVersion("newer")
				press(t, m, "esc")
				if e.detail == nil || e.detail.GetUID() != uid || e.detail.GetResourceVersion() != "newer" || e.tab != tab || e.scroll != 17 || e.relatedCursor != 2 || e.preference != preference || e.selected != selection || e.yamlOrigin != nil {
					t.Fatal("YAML back lost origin or current data")
				}
				press(t, m, "esc")
				if e.detail != nil || m.focus != 1 {
					t.Fatal("next Esc did not return to list")
				}
			})
		}
	}
}

func TestYAMLDirectListOpenAndExplicitSectionSwitch(t *testing.T) {
	m := yamlViewModel(t)
	press(t, m, "esc")
	if m.explore.detail != nil {
		t.Fatal("direct YAML open inserted an extra Summary step")
	}
	m.openExploreSelection(0)
	m.explore.detailValid = true
	typeText(m, "y")
	typeText(m, "e")
	if m.explore.tab != 2 || m.explore.yamlOrigin != nil {
		t.Fatal("explicit Events navigation retained stale YAML origin")
	}
	press(t, m, "esc")
	if m.explore.detail != nil {
		t.Fatal("Events back was captured by stale YAML history")
	}
}

func TestYAMLBackChainSurvivesLogsAndRelatedObject(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(3)
	parent := e.location()
	e.trail = []exploreLocation{parent}
	e.detail = e.collection.Objects[1].DeepCopy()
	e.tab, e.detailValid, e.scroll = 0, true, 14
	uid := e.detail.GetUID()
	typeText(m, "y")
	typeText(m, "l")
	if e.tab != 4 {
		t.Fatal("logs did not open")
	}
	press(t, m, "esc")
	if e.tab != 1 || e.yamlOrigin == nil {
		t.Fatal("logs lost YAML's return section")
	}
	press(t, m, "esc")
	if e.detail == nil || e.detail.GetUID() != uid || e.tab != 0 || e.scroll != 14 {
		t.Fatal("YAML skipped the current object's Summary")
	}
	press(t, m, "esc")
	if e.detail == nil || e.detail.GetUID() != parent.object.GetUID() || e.tab != 3 {
		t.Fatal("Summary lost related-object return")
	}
}

func TestResourceTabsExposeAccurateKeysAtNarrowAndWideWidths(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
	text := ansi.Strip(m.exploreDetailBody(120, 28))
	for _, label := range []string{"[d Describe]", "y YAML", "e Events", "r Related", "E Edit"} {
		if !strings.Contains(text, label) {
			t.Fatalf("missing %s", label)
		}
	}
	typeText(m, "y")
	m.status = ""
	for _, width := range []int{28, 48, 120} {
		text = ansi.Strip(m.exploreDetailBody(width, 28))
		if !strings.Contains(text, "[:y YAML]") || !strings.Contains(ansi.Strip(m.footer()), "Esc Describe") {
			t.Fatalf("YAML key or return target missing at %d: %s", width, text)
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("tab header overflow at %d", width)
			}
		}
	}
}

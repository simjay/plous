package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func valueModel(t *testing.T) (*model, *resourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
	m.openResourceActions("")
	a := m.explore.action
	a.pending = ""
	a.resource = kubernetes.Resource{Name: "secrets", Kind: "Secret"}
	a.object = &unstructured.Unstructured{Object: map[string]any{
		"metadata":   map[string]any{"name": "fixture", "namespace": "default"},
		"stringData": map[string]any{"config.yaml": "password: fixture-private\n" + strings.Repeat("entry: example\n", 70) + "last: value\n"},
	}}
	m.startResourceAction(resourceAction{id: "value"})
	press(t, m, "enter")
	return m, a
}

func TestValueDocumentUsesReadOnlyEditorAndClearsOnHide(t *testing.T) {
	m, a := valueModel(t)
	d := a.valueDocument
	if !d.ReadOnly || d.Syntax != "yaml" || !strings.Contains(m.render(), "Plous Editor") {
		t.Fatal("value did not use the shared read-only Editor")
	}
	original := d.Text()
	m.Update(tea.PasteMsg{Content: "replacement"})
	typeText(m, "i")
	if d.Text() != original || d.Dirty() || d.Mode == docedit.Insert {
		t.Fatal("value inspection changed the document")
	}
	press(t, m, "esc")
	typeText(m, "/fixture-private")
	m.Update(tea.PasteMsg{Content: ":q bb"})
	if d.Query != "fixture-private:q bb" || m.command != nil || m.leftHidden {
		t.Fatal("value search dispatched literal input")
	}
	press(t, m, "esc")
	if a.valueDocument != d || d.Query != "" {
		t.Fatal("closing search hid the value")
	}
	press(t, m, "end")
	if !strings.Contains(ansi.Strip(m.valueDocumentBody(80, 18)), "last: value") {
		t.Fatal("value did not share document scrolling")
	}
	typeText(m, "v")
	press(t, m, "up")
	if cmd := press(t, m, "enter"); cmd != nil || a.stage != "valueview" {
		t.Fatal("reading Enter executed an action")
	}
	typeText(m, "y")
	if !strings.Contains(m.status, "clipboard") {
		t.Fatal("selected value text did not copy")
	}
	typeText(m, "v")
	press(t, m, "esc")
	press(t, m, "esc")
	if a.stage != "values" || a.valueDocument != nil || d.Text() != "" || d.Query != "" || d.SelectedText() != "" {
		t.Fatal("hiding a Secret left text in its Editor buffer")
	}
}

func TestValueDocumentExpansionHelpFocusAndLiteralPaste(t *testing.T) {
	m, a := valueModel(t)
	d := a.valueDocument
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		view := m.render()
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("value frame height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("value frame width overflow")
			}
		}
	}
	mainCommand(t, m, "z")
	if !m.viewExpanded() || m.leftHidden || m.rightHidden {
		t.Fatal("value expansion changed saved panel visibility")
	}
	mainCommand(t, m, "L")
	if d.LineNumbers {
		t.Fatal("value did not share line number controls")
	}
	typeText(m, "hh")
	if m.help == nil || !strings.Contains(m.panelHelpText(1), "Value (read only)") {
		t.Fatal("value help was not contextual")
	}
	press(t, m, "tab")
	press(t, m, "esc")
	if !m.viewExpanded() || a.valueDocument != d {
		t.Fatal("Help lost the expanded document")
	}
	press(t, m, "tab")
	if m.focus != 2 || m.viewExpanded() || a.valueDocument != d {
		t.Fatal("Tab did not preserve the value and restore the layout")
	}
	mainCommand(t, m, "c")
	if !m.chatEditing || a.valueDocument != d {
		t.Fatal("value prevented chat entry")
	}
	m.stopResourceAction()
	if d.Text() != "" {
		t.Fatal("closing the action retained the value buffer")
	}
}

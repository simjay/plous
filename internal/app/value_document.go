package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/textview"
)

func (m *model) valueDocumentActive() bool {
	return m.explore != nil && m.exploreVisible() && m.explore.action != nil && m.explore.action.stage == "valueview" && m.explore.action.valueDocument != nil
}

func (a *resourceActionState) openValueDocument(value resourceValue) {
	a.container, a.stage, a.cursor = value.label, "valueview", 0
	d := docedit.New(value.text)
	d.ReadOnly, d.Controls = true, true
	d.Name = oneLine(a.object.GetName()) + " · " + oneLine(value.label)
	d.Target = a.context + " · " + a.object.GetNamespace() + " · " + a.resource.Kind
	if a.resource.Key() == "secrets" {
		d.Target += " · Value revealed"
	}
	if strings.HasSuffix(value.label, ".yaml") || strings.HasSuffix(value.label, ".yml") {
		d.Syntax = "yaml"
	}
	a.valueDocument = d
}

func (a *resourceActionState) clearValueDocument() {
	if a.valueDocument != nil {
		a.valueDocument.RefreshReadOnly("")
		a.valueDocument = nil
	}
	a.exportText, a.exportValue = "", false
}

func (m *model) valueDocumentKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.action
	d := a.valueDocument
	key := msg.String()
	if !d.Searching && d.Mode != docedit.Select {
		if key == "a" {
			d.Controls = !d.Controls
			return nil
		}
		if d.Controls {
			switch key {
			case "left", "right":
				count, step := len(valueControls(a.resource)), 1
				if key == "left" {
					step = count - 1
				}
				a.cursor = (a.cursor + step) % count
				return nil
			case "enter":
				switch a.cursor {
				case 0:
					a.clearValueDocument()
					a.stage, a.cursor = "values", 0
				case 1:
					text := d.Text()
					a.clearValueDocument()
					a.stage, a.note = "done", "Selected value sent to the terminal clipboard."
					return tea.SetClipboard(text)
				case 2:
					a.exportValue, a.exportText = true, d.Text()
					a.stage = "export"
					a.input = newEditor(a.object.GetName() + "-" + a.container + ".pem")
				}
				return nil
			}
		}
	}
	if key != "esc" {
		d.Controls = false
	}
	if !d.Searching {
		switch key {
		case "home":
			d.ClearPrefix()
			d.Key("g", "g")
			key = "g"
		case "end":
			key = "G"
		}
	}
	event := d.Key(key, msg.Text)
	if event.Action == "back" {
		a.clearValueDocument()
		a.stage, a.cursor = "values", 0
	}
	return m.copyWorkspaceText(event.Copy)
}

func (m *model) valueDocumentBody(width, height int) string {
	a := m.explore.action
	d := a.valueDocument
	buttons := []textview.Action{}
	for i, label := range valueControls(a.resource) {
		button := textview.Action{Label: label, Focused: d.Controls && a.cursor == i}
		if button.Focused {
			button.Key = "Enter"
		}
		buttons = append(buttons, button)
	}
	label := "Actions"
	if d.Controls {
		label = "Read"
	}
	d.Actions = []textview.ActionGroup{
		{Label: "Value", Actions: buttons},
		{Label: "Read", Actions: []textview.Action{{Key: "a", Label: label}, {Key: "/", Label: "Find"}, {Key: "v", Label: "Select"}, {Key: "y", Label: "Copy"}}},
		m.documentDisplayActions(d),
	}
	return d.View(width, height, workspaceTheme())
}

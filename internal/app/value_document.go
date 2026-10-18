package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strconv"
)

func (m *model) valueDocumentActive() bool {
	return m.explore != nil && m.exploreVisible() && m.explore.Action != nil && m.explore.Action.Stage == "valueview" && m.explore.Action.ValueDocument != nil
}

func (m *model) valueDocumentKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	d := a.ValueDocument
	key := msg.String()
	if !d.Searching && d.Mode != docedit.Select {
		if key == "a" {
			d.Controls = !d.Controls
			return nil
		}
		if d.Controls {
			switch key {
			case "left", "right":
				count, step := len(monitor.ValueControls(a.Resource)), 1
				if key == "left" {
					step = count - 1
				}
				a.Cursor = (a.Cursor + step) % count
				return nil
			case "enter":
				switch a.Cursor {
				case 0:
					a.ClearValueDocument()
					a.Stage, a.Cursor = "values", 0
				case 1:
					text := d.Text()
					a.ClearValueDocument()
					a.Stage, a.Note = "done", "Selected value copy requested."
					return m.copyWorkspaceText(text)
				case 2:
					a.ExportValue, a.ExportText = true, d.Text()
					a.Stage = "export"
					a.Input = textview.NewField(a.Object.GetName() + "-" + a.Container + ".pem")
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
		a.ClearValueDocument()
		a.Stage, a.Cursor = "values", 0
	}
	return m.copyWorkspaceText(event.Copy)
}

func (m *model) valueDocumentBody(width, height int) string {
	a := m.explore.Action
	d := a.ValueDocument
	buttons := []textview.Action{}
	for i, label := range monitor.ValueControls(a.Resource) {
		button := textview.Action{Mouse: "valueaction/" + strconv.Itoa(i), Label: label, Focused: d.Controls && a.Cursor == i}
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
	return d.View(width, height, ui.WorkspaceTheme())
}

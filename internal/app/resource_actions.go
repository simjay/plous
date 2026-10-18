package app

import (
	"github.com/simjay/plous/internal/app/monitor"

	"slices"
)

func actionCommand(command string) string {
	if canonicalCommand(command) == "actions" {
		return "menu"
	}
	for _, item := range monitor.ResourceActionDefinitions {
		if command == item.Alias || command == item.Command {
			return item.Id
		}
	}
	return ""
}

func (m *model) openResourceActions(operation string) {
	e := m.explorer()
	if e.Action != nil {
		m.status = "Finish or cancel the open action first."
		return
	}
	if e.Detail == nil {
		m.status = "Open a resource's details first to use its actions."
		return
	}
	if !e.DetailValid || e.Client == nil {
		m.status = "Wait for the current object to load, or refresh its details."
		return
	}
	a := &monitor.ResourceActionState{Resource: e.Resource, Object: e.Detail.DeepCopy(), Client: e.Client, Resources: slices.Clone(e.Resources), Stage: "menu", Pending: "permissions", Reasons: map[string]string{}}
	if c := m.currentConnection(); c != nil {
		a.Context = c.Settings["context"]
	}
	a.Items = monitor.ActionsFor(a.Resource, a.Object)
	if operation != "" && operation != "menu" {
		found := false
		for i, item := range a.Items {
			if item.Id == operation {
				a.Cursor = i
				found = true
			}
		}
		if !found {
			m.status = "That action does not apply to this resource."
			return
		}
		a.Requested = true
		a.Direct = true
	}
	e.Action = a
	m.shortcutPrefix, m.status = "", ""
}

func (m *model) stopResourceAction() { m.explore.StopAction() }

func (m *model) resourceActionVisible() bool {
	return m.exploreVisible() && m.explore.Action != nil && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil && m.command == nil
}

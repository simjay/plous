package app

import (
	"github.com/simjay/plous/internal/app/monitor"

	"github.com/simjay/plous/internal/ui"
	"strings"
)

func (m *model) runDetailActionKey(key string) bool {
	e := m.explore
	if e == nil || e.Detail == nil || e.Tab == 4 {
		return false
	}
	if key == "a" {
		m.openResourceActions("")
		return true
	}
	for _, item := range monitor.ActionsFor(e.Resource, e.Detail) {
		if item.Alias == key && item.Id != "logs" {
			m.openResourceActions(item.Id)
			return true
		}
	}
	return false
}

func (m *model) resourceActionBar(width int) string {
	e := m.explore
	if e == nil || e.Detail == nil {
		return ""
	}
	more := ui.ActionButton("a", "More ▾", false)
	var buttons []string
	for _, item := range monitor.ActionsFor(e.Resource, e.Detail) {
		label := map[string]string{"logs": "Logs", "exec": "Exec", "scale": "Scale", "restart": "Restart", "edit": "Edit", "delete": "Delete", "namespace": "Use namespace", "value": "Values"}[item.Id]
		if label == "" {
			continue
		}
		unavailable := item.Verb != "" && !e.Resource.Supports(item.Verb, item.Subresource)
		if item.Id == "logs" || item.Id == "exec" {
			unavailable = false
		}
		button := ui.ActionButton(item.Alias, label, unavailable)
		candidate := append(append([]string(nil), buttons...), button, more)
		if len(ui.ActionButtonRows(candidate, width)) > 2 {
			break
		}
		buttons = append(buttons, button)
	}
	return strings.Join(ui.ActionButtonRows(append(buttons, more), width), "\n")
}

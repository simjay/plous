package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) runDetailActionKey(key string) bool {
	e := m.explore
	if e == nil || e.detail == nil || e.tab == 4 {
		return false
	}
	if key == "a" {
		m.openResourceActions("")
		return true
	}
	for _, item := range actionsFor(e.resource, e.detail) {
		if item.alias == key && item.id != "logs" {
			m.openResourceActions(item.id)
			return true
		}
	}
	return false
}

func actionButton(key, label string, unavailable bool) string {
	keyStyle, labelStyle := selected, lipgloss.NewStyle().Foreground(foreground)
	if unavailable {
		keyStyle, labelStyle = muted, muted
	}
	return muted.Render("[") + keyStyle.Render(" "+key+" ") + labelStyle.Render(label) + muted.Render(" ]")
}

func actionButtonRows(buttons []string, width int) []string {
	var rows []string
	line := ""
	for _, button := range buttons {
		if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(button) > width {
			rows = append(rows, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += clipped(button, width)
	}
	if line != "" {
		rows = append(rows, line)
	}
	return rows
}

func (m *model) resourceActionBar(width int) string {
	e := m.explore
	if e == nil || e.detail == nil {
		return ""
	}
	more := actionButton("a", "More ▾", false)
	var buttons []string
	for _, item := range actionsFor(e.resource, e.detail) {
		label := map[string]string{"logs": "Logs", "exec": "Exec", "scale": "Scale", "restart": "Restart", "edit": "Edit", "delete": "Delete", "namespace": "Use namespace", "value": "Values"}[item.id]
		if label == "" {
			continue
		}
		unavailable := item.verb != "" && !e.resource.Supports(item.verb, item.subresource)
		if item.id == "logs" || item.id == "exec" {
			unavailable = false
		}
		button := actionButton(item.alias, label, unavailable)
		candidate := append(append([]string(nil), buttons...), button, more)
		if len(actionButtonRows(candidate, width)) > 2 {
			break
		}
		buttons = append(buttons, button)
	}
	return strings.Join(actionButtonRows(append(buttons, more), width), "\n")
}

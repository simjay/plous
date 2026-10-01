package app

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/workspace"
)

var addedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#91C9AE"))
var removedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E4A0A5"))
var changedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#DEBF86"))

func changeSummary(c workspace.Change) string {
	if c.Before == nil {
		return "Workspace created"
	}
	if c.Before.Name != c.After.Name {
		return "Workspace renamed"
	}
	before, after := len(c.Before.Connections), len(c.After.Connections)
	if after > before {
		return "Connections added"
	}
	if after < before {
		return "Connection removed"
	}
	return "Settings updated"
}

func changeAppearance(c workspace.Change) (string, lipgloss.Style) {
	if c.Before == nil {
		return "+", addedStyle
	}
	if c.Before.Name == c.After.Name {
		if len(c.After.Connections) > len(c.Before.Connections) {
			return "+", addedStyle
		}
		if len(c.After.Connections) < len(c.Before.Connections) {
			return "−", removedStyle
		}
	}
	return "~", changedStyle
}

func activityRow(c workspace.Change, active bool, width int) string {
	mark, tone := changeAppearance(c)
	if active {
		return choiceLine(mark+" "+changeSummary(c), true, width)
	}
	return clipped("  "+tone.Render(mark)+" "+changeSummary(c), width)
}

func (m *model) changeDetail(c workspace.Change) string {
	mark, tone := changeAppearance(c)
	sections := []string{
		tone.Bold(true).Render(mark + " " + changeSummary(c)),
		muted.Render(fmt.Sprintf("Revision %d · %s", c.Revision, c.Time.Local().Format("Jan 02, 2006 · 15:04 MST"))),
		muted.Render("By " + oneLine(c.Actor)),
		"",
	}
	before := map[string]workspace.Connection{}
	if c.Before == nil {
		sections = append(sections, muted.Render("Workspace"), titleStyle.Render(oneLine(c.After.Name)), "")
		if len(c.After.Connections) == 0 {
			sections = append(sections, muted.Render("Connections"), "No connections yet.")
		}
	} else {
		if c.Before.Name != c.After.Name {
			sections = append(sections, activityField("Workspace name", c.Before.Name, c.After.Name, true, true), "")
		}
		for _, connection := range c.Before.Connections {
			before[connection.ID] = connection
		}
	}
	for _, connection := range c.After.Connections {
		previous, exists := before[connection.ID]
		var old *workspace.Connection
		if exists {
			old = &previous
		}
		if detail := connectionDetail(old, &connection); detail != "" {
			sections = append(sections, detail, "")
		}
		delete(before, connection.ID)
	}
	if c.Before != nil {
		for _, connection := range c.Before.Connections {
			if _, removed := before[connection.ID]; removed {
				sections = append(sections, connectionDetail(&connection, nil), "")
			}
		}
		if len(sections) == 4 {
			sections = append(sections, "No configuration values changed.")
		}
	}
	return strings.TrimRight(strings.Join(sections, "\n"), "\n")
}

func connectionDetail(before, after *workspace.Connection) string {
	connection, label, tone := after, "+ Added connection", addedStyle
	if after == nil {
		connection, label, tone = before, "− Removed connection", removedStyle
	} else if before != nil {
		label, tone = "~ Updated connection", changedStyle
	}
	fields := []string{}
	oldSettings, newSettings := map[string]string{}, map[string]string{}
	if before != nil {
		oldSettings = before.Settings
	}
	if after != nil {
		newSettings = after.Settings
	}
	if before != nil && after != nil {
		if before.Name != after.Name {
			fields = append(fields, activityField("Name", before.Name, after.Name, true, true))
		}
		if before.Tool != after.Tool {
			fields = append(fields, activityField("Tool", toolNames[before.Tool], toolNames[after.Tool], true, true))
		}
	}
	keys := map[string]bool{}
	for key := range oldSettings {
		keys[key] = true
	}
	for key := range newSettings {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		old, had := oldSettings[key]
		next, has := newSettings[key]
		if old != next || had != has {
			fields = append(fields, activityField(settingLabel(key), old, next, had, has))
		}
	}
	if before != nil && after != nil && len(fields) == 0 {
		return ""
	}
	heading := tone.Bold(true).Render(label) + "\n" + titleStyle.Render(oneLine(connection.Name)) + muted.Render(" · "+toolNames[connection.Tool])
	if len(fields) == 0 {
		return heading
	}
	return heading + "\n\n" + strings.Join(fields, "\n\n")
}

func activityField(label, before, after string, had, has bool) string {
	lines := []string{muted.Render(oneLine(label))}
	value := func(text string) string {
		if text == "" {
			return "(empty)"
		}
		return strings.ReplaceAll(plain(text), "\n", "\n          ")
	}
	if had {
		lines = append(lines, removedStyle.Render("− Before  "+value(before)))
	}
	if has {
		lines = append(lines, addedStyle.Render("+ After   "+value(after)))
	}
	return strings.Join(lines, "\n")
}

func settingLabel(key string) string {
	if label := map[string]string{"context": "Context", "namespace": "Namespace", "kubeconfig": "Kubeconfig", "path": "Path", "workspace": "Workspace", "repo": "Repository", "server": "Server", "revision": "Revision"}[key]; label != "" {
		return label
	}
	return oneLine(key)
}

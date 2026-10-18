package activity

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
	"maps"
	"sort"
	"strings"
)

// Compare identities and values, so a replacement is not mistaken for a rename.
func ChangedConnections(c workspace.Change) []workspace.Connection {
	before := map[string]workspace.Connection{}
	if c.Before != nil {
		for _, connection := range c.Before.Connections {
			before[connection.ID] = connection
		}
	}
	changed := []workspace.Connection{}
	for _, connection := range c.After.Connections {
		old, ok := before[connection.ID]
		if !ok || old.Name != connection.Name || old.Tool != connection.Tool || !maps.Equal(old.Settings, connection.Settings) {
			changed = append(changed, connection)
		}
		delete(before, connection.ID)
	}
	if c.Before != nil {
		for _, connection := range c.Before.Connections {
			if _, ok := before[connection.ID]; ok {
				changed = append(changed, connection)
			}
		}
	}
	return changed
}

func ChangeTitle(c workspace.Change) string {
	if c.Before == nil {
		return "Workspace created · " + ui.OneLine(c.After.Name)
	}
	if c.Before.Name != c.After.Name {
		return "Workspace renamed · " + ui.OneLine(c.After.Name)
	}
	connections := ChangedConnections(c)
	if len(connections) == 1 {
		connection := connections[0]
		action := "Updated "
		before, after := false, false
		for _, old := range c.Before.Connections {
			before = before || old.ID == connection.ID
		}
		for _, next := range c.After.Connections {
			after = after || next.ID == connection.ID
		}
		if !before {
			action = "Added "
		} else if !after {
			action = "Removed "
		}
		return action + ui.OneLine(connection.Name)
	}
	if len(connections) > 1 {
		action := "Updated"
		if len(c.After.Connections)-len(c.Before.Connections) == len(connections) {
			action = "Added"
		}
		if len(c.Before.Connections)-len(c.After.Connections) == len(connections) {
			action = "Removed"
		}
		return fmt.Sprintf("%s %s + %d more", action, ui.OneLine(connections[0].Name), len(connections)-1)
	}
	return "Settings updated · " + ui.OneLine(c.After.Name)
}

func ChangeSummary(c workspace.Change) string {
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

func ChangeAppearance(c workspace.Change) (string, lipgloss.Style) {
	if c.Before == nil {
		return "+", ui.AddedStyle
	}
	if c.Before.Name == c.After.Name {
		if len(c.After.Connections) > len(c.Before.Connections) {
			return "+", ui.AddedStyle
		}
		if len(c.After.Connections) < len(c.Before.Connections) {
			return "−", ui.RemovedStyle
		}
	}
	return "~", ui.ChangedStyle
}

func ConnectionDetail(before, after *workspace.Connection) string {
	connection, label, tone := after, "+ Added connection", ui.AddedStyle
	if after == nil {
		connection, label, tone = before, "− Removed connection", ui.RemovedStyle
	} else if before != nil {
		label, tone = "~ Updated connection", ui.ChangedStyle
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
			fields = append(fields, ActivityField("Name", before.Name, after.Name, true, true))
		}
		if before.Tool != after.Tool {
			fields = append(fields, ActivityField("Tool", appsettings.ToolNames[before.Tool], appsettings.ToolNames[after.Tool], true, true))
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
			fields = append(fields, ActivityField(SettingLabel(key), old, next, had, has))
		}
	}
	if before != nil && after != nil && len(fields) == 0 {
		return ""
	}
	heading := tone.Bold(true).Render(label) + "\n" + ui.TitleStyle.Render(ui.OneLine(connection.Name)) + ui.Muted.Render(" · "+appsettings.ToolNames[connection.Tool])
	if len(fields) == 0 {
		return heading
	}
	return heading + "\n\n" + strings.Join(fields, "\n\n")
}

func ActivityField(label, before, after string, had, has bool) string {
	lines := []string{ui.Muted.Render(ui.OneLine(label))}
	value := func(text string) string {
		if text == "" {
			return "(empty)"
		}
		return strings.ReplaceAll(ui.Plain(text), "\n", "\n          ")
	}
	if had {
		lines = append(lines, ui.RemovedStyle.Render("− Before  "+value(before)))
	}
	if has {
		lines = append(lines, ui.AddedStyle.Render("+ After   "+value(after)))
	}
	return strings.Join(lines, "\n")
}

func SettingLabel(key string) string {
	if label := map[string]string{"context": "Context", "namespace": "Namespace", "kubeconfig": "Kubeconfig", "path": "Path", "workspace": "Workspace", "repo": "Repository", "server": "Server", "revision": "Revision"}[key]; label != "" {
		return label
	}
	return ui.OneLine(key)
}

func ChangeDetail(c workspace.Change) string {
	mark, tone := ChangeAppearance(c)
	sections := []string{
		tone.Bold(true).Render(mark + " " + ChangeSummary(c)),
		ui.Muted.Render(fmt.Sprintf("Revision %d · %s", c.Revision, c.Time.Local().Format("Jan 02, 2006 · 15:04 MST"))),
		ui.Muted.Render("By " + ui.OneLine(c.Actor)),
		"",
	}
	before := map[string]workspace.Connection{}
	if c.Before == nil {
		sections = append(sections, ui.Muted.Render("Workspace"), ui.TitleStyle.Render(ui.OneLine(c.After.Name)), "")
		if len(c.After.Connections) == 0 {
			sections = append(sections, ui.Muted.Render("Connections"), "No connections yet.")
		}
	} else {
		if c.Before.Name != c.After.Name {
			sections = append(sections, ActivityField("Workspace Name", c.Before.Name, c.After.Name, true, true), "")
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
		if detail := ConnectionDetail(old, &connection); detail != "" {
			sections = append(sections, detail, "")
		}
		delete(before, connection.ID)
	}
	if c.Before != nil {
		for _, connection := range c.Before.Connections {
			if _, removed := before[connection.ID]; removed {
				sections = append(sections, ConnectionDetail(&connection, nil), "")
			}
		}
		if len(sections) == 4 {
			sections = append(sections, "No configuration values changed.")
		}
	}
	return strings.TrimRight(strings.Join(sections, "\n"), "\n")
}

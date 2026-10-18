package app

import (
	"github.com/charmbracelet/x/ansi"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) settingsRows() []appsettings.Row {
	rows := []appsettings.Row{
		{Kind: appsettings.AgentRow, ID: "agent", Label: "Agent: " + m.settings.MainAgent.Effective().Label(), Section: "Application"},
		{Kind: appsettings.MouseRow, ID: "mouse", Label: m.mouseSettingLabel(), Section: "Application"},
	}
	add := func(kind appsettings.RowKind, id, label string) {
		rows = append(rows, appsettings.Row{Kind: kind, ID: id, Label: label, Section: "Workspace"})
	}
	if m.currentRoot {
		add(appsettings.ContextRow, "context", "Choose Context")
		add(appsettings.KubeconfigRow, "kubeconfig", "Choose Kubeconfig")
		add(appsettings.SaveWorkspaceRow, "workspace-save", "Save as Workspace")
	} else if w := m.current(); w != nil {
		add(appsettings.RenameWorkspaceRow, "workspace-rename", "Rename Workspace")
		add(appsettings.AddConnectionRow, "connection-add", "Add Connection")
		for _, c := range w.Connections {
			add(appsettings.ConnectionRow, "connection/"+c.ID, "Edit "+appsettings.ToolNames[c.Tool]+" / "+c.Name)
		}
	} else {
		add(appsettings.CreateWorkspaceRow, "workspace-create", "Create Workspace")
	}
	return append(rows, appsettings.Row{Kind: appsettings.CloseRow, ID: "close", Label: "Close"})
}

func (m *model) updateSettings(key string) {
	rows := m.settingsRows()
	m.cursor = max(0, min(m.cursor, len(rows)-1))
	switch key {
	case "up":
		m.cursor = max(0, m.cursor-1)
	case "down":
		m.cursor = min(len(rows)-1, m.cursor+1)
	case "enter":
		row := rows[m.cursor]
		switch row.Kind {
		case appsettings.AgentRow:
			m.beginAgentSettings()
		case appsettings.MouseRow:
			next := m.settings
			next.MouseDisabled = !next.MouseDisabled
			if m.saveAISettings(next) {
				m.status = m.mouseSettingLabel()
			}
		case appsettings.ContextRow:
			m.closeSettings()
			m.executeExplore("connections")
		case appsettings.KubeconfigRow:
			m.beginCurrentPath()
		case appsettings.SaveWorkspaceRow:
			m.beginSaveCurrent()
		case appsettings.RenameWorkspaceRow:
			m.beginRename()
		case appsettings.AddConnectionRow:
			m.beginToolSetup(false)
		case appsettings.ConnectionRow:
			for _, c := range m.current().Connections {
				if "connection/"+c.ID == row.ID {
					m.beginConnection(c.Tool, &c)
					break
				}
			}
		case appsettings.CreateWorkspaceRow:
			m.beginWorkspace()
		case appsettings.CloseRow:
			m.closeSettings()
		}
	}
}

func (m *model) settingsBody(width, height int) string {
	scope := "Current Workspace (none selected)"
	if w := m.current(); w != nil {
		scope = "Current Workspace: " + w.Name
	}
	if m.currentRoot {
		scope = m.currentSummary()
	}
	var lines []string
	selectedRow, section := 0, ""
	for i, row := range m.settingsRows() {
		if row.Section != section {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			section = row.Section
			switch section {
			case "Application":
				lines = append(lines, ui.SectionHeadingStyle.Render("Application"), ui.Muted.Render("Shared across workspaces"), "")
			case "Workspace":
				lines = append(lines, ui.SectionHeadingStyle.Render(ui.Clipped(ui.OneLine(scope), width)))
				if m.currentRoot && m.currentDirectory != nil {
					lines = append(lines, ui.Muted.Render(ansi.Wrap("Code: "+ui.OneLine(m.currentDirectory.Connections[0].Settings["path"]), width, "")))
				}
				if m.current() != nil && !m.currentRoot {
					lines = append(lines, appsettings.WorkspaceLocation(width)...)
				}
				lines = append(lines, "")
			}
		}
		if i == m.cursor {
			selectedRow = len(lines)
		}
		lines = append(lines, ui.MouseChoice(ui.SettingsChoiceLine(row.Label, i == m.cursor, width), i, row.ID))
	}
	return ui.ScrollSelection(lines, selectedRow, height)
}

func (m *model) selectSettingsRow(id string) {
	for i, row := range m.settingsRows() {
		if row.ID == id {
			m.cursor = i
			return
		}
	}
}

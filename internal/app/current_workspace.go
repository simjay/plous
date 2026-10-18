package app

import (
	"os"
	"path/filepath"

	"github.com/simjay/plous/internal/workspace"
)

func (m *model) captureCurrentDirectory() {
	path, err := os.Getwd()
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
	}
	if err != nil {
		m.currentDirectoryError = "Launch directory unavailable: " + err.Error()
		return
	}
	scope := workspace.CurrentDirectoryScope(path)
	m.currentDirectory = &workspace.Workspace{ID: scope.ID, Name: "Current Workspace", Connections: []workspace.Connection{scope.Connection}}
}

// Both transient connections share one tree root and retain independent scopes.
func (m *model) currentTreeKey() string {
	if m.currentCluster != nil {
		return m.currentCluster.ID
	}
	if m.currentDirectory != nil {
		return m.currentDirectory.ID
	}
	return "current"
}

func (m *model) currentConnections() []workspace.Connection {
	var connections []workspace.Connection
	for _, w := range []*workspace.Workspace{m.currentCluster, m.currentDirectory} {
		if w != nil {
			connections = append(connections, w.Connections...)
		}
	}
	return connections
}

func (m *model) saveCurrentWorkspace(name string) {
	connections := m.currentConnections()
	if len(connections) == 0 {
		m.status = "No current connection is available to save."
		return
	}
	origin, tool, selected := m.currentTreeKey(), "", 0
	if m.currentRoot && m.savedConnection() != nil {
		origin, tool = m.active, m.tool
	}
	for i := range connections {
		if m.currentRoot && connections[i].ID == m.connection {
			selected = i
		}
		connections[i].ID, connections[i].Target = "", nil
		connections[i].TargetCaptured = false
	}
	if m.saveWorkspace(workspace.Workspace{Name: name, OriginScope: origin, Connections: connections}) {
		w := m.current()
		m.form = nil
		m.closeSettings()
		if tool == "" || tool == "monitor" || tool == "code" {
			tool = w.Connections[selected].Tool
			if sourceConnection(tool) {
				tool = "files"
			}
		}
		m.selectScope(w.ID, tool, w.Connections[selected].ID)
		m.mode, m.focus = 1, 1
		m.status = "Workspace saved. Earlier conversations remain in Current Workspace history."
	}
}

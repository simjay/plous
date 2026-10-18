package app

import (
	"strings"
)

func (m *model) restoreCollapsedTree(key string) {
	for _, w := range m.treeWorkspaces() {
		for _, c := range w.Connections {
			m.setTreeExpanded(c.ID, false)
			if strings.HasPrefix(key, c.ID+"/") {
				key = c.ID
			}
		}
	}
	parents := map[string]string{}
	for _, row := range m.treeRowsExpanded(true) {
		parents[row.key] = row.parent
	}
	rows := m.treeRows()
	for key != "" {
		for i, row := range rows {
			if row.key == key {
				m.treeCursor = i
				return
			}
		}
		key = parents[key]
	}
}

// A tree refresh reads saved source configuration without selecting that scope.
func (m *model) queueTreeSources(row treeRow) int {
	if !row.sourceRefreshable() {
		return 0
	}
	count := 0
	for _, w := range m.treeWorkspaces() {
		if w.ID != row.workspace {
			continue
		}
		for i := range w.Connections {
			c := &w.Connections[i]
			if !sourceConnection(c.Tool) || row.connection != "" && c.ID != row.connection {
				continue
			}
			s := m.codeStateFor(w.ID, c)
			s.Rescan = true
			count++
		}
	}
	return count
}

func (row treeRow) sourceRefreshable() bool {
	return row.kind == "workspace" || row.kind == "area" && row.tool == "code" || row.kind == "connection" && row.tool == "files" || row.kind == "codeview"
}

func (m *model) refreshTree() {
	rows := m.treeRows()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		row := rows[m.treeCursor]
		if row.kind == "current" {
			m.requestCurrent("", false)
			if m.currentDirectory != nil {
				m.queueTreeSources(treeRow{kind: "area", tool: "code", workspace: m.currentDirectory.ID})
			}
			m.status = "Refreshing current connections. Use :K to select a context."
			return
		}
		if row.kind == "connection" && monitorCluster(row.tool) || row.kind == "capability" || row.kind == "gitopssection" || row.kind == "helmsection" || row.kind == "discovery" {
			for _, w := range m.treeWorkspaces() {
				for _, c := range w.Connections {
					if w.ID != row.workspace || c.ID != row.connection {
						continue
					}
					cluster := m.clusterFor(w.ID, c)
					cluster.refresh()
					for _, v := range m.gitOpsViews {
						if v.Cluster == cluster {
							if v.Cancel != nil {
								v.Cancel()
								v.Cancel = nil
								v.Run++
							}
							v.State.Loaded = false
						}
					}
					m.status = "Cluster discovery refresh requested."
					return
				}
			}
		}
	}
	if m.treeCursor < 0 || m.treeCursor >= len(rows) || m.queueTreeSources(rows[m.treeCursor]) == 0 {
		m.status = "Select a source, Code section, or workspace to refresh its sources."
		return
	}
	m.status = "Source refresh requested."
}

func (m *model) expandTreeRow(row treeRow, open bool) {
	m.setTreeExpanded(row.key, open)
	if open && !row.open {
		m.queueTreeSources(row)
	}
}

package app

type treeRow struct {
	key, kind, label, workspace, tool, connection, parent string
	depth                                                 int
	open                                                  bool
}

func (m *model) treeRows() []treeRow {
	rows := []treeRow{}
	for _, w := range m.workspaces {
		rows = append(rows, treeRow{key: w.ID, kind: "workspace", label: w.Name, workspace: w.ID, open: m.expanded[w.ID]})
		if !m.expanded[w.ID] {
			continue
		}
		for _, tool := range toolTypes {
			count := 0
			for _, c := range w.Connections {
				if c.Tool == tool {
					count++
				}
			}
			if count == 0 {
				continue
			}
			key := w.ID + "/" + tool
			rows = append(rows, treeRow{key: key, kind: "tool", label: toolNames[tool], workspace: w.ID, tool: tool, parent: w.ID, depth: 1, open: m.expanded[key]})
			if m.expanded[key] {
				for _, c := range w.Connections {
					if c.Tool == tool {
						rows = append(rows, treeRow{key: c.ID, kind: "connection", label: c.Name, workspace: w.ID, tool: tool, connection: c.ID, parent: key, depth: 2})
					}
				}
			}
		}
		rows = append(rows, treeRow{key: w.ID + "/settings", kind: "settings", label: "Workspace settings", workspace: w.ID, parent: w.ID, depth: 1})
	}
	return append(rows, treeRow{key: "new", kind: "new", label: "+ New workspace"})
}
func (m *model) updateTree(key string) {
	rows := m.treeRows()
	m.treeCursor = max(0, min(m.treeCursor, len(rows)-1))
	row := rows[m.treeCursor]
	switch key {
	case "up":
		m.treeCursor = max(0, m.treeCursor-1)
	case "down":
		m.treeCursor = min(len(rows)-1, m.treeCursor+1)
	case "home":
		m.treeCursor = 0
	case "end":
		m.treeCursor = len(rows) - 1
	case "pgup":
		m.treeCursor = max(0, m.treeCursor-10)
	case "pgdown":
		m.treeCursor = min(len(rows)-1, m.treeCursor+10)
	case "right":
		if row.kind == "workspace" || row.kind == "tool" {
			m.expanded[row.key] = true
		}
	case "left":
		if row.open {
			m.expanded[row.key] = false
		} else {
			for i, r := range rows {
				if r.key == row.parent {
					m.treeCursor = i
					break
				}
			}
		}
	case "enter":
		if m.form != nil {
			m.status = "Finish or cancel the open form in the main panel first."
			return
		}
		switch row.kind {
		case "workspace", "tool", "connection":
			m.selectScope(row.workspace, row.tool, row.connection)
		case "settings":
			m.selectScope(row.workspace, "", "")
			m.page = "settings"
			m.cursor = 0
			m.focus = 1
		case "new":
			m.beginWorkspace()
			m.focus = 1
		}
	}
}

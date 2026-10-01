package app

import (
	"slices"
	"strings"

	"github.com/simjay/plous/internal/kubernetes"
)

type treeRow struct {
	key, kind, label, workspace, tool, connection, parent string
	depth                                                 int
	open                                                  bool
	resource                                              string
}

func (row treeRow) isBranch() bool {
	return row.kind == "workspace" || row.kind == "tool" || row.kind == "category" || row.kind == "connection" && row.tool == "kubernetes"
}

func (m *model) treeRows() []treeRow {
	query := m.searchQuery(0)
	rows := m.treeRowsExpanded(strings.TrimSpace(query) != "")
	if strings.TrimSpace(query) == "" {
		return rows
	}
	parents, matches, keep := map[string]string{}, map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		parents[row.key] = row.parent
		if row.kind == "new" || row.kind == "addtool" || row.kind == "addcontext" || row.kind == "settings" {
			continue
		}
		matches[row.key] = searchContains(row.label+" "+row.resource, query) || matches[row.parent]
		if matches[row.key] {
			for key := row.key; key != ""; key = parents[key] {
				keep[key] = true
			}
		}
	}
	filtered := make([]treeRow, 0, len(rows))
	for _, row := range rows {
		if keep[row.key] || row.kind == "settings" {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (m *model) treeRowsExpanded(all bool) []treeRow {
	m.migrateCategoryExpansion()
	rows := []treeRow{}
	for _, w := range m.workspaces {
		rows = append(rows, treeRow{key: w.ID, kind: "workspace", label: w.Name, workspace: w.ID, open: m.expanded[w.ID]})
		if !all && !m.expanded[w.ID] {
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
			if all || m.expanded[key] {
				for _, c := range w.Connections {
					if c.Tool == tool {
						rows = append(rows, treeRow{key: c.ID, kind: "connection", label: c.Name, workspace: w.ID, tool: tool, connection: c.ID, parent: key, depth: 2, open: m.expanded[c.ID]})
						if tool == "kubernetes" && (all || m.expanded[c.ID]) {
							rows = append(rows, m.resourceTreeRows(w.ID, c.ID, all)...)
						}
					}
				}
				if tool == "kubernetes" {
					rows = append(rows, treeRow{key: key + "/add", kind: "addcontext", label: "+ Add context", workspace: w.ID, tool: tool, parent: key, depth: 2})
				}
			}
		}
		rows = append(rows, treeRow{key: w.ID + "/add", kind: "addtool", label: "+ Add tool", workspace: w.ID, parent: w.ID, depth: 1})
	}
	if all {
		for i := range rows {
			rows[i].open = rows[i].isBranch()
		}
	}
	return append(rows, treeRow{key: "new", kind: "new", label: "+ New workspace"}, treeRow{key: "settings", kind: "settings", label: "Settings"})
}
func (m *model) updateTree(key string) {
	if m.explore != nil && m.explore.action != nil && key == "enter" {
		m.status = "Finish or cancel the open resource action first."
		m.focus = 1
		return
	}
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
		if row.isBranch() {
			m.setTreeExpanded(row.key, true)
		}
	case "left":
		if row.isBranch() && row.open {
			m.setTreeExpanded(row.key, false)
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
		case "resource":
			m.selectScope(row.workspace, row.tool, row.connection)
			m.mode = 1
			m.chooseExploreResource(row.resource)
			m.focus = 1
		case "addtool", "addcontext":
			m.selectScope(row.workspace, row.tool, "")
			m.beginToolSetup(row.kind == "addcontext")
		case "workspace", "tool", "connection":
			m.selectScope(row.workspace, row.tool, row.connection)
		case "settings":
			m.openSettings()
			m.focus = 1
		case "new":
			m.beginWorkspace()
			m.focus = 1
		}
		if row.isBranch() {
			// Search exposes folded paths temporarily; Enter expands its result.
			m.setTreeExpanded(row.key, !row.open || m.searches[0] != nil)
		}
	}
}

func (m *model) setTreeExpanded(key string, open bool) {
	m.expanded[key] = open
	delete(m.autoExpanded, key)
}

func categoryTreeKey(connection, category string) string {
	return connection + "/category/" + kubernetes.CategoryID(category)
}

func canonicalTreeKey(key string) string {
	connection, category, ok := strings.Cut(key, "/category/")
	if !ok {
		return key
	}
	return categoryTreeKey(connection, category)
}

func (m *model) migrateCategoryExpansion() {
	for key, open := range m.expanded {
		canonical := canonicalTreeKey(key)
		if canonical != key {
			if _, exists := m.expanded[canonical]; !exists {
				m.expanded[canonical] = open
			}
			delete(m.expanded, key)
		}
	}
}

func (m *model) resourceTreeRows(workspace, connection string, all bool) []treeRow {
	resources := m.exploreCatalogs[workspace+"/"+connection]
	if workspace == m.active && connection == m.connection && m.explore != nil && m.explore.key != "" && !m.explore.connecting {
		resources = m.explore.resources
	}
	var rows []treeRow
	for _, category := range kubernetes.Categories {
		var members []kubernetes.Resource
		for _, resource := range resources {
			if kubernetes.Category(resource) == category {
				members = append(members, resource)
			}
		}
		if len(members) == 0 {
			continue
		}
		slices.SortFunc(members, kubernetes.CompareResources)
		key := categoryTreeKey(connection, category)
		rows = append(rows, treeRow{key: key, kind: "category", label: category, workspace: workspace, tool: "kubernetes", connection: connection, parent: connection, depth: 3, open: m.expanded[key]})
		if !all && !m.expanded[key] {
			continue
		}
		for _, resource := range members {
			rows = append(rows, treeRow{key: connection + "/resource/" + resource.Key(), kind: "resource", label: resourceTitle(resource), resource: resource.Key(), workspace: workspace, tool: "kubernetes", connection: connection, parent: key, depth: 4})
		}
	}
	return rows
}

func (m *model) syncExploreTree() {
	e := m.explore
	if e == nil || e.key == "" || e.connecting || !m.exploreActive() || len(e.resources) == 0 {
		return
	}
	if m.exploreCatalogs == nil {
		m.exploreCatalogs = map[string][]kubernetes.Resource{}
	}
	rows := m.treeRows()
	cursorKey := ""
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		cursorKey = rows[m.treeCursor].key
	}
	m.exploreCatalogs[m.active+"/"+m.connection] = slices.Clone(e.resources)
	selection := m.active + "/" + m.connection + "/" + e.preference.Resource
	if selection != m.exploreTreeSelection {
		m.exploreTreeSelection = selection
		path := []string{m.active, m.active + "/kubernetes", m.connection}
		if resource, ok := kubernetes.Find(e.resources, e.preference.Resource); ok {
			path = append(path, categoryTreeKey(m.connection, kubernetes.Category(resource)))
		}
		if m.autoExpanded == nil {
			m.autoExpanded = map[string]bool{}
		}
		for key := range m.autoExpanded {
			if !slices.Contains(path, key) {
				delete(m.expanded, key)
				delete(m.autoExpanded, key)
			}
		}
		for _, key := range path {
			if _, saved := m.expanded[key]; !saved {
				m.expanded[key] = true
				m.autoExpanded[key] = true
			}
		}
	}
	for i, row := range m.treeRows() {
		if row.key == cursorKey {
			m.treeCursor = i
			break
		}
	}
}

// Root back-navigation follows the visible path without undoing deliberate folds.
func (m *model) focusRootSidebar() {
	if m.focus != 1 || m.leftHidden || m.tooSmall() {
		return
	}
	rows := m.treeRows()
	path := []string{}
	if m.mode == 1 && m.tool == "kubernetes" && m.explore != nil {
		key := m.explore.preference.Resource
		path = append(path, m.connection+"/resource/"+key)
		if resource, ok := kubernetes.Find(m.explore.resources, key); ok {
			path = append(path, categoryTreeKey(m.connection, kubernetes.Category(resource)))
		}
	}
	path = append(path, m.connection, m.active+"/"+m.tool, m.active)
	for _, key := range path {
		for i, row := range rows {
			if key != "" && row.key == key {
				m.treeCursor = i
				m.focus = 0
				m.shortcutPrefix = ""
				return
			}
		}
	}
	m.treeCursor = max(0, min(m.treeCursor, len(rows)-1))
	m.focus = 0
	m.shortcutPrefix = ""
}

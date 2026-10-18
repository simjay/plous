package app

import (
	"github.com/simjay/plous/internal/workspace"
	"slices"
	"strings"

	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

type treeRow struct {
	badge                                                 string
	key, kind, label, workspace, tool, connection, parent string
	depth                                                 int
	open                                                  bool
	resource                                              string
}

func (row treeRow) isBranch() bool {
	return row.kind == "current" || row.kind == "savedgroup" || row.kind == "workspace" || row.kind == "area" || row.kind == "capability" || row.kind == "tool" || row.kind == "category" || row.kind == "connection" && (monitorCluster(row.tool) || row.tool == "files")
}

func (row treeRow) isAddAction() bool {
	return row.kind == "new" || row.kind == "addtool" || row.kind == "addcontext" || row.kind == "addarea"
}

func (m *model) treeRows() []treeRow {
	m.migrateCategoryExpansion()
	return m.treeRowsRead()
}

func (m *model) treeRowsRead() []treeRow {
	query := m.searchQuery(0)
	rows := m.treeRowsExpandedRead(strings.TrimSpace(query) != "")
	if strings.TrimSpace(query) == "" {
		return rows
	}
	parents, matches, keep := map[string]string{}, map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		parents[row.key] = row.parent
		if row.kind == "new" || row.kind == "addtool" || row.kind == "addcontext" || row.kind == "addarea" || row.kind == "settings" {
			continue
		}
		matches[row.key] = ui.SearchContains(row.label+" "+row.badge+" "+row.resource, query) || matches[row.parent]
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
	return m.treeRowsExpandedRead(all)
}

func (m *model) treeRowsExpandedRead(all bool) []treeRow {
	root := m.currentTreeKey()
	currentOpen := all || m.expanded[root]
	rows := []treeRow{{key: root, kind: "current", label: "Current Workspace", open: currentOpen}}
	if currentOpen && m.currentCluster == nil {
		open, set := m.expanded["current/monitor"]
		open = all || !set || open
		rows = append(rows, treeRow{key: "current/monitor", kind: "area", label: "Monitor", tool: "monitor", parent: root, depth: 1, open: open})
		if open {
			rows = append(rows, treeRow{key: "choose-current", kind: "choosecurrent", label: "Choose Context", parent: "current/monitor", depth: 2})
		}
	}
	finishCurrent := func() {
		if currentOpen && len(m.currentConnections()) > 0 {
			rows = append(rows, treeRow{key: "save-current", kind: "savecurrent", label: "Save as Workspace", parent: root, depth: 1})
		}
		rows = append(rows, treeRow{key: "saved-workspaces", kind: "savedgroup", label: "Workspaces", open: true})
	}
	savedAdded := false
	for _, w := range m.treeWorkspaces() {
		current := workspace.IsCurrentScope(w.ID)
		if !current && !savedAdded {
			finishCurrent()
			savedAdded = true
		}
		parent := w.ID
		if current {
			parent = root
			if !currentOpen {
				continue
			}
		} else {
			rows = append(rows, treeRow{key: w.ID, kind: "workspace", label: w.Name, workspace: w.ID, open: m.expanded[w.ID]})
			if !all && !m.expanded[w.ID] {
				continue
			}
		}
		connections := m.sidebarConnections(w)
		for _, area := range []string{"monitor", "code"} {
			if current && area != connectionArea(w.Connections[0].Tool) {
				continue
			}
			key := w.ID + "/" + area
			open, exists := m.expanded[key]
			if !exists {
				open = true
			}
			label := "Monitor"
			if area == "code" {
				label = "Code"
			}
			rows = append(rows, treeRow{key: key, kind: "area", label: label, workspace: w.ID, tool: area, parent: parent, depth: 1, open: open})
			if !all && !open {
				continue
			}
			for _, c := range connections {
				if connectionArea(c.Tool) != area {
					continue
				}
				badge := c.Tool
				if monitorCluster(c.Tool) {
					badge = "k8s"
				}
				if c.Tool == "terraform" {
					badge = "tf"
				}
				tool := c.Tool
				if sourceConnection(c.Tool) {
					badge, tool = "local", "files"
					if c.Settings["source_type"] == "Git repository" {
						badge = "remote"
					}
				}
				rows = append(rows, treeRow{key: c.ID, kind: "connection", label: c.Name, badge: badge, workspace: w.ID, tool: tool, connection: c.ID, parent: key, depth: 2, open: m.expanded[c.ID]})
				if !all && !m.expanded[c.ID] {
					continue
				}
				if sourceConnection(c.Tool) {
					for _, view := range appcode.CodeViews {
						status := m.codeViewScanStatus(w.ID, c.ID, view.Id)
						tool := appcode.BrowserTool(view.Id)
						rows = append(rows, treeRow{key: c.ID + "/" + view.Id, kind: "codeview", label: view.Label, badge: status, workspace: w.ID, tool: tool, connection: c.ID, parent: c.ID, depth: 3, resource: view.Id})
					}
				} else if monitorCluster(c.Tool) {
					for _, view := range m.clusterCapabilities(w.ID, c) {
						viewKey := c.ID + "/view/" + view
						opened := m.expanded[viewKey]
						rows = append(rows, treeRow{key: viewKey, kind: "capability", label: appsettings.ToolNames[view], workspace: w.ID, tool: view, connection: c.ID, parent: c.ID, depth: 3, open: opened})
						if !all && !opened {
							continue
						}
						if gitOpsTool(view) {
							for _, section := range gitops.Sections(gitops.Tool(view)) {
								rows = append(rows, treeRow{key: c.ID + "/" + view + "/" + string(section), kind: "gitopssection", label: string(section), workspace: w.ID, tool: view, connection: c.ID, parent: viewKey, depth: 4, resource: string(section)})
							}
						} else if view == "kubernetes" {
							children := m.resourceTreeRows(w.ID, c.ID, all)
							for i := range children {
								children[i].depth++
								if children[i].parent == c.ID {
									children[i].parent = viewKey
								}
							}
							rows = append(rows, children...)
						} else {
							for _, section := range monitor.HelmSections {
								rows = append(rows, treeRow{key: c.ID + "/helm/" + section, kind: "helmsection", label: section, workspace: w.ID, tool: view, connection: c.ID, parent: viewKey, depth: 4, resource: section})
							}
						}
					}
					if cluster := m.clusterStates[clusterKey(w.ID, c)]; cluster != nil && (cluster.Err != nil || cluster.Warning != "" || cluster.HelmErr != nil) {
						rows = append(rows, treeRow{key: c.ID + "/discovery", kind: "discovery", label: "Tool discovery incomplete", badge: "g retry", workspace: w.ID, tool: c.Tool, connection: c.ID, parent: c.ID, depth: 3})
					}
				}
			}
			if workspace.IsCurrentScope(w.ID) {
				continue
			}
			label = "+ Add Context"
			if area == "code" {
				label = "+ Add Source"
			}
			rows = append(rows, treeRow{key: key + "/add", kind: "addarea", label: label, workspace: w.ID, tool: area, parent: key, depth: 2})
		}
		if workspace.IsCurrentScope(w.ID) {
			continue
		}
		rows = append(rows, treeRow{key: w.ID + "/add", kind: "addtool", label: "+ Add Connection", workspace: w.ID, parent: w.ID, depth: 1})
	}
	if all {
		for i := range rows {
			rows[i].open = rows[i].isBranch()
		}
	}
	if !savedAdded {
		finishCurrent()
	}
	savedOpen, set := m.expanded["saved-workspaces"]
	if !set {
		savedOpen = true
	}
	if all {
		savedOpen = true
	}
	visible := rows[:0]
	for _, row := range rows {
		if row.kind == "savedgroup" {
			row.open = savedOpen
		}
		if row.workspace != "" && !workspace.IsCurrentScope(row.workspace) {
			if !savedOpen {
				continue
			}
			row.depth++
			if row.kind == "workspace" {
				row.parent = "saved-workspaces"
			}
		}
		visible = append(visible, row)
	}
	rows = visible
	if savedOpen {
		rows = append(rows, treeRow{key: "new", kind: "new", label: "+ New Workspace", parent: "saved-workspaces", depth: 1})
	}
	return append(rows, treeRow{key: "settings", kind: "settings", label: "Settings"})
}

func (m *model) updateTree(key string) {
	if m.explore != nil && m.explore.Action != nil && key == "enter" {
		m.status = "Finish or cancel the open resource action first."
		m.focus = 1
		return
	}
	rows := m.treeRows()
	m.treeCursor = max(0, min(m.treeCursor, len(rows)-1))
	row := rows[m.treeCursor]
	sourceBranch := row.kind == "connection" && row.tool == "files"
	selectingConnection := row.kind == "connection" && (row.workspace != m.active || row.connection != m.connection || !sourceBranch && row.tool != m.tool)
	switch key {
	case "g":
		m.refreshTree()
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
			m.expandTreeRow(row, true)
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
		case "discovery":
			m.refreshTree()
		case "current":
			m.enterCurrent()
		case "choosecurrent":
			m.enterCurrent()
			m.executeExplore("connections")
		case "savecurrent":
			m.beginSaveCurrent()
		case "savedgroup":
		case "addarea":
			m.selectScope(row.workspace, "", "")
			m.beginToolSetup(row.tool == "monitor")
			if row.tool == "code" {
				m.toolSetup.RepositoryOnly = true
				m.beginConnection("code", nil)
			}
		case "capability":
			m.selectScope(row.workspace, row.tool, row.connection)
			m.mode = 1
			if gitOpsTool(row.tool) {
				m.focus = 1
			}
		case "codeview":
			m.openCodeView(row)
		case "gitopssection":
			m.selectScope(row.workspace, row.tool, row.connection)
			m.chooseGitOpsSection(row.resource)
			m.mode, m.focus = 1, 1
		case "helmsection":
			m.selectScope(row.workspace, row.tool, row.connection)
			m.mode = 1
			m.chooseHelmSection(row.resource)
			m.focus = 1
		case "resource":
			m.selectScope(row.workspace, row.tool, row.connection)
			m.mode = 1
			m.chooseExploreResource(row.resource)
			m.focus = 1
		case "addtool", "addcontext":
			m.selectScope(row.workspace, row.tool, "")
			m.beginToolSetup(row.kind == "addcontext")
		case "connection":
			if sourceBranch {
				if selectingConnection {
					view := row
					view.tool, view.resource = "files", appcode.CodeViews[0].Id
					if !m.openCodeView(view) {
						return
					}
					m.focus = 0
				}
			} else {
				m.selectScope(row.workspace, row.tool, row.connection)
			}
		case "workspace", "area", "tool":
			if row.key == "current/monitor" {
				m.enterCurrent()
			} else {
				m.selectScope(row.workspace, row.tool, row.connection)
			}
		case "settings":
			m.openSettings()
			m.focus = 1
		case "new":
			m.beginWorkspace()
			m.focus = 1
		}
		if row.isBranch() {
			// Search exposes folded paths temporarily; Enter expands its result.
			open := !row.open || selectingConnection || m.searches[0] != nil
			if row.kind == "current" {
				open = true
			}
			m.setTreeExpanded(row.key, open)
			if open {
				m.queueTreeSources(row)
			}
		}
	}
}

func (m *model) setTreeExpanded(key string, open bool) {
	m.expanded[key] = open
	delete(m.autoExpanded, key)
	if open {
		return
	}
	for _, w := range m.treeWorkspaces() {
		for _, c := range w.Connections {
			if c.ID != key || !monitorCluster(c.Tool) {
				continue
			}
			for _, tool := range []string{"kubernetes", "helm", "argocd", "flux"} {
				branch := c.ID + "/view/" + tool
				m.expanded[branch] = false
				delete(m.autoExpanded, branch)
			}
			return
		}
	}
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
	if workspace == m.active && connection == m.connection && m.explore != nil && m.explore.Key != "" && !m.explore.Connecting {
		resources = m.explore.Resources
	}
	var rows []treeRow
	for _, category := range kubernetes.Categories {
		var members []kubecommon.Resource
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
			rows = append(rows, treeRow{key: connection + "/resource/" + resource.Key(), kind: "resource", label: monitor.ResourceTitle(resource), resource: resource.Key(), workspace: workspace, tool: "kubernetes", connection: connection, parent: key, depth: 4})
		}
	}
	return rows
}

func (m *model) syncExploreTree() {
	e := m.explore
	if e == nil || e.Key == "" || e.Connecting || !m.exploreActive() || len(e.Resources) == 0 {
		return
	}
	if m.exploreCatalogs == nil {
		m.exploreCatalogs = map[string][]kubecommon.Resource{}
	}
	rows := m.treeRows()
	cursorKey := ""
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		cursorKey = rows[m.treeCursor].key
	}
	m.exploreCatalogs[m.active+"/"+m.connection] = slices.Clone(e.Resources)
	selection := m.active + "/" + m.connection + "/" + e.Preference.Resource
	if selection != m.exploreTreeSelection {
		m.exploreTreeSelection = selection
		path := []string{m.active, m.active + "/monitor", m.connection, m.connection + "/view/kubernetes"}
		if resource, ok := kubernetes.Find(e.Resources, e.Preference.Resource); ok {
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
		// Discovery restores the main view without opening its sidebar path.
		if !e.PreserveTree {
			for _, key := range path {
				if _, saved := m.expanded[key]; !saved {
					m.expanded[key] = true
					m.autoExpanded[key] = true
				}
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
	if gitOpsTool(m.tool) {
		if view := m.gitOpsCurrent(); view != nil {
			path = append(path, m.connection+"/"+m.tool+"/"+string(view.State.Section))
		}
	}
	if m.codeActive() {
		path = append(path, m.connection+"/"+m.coder().ViewID())
	}
	if m.mode == 1 && m.tool == "kubernetes" && m.explore != nil {
		key := m.explore.Preference.Resource
		path = append(path, m.connection+"/resource/"+key)
		if resource, ok := kubernetes.Find(m.explore.Resources, key); ok {
			path = append(path, categoryTreeKey(m.connection, kubernetes.Category(resource)))
		}
	}
	path = append(path, m.connection+"/"+m.tool, m.connection+"/view/"+m.tool, m.connection, m.active+"/"+connectionArea(m.tool), m.active)
	if m.currentRoot {
		path = append(path, m.currentTreeKey())
	}
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

func (m *model) treeWorkspaces() []workspace.Workspace {
	result := make([]workspace.Workspace, 0, len(m.workspaces)+2)
	for _, w := range []*workspace.Workspace{m.currentCluster, m.currentDirectory} {
		if w != nil {
			result = append(result, *w)
		}
	}
	return append(result, m.workspaces...)
}

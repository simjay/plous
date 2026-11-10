package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) beginWorkspace() {
	m.toolSetup = nil
	m.beginSettingsOverlay()
	m.status = ""
	m.form = &appsettings.Form{Kind: "workspace", Title: "Create Workspace", Fields: []appsettings.Field{{Key: "name", Label: "Workspace Name", Required: true, Value: textview.NewField("")}}}
}

func (m *model) beginRename() {
	w := m.current()
	if w == nil {
		return
	}
	m.form = &appsettings.Form{Kind: "rename", Title: "Rename Workspace", Fields: []appsettings.Field{{Key: "name", Label: "Workspace Name", Required: true, Value: textview.NewField(w.Name)}}}
	m.status = ""
}

func (m *model) beginConnection(tool string, c *workspace.Connection) {
	f := &appsettings.Form{Kind: "connection", Title: "Add " + appsettings.ToolNames[tool], Tool: tool, Fields: []appsettings.Field{{Key: "name", Label: "Name", Required: true}}}
	switch tool {
	case "kubernetes", "helm", "kargo":
		f.Fields = append(f.Fields, appsettings.Field{Key: "kubeconfig", Label: "Kubeconfig Path (Optional)"}, appsettings.Field{Key: "context", Label: "Kube Context", Required: true}, appsettings.Field{Key: "namespace", Label: "Namespace / Project (Optional)"})
	}
	if monitorCluster(tool) {
		f.Fields = append(f.Fields, appsettings.Field{Key: "driver", Label: "Release Storage", Options: []string{"secret", "configmap"}, Value: textview.NewField("secret")}, appsettings.Field{Key: "repository_config", Label: "Repository Config Path (Optional)"}, appsettings.Field{Key: "repository_cache", Label: "Repository Cache Path (Optional)"}, appsettings.Field{Key: "registry_config", Label: "Registry Config Path (Optional)"})
	}
	if c != nil {
		f.Connection = c.ID
		f.Remove = true
		f.Title = "Edit " + appsettings.ToolNames[tool]
		for i := range f.Fields {
			v := c.Settings[f.Fields[i].Key]
			if f.Fields[i].Key == "name" {
				v = c.Name
			}
			if monitorCluster(tool) && f.Fields[i].Key == "driver" && v == "" {
				v = "secret"
			}
			f.Fields[i].Value = textview.NewField(v)
		}
	}

	m.form = f
	m.status = ""
}

func (m *model) updateForm(k tea.KeyPressMsg) {
	f := m.form

	if f.Kind == "kube" {
		m.updateContextPicker(k)
		return
	}
	n := len(f.Fields)
	maxIndex := n + len(f.Actions()) - 1
	switch k.String() {
	case "esc":
		m.cancelForm()
	case "up":
		f.Index = max(0, f.Index-1)
	case "down":
		f.Index = min(maxIndex, f.Index+1)
	case "enter":
		if f.Index < n {
			if f.Fields[f.Index].Required && strings.TrimSpace(f.Fields[f.Index].Value.Text) == "" {
				m.status = f.Fields[f.Index].Label + " is required"
				return
			}
			f.Index++
			return
		}
		if f.Kind == "mainagent" {
			m.activateAgentRow(f.Rows[f.Index-n])
			return
		}
		if f.Index == n {
			m.submitForm()
			return
		}
		if f.Index == n+1 {
			m.cancelForm()
			return
		}
		if f.Kind == "appsettings" && f.Index == n+2 {
			m.captureAIFields()
			f.Advanced = !f.Advanced
			m.buildAIFields()
			f.Index = len(f.Fields)
			if f.Advanced {
				f.Index = 2
			}
			return
		}
		m.removeID = f.Connection
		m.removeProfile = f.Profile
		m.form = nil
		m.page = "remove"
		if f.Kind == "appsettings" {
			m.page = "removeprovider"
			m.removeForm = f
		}
		m.cursor = 0
		m.status = ""
	default:
		if f.Index < n {
			field := &f.Fields[f.Index]
			if len(field.Options) == 0 {
				before := field.Value.Text
				field.Value.Key(k, false)
				if field.Key == "entry_command" && field.Value.Text != before {
					f.RefreshAgentNote()
					m.nativeChange = nil
					m.status = ""
				}
			} else if len(field.Options) > 0 && (k.String() == "left" || k.String() == "right" || k.String() == "space") {
				i := -1
				for index, value := range field.Options {
					if value == field.Value.Text {
						i = index
					}
				}
				delta := 1
				if k.String() == "left" {
					delta = len(field.Options) - 1
				}
				field.Value = textview.NewField(field.Options[max(0, (i+delta)%len(field.Options))])
				if field.Key == "agent" {
					m.nativeChange = nil
					kind := appsettings.AgentKind(field.Value.Text)
					f.RefreshAgent(m.settings.Configs())
					m.status = appsettings.AgentAvailability(kind, f.AgentCommands().For(kind))
				}
				if field.Key == "provider" {
					m.changeAIProvider()
				}
			}
		}
	}
}

func (m *model) submitForm() {
	f := m.form

	values := map[string]string{}
	for i, v := range f.Fields {
		value := strings.TrimSpace(v.Value.Text)
		if v.Required && value == "" {
			m.status = v.Label + " is required"
			f.Index = i
			return
		}
		values[v.Key] = value
	}
	switch f.Kind {
	case "contextname":
		m.finishContextName(true)
	case "mainagent":
		m.saveMainAgent(appsettings.AgentKind(values["agent"]))
	case "appsettings":
		m.saveAppSettings(values)
	case "currentpath":
		m.currentPaths = values["path"]
		m.form = nil
		m.closeSettings()
		m.requestCurrent("", true)
	case "savecurrent":
		m.saveCurrentWorkspace(values["name"])
	case "workspace":
		if m.saveWorkspace(workspace.Workspace{Name: values["name"]}) {
			m.connection = ""
			m.tool = ""
			m.form = nil
			m.closeSettings()
			m.page, m.detail, m.focus = "", "", 1
			m.beginToolSetup(false)
			m.status = "Workspace created. Choose a connection, or Esc to start empty."
		}
	case "rename":
		w := m.current()
		if w != nil {
			copy := *w
			copy.Name = values["name"]
			if m.saveWorkspace(copy) {
				m.form = nil
				m.page = "settings"
				m.cursor = 0
			}
		}
	case "connection":
		w := m.current()
		if w == nil {
			return
		}
		copy := *w
		copy.Connections = append([]workspace.Connection(nil), w.Connections...)
		c := workspace.Connection{ID: f.Connection, Tool: f.Tool, Name: values["name"], Settings: map[string]string{}}
		for _, existing := range copy.Connections {
			if existing.ID == c.ID && c.ID != "" {
				for k, v := range existing.Settings {
					c.Settings[k] = v
				}
				break
			}
		}
		for k, v := range values {
			if k != "name" {
				if v == "" {
					delete(c.Settings, k)
				} else {
					c.Settings[k] = v
				}
			}
		}

		unchanged := false
		for _, existing := range copy.Connections {
			if c.ID != "" && existing.ID == c.ID && sameClusterTarget(existing, c) {
				unchanged = true
			}
		}
		if !unchanged {
			for _, existing := range copy.Connections {
				if existing.ID != c.ID && sameClusterTarget(existing, c) {
					m.status = fmt.Sprintf("This cluster connection already exists as %q. Edit it in Settings.", existing.Name)
					return
				}
			}
		}
		found := false
		for i, existing := range copy.Connections {
			if existing.ID == c.ID && c.ID != "" {
				copy.Connections[i] = c
				found = true
				break
			}
		}
		if !found {
			copy.Connections = append(copy.Connections, c)
		}
		if m.saveWorkspace(copy) {
			m.expanded[m.active+"/"+f.Tool] = true

			m.form = nil
			if m.toolSetup != nil {
				m.finishToolSetup(f.Tool)
			} else {
				m.page = "settings"
				m.cursor = 0
			}
		}
	}
}

func (m *model) updateMain(key string) {
	if m.mode == 2 && m.page == "" && m.form == nil {
		if (key == "right" || key == "left") && m.detail == "" {
			step := 1
			if key == "left" {
				step = -1
			}
			m.cycleActivityType(step)
			return
		}
		if key == "O" && m.activityCommand("observations") || key == "g" && m.activityCommand("refresh") || key == "d" && m.activityCommand("describe") {
			return
		}
	}
	if m.scrollActivityDetail(key) {
		return
	}
	if m.mode == 2 && m.detail != "" && m.page == "" && m.operationDetail != nil && key == "enter" {
		m.openOperationRelease()
		return
	}

	if m.gitOpsVisible() {
		m.updateGitOps(key)
		return
	}
	if m.helmVisible() {
		m.updateHelm(key)
		return
	}
	if m.exploreVisible() {
		m.updateExplore(key)
		return
	}
	if key == "esc" {
		if m.page == "removeprovider" {
			m.cancelProviderRemoval()
			return
		}
		if m.page == "" && m.detail == "" {
			m.focusRootSidebar()
			return
		}
		if m.toolSetup != nil {
			m.backFromToolSetup()
			return
		}
		if m.page == "settings" && m.settingsOverlay != nil {
			m.closeSettings()
			m.status = ""
			return
		} else if m.detail != "" {
			m.detail = ""
			if m.mode == 2 {
				m.restoreActivitySearch()
			}
		} else if m.page == "tools" || m.page == "contexts" || m.page == "remove" || m.page == "removeprovider" {
			m.openSettings()
		} else {
			m.page = ""
		}
		m.cursor = 0
		m.status = ""
		return
	}
	switch m.page {
	case "settings":
		m.updateSettings(key)
	case "removeprovider":
		if key == "up" || key == "down" {
			m.cursor = 1 - m.cursor
		}
		if key == "enter" {
			if m.cursor == 0 {
				m.cancelProviderRemoval()
			} else {
				m.removeAIConnection()
			}
		}
	case "tools":
		if key == "up" {
			m.cursor = max(0, m.cursor-1)
		}
		if key == "down" {
			m.cursor = min(len(appsettings.ToolTypes)-1, m.cursor+1)
		}
		if key == "enter" {
			if appsettings.ToolTypes[m.cursor] == "kubernetes" || appsettings.ToolTypes[m.cursor] == "helm" {
				m.toolSetup.Tool = appsettings.ToolTypes[m.cursor]
				m.beginKube()
			} else {
				m.beginConnection(appsettings.ToolTypes[m.cursor], nil)
			}
		}
	case "remove":
		if key == "up" || key == "down" {
			m.cursor = 1 - m.cursor
		}
		if key == "enter" {
			if m.cursor == 0 {
				m.page = "settings"
				return
			}
			w := m.current()
			if w == nil {
				return
			}
			copy := *w
			copy.Connections = nil
			for _, c := range w.Connections {
				if c.ID != m.removeID {
					copy.Connections = append(copy.Connections, c)
				}
			}
			if m.saveWorkspace(copy) {
				m.page = "settings"
				m.cursor = 0
				m.status = "Connection removed from this workspace"
			}
		}
	default:
		if m.currentRoot && m.current() == nil {
			if key == "enter" {
				m.executeExplore("connections")
			}
			return
		}
		if m.current() == nil {
			if key == "enter" {
				m.beginWorkspace()
			}
			return
		}
		pos := max(0, m.positions[m.viewKey()])
		if key == "enter" && m.mode == 0 && m.page == "" && len(m.sidebarConnections(*m.current())) == 0 {
			m.beginToolSetup(false)
			return
		}
		if m.mode == 2 && m.page == "" && m.detail == "" {
			changes := m.activityEntries()
			pos = min(max(0, m.activityPosition()), max(0, len(changes)-1))
			if len(changes) == 0 && (m.activityUI.Pending || m.activityUI.Load != nil) {
				return
			}
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, (m.height-12)/2)
			}
			if key == "up" || key == "pgup" {
				pos = max(0, pos-step)
			}
			if key == "down" || key == "pgdown" {
				pos = min(max(0, len(changes)-1), pos+step)
			}
			if key == "home" {
				pos = 0
			}
			if key == "end" {
				pos = max(0, len(changes)-1)
			}
			if key == "enter" && len(changes) > 0 {
				m.openActivityEntry(changes[min(pos, len(changes)-1)])
			}
			m.setActivityPosition(pos)
			return
		} else {
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.height-12)
			}
			offset := pos
			if m.detail != "" {
				offset = m.cursor
			}
			if key == "up" || key == "pgup" {
				offset = max(0, offset-step)
			}
			if key == "down" || key == "pgdown" {
				offset += step
			}
			if m.detail != "" {
				m.cursor = offset
			} else {
				pos = offset
			}
		}
		m.positions[m.viewKey()] = pos
	}
}

func (m *model) addContexts() {
	if !m.contextsReady() {
		m.status = "Refresh Contexts before adding."
		return
	}
	w := m.current()
	if w == nil {
		return
	}
	copy := *w
	copy.Connections = append([]workspace.Connection(nil), w.Connections...)
	chosen := false
	for _, yes := range m.chosen {
		chosen = chosen || yes
	}
	if !chosen {
		m.status = "Select at least one context with Space."
		return
	}
	added := 0
	for i, c := range m.contexts {
		if !m.chosen[i] {
			continue
		}
		if m.contextAlreadyAdded(i) {
			continue
		}
		copy.Connections = append(copy.Connections, workspace.Connection{Tool: m.toolSetup.Tool, Name: m.pendingContextName(i), Settings: map[string]string{"kubeconfig": c.Kubeconfig, "context": c.Name, "namespace": c.Namespace}})
		added++
	}
	if added == 0 {
		m.status = "Selected contexts are already in this workspace"
		return
	}
	if m.saveWorkspace(copy) {
		tool := m.toolSetup.Tool
		m.finishToolSetup(tool)
		m.status = fmt.Sprintf("Added %d %s connection(s)", added, appsettings.ToolNames[tool])
	}
}

func (m *model) formBody(width, height int) string {
	f := m.form
	lines := []string{}
	if f.Note != "" && f.Kind != "mainagent" {
		lines = append(lines, strings.Split(ui.Muted.Render(ansi.Wrap(f.Note, width, "")), "\n")...)
		lines = append(lines, "")
	}
	if f.Kind == "workspace" {
		lines = append(lines, appsettings.WorkspaceLocation(width)...)
		if height >= 10 {
			lines = append(lines, "")
		}
	}
	selectedRow := 0
	for i, field := range f.Fields {
		label := field.Label
		if field.Required {
			label += " *"
		}
		active := m.focus == 1 && f.Index == i
		value := ui.EditorLine(field.Value, width-4, active, "field/"+field.Key)
		if field.Secret {
			masked := textview.NewField(strings.Repeat("•", len([]rune(field.Value.Text))))
			masked.Cursor = field.Value.Cursor
			masked.Anchor, masked.Selecting = field.Value.Anchor, field.Value.Selecting
			value = ui.EditorLine(masked, width-4, active, "secret/"+field.Key)
		}
		if len(field.Options) > 0 {
			value = "‹ " + field.Value.Text + " ›"
			if field.Key == "provider" {
				value = "‹ " + appsettings.ProviderLabel(field.Value.Text) + " ›"
			}
		}
		if value == "" {
			value = "(empty)"
		}
		style := ui.CanvasStyle.Width(width).Padding(0, 1)
		labelStyle := ui.Muted
		if f.Index == i {
			selectedRow = len(lines)
			style = style.Background(ui.Highlight)
			labelStyle = ui.ShortcutStyle
		}
		target := "field/" + field.Key
		if len(field.Options) > 0 {
			target = "option/" + field.Key
		}
		lines = append(lines, ui.MouseTarget(labelStyle.Render(ui.Clipped(label, width)), "field/"+field.Key), ui.MouseTarget(style.Render(value), target))

		if height >= 10 {
			lines = append(lines, "")
		}
	}
	if f.Kind == "appsettings" {
		lines = append(lines, ui.Muted.Render(ui.Clipped(m.aiCredentialHint(), width)), "")
	}
	if f.Kind == "mainagent" {
		lines = append(lines, strings.Split(ui.Muted.Render(ansi.Wrap(f.Note, width, "")), "\n")...)
		lines = append(lines, "")
	}
	actions := f.Actions()
	section := ""
	for i, action := range actions {
		if f.Kind == "mainagent" && f.Rows[i].Section != section {
			section = f.Rows[i].Section
			if section != "" {
				lines = append(lines, ui.SectionHeadingStyle.Render(section))
			} else {
				lines = append(lines, "")
			}
		}
		if f.Index == len(f.Fields)+i {
			selectedRow = len(lines)
		}
		primary := i == 0

		if f.Kind == "mainagent" {
			primary = f.Rows[i].Kind == appsettings.SaveAgentRow
		}
		label := ui.FormAction(action, primary, f.Index == len(f.Fields)+i, width)
		if f.Kind == "mainagent" && f.Rows[i].Section != "" {
			label = ui.SettingsChoiceLine(action, f.Index == len(f.Fields)+i, width)
		}
		lines = append(lines, ui.MouseTarget(label, fmt.Sprintf("formaction/%d", i)))
		if f.Kind == "mainagent" && f.Rows[i].Detail != "" {
			lines = append(lines, ui.Muted.Render(ui.Clipped("  "+ui.OneLine(f.Rows[i].Detail), width)))
		}
	}
	heading := ui.SectionHeadingStyle.Render(ui.Clipped(f.Title, width)) + "\n\n"

	available := max(1, height-strings.Count(heading, "\n"))

	// Keep the current field label and input together when scrolling.
	start := max(0, selectedRow-available+2)
	if f.Index < len(f.Fields) {
		start = min(start, selectedRow)
	}
	end := min(len(lines), start+available)
	return heading + strings.Join(lines[start:end], "\n")
}

func (m *model) connectionPickerBody(width, height int) string {
	lines := []string{}
	selected := 0
	for i, tool := range appsettings.ToolTypes {
		group, hint, label := "", "", appsettings.ToolNames[tool]
		switch tool {
		case "kubernetes":
			group, label = "Monitor", "Kubernetes Cluster"
			hint = "Kubernetes resources and Helm releases"
		}
		if group != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, ui.SectionHeadingStyle.Render(group), ui.Muted.Render(ui.Clipped(hint, width)))
		}
		if i == m.cursor {
			selected = len(lines)
		}
		lines = append(lines, ui.MouseChoice(ui.SettingsChoiceLine(label, i == m.cursor, width), i, label))
	}
	start := max(0, selected-height+1)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (m *model) beginToolSetup(contextsOnly bool) {
	if m.current() == nil || m.form != nil {
		return
	}
	fromSettings := m.page == "settings"
	m.beginSettingsOverlay()
	m.toolSetup = &appsettings.ToolSetup{FromSettings: fromSettings, ContextsOnly: contextsOnly, Tool: "kubernetes"}
	m.page, m.cursor, m.status = "tools", 0, ""
	if contextsOnly {
		if m.tool == "helm" {
			m.toolSetup.Tool = "helm"
		}
		m.beginKube()
	}
}

func (m *model) backFromToolSetup() {
	m.form = nil
	if m.page == "contexts" && !m.toolSetup.ContextsOnly {
		m.page, m.cursor = "tools", 0
	} else if m.toolSetup.FromSettings {
		m.openSettings()
		m.selectSettingsRow("connection-add")
	} else {
		m.closeSettings()
	}
	m.status = ""
}

func (m *model) finishToolSetup(tool string) {
	m.form = nil
	m.closeSettings()
	m.selectScope(m.active, "", "")
	m.expanded[m.active] = true
	m.expanded[m.active+"/monitor"] = true
	for i, row := range m.treeRows() {
		if row.workspace == m.active && row.tool == tool && row.kind == "connection" {
			m.treeCursor = i
		}
	}
	m.focus = 0
	m.ensureVisibleFocus()
	m.status = appsettings.ToolNames[tool] + " connection added"
}

func (m *model) beginKube() {
	m.form = &appsettings.Form{Kind: "kube", Fields: []appsettings.Field{{Key: "kubeconfig", Value: textview.NewField("~/.kube/config")}}}
	m.page, m.status = "contexts", ""
	m.loadKubecontexts()
}

func (m *model) invalidateContexts() {
	m.toolSetup.RefreshRequired, m.toolSetup.Err = true, ""
	m.contexts, m.chosen = nil, map[int]bool{}
	m.toolSetup.Names = map[int]string{}
	m.status = ""
}

func (m *model) contextsReady() bool {
	return m.toolSetup != nil && m.form != nil && m.form.Kind == "kube" &&
		!m.toolSetup.RefreshRequired && m.toolSetup.Path == strings.TrimSpace(m.form.Fields[0].Value.Text)
}

func (m *model) loadKubecontexts() {
	m.invalidateContexts()
	path := strings.TrimSpace(m.form.Fields[0].Value.Text)
	if path == "" {
		m.toolSetup.Err = "Enter a kubeconfig file path."
		m.form.Index = 0
		return
	}
	contexts, err := kubecommon.DiscoverContexts(path)
	if err != nil {
		m.toolSetup.Err = err.Error()
		m.form.Index = 0
		return
	}
	m.toolSetup.Path, m.toolSetup.Err = path, ""
	m.toolSetup.RefreshRequired = false
	m.contexts, m.form.Index = contexts, 2
}

func (m *model) contextAlreadyAdded(index int) bool {
	return m.existingContext(index) != nil
}

func (m *model) existingContext(index int) *workspace.Connection {
	if index < 0 || index >= len(m.contexts) || m.current() == nil {
		return nil
	}
	c := m.contexts[index]
	target := workspace.Connection{Tool: "kubernetes", Settings: map[string]string{"context": c.Name, "kubeconfig": c.Kubeconfig}}
	for _, connection := range m.current().Connections {
		if sameClusterTarget(connection, target) {
			return &connection
		}
	}
	return nil
}

func (m *model) pendingContextName(index int) string {
	if name := m.toolSetup.Names[index]; name != "" {
		return name
	}
	return m.contexts[index].Name
}

func (m *model) beginContextName(index int) {
	if !m.contextsReady() || index < 0 || index >= len(m.contexts) || m.contextAlreadyAdded(index) {
		return
	}
	m.toolSetup.Picker, m.toolSetup.NameIndex = m.form, index
	m.form = &appsettings.Form{Kind: "contextname", Title: "Name This Connection", Note: "Context: " + ui.OneLine(m.contexts[index].Name) + "\nThis name is used in Plous. The kubeconfig context stays unchanged. Leave Name blank to use the context name.", Fields: []appsettings.Field{{Key: "name", Label: "Name", Value: textview.NewField(m.toolSetup.Names[index])}}}
	m.status = ""
}

func (m *model) finishContextName(save bool) {
	if save {
		name := strings.TrimSpace(m.form.Fields[0].Value.Text)
		if strings.IndexFunc(name, unicode.IsControl) >= 0 {
			m.status = "Connection names cannot contain control characters."
			return
		}
		m.toolSetup.Names[m.toolSetup.NameIndex] = name
	}
	m.form, m.toolSetup.Picker = m.toolSetup.Picker, nil
	m.form.Index = m.toolSetup.NameIndex + 2
	m.status = ""
}

func (m *model) updateContextPicker(k tea.KeyPressMsg) {
	f := m.form
	n := len(m.contexts)
	key := k.String()
	switch key {
	case "esc":
		m.backFromToolSetup()
	case "up":
		f.Index = max(0, f.Index-1)
	case "down":
		f.Index = min(n+3, f.Index+1)
	case "right":
		if f.Index >= 2 && f.Index < n+2 {
			m.beginContextName(f.Index - 2)
		} else if f.Index == 0 {
			f.Fields[0].Value.Key(k, false)
		}
	case "enter":
		switch {
		case f.Index == 0:
			f.Index = 1
		case f.Index == 1:
			m.loadKubecontexts()
		case f.Index == n+2:
			m.addContexts()
		case f.Index == n+3:
			m.backFromToolSetup()
		default:
			if m.contextsReady() && !m.contextAlreadyAdded(f.Index-2) {
				m.chosen[f.Index-2] = !m.chosen[f.Index-2]
			}
		}
	default:
		if f.Index == 0 {
			before := f.Fields[0].Value.Text
			f.Fields[0].Value.Key(k, false)
			if f.Fields[0].Value.Text != before {
				m.invalidateContexts()
			}
		} else if key == "home" {
			f.Index = 0
		} else if key == "end" {
			f.Index = n + 3
		} else if key == "space" && f.Index >= 2 && f.Index < n+2 && m.contextsReady() && !m.contextAlreadyAdded(f.Index-2) {
			m.chosen[f.Index-2] = !m.chosen[f.Index-2]
		}
	}
}

func (m *model) contextPickerBody(width, height int) string {
	f := m.form
	pathStyle := ui.CanvasStyle.Width(width).Padding(0, 1)
	if f.Index == 0 {
		pathStyle = pathStyle.Background(ui.Highlight)
	}
	lines := []string{
		ui.SectionHeadingStyle.Render("Kubeconfig File"),
		ui.MouseTarget(pathStyle.Render(ui.EditorLine(f.Fields[0].Value, width-2, f.Index == 0, "field/"+f.Fields[0].Key)), "field/"+f.Fields[0].Key),
		ui.MouseTarget(ui.SettingsChoiceLine("[ Refresh Contexts ]", f.Index == 1, width), "context/1"),
	}
	message := ""
	messageStyle := ui.Muted
	if m.toolSetup.RefreshRequired {
		message = "Refresh required"
		messageStyle = ui.ChangedStyle
	}
	if m.toolSetup.Err != "" {
		message = m.toolSetup.Err
		messageStyle = ui.RemovedStyle
	}
	lines = append(lines, messageStyle.Render(ui.Clipped(ui.OneLine(message), width)))
	if height >= 12 {
		lines = append(lines, "")
	}
	lines = append(lines, ui.SectionHeadingStyle.Render("Contexts"))
	items := []string{}
	for i := range m.contexts {
		label := "[ ] " + m.pendingContextName(i)
		existing := m.existingContext(i)
		added := existing != nil
		if added {
			suffix := " · Already added"
			label = ansi.Truncate("[✓] "+ui.OneLine(existing.Name), max(1, width-2-ansi.StringWidth(suffix)), "...") + suffix
		} else if m.chosen[i] {
			label = "[x] " + m.pendingContextName(i)
		}
		rowWidth := width
		if !added {
			rowWidth = max(1, width-9)
		}
		row := ui.MouseTarget(ui.SettingsChoiceLine(label, f.Index == i+2, rowWidth), fmt.Sprintf("context/%d", i+2))
		if !added {
			row += " " + ui.MouseTarget(ui.ShortcutStyle.Render("[→ Name]"), fmt.Sprintf("contextname/%d", i))
		}
		items = append(items, row)
	}
	if len(items) == 0 {
		items = append(items, ui.Muted.Render("  No contexts loaded"))
	}
	actions := []string{ui.MouseTarget(ui.SettingsChoiceLine("[ Add Selected Contexts ]", f.Index == len(m.contexts)+2, width), fmt.Sprintf("context/%d", len(m.contexts)+2)), ui.MouseTarget(ui.SettingsChoiceLine("[ Cancel ]", f.Index == len(m.contexts)+3, width), fmt.Sprintf("context/%d", len(m.contexts)+3))}
	if !m.contextsReady() {
		actions[0] = ui.SettingsChoiceLine("[ Add Selected Contexts ] · disabled", f.Index == len(m.contexts)+2, width)
	}
	selectedRow := f.Index + 1
	if f.Index >= 2 {
		selectedRow = len(lines) + f.Index - 2
		if len(m.contexts) == 0 {
			selectedRow++
		}
	}
	available := height - len(lines) - len(actions)
	if available < 1 {
		return ui.ScrollSelection(append(append(lines, items...), actions...), selectedRow, height)
	}
	return strings.Join(lines, "\n") + "\n" + lipgloss.NewStyle().Height(available).Render(ui.ScrollSelection(items, f.Index-2, available)) + "\n" + strings.Join(actions, "\n")
}

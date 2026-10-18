package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"strings"
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
	case "kustomize", "code":
		f.Source = appsettings.NewSourceSetup()
		f.Fields = append(f.Fields,
			appsettings.Field{Key: "source_type", Label: "Location", Options: []string{"Local directory", "Git repository"}, Value: textview.NewField("Local directory")},
			appsettings.Field{Key: "path", Label: "Local Directory", Required: true},
			appsettings.Field{Key: "branch", Label: "Branch (Optional)"},
			appsettings.Field{Key: "browse_path", Label: "Path (Optional)"})
	case "terraform":
		f.Fields = append(f.Fields, appsettings.Field{Key: "path", Label: "Working Directory", Required: true}, appsettings.Field{Key: "workspace", Label: "Terraform Workspace (Optional)"})
	case "argo":
		f.Fields = append(f.Fields, appsettings.Field{Key: "repo", Label: "Git Repository URL or path", Required: true}, appsettings.Field{Key: "server", Label: "Argo Server (Optional)"}, appsettings.Field{Key: "revision", Label: "Git Revision (Optional)"}, appsettings.Field{Key: "path", Label: "Repository Path (Optional)"})
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
			if (tool == "kustomize" || tool == "code") && f.Fields[i].Key == "source_type" && v == "" {
				v = "Local directory"
			}
			if f.Fields[i].Key == "name" {
				v = c.Name
			}
			if monitorCluster(tool) && f.Fields[i].Key == "driver" && v == "" {
				v = "secret"
			}
			f.Fields[i].Value = textview.NewField(v)
		}
	}
	if f.Source != nil {
		f.Title = "Add Source"
		if c != nil {
			f.Title = "Edit Source"
			f.Source.SavedRevision = c.Settings["revision"]
			if c.Settings["branch"] == "" && f.Source.SavedRevision != "" {
				f.Note = "Saved Git reference: " + ui.OneLine(f.Source.SavedRevision) + "\nKept unless you enter a branch."
			}
			if subdir := c.Settings["subdirectory"]; subdir != "" {
				f.Source.Drafts["browse_path"] = textview.NewField(subdir)
				if _, set := c.Settings["browse_path"]; !set {
					for i := range f.Fields {
						if f.Fields[i].Key == "browse_path" {
							f.Fields[i].Value = textview.NewField(subdir)
						}
					}
				}
			}
		}
		if f.Fields[1].Value.Text == "Git repository" {
			f.Fields[2].Label = "Git Repository URL"
		}
		f.ChangeSourceType()
	}
	m.form = f
	m.status = ""
}

func (m *model) updateForm(k tea.KeyPressMsg) {
	f := m.form
	if f.Source != nil && m.updateSourceSetup(k) {
		return
	}
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
				field.Value.Key(k, false)
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
					m.status = appsettings.AgentAvailability(kind)
				}
				if field.Key == "provider" {
					m.changeAIProvider()
				}
				if field.Key == "source_type" && f.Source != nil {
					f.ChangeSourceType()
				}
			}
		}
	}
}

func (m *model) submitForm() {
	f := m.form
	if f.LocalDirectory() && !m.validateLocalDirectory() {
		return
	}
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
				if v == "" && k != "browse_path" {
					delete(c.Settings, k)
				} else {
					c.Settings[k] = v
				}
			}
		}
		if sourceConnection(c.Tool) {
			if c.Settings["source_type"] == "Git repository" && c.Settings["branch"] != "" {
				if err := appsettings.ValidateRepositoryBranch(c.Settings["branch"]); err != nil {
					m.status = err.Error()
					f.Index = 3
					return
				}
				delete(c.Settings, "revision")
			}
			if err := appcode.ConfigForConnection(&c).Validate(); err != nil {
				m.status = err.Error()
				return
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
			if !found && sourceConnection(f.Tool) {
				connections := m.current().Connections
				m.expanded[connections[len(connections)-1].ID] = true
			}
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
	if m.codeVisible() {
		m.updateCode(key)
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
		if key == "enter" && m.mode == 0 && m.page == "" && len(m.current().Connections) == 0 {
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
	selectedRow, selectedEnd := 0, 2
	fieldRows := []int{}
	for i, field := range f.Fields {
		fieldRows = append(fieldRows, len(lines))
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
			if field.Key == "source_type" {
				value = "‹ Local Directory ›"
				if field.Value.Text == "Git repository" {
					value = "‹ Git Repository ›"
				}
			}
			if field.Key == "provider" {
				value = "‹ " + appsettings.ProviderLabel(field.Value.Text) + " ›"
			}
		}
		if value == "" {
			value = "(empty)"
			if field.Key == "branch" {
				value = ui.Muted.Render("Default branch")
				if f.Source != nil && f.Source.SavedRevision != "" {
					value = ui.Muted.Render("Keep saved reference")
				}
			}
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
		if f.Source != nil && field.Key == "branch" {
			hint := "Leave blank to use the default branch."
			if f.Source.SavedRevision != "" {
				hint = "Leave blank to keep the saved reference."
			}
			lines = append(lines, ui.Muted.Render(ui.Clipped(hint, width)))
		}
		if f.LocalDirectory() && field.Key == "path" {
			lines = append(lines, strings.Split(f.DirectoryHint(width), "\n")...)
		}
		if f.Index == i {
			selectedEnd = len(lines)
		}
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
	actionsStart := len(lines)
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
		if f.LocalDirectory() {
			primary = i == 1
		}
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
	if f.Source != nil && m.settingsOverlay != nil {
		heading = ""
	}
	available := max(1, height-strings.Count(heading, "\n"))
	if f.Source != nil {
		// Keep source actions visible while the fields scroll in compact terminals.
		fieldHeight := max(2, available-len(actions)-1)
		start := max(0, actionsStart-fieldHeight)
		if f.Index < len(f.Fields) {
			start = max(0, min(selectedEnd-fieldHeight, selectedRow, start))
		}
		if start > 0 {
			for _, row := range fieldRows {
				if row >= start && row <= selectedRow {
					start = row
					break
				}
			}
		}
		visible := append([]string(nil), lines[start:min(actionsStart, start+fieldHeight)]...)
		for len(visible) < fieldHeight+1 {
			visible = append(visible, "")
		}
		return heading + strings.Join(append(visible, lines[actionsStart:]...), "\n")
	}
	// Keep the current field label and input together when scrolling.
	start := max(0, selectedRow-available+2)
	if f.Index < len(f.Fields) {
		start = min(start, selectedRow)
	}
	end := min(len(lines), start+available)
	return heading + strings.Join(lines[start:end], "\n")
}

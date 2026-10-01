package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

type field struct {
	key, label string
	required   bool
	value      editor
	options    []string
	secret     bool
}
type form struct {
	kind, title, tool, connection string
	profile                       string
	fields                        []field
	index                         int
	remove                        bool
	advanced                      bool
	agent                         ai.Config
	apiKey                        string
}

func (f *form) paste(s string) {
	if f.index < len(f.fields) && len(f.fields[f.index].options) == 0 {
		f.fields[f.index].value.insert(s, false)
	}
}
func (m *model) beginWorkspace() {
	m.toolSetup = nil
	m.beginSettingsOverlay()
	m.status = ""
	m.form = &form{kind: "workspace", title: "Create workspace", fields: []field{{key: "name", label: "Workspace name", required: true, value: newEditor("")}}}
}
func (m *model) beginRename() {
	w := m.current()
	if w == nil {
		return
	}
	m.form = &form{kind: "rename", title: "Rename workspace", fields: []field{{key: "name", label: "Workspace name", required: true, value: newEditor(w.Name)}}}
	m.status = ""
}
func (m *model) beginConnection(tool string, c *workspace.Connection) {
	f := &form{kind: "connection", title: "Add " + toolNames[tool], tool: tool, fields: []field{{key: "name", label: "Connection name", required: true}}}
	switch tool {
	case "kubernetes", "helm", "kargo":
		f.fields = append(f.fields, field{key: "kubeconfig", label: "Kubeconfig path (optional)"}, field{key: "context", label: "Kube context", required: true}, field{key: "namespace", label: "Namespace / project (optional)"})
	case "kustomize":
		f.fields = append(f.fields, field{key: "path", label: "Directory / overlay path", required: true})
	case "terraform":
		f.fields = append(f.fields, field{key: "path", label: "Working directory", required: true}, field{key: "workspace", label: "Terraform workspace (optional)"})
	case "argo":
		f.fields = append(f.fields, field{key: "repo", label: "Git repository URL or path", required: true}, field{key: "server", label: "Argo server (optional)"}, field{key: "revision", label: "Git revision (optional)"}, field{key: "path", label: "Repository path (optional)"})
	}
	if c != nil {
		f.connection = c.ID
		f.remove = true
		f.title = "Edit " + toolNames[tool]
		for i := range f.fields {
			v := c.Settings[f.fields[i].key]
			if f.fields[i].key == "name" {
				v = c.Name
			}
			f.fields[i].value = newEditor(v)
		}
	}
	m.form = f
	m.status = ""
}
func (m *model) updateForm(k tea.KeyPressMsg) {
	f := m.form
	if f.kind == "kube" {
		m.updateContextPicker(k)
		return
	}
	n := len(f.fields)
	maxIndex := n + len(f.actions()) - 1
	switch k.String() {
	case "esc":
		m.cancelForm()
	case "up":
		f.index = max(0, f.index-1)
	case "down":
		f.index = min(maxIndex, f.index+1)
	case "enter":
		if f.index < n {
			if f.fields[f.index].required && strings.TrimSpace(f.fields[f.index].value.text) == "" {
				m.status = f.fields[f.index].label + " is required"
				return
			}
			f.index++
			return
		}
		if f.index == n {
			m.submitForm()
			return
		}
		if f.index == n+1 {
			m.cancelForm()
			return
		}
		if f.kind == "appsettings" && f.index == n+2 {
			m.captureAIFields()
			f.advanced = !f.advanced
			m.buildAIFields()
			f.index = len(f.fields)
			if f.advanced {
				f.index = 2
			}
			return
		}
		m.removeID = f.connection
		m.removeProfile = f.profile
		m.form = nil
		m.page = "remove"
		if f.kind == "appsettings" {
			m.page = "removeprovider"
		}
		m.cursor = 0
		m.status = ""
	default:
		if f.index < n {
			field := &f.fields[f.index]
			if len(field.options) == 0 {
				field.value.key(k, false)
			} else if len(field.options) > 0 && (k.String() == "left" || k.String() == "right" || k.String() == "space") {
				i := -1
				for index, value := range field.options {
					if value == field.value.text {
						i = index
					}
				}
				delta := 1
				if k.String() == "left" {
					delta = len(field.options) - 1
				}
				field.value = newEditor(field.options[max(0, (i+delta)%len(field.options))])
				if field.key == "provider" {
					m.changeAIProvider()
				}
			}
		}
	}
}
func (m *model) submitForm() {
	f := m.form
	values := map[string]string{}
	for i, v := range f.fields {
		value := strings.TrimSpace(v.value.text)
		if v.required && value == "" {
			m.status = v.label + " is required"
			f.index = i
			return
		}
		values[v.key] = value
	}
	switch f.kind {
	case "appsettings":
		m.saveAppSettings(values)
	case "workspace":
		if m.saveWorkspace(workspace.Workspace{Name: values["name"]}) {
			m.connection = ""
			m.tool = ""
			m.form = nil
			m.closeSettings()
			m.page, m.detail, m.focus = "", "", 1
			m.beginToolSetup(false)
			m.status = "Workspace created. Choose a tool, or Esc to start empty."
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
		c := workspace.Connection{ID: f.connection, Tool: f.tool, Name: values["name"], Settings: map[string]string{}}
		for k, v := range values {
			if k != "name" && v != "" {
				c.Settings[k] = v
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
			m.expanded[m.active+"/"+f.tool] = true
			m.form = nil
			if m.toolSetup != nil {
				m.finishToolSetup(f.tool)
			} else {
				m.page = "settings"
				m.cursor = 0
			}
		}
	}
}
func (m *model) updateMain(key string) {
	if m.exploreVisible() {
		m.updateExplore(key)
		return
	}
	if key == "esc" {
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
				m.openSettings()
			} else {
				m.removeAIConnection()
			}
		}
	case "tools":
		if key == "up" {
			m.cursor = max(0, m.cursor-1)
		}
		if key == "down" {
			m.cursor = min(len(toolTypes)-1, m.cursor+1)
		}
		if key == "enter" {
			if toolTypes[m.cursor] == "kubernetes" {
				m.beginKube()
			} else {
				m.beginConnection(toolTypes[m.cursor], nil)
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
			changes := m.filteredChanges()
			if key == "up" {
				pos = max(0, pos-1)
			}
			if key == "down" {
				pos = min(max(0, len(changes)-1), pos+1)
			}
			if key == "enter" && len(changes) > 0 {
				m.setActivityDetail(changes[min(pos, len(changes)-1)])
			}
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
		m.status = "Refresh contexts before adding."
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
		exists := false
		for _, old := range copy.Connections {
			if old.Tool == "kubernetes" && old.Settings["context"] == c.Name && old.Settings["kubeconfig"] == c.Kubeconfig {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		copy.Connections = append(copy.Connections, workspace.Connection{Tool: "kubernetes", Name: c.Name, Settings: map[string]string{"kubeconfig": c.Kubeconfig, "context": c.Name, "namespace": c.Namespace}})
		added++
	}
	if added == 0 {
		m.status = "Selected contexts are already in this workspace"
		return
	}
	if m.saveWorkspace(copy) {
		m.finishToolSetup("kubernetes")
		m.status = fmt.Sprintf("Added %d Kubernetes connection(s)", added)
	}
}

func (m *model) formBody(width, height int) string {
	f := m.form
	lines := []string{}
	if f.kind == "workspace" {
		lines = append(lines, workspaceLocation(width)...)
		if height >= 10 {
			lines = append(lines, "")
		}
	}
	selectedRow := 0
	for i, field := range f.fields {
		label := field.label
		if field.required {
			label += " *"
		}
		active := m.focus == 1 && f.index == i
		value := editorLine(field.value, width-4, active)
		if field.secret {
			masked := newEditor(strings.Repeat("•", len([]rune(field.value.text))))
			masked.cursor = field.value.cursor
			value = editorLine(masked, width-4, active)
		}
		if len(field.options) > 0 {
			value = "‹ " + field.value.text + " ›"
			if field.key == "provider" {
				value = "‹ " + providerLabel(field.value.text) + " ›"
			}
		}
		if value == "" {
			value = "(empty)"
		}
		style := canvasStyle.Width(width).Padding(0, 1)
		labelStyle := muted
		if f.index == i {
			selectedRow = len(lines)
			style = style.Background(highlight)
			labelStyle = shortcutStyle
		}
		lines = append(lines, labelStyle.Render(clipped(label, width)), style.Render(value))
		if height >= 10 {
			lines = append(lines, "")
		}
	}
	if f.kind == "appsettings" {
		lines = append(lines, muted.Render(clipped(m.aiCredentialHint(), width)), "")
	}
	actions := f.actions()
	for i, action := range actions {
		if f.index == len(f.fields)+i {
			selectedRow = len(lines)
		}
		lines = append(lines, formAction(action, i == 0, f.index == len(f.fields)+i, width))
	}
	available := max(1, height-2)
	// Keep the current field label and input together when scrolling.
	start := max(0, selectedRow-available+2)
	if f.index < len(f.fields) {
		start = min(start, selectedRow)
	}
	end := min(len(lines), start+available)
	return sectionHeadingStyle.Render(clipped(f.title, width)) + "\n\n" + strings.Join(lines[start:end], "\n")
}

func (f *form) actions() []string {
	action := "Save"
	actions := []string{action, "Cancel"}
	if f.kind == "appsettings" {
		label := "Advanced…"
		if f.advanced {
			label = "Hide advanced"
		}
		actions = append(actions, label)
	}
	if f.remove {
		label := "Remove connection"
		if f.kind == "appsettings" {
			label = "Remove AI connection"
		}
		actions = append(actions, label)
	}
	return actions
}

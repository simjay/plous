package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubeconfig"
	"github.com/simjay/plous/internal/workspace"
)

type field struct {
	key, label string
	required   bool
	value      editor
}
type form struct {
	kind, title, tool, connection string
	fields                        []field
	index                         int
	remove                        bool
}

func (f *form) paste(s string) {
	if f.index < len(f.fields) {
		f.fields[f.index].value.insert(s, false)
	}
}
func (m *model) beginWorkspace() {
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
func (m *model) beginKube() {
	m.form = &form{kind: "kube", title: "Add Kubernetes", fields: []field{{key: "kubeconfig", label: "Kubeconfig path (blank = default)", value: newEditor("")}}}
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
	n := len(f.fields)
	maxIndex := n + 1
	if f.remove {
		maxIndex++
	}
	switch k.String() {
	case "esc":
		m.form = nil
		m.page = "settings"
		m.cursor = 0
		m.status = "Edits canceled"
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
			m.form = nil
			m.page = "settings"
			m.cursor = 0
			m.status = "Edits canceled"
			return
		}
		m.removeID = f.connection
		m.form = nil
		m.page = "remove"
		m.cursor = 0
		m.status = ""
	default:
		if f.index < n {
			f.fields[f.index].value.key(k, false)
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
	case "workspace":
		if m.saveWorkspace(workspace.Workspace{Name: values["name"]}) {
			m.connection = ""
			m.tool = ""
			m.form = nil
			m.page = "settings"
			m.cursor = 1
			m.status = "Workspace created. Choose Add tool, or Back to start empty."
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
	case "kube":
		contexts, err := kubeconfig.Discover(values["kubeconfig"])
		if err != nil {
			m.status = "Could not load kubecontexts. Check the path. " + err.Error()
			return
		}
		m.contexts = contexts
		m.chosen = map[int]bool{}
		m.cursor = 0
		m.form = nil
		m.page = "contexts"
		m.status = "Space selects contexts; Enter adds selected (or highlighted) context."
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
			m.page = "settings"
			m.cursor = 0
		}
	}
}
func (m *model) settingsItems() []string {
	items := []string{"Rename workspace", "Add tool"}
	if w := m.current(); w != nil {
		for _, c := range w.Connections {
			items = append(items, "Edit "+toolNames[c.Tool]+" / "+c.Name)
		}
	}
	return append(items, "Back")
}
func (m *model) updateMain(key string) {
	if key == "esc" {
		if m.detail != "" {
			m.detail = ""
		} else {
			m.page = ""
		}
		m.cursor = 0
		m.status = ""
		return
	}
	switch m.page {
	case "settings":
		if m.current() == nil {
			m.page = ""
			return
		}
		items := m.settingsItems()
		m.cursor = max(0, min(m.cursor, len(items)-1))
		if key == "up" {
			m.cursor = max(0, m.cursor-1)
		}
		if key == "down" {
			m.cursor = min(len(items)-1, m.cursor+1)
		}
		if key == "enter" {
			switch m.cursor {
			case 0:
				m.beginRename()
			case 1:
				m.page = "tools"
				m.cursor = 0
			default:
				if m.cursor == len(items)-1 {
					m.page = ""
				} else {
					c := m.current().Connections[m.cursor-2]
					m.beginConnection(c.Tool, &c)
				}
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
	case "contexts":
		if key == "up" {
			m.cursor = max(0, m.cursor-1)
		}
		if key == "down" {
			m.cursor = min(len(m.contexts)-1, m.cursor+1)
		}
		if key == "space" {
			m.chosen[m.cursor] = !m.chosen[m.cursor]
		}
		if key == "enter" && len(m.contexts) > 0 {
			m.addContexts()
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
		if m.mode == 3 && m.page == "" && m.detail == "" {
			if key == "up" {
				pos = max(0, pos-1)
			}
			if key == "down" {
				pos = min(max(0, len(m.changes)-1), pos+1)
			}
			if key == "enter" && len(m.changes) > 0 {
				m.detail = m.changeDetail(m.changes[min(pos, len(m.changes)-1)])
				m.cursor = 0
			}
		} else {
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.height-12)
			}
			offset := pos
			if m.detail != "" || m.page == "help" {
				offset = m.cursor
			}
			if key == "up" || key == "pgup" {
				offset = max(0, offset-step)
			}
			if key == "down" || key == "pgdown" {
				offset += step
			}
			if m.detail != "" || m.page == "help" {
				m.cursor = offset
			} else {
				pos = offset
			}
		}
		m.positions[m.viewKey()] = pos
	}
}
func (m *model) addContexts() {
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
		m.chosen[m.cursor] = true
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
		m.expanded[m.active+"/kubernetes"] = true
		m.page = "settings"
		m.cursor = 1
		m.status = fmt.Sprintf("Added %d Kubernetes connection(s)", added)
	}
}

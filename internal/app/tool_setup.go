package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/kubeconfig"
)

type toolSetup struct {
	fromSettings, contextsOnly bool
	path, err                  string
	refreshRequired            bool
}

func (m *model) beginToolSetup(contextsOnly bool) {
	if m.current() == nil || m.form != nil {
		return
	}
	fromSettings := m.page == "settings"
	m.beginSettingsOverlay()
	m.toolSetup = &toolSetup{fromSettings: fromSettings, contextsOnly: contextsOnly}
	m.page, m.cursor, m.status = "tools", 0, ""
	if contextsOnly {
		m.beginKube()
	}
}

func (m *model) backFromToolSetup() {
	m.form = nil
	if m.page == "contexts" && !m.toolSetup.contextsOnly {
		m.page, m.cursor = "tools", 0
	} else if m.toolSetup.fromSettings {
		m.openSettings()
		m.selectSettingsItem("Add tool")
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
	m.expanded[m.active+"/"+tool] = true
	for i, row := range m.treeRows() {
		if row.workspace == m.active && row.tool == tool && row.kind == "connection" {
			m.treeCursor = i
		}
	}
	m.focus = 0
	m.ensureVisibleFocus()
	m.status = toolNames[tool] + " connection added"
}

func (m *model) beginKube() {
	m.form = &form{kind: "kube", fields: []field{{key: "kubeconfig", value: newEditor("~/.kube/config")}}}
	m.page, m.status = "contexts", ""
	m.loadKubecontexts()
}

func (m *model) invalidateContexts() {
	m.toolSetup.refreshRequired, m.toolSetup.err = true, ""
	m.contexts, m.chosen = nil, map[int]bool{}
	m.status = ""
}

func (m *model) contextsReady() bool {
	return m.toolSetup != nil && m.form != nil && m.form.kind == "kube" &&
		!m.toolSetup.refreshRequired && m.toolSetup.path == strings.TrimSpace(m.form.fields[0].value.text)
}

func (m *model) loadKubecontexts() {
	m.invalidateContexts()
	path := strings.TrimSpace(m.form.fields[0].value.text)
	if path == "" {
		m.toolSetup.err = "Enter a kubeconfig file path."
		m.form.index = 0
		return
	}
	contexts, err := kubeconfig.Discover(path)
	if err != nil {
		m.toolSetup.err = err.Error()
		m.form.index = 0
		return
	}
	m.toolSetup.path, m.toolSetup.err = path, ""
	m.toolSetup.refreshRequired = false
	m.contexts, m.form.index = contexts, 2
}

func (m *model) contextAlreadyAdded(index int) bool {
	c := m.contexts[index]
	for _, connection := range m.current().Connections {
		if connection.Tool == "kubernetes" && connection.Settings["context"] == c.Name && connection.Settings["kubeconfig"] == c.Kubeconfig {
			return true
		}
	}
	return false
}

func (m *model) updateContextPicker(k tea.KeyPressMsg) {
	f := m.form
	n := len(m.contexts)
	key := k.String()
	switch key {
	case "esc":
		m.backFromToolSetup()
	case "up":
		f.index = max(0, f.index-1)
	case "down":
		f.index = min(n+3, f.index+1)
	case "enter":
		switch {
		case f.index == 0:
			f.index = 1
		case f.index == 1:
			m.loadKubecontexts()
		case f.index == n+2:
			m.addContexts()
		case f.index == n+3:
			m.backFromToolSetup()
		default:
			if m.contextsReady() && !m.contextAlreadyAdded(f.index-2) {
				m.chosen[f.index-2] = !m.chosen[f.index-2]
			}
		}
	default:
		if f.index == 0 {
			before := f.fields[0].value.text
			f.fields[0].value.key(k, false)
			if f.fields[0].value.text != before {
				m.invalidateContexts()
			}
		} else if key == "home" {
			f.index = 0
		} else if key == "end" {
			f.index = n + 3
		} else if key == "space" && f.index >= 2 && f.index < n+2 && m.contextsReady() && !m.contextAlreadyAdded(f.index-2) {
			m.chosen[f.index-2] = !m.chosen[f.index-2]
		}
	}
}

func (m *model) contextPickerBody(width, height int) string {
	f := m.form
	pathStyle := canvasStyle.Width(width).Padding(0, 1)
	if f.index == 0 {
		pathStyle = pathStyle.Background(highlight)
	}
	lines := []string{
		sectionHeadingStyle.Render("Kubeconfig file"),
		pathStyle.Render(editorLine(f.fields[0].value, width-2, f.index == 0)),
		settingsChoiceLine("[ Refresh contexts ]", f.index == 1, width),
	}
	message := ""
	messageStyle := muted
	if m.toolSetup.refreshRequired {
		message = "Refresh required"
		messageStyle = changedStyle
	}
	if m.toolSetup.err != "" {
		message = m.toolSetup.err
		messageStyle = removedStyle
	}
	lines = append(lines, messageStyle.Render(clipped(oneLine(message), width)))
	if height >= 12 {
		lines = append(lines, "")
	}
	lines = append(lines, sectionHeadingStyle.Render("Contexts"))
	items := []string{}
	for i, c := range m.contexts {
		label := "[ ] " + c.Name
		if m.contextAlreadyAdded(i) {
			label = "[✓] " + c.Name + " · Already added"
		} else if m.chosen[i] {
			label = "[x] " + c.Name
		}
		items = append(items, settingsChoiceLine(label, f.index == i+2, width))
	}
	if len(items) == 0 {
		items = append(items, muted.Render("  No contexts loaded"))
	}
	actions := []string{settingsChoiceLine("[ Add selected contexts ]", f.index == len(m.contexts)+2, width), settingsChoiceLine("[ Cancel ]", f.index == len(m.contexts)+3, width)}
	if !m.contextsReady() {
		actions[0] = settingsChoiceLine("[ Add selected contexts ] · disabled", f.index == len(m.contexts)+2, width)
	}
	selectedRow := f.index + 1
	if f.index >= 2 {
		selectedRow = len(lines) + f.index - 2
		if len(m.contexts) == 0 {
			selectedRow++
		}
	}
	available := height - len(lines) - len(actions)
	if available < 1 {
		return scrollSelection(append(append(lines, items...), actions...), selectedRow, height)
	}
	return strings.Join(lines, "\n") + "\n" + lipgloss.NewStyle().Height(available).Render(scrollSelection(items, f.index-2, available)) + "\n" + strings.Join(actions, "\n")
}

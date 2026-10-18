package app

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) connectionPickerBody(width, height int) string {
	lines := []string{}
	selected := 0
	for i, tool := range appsettings.ToolTypes {
		group, hint, label := "", "", appsettings.ToolNames[tool]
		switch tool {
		case "kubernetes":
			group, label = "Monitor", "Kubernetes Cluster"
			hint = "Kubernetes resources and Helm releases"
		case "code":
			group, label = "Code", "Source"
			hint = "Local directory or Git repository"
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
	m.expanded[m.active+"/"+connectionArea(tool)] = true
	for i, row := range m.treeRows() {
		if row.workspace == m.active && (row.tool == tool || sourceConnection(tool) && row.tool == "files") && row.kind == "connection" {
			m.treeCursor = i
		}
	}
	m.focus = 0
	m.ensureVisibleFocus()
	m.status = appsettings.ToolNames[tool] + " connection added"
	if sourceConnection(tool) {
		m.status = "Source added"
	}
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

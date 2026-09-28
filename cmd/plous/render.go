package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/workspace"
)

var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B949E"))
var accent = lipgloss.Color("#7AA2F7")
var selected = lipgloss.NewStyle().Foreground(accent).Bold(true)

func plain(s string) string              { return cleanInput(s, true) }
func oneLine(s string) string            { return strings.ReplaceAll(plain(s), "\n", " ") }
func clipped(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }
func (m *model) View() tea.View          { v := tea.NewView(m.render()); v.AltScreen = true; return v }
func (m *model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		text := "Plous " + version() + "\nResize terminal to at least 80 x 16.\nTab to main; :quit Enter to exit.\nFocus: " + []string{"tree", "main", "chat"}[m.focus]
		if m.command != nil {
			text += "\n:" + m.command.display(m.focus == 1)
		}
		if m.status != "" {
			text += "\n" + oneLine(m.status)
		}
		return lipgloss.NewStyle().Width(m.width).Height(m.height).MaxWidth(m.width).MaxHeight(m.height).Render(text)
	}
	left := min(28, max(18, m.width/5))
	right := min(48, max(24, m.width*3/10))
	center := m.width - left - right
	height := m.height - 3
	header := selected.Render(" plous")
	label := muted.Render("v" + version() + " ")
	header += strings.Repeat(" ", m.width-lipgloss.Width(header)-lipgloss.Width(label)) + label
	bodyHeight := height - 4
	panels := lipgloss.JoinHorizontal(lipgloss.Top,
		panel("Workspaces", m.treeBody(left-4, bodyHeight), left, height, m.focus == 0),
		panel("Main", m.mainBody(center-4, bodyHeight), center, height, m.focus == 1),
		panel("AI assistant", m.chatBody(right-4, bodyHeight), right, height, m.focus == 2))
	footer := muted.Width(m.width).Render(clipped(" Tab / Shift+Tab panels · "+m.hint(), m.width))
	status := m.status
	if status == "" {
		status = "Local workspace layer · AI and live tool operations are not connected"
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, panels, footer, muted.Width(m.width).Render(clipped(" "+oneLine(status), m.width)))
}
func panel(title, content string, width, height int, focused bool) string {
	color := lipgloss.Color("#444C56")
	if focused {
		color = accent
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(color).Render(title)
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(color).Padding(0, 1).Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(heading + "\n\n" + content)
}
func listView(items []string, index, width, height int) string {
	if height <= 0 {
		return ""
	}
	index = max(0, min(index, len(items)-1))
	start := max(0, index-height+1)
	end := min(len(items), start+height)
	lines := []string{}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == index {
			prefix = "› "
		}
		line := clipped(prefix+oneLine(items[i]), width)
		if i == index {
			line = selected.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func textView(text string, width, height, offset int) string {
	if height <= 0 {
		return ""
	}
	wrapped := ansi.Wrap(plain(text), max(1, width), "")
	lines := strings.Split(wrapped, "\n")
	offset = max(0, min(offset, max(0, len(lines)-height)))
	return strings.Join(lines[offset:min(len(lines), offset+height)], "\n")
}
func (m *model) treeBody(width, height int) string {
	rows := m.treeRows()
	items := make([]string, len(rows))
	for i, row := range rows {
		marker := ""
		if row.kind == "workspace" || row.kind == "tool" {
			marker = "▸ "
			if row.open {
				marker = "▾ "
			}
		}
		items[i] = strings.Repeat(" ", row.depth) + marker + row.label
		if row.kind == "connection" && row.connection == m.connection {
			items[i] += " •"
		} else if row.kind == "workspace" && row.workspace == m.active {
			items[i] += " •"
		}
	}
	return listView(items, m.treeCursor, width, height)
}
func (m *model) tabs(width int) string {
	labels := make([]string, 4)
	for i, name := range modes {
		labels[i] = fmt.Sprintf(":%d %s", i+1, name)
		if m.mode == i {
			labels[i] = selected.Render(labels[i])
		} else {
			labels[i] = muted.Render(labels[i])
		}
	}
	return clipped(labels[0]+"  "+labels[1], width) + "\n" + clipped(labels[2]+"  "+labels[3], width)
}
func (m *model) mainBody(width, height int) string {
	scope := "No workspace"
	if w := m.current(); w != nil {
		scope = w.Name
		if m.tool != "" {
			scope += " / " + toolNames[m.tool]
		}
		if c := m.currentConnection(); c != nil {
			scope += " / " + c.Name
		}
	}
	heading := m.tabs(width) + "\n" + muted.Render(clipped(oneLine(scope), width)) + "\n\n"
	available := max(1, height-4)
	var body string
	switch {
	case m.form != nil:
		f := m.form
		items := []string{}
		for i, field := range f.fields {
			label := field.label
			if field.required {
				label += " *"
			}
			value := editorLine(field.value, width-2, m.focus == 1 && f.index == i)
			if value == "" {
				value = "(empty)"
			}
			items = append(items, label, value)
		}
		action := "Save"
		if f.kind == "kube" {
			action = "Find contexts"
		}
		items = append(items, "[ "+action+" ]", "[ Cancel ]")
		if f.remove {
			items = append(items, "[ Remove connection ]")
		}
		selection := len(f.fields)*2 + f.index - len(f.fields)
		if f.index < len(f.fields) {
			selection = f.index*2 + 1
		}
		body = clipped(f.title, width) + "\n" + listView(items, selection, width, available-1)
	case m.command != nil:
		body = ": " + m.command.display(m.focus == 1) + "\n\n1 Overview · 2 Resources\n3 Deployment · 4 Activity\nnew · settings · help · quit"
	case m.page == "settings":
		body = "Workspace settings\n" + listView(m.settingsItems(), m.cursor, width, available-1)
	case m.page == "tools":
		items := []string{}
		for _, tool := range toolTypes {
			items = append(items, toolNames[tool])
		}
		body = "Add tool\n" + listView(items, m.cursor, width, available-1)
	case m.page == "contexts":
		items := []string{}
		for i, c := range m.contexts {
			mark := "[ ] "
			if m.chosen[i] {
				mark = "[x] "
			}
			items = append(items, mark+c.Name)
		}
		body = "Choose kubecontexts\n" + listView(items, m.cursor, width, available-1)
	case m.page == "remove":
		name := "connection"
		if w := m.current(); w != nil {
			for _, c := range w.Connections {
				if c.ID == m.removeID {
					name = c.Name
				}
			}
		}
		body = "Remove " + name + "?\n" + listView([]string{"Cancel", "Remove from workspace"}, m.cursor, width, available-1)
	case m.page == "help":
		body = textView("Main commands (Enter to run)\n:1 Overview\n:2 Resources\n:3 Deployment\n:4 Activity\n:new Create workspace\n:settings Workspace settings\n:help This help\n:quit Save session and exit\n\nTab / Shift+Tab switch panels.\nArrows navigate. Enter activates.\nForms: Enter advances; arrows select fields or Save/Cancel. Esc cancels.\nText and pasted text stay literal in forms and chat.\nChat drafts save locally; Enter inserts a newline while editing. Esc leaves editing.\nNo resource action keys are assigned yet.", width, available, m.cursor)
	case m.current() == nil:
		body = "welcome to plous\n\nCreate your first workspace.\n\n[ Enter: Create workspace ]\nYou can add tools now or later."
	case m.detail != "":
		body = textView(m.detail, width, available, m.cursor)
	case m.mode == 3:
		if len(m.changes) == 0 {
			body = "No workspace changes yet."
		} else {
			items := []string{}
			for _, c := range m.changes {
				items = append(items, fmt.Sprintf("r%d %s · %s", c.Revision, c.Time.Local().Format("Jan 02 15:04"), changeSummary(c)))
			}
			body = listView(items, m.positions[m.viewKey()], width, available)
		}
	default:
		body = textView(m.modeContent(), width, available, m.positions[m.viewKey()])
	}
	return heading + styledView(body, width, available)
}
func (m *model) modeContent() string {
	w := m.current()
	if w == nil {
		return ""
	}
	switch m.mode {
	case 0:
		text := fmt.Sprintf("%s\n%d saved connection(s) · revision %d\n\n", w.Name, len(w.Connections), w.Revision)
		if m.tool != "" {
			text += "Selected tool: " + toolNames[m.tool] + "\n"
		}
		for _, c := range w.Connections {
			if m.tool == "" || m.tool == c.Tool {
				text += toolNames[c.Tool] + " / " + c.Name + "\n"
			}
		}
		if len(w.Connections) == 0 {
			text += "No tools added yet.\n"
		}
		return text + "\n:settings edits this workspace.\n:new creates another workspace.\nConfiguration is saved locally. Live connectivity has not been checked."
	case 1:
		c := m.currentConnection()
		if c == nil {
			return "Select a connection in the workspace tree.\n\nLive resource browsing is not implemented in this round."
		}
		text := toolNames[c.Tool] + " / " + c.Name + "\n\n"
		keys := []string{}
		for k := range c.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if c.Settings[k] != "" {
				text += k + ": " + c.Settings[k] + "\n"
			}
		}
		return text + "\nSaved configuration only.\nLive resource browsing is not implemented in this round."
	case 2:
		return "No deployments yet.\n\nDeployment creation and execution will be added in a later step. Your saved connections are available in Workspace settings."
	}
	return ""
}
func changeSummary(c workspace.Change) string {
	if c.Before == nil {
		return "Workspace created"
	}
	if c.Before.Name != c.After.Name {
		return "Workspace renamed"
	}
	before, after := len(c.Before.Connections), len(c.After.Connections)
	if after > before {
		return "Connections added"
	}
	if after < before {
		return "Connection removed"
	}
	return "Settings updated"
}
func (m *model) changeDetail(c workspace.Change) string {
	before, _ := json.MarshalIndent(c.Before, "", "  ")
	after, _ := json.MarshalIndent(c.After, "", "  ")
	return fmt.Sprintf("Revision %d · %s\n%s\nActor: %s\n\nBEFORE\n%s\n\nAFTER\n%s\n\nEsc: back", c.Revision, changeSummary(c), c.Time.Local().Format("2006-01-02 15:04:05 MST"), c.Actor, before, after)
}
func (m *model) chatBody(width, height int) string {
	if m.current() == nil {
		return textView("Create or select a workspace to keep a draft here.\n\nAI is not connected.", width, height, 0)
	}
	e := m.draft()
	label := "Draft · saved locally\nAI not connected\n\n"
	text := e.display(m.focus == 2 && m.chatEditing)
	if text == "" {
		text = "Type a draft here.\nNothing will be sent."
	}
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	offset := 0
	if m.focus == 2 && m.chatEditing {
		prefix := string([]rune(e.text)[:min(e.cursor, len([]rune(e.text)))])
		cursorLine := len(strings.Split(ansi.Wrap(prefix+"▏", max(1, width), ""), "\n")) - 1
		offset = max(0, cursorLine-max(1, height-4)+1)
	} else {
		offset = max(0, len(lines)-max(1, height-4))
	}
	return label + textView(text, width, max(1, height-4), offset)
}
func (m *model) hint() string {
	if m.focus == 0 {
		return "↑↓ select · ←→ fold · Enter open"
	}
	if m.focus == 2 {
		return "Type draft · Enter newline · Esc leave editing"
	}
	if m.form != nil {
		return "↑↓ fields · Enter next/activate · Esc cancel"
	}
	if m.command != nil {
		return "Enter run · Esc cancel"
	}
	if m.page == "contexts" {
		return "↑↓ contexts · Space select · Enter add · Esc back"
	}
	if m.page != "" {
		return "↑↓ select/scroll · Enter open · Esc back"
	}
	return ":1–:4 modes · :settings · :new · :help · :quit"
}

// Body components already sanitize user text before applying terminal styles.
func styledView(text string, width, height int) string {
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	return strings.Join(lines[:min(len(lines), max(0, height))], "\n")
}

func editorLine(e editor, width int, active bool) string {
	text := e.display(active)
	if active {
		prefix := string([]rune(e.text)[:min(e.cursor, len([]rune(e.text)))]) + "▏"
		skip := max(0, ansi.StringWidth(prefix)-max(1, width)+1)
		text = ansi.TruncateLeft(text, skip, "")
	}
	return clipped(text, width)
}

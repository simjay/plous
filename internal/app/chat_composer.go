package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) chatScope() string { return m.assistantScopeLabel() }

func (m *model) chatPanelTitle(width int) string {
	if m.externalAgentSelected() {
		label := m.settings.MainAgent.Label()
		if m.nativeAgent != nil {
			label += " · " + m.nativeAgent.Label
		}
		return ui.Clipped(label, width)
	}
	if width < 30 {
		return "AI assistant"
	}
	return "AI assistant · " + appchat.ChatAssistantStyle.Render(ui.Clipped(m.chatScope(), width-15))
}

func (m *model) chatBody(width, height int) string {
	if m.externalAgentSelected() {
		return m.nativeAgentBody(width, height)
	}
	if m.chat().Models {
		return m.modelPickerBody(width, height)
	}
	historyLabel := "History"
	if width < 24 {
		historyLabel = "Chats"
	}
	controls := textview.ActionBar([]textview.ActionGroup{{Actions: []textview.Action{
		{Key: ":H", Label: historyLabel}, {Key: ":C", Label: "New"},
	}}}, width, 1, ui.WorkspaceTheme())
	if width < 30 {
		controls = appchat.ChatAssistantStyle.Render(ui.Clipped(m.chatScope(), width)) + "\n" + controls
	}
	active := m.focus == 2 && m.chatEditing && !m.chat().History
	e := m.draft()
	inputWidth := max(1, width-4)
	text := e.MouseDisplay(active, "composer", ui.WorkspaceTheme())
	if e.Text == "" {
		text = "cc Write a message"
		if active {
			text = "▏Ask about " + m.chatScope() + "…"
		}
		text = ui.Muted.Render(ui.Clipped(text, inputWidth))
	}
	lines := strings.Split(ansi.Hardwrap(text, inputWidth, true), "\n")
	inputHeight := min(len(lines), min(5, max(1, height/3-3)))
	offset := max(0, len(lines)-inputHeight)
	if active {
		prefix := string([]rune(e.Text)[:min(e.Cursor, len([]rune(e.Text)))]) + "▏"
		cursorLine := len(strings.Split(ansi.Hardwrap(prefix, inputWidth, true), "\n")) - 1
		offset = max(0, cursorLine-inputHeight+1)
	}
	offset = min(offset, max(0, len(lines)-inputHeight))
	editorContent := strings.Join(lines[offset:min(len(lines), offset+inputHeight)], "\n")
	border := ui.BorderColor
	if active {
		border = ui.Accent
	}
	kind := m.settings.MainAgent.Effective()
	if m.run != nil && m.run.Chat == m.chat() {
		kind = m.run.Agent.Effective()
	}
	label := kind.Label()
	if kind == agent.Plous {
		label = appchat.ChatModelName(m.settings.Agent)
		if m.settings.Agent.Model == "" {
			label = "No model"
		}
		label = ui.Clipped(ui.OneLine(label), max(1, inputWidth-7))
		label += strings.Repeat(" ", max(1, inputWidth-ansi.StringWidth(label)-6)) + ui.MouseTarget("/model", "models")
	} else {
		label = ui.Clipped(label+" · CLI model", inputWidth)
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(width).MaxWidth(width).
		Render(editorContent + "\n" + ui.Muted.Render(label))
	progress := m.chatProgress(width)
	progressHeight := 0
	if progress != "" {
		progress += "\n"
		progressHeight = 1
	}
	transcriptHeight := max(0, height-lipgloss.Height(controls)-lipgloss.Height(box)-1-progressHeight)
	transcript := lipgloss.NewStyle().Width(width).Height(transcriptHeight).MaxWidth(width).MaxHeight(transcriptHeight).
		Render(m.chatTranscript(width, transcriptHeight))
	return textview.Fit(controls+"\n"+transcript+"\n"+progress+ui.MouseTarget(box, "composer")+"\n"+ui.Muted.Render(m.chatContextLine(width)), width, height)
}

func (m *model) chatProgress(width int) string {
	if m.run == nil {
		return ""
	}
	label := m.run.Chat.Progress
	if label == "" {
		label = "Responding…"
	}
	if m.run.Chat != m.chat() {
		label = "Reply running in another conversation"
	}
	stop := textview.ActionChip(textview.Action{Key: ":X", Label: "Stop"}, width, ui.WorkspaceTheme())
	room := width - ansi.StringWidth(stop) - 1
	if room < 4 {
		return stop
	}
	return ui.Clipped(m.loading(label), room) + " " + stop
}

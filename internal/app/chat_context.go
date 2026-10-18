package app

import (
	"encoding/json"
	"fmt"
	agent "github.com/simjay/plous/internal/ai"
	backend "github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) chatMessages(draft string, preview bool) ([]ai.Message, string) {
	contextText := m.contextText()
	system := agent.Instructions
	if m.settings.Agent.Instructions != "" {
		system += "\n\n" + m.settings.Agent.Instructions
	}
	c := m.chat()
	response := ""
	if preview && m.run != nil && m.run.Chat == c && len(c.Turns) > 0 {
		t := c.Turns[len(c.Turns)-1]
		if c.ActiveTurn != nil {
			t = *c.ActiveTurn
		}
		if t.Response.Status == "streaming" {
			draft, response = t.Request.Prompt, t.Response.Text
		}
	}
	messages := agent.PreviewMessages(system, contextText, draft)
	if len(c.Messages) > 0 {
		recent, err := ai.Prepare(c.Messages, nil)
		if err == nil {
			previewMessages := messages[:1:1]
			for _, message := range recent.Messages {
				data, _ := json.Marshal(message)
				previewMessages = append(previewMessages, ai.Message{Role: string(message.Role), Text: string(data)})
			}
			messages = append(previewMessages, messages[len(messages)-1])
		}
	}
	if response != "" {
		messages = append(messages, ai.Message{Role: "assistant", Text: response})
	}
	return messages, contextText
}

func (m *model) chatContextUsage() (used, limit int) {
	if m.settings.MainAgent.Effective() != backend.Plous {
		return 0, 0
	}
	if m.run != nil && m.run.Chat == m.chat() && m.run.ContextLimit > 0 {
		return m.run.ContextUsed, m.run.ContextLimit
	}
	model, ok := ai.LookupModel(m.settings.Agent.Provider, m.settings.Agent.Model)
	if !ok {
		return 0, 0
	}
	messages, _ := m.chatMessages(m.draft().Text, true)
	return appchat.EstimateTokens(messages) + agent.EstimateTools(agent.ToolDefinitions(agent.Options{UI: m.uiTools(m.assistantConnections()), Store: m.store, WorkspaceID: m.conversationScope(), Connections: m.assistantConnections()})), model.ContextWindow
}

func (m *model) chatContextLine(width int) string {
	if m.chat().Restore != nil {
		return ui.Clipped("Context: restoring saved conversation", width)
	}
	used, limit := m.chatContextUsage()
	if limit == 0 {
		if m.settings.MainAgent.Effective() != backend.Plous {
			return ui.Clipped("Context: managed by "+m.settings.MainAgent.Label(), width)
		}
		return ui.Clipped("Context: no model", width)
	}
	label := fmt.Sprintf("Context ~%s / %s tokens", appchat.CompactTokens(used), appchat.CompactTokens(limit))
	if len(label) > width {
		label = fmt.Sprintf("~%s/%s tokens", appchat.CompactTokens(used), appchat.CompactTokens(limit))
	}
	return ui.Clipped(label, width)
}

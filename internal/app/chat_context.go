package app

import (
	"fmt"
	"strings"

	"github.com/simjay/plous/internal/agent"
	"github.com/simjay/plous/internal/ai"
)

func (m *model) chatMessages(draft string, preview bool) ([]ai.Message, string) {
	contextText := m.contextText()
	system := agent.Instructions
	if m.settings.Agent.Instructions != "" {
		system += "\n\n" + m.settings.Agent.Instructions
	}
	if contextText != "" {
		system += "\n\nCaptured workspace and page context:\n" + contextText
	}
	c := m.chat()
	messages := agent.Messages(system, c.turns, c.summary, draft)
	if preview && m.run != nil && m.run.chat == c && len(c.turns) > 0 {
		t := c.turns[len(c.turns)-1]
		if t.Response.Status == "streaming" {
			messages = append(messages, ai.Message{Role: "user", Text: t.Request.Prompt}, ai.Message{Role: "assistant", Text: t.Response.Text})
		}
	}
	return messages, contextText
}

func estimateTokens(messages []ai.Message) int { return agent.Estimate(messages) }

func (m *model) chatContextUsage() (used, limit int) {
	model, ok := ai.LookupModel(m.settings.Agent.Provider, m.settings.Agent.Model)
	if !ok {
		return 0, 0
	}
	messages, _ := m.chatMessages(m.draft().text, true)
	return estimateTokens(messages), model.ContextWindow
}

func (m *model) chatContextLine(width int) string {
	used, limit := m.chatContextUsage()
	if limit == 0 {
		return clipped("Context: no model", width)
	}
	label := fmt.Sprintf("Context ~%s / %s tokens", compactTokens(used), compactTokens(limit))
	if len(label) > width {
		label = fmt.Sprintf("~%s/%s tokens", compactTokens(used), compactTokens(limit))
	}
	return clipped(label, width)
}

func compactTokens(tokens int) string {
	if tokens >= 1_000_000 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(tokens)/1_000_000), "0"), ".") + "M"
	}
	if tokens >= 1_000 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", float64(tokens)/1_000), "0"), ".") + "k"
	}
	return fmt.Sprint(tokens)
}

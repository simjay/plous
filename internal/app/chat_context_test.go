package app

import (
	"github.com/charmbracelet/x/ansi"
	agent "github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"

	"reflect"
	"strings"
	"testing"
)

func TestChatContextPinsTaskAndScopeWithoutReadingLegacyHistory(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	selectChatConnection(t, m)
	m.settings.Agent = ai.DefaultConfig()
	m.settings.Agent.Model = "gpt-5.4-mini"
	m.settings.Agent.Instructions = "Keep answers concise."
	messages, metadata := m.chatMessages("  next question  ", false)
	if !strings.Contains(metadata, "Payments") || len(messages) != 2 || !strings.Contains(messages[0].Text, "Keep answers concise.") || !strings.Contains(messages[0].Text, metadata) {
		t.Fatalf("unexpected context: %#v, %q", messages, metadata)
	}
	if messages[1].Role != "user" || messages[1].Text != "  next question  " || strings.Contains(messages[0].Text, "first question") {
		t.Fatalf("task or history boundary changed: %#v", messages)
	}
	preview, _ := m.chatMessages("  next question  ", true)
	if !reflect.DeepEqual(messages, preview) {
		t.Fatal("inactive preview must match the outgoing request")
	}

}

func TestContextMeterTracksDraftRepliesAndModelWithoutCountingBilledUsage(t *testing.T) {
	m, _ := testModel(t)
	m.settings.Agent.Model = "gpt-5.4-mini"
	base, limit := m.chatContextUsage()
	if base <= 0 || limit != 400_000 {
		t.Fatal("system instructions and model capacity are missing")
	}
	*m.draft() = textview.NewField(strings.Repeat("hello ", 100))
	withDraft, _ := m.chatContextUsage()
	if withDraft <= base {
		t.Fatal("draft is not counted")
	}
	c := m.chat()
	c.Turns = []history.Turn{{Sequence: 1, Request: history.Request{ID: "active", Prompt: m.draft().Text}, Response: history.Response{Status: "streaming"}}}
	*m.draft() = textview.NewField("")
	m.run = &appchat.ChatRun{Chat: c}
	beforeReply, _ := m.chatContextUsage()
	c.Turns[0].Response.Text = strings.Repeat("answer ", 100)
	afterReply, _ := m.chatContextUsage()
	if beforeReply < withDraft || afterReply <= beforeReply {
		t.Fatal("active prompt or streamed reply disappeared from the meter")
	}
	c.Turns[0].Response.Status = "completed"
	m.run = nil
	completed, _ := m.chatContextUsage()
	if completed != base {
		t.Fatal("completed history must return through recall, not the next prompt preview")
	}
	c.Turns[0].Response.Usage.Total = 999_999
	if got, _ := m.chatContextUsage(); got != completed {
		t.Fatal("billed usage entered context estimate")
	}
	m.settings.Agent.Provider, m.settings.Agent.Model = "anthropic", "claude-haiku-4-5-20251001"
	switched, limit := m.chatContextUsage()
	if switched != completed || limit != 200_000 {
		t.Fatal("switching vendors should retain history and update capacity")
	}
	c.Turns[0].Response.Status = "cancelled"
	c.Messages = nil
	if got, _ := m.chatContextUsage(); got != base {
		t.Fatal("cancelled response must leave future context")
	}
	m.settings.Agent.Model = "unknown"
	if _, limit := m.chatContextUsage(); limit != 0 {
		t.Fatal("unknown models must not show an invented capacity")
	}
}

func TestContextMeterUsesOnlyCurrentConversation(t *testing.T) {
	m, _ := testModel(t)
	m.settings.Agent.Model = "gpt-5.4-mini"
	first := m.chat()
	first.Turns = []history.Turn{{Request: history.Request{Prompt: strings.Repeat("large history ", 100)}, Response: history.Response{Status: "streaming", Text: "partial"}}}
	m.run = &appchat.ChatRun{Chat: first}
	createWorkspace(t, m, "Second")
	selectChatConnection(t, m)
	messages, metadata := m.chatMessages("", false)
	want := agent.Estimate(agent.PreviewMessages(agent.Instructions, metadata, "")) + agent.EstimateTools(agent.ToolDefinitions(agent.Options{UI: m.uiTools(m.assistantConnections()), Store: m.store, WorkspaceID: m.conversationScope(), Connections: m.assistantConnections()}))
	if got, _ := m.chatContextUsage(); got != want || len(messages) != 2 || !strings.Contains(metadata, "Second") {
		t.Fatal("another conversation's active request leaked into the counter")
	}
}

func TestContextMeterRebuildsFromSavedConversationAndDraft(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "OpenAI", "openai")
	conversation, err := m.history.NewConversation("", "Saved context")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: m.settings.Agent, Prompt: "Remember this question"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Response.Status, turn.Response.Text = "completed", "Saved answer"
	if err := m.history.SaveResponse("", conversation.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	m.openConversation(m.chat(), conversation)
	finishChatRestore(t, m)
	*m.draft() = textview.NewField("Follow-up draft")
	before, limit := m.chatContextUsage()
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newModel(m.store, m.history, "0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	finishChatRestore(t, restarted)
	if got, capacity := restarted.chatContextUsage(); got != before || capacity != limit {
		t.Fatal("reopening history changed its context estimate")
	}
	mainCommand(t, m, "C")
	if got, _ := m.chatContextUsage(); got != before || m.draft().Text != "Follow-up draft" {
		t.Fatal("new conversation should preserve the draft estimate without importing saved history")
	}
}

func TestContextLineFitsAndPreservesCapacity(t *testing.T) {
	m, _ := testModel(t)
	for _, provider := range ai.Providers() {
		for _, model := range ai.Models(provider) {
			m.settings.Agent.Provider, m.settings.Agent.Model = provider, model.ID
			for _, draft := range []string{"", strings.Repeat("long draft ", 24_000), strings.Repeat("한글 문장 ", 100)} {
				*m.draft() = textview.NewField(draft)
				for _, width := range []int{20, 32, 60} {
					line := m.chatContextLine(width)
					if ansi.StringWidth(line) > width || !strings.Contains(line, "~") || !strings.Contains(line, appchat.CompactTokens(model.ContextWindow)+" tokens") || strings.Contains(line, "…") {
						t.Fatalf("incomplete context indicator: %q", line)
					}
				}
			}
		}
	}
}

package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

func TestChatContextIncludesCompletedHistoryAndAutomaticWorkspaceDetails(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.settings.Agent.Instructions = "Keep answers concise."
	m.chat().turns = []workspace.Turn{
		{Request: workspace.Request{Prompt: "first question"}, Response: workspace.Response{Status: "completed", Text: "first answer"}},
	}
	for _, status := range []string{"error", "cancelled", "interrupted", "streaming"} {
		m.chat().turns = append(m.chat().turns, workspace.Turn{Request: workspace.Request{Prompt: status}, Response: workspace.Response{Status: status, Text: "excluded"}})
	}
	messages, metadata := m.chatMessages("  next question  ", false)
	if !strings.Contains(metadata, "Payments") || len(messages) != 4 || !strings.Contains(messages[0].Text, "Keep answers concise.") || strings.Count(messages[0].Text, metadata) != 1 {
		t.Fatalf("unexpected context: %#v, %q", messages, metadata)
	}
	want := []ai.Message{{Role: "user", Text: "first question"}, {Role: "assistant", Text: "first answer"}, {Role: "user", Text: "next question"}}
	if !reflect.DeepEqual(messages[1:], want) {
		t.Fatalf("history changed: %#v", messages[1:])
	}
	preview, _ := m.chatMessages("next question", true)
	if !reflect.DeepEqual(messages, preview) {
		t.Fatal("inactive preview must match the outgoing request")
	}
	withLegacyFlag, legacyMetadata := m.chatMessages("next question", false)
	if !reflect.DeepEqual(messages, withLegacyFlag) || legacyMetadata != metadata {
		t.Fatal("legacy context flag changed automatic workspace details")
	}
}

func TestContextMeterTracksDraftRepliesAndModelWithoutCountingBilledUsage(t *testing.T) {
	m, _ := testModel(t)
	m.settings.Agent.Model = "gpt-5.4-mini"
	base, limit := m.chatContextUsage()
	if base <= 0 || limit != 400_000 {
		t.Fatal("system instructions and model capacity are missing")
	}
	*m.draft() = newEditor(strings.Repeat("hello ", 100))
	withDraft, _ := m.chatContextUsage()
	if withDraft <= base {
		t.Fatal("draft is not counted")
	}
	c := m.chat()
	c.turns = []workspace.Turn{{Request: workspace.Request{Prompt: m.draft().text}, Response: workspace.Response{Status: "streaming"}}}
	*m.draft() = newEditor("")
	m.run = &chatRun{chat: c}
	beforeReply, _ := m.chatContextUsage()
	c.turns[0].Response.Text = strings.Repeat("answer ", 100)
	afterReply, _ := m.chatContextUsage()
	if beforeReply < withDraft || afterReply <= beforeReply {
		t.Fatal("active prompt or streamed reply disappeared from the meter")
	}
	c.turns[0].Response.Status = "completed"
	c.turns[0].Response.Usage.Total = 999_999
	m.run = nil
	completed, _ := m.chatContextUsage()
	if completed != afterReply {
		t.Fatal("completion must not double count tokens or sum billed usage")
	}
	m.settings.Agent.Provider, m.settings.Agent.Model = "anthropic", "claude-haiku-4-5-20251001"
	switched, limit := m.chatContextUsage()
	if switched != completed || limit != 200_000 {
		t.Fatal("switching vendors should retain history and update capacity")
	}
	c.turns[0].Response.Status = "cancelled"
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
	first.turns = []workspace.Turn{{Request: workspace.Request{Prompt: strings.Repeat("large history ", 100)}, Response: workspace.Response{Status: "streaming", Text: "partial"}}}
	m.run = &chatRun{chat: first}
	createWorkspace(t, m, "Second")
	messages, metadata := m.chatMessages("", false)
	if got, _ := m.chatContextUsage(); got != estimateTokens(messages) || len(messages) != 1 || !strings.Contains(metadata, "Second") {
		t.Fatal("another conversation's active request leaked into the counter")
	}
}

func TestContextMeterRebuildsFromSavedConversationAndDraft(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "OpenAI", "openai")
	conversation, err := m.store.NewConversation("", "Saved context")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.store.StartTurn("", conversation.ID, workspace.Request{Agent: m.settings.Agent, Prompt: "Remember this question"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Response.Status, turn.Response.Text = "completed", "Saved answer"
	if err := m.store.SaveResponse("", conversation.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	m.openConversation(m.chat(), conversation)
	*m.draft() = newEditor("Follow-up draft")
	before, limit := m.chatContextUsage()
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newModel(m.store, "0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got, capacity := restarted.chatContextUsage(); got != before || capacity != limit {
		t.Fatal("reopening history changed its context estimate")
	}
	mainCommand(t, m, "C")
	if got, _ := m.chatContextUsage(); got >= before {
		t.Fatal("new conversation retained the previous conversation's tokens")
	}
}

func TestContextLineFitsAndPreservesCapacity(t *testing.T) {
	m, _ := testModel(t)
	for _, provider := range ai.Providers() {
		for _, model := range ai.Models(provider) {
			m.settings.Agent.Provider, m.settings.Agent.Model = provider, model.ID
			for _, draft := range []string{"", strings.Repeat("long draft ", 24_000), strings.Repeat("한글 문장 ", 100)} {
				*m.draft() = newEditor(draft)
				for _, width := range []int{20, 32, 60} {
					line := m.chatContextLine(width)
					if ansi.StringWidth(line) > width || !strings.Contains(line, "~") || !strings.Contains(line, compactTokens(model.ContextWindow)+" tokens") || strings.Contains(line, "…") {
						t.Fatalf("incomplete context indicator: %q", line)
					}
				}
			}
		}
	}
}

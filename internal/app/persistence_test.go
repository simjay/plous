package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"sync"
	"testing"
	"time"
)

func blockPersistence(t *testing.T, m *model) func() {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	m.enqueuePersistence(&persistenceJob{write: func() any { close(entered); <-release; return nil }})
	m.syncPersistence()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	return unblock
}

func persistenceRun(t *testing.T, m *model) *appchat.ChatRun {
	t.Helper()
	c := m.chat()
	conversation, err := m.history.NewConversation("", "Durable response")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Prompt: "Question", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	c.Conversation, c.Turns, c.ActiveTurn = conversation, []history.Turn{turn}, &turn
	m.chatIDs[""] = conversation.ID
	_, cancel := context.WithCancel(context.Background())
	run := &appchat.ChatRun{Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent, 2)}
	m.run, m.focus, m.chatEditing = run, 2, true
	return run
}

func TestCompletionWaitsForDurableSaveWithoutBlockingInput(t *testing.T) {
	m, _ := testModel(t)
	run := persistenceRun(t, m)
	release := blockPersistence(t, m)
	m.updateChatEvent(appchat.ChatEvent{Run: run, Done: true, Result: ai.Result{Text: "Final answer"}})
	if run.Chat.ActiveTurn.Response.Status != "streaming" || run.Chat.Progress != "Saving response…" || m.run != run {
		t.Fatal("reply completed before persistence acknowledgment")
	}
	typeText(m, "Next draft")
	m.Update(autosaveMsg{})
	m.stopChat()
	if m.draft().Text != "Next draft" || run.Stopping {
		t.Fatal("save blocked typing or Stop changed a finished result")
	}
	release()
	m.drainPersistence()
	if m.run != nil || run.Chat.ActiveTurn != nil || run.Chat.Turns[0].Response.Status != "completed" {
		t.Fatal("durable closure not acknowledged")
	}
	outcome, err := m.history.TurnOutcome("", run.Chat.Conversation.ID, run.Chat.Turns[0].Request.ID)
	if err != nil || !outcome.Closed || outcome.Status != "completed" {
		t.Fatal(outcome, err)
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, err := m.store.ReadSession()
	if err != nil || !strings.Contains(string(data), "Next draft") {
		t.Fatal(string(data), err)
	}
}

func TestStopBeforeQueuedStartPreservesDraftAndCreatesNoRequest(t *testing.T) {
	m, _ := testModel(t)
	m.settings.Agent = ai.DefaultConfig()
	m.settings.Agent.Model = "gpt-5.4-mini"
	m.settings.Agent.Endpoint = "http://127.0.0.1:1/v1"
	m.settings.Agent.KeyEnv, m.settings.Agent.CredentialRef = "", ""
	*m.draft() = textview.NewField("Queued question")
	release := blockPersistence(t, m)
	m.sendChat()
	if m.run == nil || !m.run.Starting {
		t.Fatalf("start was not queued: %s", m.status)
	}
	m.stopChat()
	typeText(m, "")
	release()
	m.drainPersistence()
	conversations, err := m.history.Conversations("")
	if err != nil || len(conversations) != 0 || m.run != nil || m.draft().Text != "Queued question" {
		t.Fatal(conversations, err, m.status)
	}
}

func TestFailedClosureKeepsTextAndSavesDraftOnShutdown(t *testing.T) {
	m, _ := testModel(t)
	run := persistenceRun(t, m)
	text := strings.Repeat("x", (1<<20)+1)
	m.updateChatEvent(appchat.ChatEvent{Run: run, Done: true, Result: ai.Result{Text: text}})
	m.drainPersistence()
	if m.run != run || run.Chat.Turns[0].Response.Status != "streaming" || run.Chat.Turns[0].Response.Text != text || run.Chat.Err == "" {
		t.Fatal("failed save appeared completed or lost output")
	}
	m.openChatReply()
	if m.chatSources == nil || !strings.Contains(m.chatSources.Reply.Name, "Unsaved") || m.chatSources.Evidence.Err == nil {
		t.Fatal("unsaved text cannot be read and copied")
	}
	m.closeChatSources()
	m.Update(autosaveMsg{})
	for _, job := range m.persistenceQueue {
		if strings.HasPrefix(job.key, "response/") {
			t.Fatal("unchanged failed response retried automatically")
		}
	}
	*m.draft() = textview.NewField("Preserve despite save error")
	if err := m.saveSession(); err == nil {
		t.Fatal("shutdown hid the persistence failure")
	}
	data, err := m.store.ReadSession()
	if err != nil || !strings.Contains(string(data), "Preserve despite save error") {
		t.Fatal(string(data), err)
	}
	outcome, err := m.history.TurnOutcome("", run.Chat.Conversation.ID, run.Chat.Turns[0].Request.ID)
	if err != nil || outcome.Closed {
		t.Fatal("failed closure became durable", outcome, err)
	}
}

func TestSessionQueueCoalescesLatestDraftIncludingReturnToSavedValue(t *testing.T) {
	m, _ := testModel(t)
	*m.draft() = textview.NewField("original")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	release := blockPersistence(t, m)
	*m.draft() = textview.NewField("transient")
	m.persist()
	m.Update(autosaveMsg{})
	*m.draft() = textview.NewField("original")
	m.persist()
	m.Update(autosaveMsg{})
	release()
	m.drainPersistence()
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil || saved.Drafts[""] != "original" {
		t.Fatal(saved.Drafts, err)
	}
}

func TestQuitReturnsWhilePersistenceIsBlocked(t *testing.T) {
	m, _ := testModel(t)
	release := blockPersistence(t, m)
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !m.quitting {
		t.Fatal("quit did not request shutdown")
	}
	release()
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalSaveInvalidatesOlderRestoreSnapshot(t *testing.T) {
	m, _ := testModel(t)
	run := persistenceRun(t, m)
	c := run.Chat
	m.beginChatRestore(c)
	stale := m.syncChatRestore()().(appchat.ChatRestoreResult)
	if stale.Page.Turns[0].Response.Status != "streaming" {
		t.Fatal("fixture did not capture an unfinished turn")
	}
	m.updateChatEvent(appchat.ChatEvent{Run: run, Done: true, Result: ai.Result{Text: "Durable final"}})
	m.drainPersistence()
	m.updateChatRestore(stale)
	finishChatRestore(t, m)
	if c.Turns[0].Response.Status != "completed" || c.Turns[0].Response.Text != "Durable final" {
		t.Fatal("old restore replaced acknowledged response")
	}
}

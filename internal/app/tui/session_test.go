package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/app/activity"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
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

func TestEveryExitCancelsViewWork(t *testing.T) {
	for _, quit := range []bool{false, true} {
		t.Run(fmt.Sprintf("quit=%t", quit), func(t *testing.T) {
			activityCtx, activityCancel := context.WithCancel(t.Context())
			helmCtx, helmCancel := context.WithCancel(t.Context())
			exploreCtx, exploreCancel := context.WithCancel(t.Context())
			m := &model{
				activityUI:    activity.Controller{Load: &activity.ActivityLoad{Ctx: activityCtx, Cancel: activityCancel}},
				helmStates:    map[string]*monitor.HelmState{"cluster": {Cancel: helmCancel}},
				clusterStates: map[string]*clusterState{"cluster": {Cancel: exploreCancel}},
			}
			if quit {
				m.quitting = true
				cmd := m.syncQuit()
				if cmd == nil {
					t.Fatal("quit did not finish")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("quit returned an unrelated command")
				}
			} else {
				m.stopViews()
			}
			for name, ctx := range map[string]context.Context{"Activity": activityCtx, "Helm": helmCtx, "Kubernetes": exploreCtx} {
				if ctx.Err() != context.Canceled {
					t.Fatal("view work survived shutdown", name)
				}
			}
			if !m.exploreClosed {
				t.Fatal("view work survived shutdown")
			}
		})
	}
}

func TestCtrlCQuitsFromEveryPanelAndSavesSession(t *testing.T) {
	for _, small := range []bool{false, true} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("focus=%d/small=%t", focus, small), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				startChat(t, m)
				typeText(m, "latest draft\n:q is literal")
				mainCommand(t, m, "3")
				focusPanel(t, m, focus)
				if small {
					m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
				}
				assertCtrlCQuit(t, m)
				saved := readExitSession(t, m)
				if saved.Workspace != id || saved.Mode != 2 || saved.Focus != focus || saved.Drafts[m.conversationScope()] != "latest draft\n:q is literal" {
					t.Fatalf("Ctrl+C must save the latest session before exit: %#v", saved)
				}
			})
		}
	}
}

func TestCtrlCQuitsWithUnfinishedCommand(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "3")
	typeText(m, ":new")
	assertCtrlCQuit(t, m)
	if m.form != nil || readExitSession(t, m).Mode != 2 {
		t.Fatal("Ctrl+C must exit without executing the unfinished command")
	}
}

func TestCtrlCDoesNotApplyUnfinishedForm(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.beginRename()
	press(t, m, "ctrl+u")
	typeText(m, "Unsubmitted name")
	press(t, m, "enter")
	startChat(t, m)
	typeText(m, "keep this chat draft")
	focusPanel(t, m, 1)
	assertCtrlCQuit(t, m)
	workspaces, err := m.store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "Payments" || workspaces[0].Revision != 1 {
		t.Fatalf("exit must not submit unfinished form edits: %#v", workspaces)
	}
	if readExitSession(t, m).Drafts[m.conversationScope()] != "keep this chat draft" {
		t.Fatal("exit from a form must still save the latest chat draft")
	}
}

func TestCtrlCQuitsDuringFirstRun(t *testing.T) {
	for _, editing := range []bool{false, true} {
		t.Run(fmt.Sprintf("editing=%t", editing), func(t *testing.T) {
			m, _ := testModel(t)
			if editing {
				press(t, m, "enter")
				typeText(m, "Unsubmitted workspace")
			}
			assertCtrlCQuit(t, m)
			workspaces, err := m.store.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(workspaces) != 0 || readExitSession(t, m).Workspace != "" {
				t.Fatal("exit during first run must not create an unfinished workspace")
			}
		})
	}
}

func TestCtrlCSaveFailureKeepsAppOpenAndDraftRecoverable(t *testing.T) {
	for _, small := range []bool{false, true} {
		t.Run(fmt.Sprintf("small=%t", small), func(t *testing.T) {
			m, root := testModel(t)
			createWorkspace(t, m, "Payments")
			startChat(t, m)
			typeText(m, "draft survives storage failure")
			if small {
				m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
			}
			// A directory at the session path causes a real storage failure,
			// independently of platform permission behavior or test privileges.
			path := filepath.Join(root, "session.json")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			press(t, m, "ctrl+c")
			m.drainPersistence()
			if cmd := m.syncQuit(); cmd != nil || m.quitting {
				t.Fatal("failed session save must leave the application open")
			}
			if !strings.Contains(ansi.Strip(m.render()), "Cannot save before exit") {
				t.Fatalf("failed exit must show its storage error: %q", m.status)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			assertCtrlCQuit(t, m)
			if readExitSession(t, m).Drafts[m.conversationScope()] != "draft survives storage failure" {
				t.Fatal("retrying exit after storage recovery lost the draft")
			}
		})
	}
}

func assertCtrlCQuit(t *testing.T, m *model) {
	t.Helper()
	press(t, m, "ctrl+c")
	m.drainPersistence()
	cmd := m.syncQuit()
	if cmd == nil {
		t.Fatalf("Ctrl+C did not request exit: %s", m.status)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not return a quit message")
	}
}

func readExitSession(t *testing.T, m *model) session {
	t.Helper()
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	return saved
}

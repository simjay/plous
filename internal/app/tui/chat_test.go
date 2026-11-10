package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func TestChatCommandFocusesComposerFromNavigation(t *testing.T) {
	for _, command := range []string{"c", "chat"} {
		for _, from := range []string{"tree", "main", "chat", "settings", "history", "help", "hidden"} {
			t.Run(fmt.Sprintf("%s/from-%s", command, from), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				mainCommand(t, m, "1")
				*m.draft() = textview.NewField("Saved draft")
				switch from {
				case "tree":
					focusPanel(t, m, 0)
				case "chat":
					focusPanel(t, m, 2)
				case "settings":
					mainCommand(t, m, "s")
				case "history":
					mainCommand(t, m, "H")
				case "help":
					mainCommand(t, m, "h")
				case "hidden":
					hideAssistant(t, m)
				}
				page := m.page
				typeText(m, ":"+command)
				press(t, m, "enter")
				typeText(m, " continued")
				if m.focus != 2 || !m.chatEditing || m.rightHidden || m.help != nil || m.chat().History || m.command != nil {
					t.Fatalf("chat composer did not receive focus: %s", m.status)
				}
				if m.draft().Text != "Saved draft continued" || m.active != id || m.page != page {
					t.Fatal("chat entry changed workspace, view, or draft")
				}
			})
		}
	}
}

func TestChatCommandRevealsPanelBeforeWorkspaceCreation(t *testing.T) {
	for _, command := range []string{"c", "chat"} {
		m, _ := testModel(t)
		hideAssistant(t, m)
		mainCommand(t, m, "h")
		mainCommand(t, m, command)
		if m.focus != 2 || m.rightHidden || m.help != nil || !m.chatEditing {
			t.Fatal("chat entry should reveal the composer before workspace setup")
		}
		typeText(m, "Hello")
		m.Update(tea.PasteMsg{Content: " :s 11"})
		if m.draft().Text != "Hello :s 11" || len(m.workspaces) != 0 {
			t.Fatal("general chat must accept literal text without creating a workspace")
		}
	}
}

func TestApplicationSettingsSaveAndCancelThroughSettings(t *testing.T) {
	for _, command := range []string{"s", "settings"} {
		t.Run(command, func(t *testing.T) {
			m, _ := testModel(t)
			mainCommand(t, m, command)
			press(t, m, "enter")
			press(t, m, "enter")
			press(t, m, "enter")
			if m.form == nil || m.form.Kind != "appsettings" {
				t.Fatal("global settings unavailable before first workspace")
			}
			completeAIForm(t, m, map[string]string{"name": "Review helper", "provider": "openai", "endpoint": "http://localhost:1234/v1", "instructions": "Be concise"})
			if m.settings.Agent.Model != "gpt-5.4-mini" || m.settings.Revision != 1 || len(m.workspaces) != 0 {
				t.Fatal("settings were not saved independently")
			}
			mainCommand(t, m, command)
			press(t, m, "enter")
			press(t, m, "enter")
			press(t, m, "enter")
			press(t, m, "down")
			typeText(m, "unsaved")
			press(t, m, "esc")
			saved, err := m.store.ReadSettings()
			if err != nil || saved.Agent.Name != "Review helper" {
				t.Fatal("cancel applied edits", err)
			}
			if m.form == nil || m.form.Kind != "mainagent" {
				t.Fatal("cancel did not return to Agent setup")
			}
		})
	}
}

func TestChatSendKeysAndLiteralPaste(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Test")
	startChat(t, m)
	typeText(m, "hello")
	press(t, m, "shift+enter")
	typeText(m, ":q")
	press(t, m, "ctrl+j")
	m.Update(tea.PasteMsg{Content: "/new\n:setting"})
	if m.run != nil || m.command != nil || m.draft().Text != "hello\n:q\n/new\n:setting" {
		t.Fatal("text was interpreted as commands")
	}
	press(t, m, "enter")
	if m.run != nil || m.draft().Text == "" || !strings.Contains(m.status, "connect a vendor") {
		t.Fatalf("unconfigured send lost draft: %s", m.status)
	}
}

func TestChatRunRoutesUpdatesAndPreservesCancellation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "First")
	first := selectChatConnection(t, m)
	c := m.chat()
	conversation, err := m.history.NewConversation(first, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	c.Conversation = conversation
	turn, err := m.history.StartTurn(first, conversation.ID, history.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	c.Turns = []history.Turn{turn}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &appchat.ChatRun{Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent, 1)}
	m.run = run
	second := createWorkspace(t, m, "Second")
	selectChatConnection(t, m)
	m.Update(appchat.ChatEvent{Run: run, Text: "partial from first"})
	if m.chat() == c || m.active != second || c.ActiveTurn == nil || c.ActiveTurn.Response.Text != "partial from first" {
		t.Fatal("response landed in wrong workspace")
	}
	press(t, m, "ctrl+c")
	if !m.quitting {
		t.Fatal("Ctrl+C must request shutdown while streaming")
	}
	if ctx.Err() == nil {
		t.Fatal("request not cancelled")
	}
	if m.run != run || !run.Stopping {
		t.Fatal("cancellation released the run before producer completion")
	}
	m.Update(appchat.ChatEvent{Run: run, Done: true, Result: ai.Result{Text: "late result"}})
	m.drainPersistence()
	saved, err := m.history.Turns(first, conversation.ID)
	if err != nil || saved[0].Response.Status != "cancelled" || saved[0].Response.Text != "partial from first" {
		t.Fatal(saved, err)
	}
	if len(c.Turns) != 0 {
		t.Fatal("inactive response repopulated the evicted transcript")
	}
}

func TestChatRecoveryAndNewConversationSurviveRestart(t *testing.T) {
	m, root := testModel(t)
	createWorkspace(t, m, "Test")
	id := selectChatConnection(t, m)
	owner, connection := m.active, m.connection
	c, err := m.history.NewConversation(id, "unfinished")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn(id, c.ID, history.Request{Agent: ai.DefaultConfig(), Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Response.Text = "checkpoint"
	if err = m.history.SaveResponse(id, c.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	m.chatIDs[id] = c.ID
	if err = m.saveSession(); err != nil {
		t.Fatal(err)
	}
	m.store.Close()
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := newModel(store, openTestHistory(t, store), "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.width, restored.height = 120, 30
	restored.selectScope(owner, "files", connection)
	finishChatRestore(t, restored)
	if got := restored.chat().Turns[0].Response; got.Status != "interrupted" || got.Text != "checkpoint" {
		t.Fatal(got)
	}
	mainCommand(t, restored, "C")
	if err = restored.saveSession(); err != nil {
		t.Fatal(err)
	}
	again, err := newModel(store, restored.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	again.selectScope(owner, "files", connection)
	if again.chat().Conversation.ID != "" {
		t.Fatal("explicit new conversation reopened old history")
	}
	again.width, again.height = 120, 30
	mainCommand(t, again, "H")
	press(t, again, "down")
	press(t, again, "enter")
	if again.chat().Conversation.ID != c.ID {
		t.Fatal("history did not reopen conversation")
	}
}

func TestWorkspaceContextIsAutomaticAndContainsOnlySavedScope(t *testing.T) {
	m, _ := testModel(t)
	if m.contextText() != "" {
		t.Fatal("General chat must not have a workspace context")
	}
	createWorkspace(t, m, "Payments")
	m.workspaces[0].Connections = []workspace.Connection{
		{ID: "conn", Name: "dev", Tool: "kubernetes", Settings: map[string]string{"context": "development", "namespace": "payments", "kubeconfig": "SECRET_PATH", "token": "SECRET_TOKEN"}},
		{ID: "other", Name: "staging", Tool: "kubernetes", Settings: map[string]string{"context": "stage-context"}},
	}
	m.connection = "conn"
	m.tool = "kubernetes"
	text := m.contextText()
	if strings.Contains(text, "SECRET") || strings.Contains(text, "stage-context") || !strings.Contains(text, "development") || !strings.Contains(text, "payments") || !strings.Contains(text, "Payments") {
		t.Fatal(text)
	}
	m.connection = "other"
	if next := m.contextText(); !strings.Contains(next, "stage-context") || strings.Contains(next, "development") {
		t.Fatal("connection selection did not update the next request's scope", next)
	}
	m.active = ""
	if m.contextText() != "" {
		t.Fatal("General chat inherited the previous workspace context")
	}
}

func TestGeneralDraftPersistsAndStaysSeparateFromWorkspace(t *testing.T) {
	m, _ := testModel(t)
	typeText(m, "cc")
	typeText(m, "General :s 11 draft")
	press(t, m, "esc")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newModel(m.store, m.history, "test-version")
	if err != nil || reloaded.draft().Text != "General :s 11 draft" {
		t.Fatal("general draft did not survive restart", err)
	}
	focusPanel(t, m, 1)
	createWorkspace(t, m, "Payments")
	selectChatConnection(t, m)
	if m.draft().Text != "" {
		t.Fatal("general draft leaked into a workspace")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	reloaded, err = newModel(m.store, m.history, "test-version")
	if err != nil || reloaded.active != "" || reloaded.draft().Text != "General :s 11 draft" {
		t.Fatal("restart changed the selected workspace chat", err)
	}
	if draft := reloaded.drafts[""]; draft == nil || draft.Text != "General :s 11 draft" {
		t.Fatal("removing sidebar access must not erase a saved general draft")
	}
}

func selectChatConnection(t *testing.T, m *model) string {
	t.Helper()
	w := *m.current()
	w.Connections = append(w.Connections, workspace.Connection{Name: "Cluster", Tool: "kubernetes", Settings: map[string]string{"context": "test"}})
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	w = *m.current()
	m.selectScope(w.ID, "kubernetes", w.Connections[len(w.Connections)-1].ID)
	return m.conversationScope()
}

func TestAgentPickerExcludesUnsupportedAgents(t *testing.T) {
	m, n := nativeModel(t)
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	if !slices.Equal(m.form.Fields[0].Options, []string{"Plous", "Claude Code", "Codex"}) {
		t.Fatal("agent picker offers unsupported choices", m.form.Fields[0].Options)
	}
	m.form.Fields[0].Value = textview.NewField("OpenCode")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent != agent.ClaudeCode || m.form == nil || m.nativeChange != nil || n.Input.Ctx.Err() != nil {
		t.Fatal("unsupported choice replaced or closed the active agent")
	}
	if !strings.Contains(m.status, "not supported") {
		t.Fatal("unsupported choice was reported as available", m.status)
	}
}

func TestNativeAgentChangeWaitsForEvidenceAndPreservesFocus(t *testing.T) {
	m, n := nativeModel(t)
	t.Setenv("PATH", t.TempDir())
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	press(t, m, "right")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent != agent.ClaudeCode || n.Input.Ctx.Err() != context.Canceled || m.nativeChange == nil || n.Captured {
		t.Fatal("Save did not close the native session before applying the choice")
	}
	m.focus = 0
	m.openHelp()
	help := m.help
	m.draft().Insert("retained Plous draft", true)
	m.Update(nativeAgentResult{session: n})
	saved, err := m.store.ReadSettings()
	if err != nil || saved.MainAgent != agent.Codex || m.form != nil || m.nativeChange != nil || m.page != "settings" {
		t.Fatal("successful closure did not save the requested agent", saved, err)
	}
	if m.focus != 0 || m.help != help || m.draft().Text != "retained Plous draft" || m.nativeAgent != nil {
		t.Fatal("switch stole focus, lost the draft, or retained the old terminal")
	}
	if !strings.Contains(m.status, "unavailable") {
		t.Fatal("missing new executable was not reported", m.status)
	}
}

func TestNativeAgentChangeDoesNotApplyCanceledOrFailedSave(t *testing.T) {
	for _, action := range []string{"cancel", "edit", "quit", "evidence failure", "settings conflict"} {
		t.Run(action, func(t *testing.T) {
			m, n := nativeModel(t)
			t.Setenv("PATH", t.TempDir())
			if !m.saveAISettings(m.settings) {
				t.Fatal(m.status)
			}
			m.openSettings()
			m.selectSettingsItem("Agent: Claude Code")
			m.updateSettings("enter")
			press(t, m, "right")
			form := m.form
			m.submitForm()
			result := nativeAgentResult{session: n}
			switch action {
			case "cancel":
				m.cancelForm()
			case "edit":
				press(t, m, "right")
			case "quit":
				m.quitting = true
			case "evidence failure":
				result.saveErr = errors.New("fixture storage unavailable")
			case "settings conflict":
				next := m.settings
				next.MouseDisabled = !next.MouseDisabled
				if _, err := m.store.SaveSettings(next); err != nil {
					t.Fatal(err)
				}
			}
			m.Update(result)
			if m.settings.MainAgent != agent.ClaudeCode || m.nativeChange != nil {
				t.Fatal("late closure applied an unconfirmed choice")
			}
			if action == "evidence failure" || action == "settings conflict" {
				if m.form != form || appsettings.AgentKind(form.Fields[0].Value.Text) != agent.Codex {
					t.Fatal("failed save lost the pending form choice")
				}
				status := m.status
				m.View()
				if m.status != status {
					t.Fatal("rendering hid the save error", m.status)
				}
			}
		})
	}
}

func TestSavingCurrentAgentKeepsNativeSession(t *testing.T) {
	m, n := nativeModel(t)
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	m.submitForm()
	if n.Input.Ctx.Err() != nil || m.nativeAgent != n || m.nativeChange != nil || m.form != nil {
		t.Fatal("saving an unchanged choice restarted the native session")
	}
}

func TestMainAgentSettingsAndMissingExecutable(t *testing.T) {
	m, _ := testModel(t)
	if m.settings.MainAgent.Effective() != agent.Plous {
		t.Fatal("fresh profile did not choose Plous")
	}
	t.Setenv("PATH", t.TempDir())
	m.openSettings()
	m.selectSettingsItem("Agent: Plous")
	m.updateSettings("enter")
	if m.form == nil || m.form.Kind != "mainagent" {
		t.Fatal("missing Settings agent picker")
	}
	press(t, m, "right")
	if m.form.Fields[0].Value.Text != "Claude Code" {
		t.Fatal("wrong first external choice")
	}
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	saved, err := m.store.ReadSettings()
	if err != nil || saved.MainAgent != agent.ClaudeCode {
		t.Fatal(saved, err)
	}
	if !strings.Contains(m.status, "unavailable") {
		t.Fatal("missing executable not explained", m.status)
	}
	m.closeSettings()
	m.loadChat()
	m.focusChat(true)
	*m.draft() = textview.NewField("Keep this question")
	if cmd := m.sendChat(); cmd != nil || m.run != nil || m.draft().Text != "Keep this question" || !strings.Contains(m.status, "unavailable") {
		t.Fatal("unavailable runtime lost draft or dispatched", m.status)
	}
	if _, exists := helpEntries(m.chatHelp())["model"]; exists {
		t.Fatal("external-agent Help advertised the Plous model picker")
	}
	if strings.Contains(m.chatBody(60, 24), "/model") {
		t.Fatal("external agent offered Plous model selection")
	}
	*m.draft() = textview.NewField("/model")
	press(t, m, "enter")
	if m.chat().Models || m.draft().Text != "/model" {
		t.Fatal("external CLI input opened Plous model picker")
	}
}

func TestMainAgentCannotReplaceRunningTurn(t *testing.T) {
	m, _ := testModel(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	run := &appchat.ChatRun{Agent: agent.Plous, Chat: m.chat(), Cancel: cancel}
	m.run = run
	m.openSettings()
	m.selectSettingsItem("Agent: Plous")
	m.updateSettings("enter")
	press(t, m, "right")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent.Effective() != agent.Plous || m.run != run || ctx.Err() != nil || m.form == nil {
		t.Fatal("agent choice changed an active turn")
	}
	m.View()
	if !strings.Contains(m.status, "current response") {
		t.Fatal("rendering hid the reason switching is blocked", m.status)
	}
	m.run = nil
}

package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	agent "github.com/simjay/plous/internal/ai"
	backend "github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"

	"strings"
)

func (m *model) chat() *appchat.ChatState {
	if c := m.chats[m.conversationScope()]; c != nil {
		return c
	}
	return &appchat.ChatState{}
}

func (m *model) loadChat() {
	if m.chats == nil {
		m.chats = map[string]*appchat.ChatState{}
	}
	m.evictInactiveChats()
	if c := m.chats[m.conversationScope()]; c != nil {
		if c.Evicted {
			m.rehydrateChat(c)
		}
		return
	}
	c := &appchat.ChatState{}
	m.chats[m.conversationScope()] = c
	if m.history == nil {
		return
	}
	if err := m.ensureConversationScope(); err != nil {
		c.Err = err.Error()
		return
	}
	choices, err := m.history.Conversations(m.conversationScope())
	if err != nil {
		c.Err = err.Error()
		return
	}
	id, selected := m.chatIDs[m.conversationScope()]
	if !selected && len(choices) > 0 {
		id = choices[len(choices)-1].ID
	}
	for _, conversation := range choices {
		if conversation.ID == id {
			m.openConversation(c, conversation)
			break
		}
	}
}

func (m *model) openConversation(c *appchat.ChatState, conversation history.Conversation) {
	if c.Restore != nil {
		c.Restore.Cancel()
	}
	*c = appchat.ChatState{Conversation: conversation}
	m.beginChatRestore(c)
	if m.chatIDs == nil {
		m.chatIDs = map[string]string{}
	}
	m.chatIDs[conversation.WorkspaceID] = conversation.ID
	m.persist()
}

func (m *model) contextText() string {
	if m.savedConnection() == nil {
		return ""
	}
	// Capture identities and filters only; resource bodies are retrieved through tools.
	value := struct {
		Workspace   string                `json:"workspace"`
		Revision    uint64                `json:"revision"`
		Connections []agent.Connection    `json:"connections"`
		Viewing     appchat.AssistantPage `json:"viewing"`
	}{m.current().Name, m.current().Revision, m.assistantConnections(), m.assistantPageContext()}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func (m *model) sendChat() tea.Cmd {
	if m.externalAgentSelected() {
		if _, err := cli.Executable(m.settings.MainAgent); err != nil {
			m.status = err.Error()
			return nil
		}
		m.focusChat(true)
		return nil
	}
	if m.historyView != "" {
		m.status = "This retained history is read-only. Select a connection or start a new chat to continue."
		return nil
	}
	if err := m.ensureConversationScope(); err != nil {
		m.status = "Cannot capture connection scope: " + err.Error()
		return nil
	}
	c := m.chat()
	if c.Conversation.ID != "" && c.Conversation.WorkspaceID != m.conversationScope() {
		m.status = "This conversation belongs to another connection. Start a new chat to continue."
		return nil
	}
	draft := m.draft().Text
	text := strings.TrimSpace(draft)
	if text == "" {
		return nil
	}

	if m.run != nil {
		m.status = "An AI response is still running or being saved."
		return nil
	}
	if c.Restore != nil || m.chatRestore.Worker != nil {
		m.status = "Wait for saved conversation restoration to finish."
		return nil
	}
	if c.Err != "" {
		m.status = "Could not load conversation: " + c.Err
		return nil
	}
	kind := m.settings.MainAgent.Effective()
	config := m.settings.Agent
	if err := config.Validate(); err != nil {
		m.status = err.Error()
		return nil
	}
	if err := config.Supported(); err != nil {
		m.status = err.Error()
		return nil
	}
	if len(text) > 256<<10 {
		m.status = "The message exceeds the 256 KiB limit. Shorten the draft before sending."
		return nil
	}
	_, contextText := m.chatMessages(text, false)
	system := agent.Instructions
	if config.Instructions != "" {
		system += "\n\n" + config.Instructions
	}
	r := history.Request{Backend: kind, Role: "chat", SettingsRevision: m.settings.Revision, Agent: config, Prompt: text, Context: contextText}
	profile := m.store
	connections, store, workspaceID, conversation := m.assistantConnections(), m.history, m.conversationScope(), c.Conversation
	m.captureSourceReads(connections)
	presentation := m.uiTools(connections)
	newer := c.Newer
	ctx, cancel := context.WithCancel(context.Background())
	run := &appchat.ChatRun{Agent: kind, Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent, 8), Starting: true}
	m.run = run
	c.Progress = "Saving question…"
	m.status = ""
	type startResult struct {
		conversation history.Conversation
		turn         history.Turn
		messages     []aimodel.RequestMessage
		page         history.HistoryPage
		key          string
		err          error
	}
	m.enqueuePersistence(&persistenceJob{write: func() any {
		result := startResult{conversation: conversation}
		if result.err = ctx.Err(); result.err != nil {
			return result
		}
		if kind == backend.Plous {
			result.key, result.err = profile.Credential(config)
			if result.err == nil {
				result.err = config.Ready(result.key)
			}
			if result.err != nil {
				return result
			}
		}
		if result.conversation.ID == "" {
			title := strings.Split(text, "\n")[0]
			if len([]rune(title)) > 60 {
				title = string([]rune(title)[:60])
			}
			result.conversation, result.err = store.NewConversation(workspaceID, title)
			if result.err != nil {
				return result
			}
		}
		result.messages, result.err = store.AgentMessages(context.Background(), workspaceID, result.conversation.ID)
		if result.err != nil {
			return result
		}
		if newer {
			result.page, result.err = store.TurnPage(ctx, workspaceID, result.conversation.ID, 0, appchat.ChatCacheTurns, appchat.ChatCacheBytes)
			if result.err != nil {
				return result
			}
		}
		if result.err = ctx.Err(); result.err != nil {
			return result
		}
		result.turn, result.err = store.StartTurn(workspaceID, result.conversation.ID, r)
		return result
	}, apply: func(value any) tea.Cmd {
		result := value.(startResult)
		run.Starting = false
		if result.err != nil {
			cancel()
			if result.conversation.ID != "" {
				c.Conversation = result.conversation
				m.chatIDs[workspaceID] = result.conversation.ID
				m.persist()
			}
			if result.turn.Request.ID != "" {
				c.Err = "Question save could not be confirmed. Restart Plous to recover the saved request. " + result.err.Error()
				if result.turn.Sequence != 0 && !c.Evicted {
					c.Turns = append(c.Turns, result.turn)
				}
			}
			if m.run == run {
				m.run = nil
			}
			c.Progress = ""
			m.status = result.err.Error()
			return nil
		}
		c.Conversation, c.Messages = result.conversation, result.messages
		m.chatIDs[workspaceID] = result.conversation.ID
		if newer {
			c.Turns, c.Older = result.page.Turns, result.page.HasOlder
		}
		turn := result.turn
		c.ActiveTurn = &turn
		if !c.Evicted {
			c.Turns = append(c.Turns, turn)
			c.TrimTranscript()
		}
		c.Newer, c.Scroll, c.Reading, c.ReplySelected = false, 0, false, false
		if currentDraft := m.drafts[workspaceID]; currentDraft != nil && currentDraft.Text == draft && !run.Stopping && !m.quitting {
			*currentDraft = textview.NewField("")
		}
		c.Progress = "Thinking…"
		m.persist()
		if run.Stopping || m.quitting || ctx.Err() != nil {
			run.Stopping = true
			run.Terminal = appchat.ChatEvent{Run: run, Done: true, Err: context.Canceled}
			close(run.Events)
			return appchat.WaitChat(run)
		}
		options := agent.Options{UI: presentation, Agent: kind, Store: profile, History: store, WorkspaceID: workspaceID, ConversationID: result.conversation.ID, TurnID: turn.Request.ID, Config: config, Key: result.key, System: system, ScopeContext: contextText, Prompt: text, Connections: connections}
		go run.Produce(ctx, options)
		return appchat.WaitChat(run)
	}})
	return m.syncPersistence()
}

func (m *model) updateChatEvent(e appchat.ChatEvent) tea.Cmd {
	if m.run != e.Run || e.Run.Finished {
		return nil
	}
	c := e.Run.Chat
	if e.ContextLimit > 0 {
		e.Run.ContextUsed, e.Run.ContextLimit = e.ContextUsed, e.ContextLimit
		return appchat.WaitChat(e.Run)
	}
	if e.Progress {
		if !e.Run.Stopping {
			c.Progress = e.Status
		}
		return appchat.WaitChat(e.Run)
	}
	t := c.ActiveTurn
	if t == nil {
		t = &c.Turns[len(c.Turns)-1]
	}
	if !e.Done {
		t.Response.Text = e.Text
		c.SyncActiveTurn()
		m.boundStreamingWindow(c)
		c.Dirty, c.SaveFailed = true, false
		c.SaveVersion++
		m.persist()
		return appchat.WaitChat(e.Run)
	}
	if e.HasText {
		t.Response.Text = e.Text
	}
	if e.Result.Text != "" && (!e.Run.Stopping || strings.HasPrefix(e.Result.Text, t.Response.Text)) {
		t.Response.Text = e.Result.Text
	}
	response := t.Response
	response.Usage, response.Finish = e.Result.Usage, e.Result.Finish
	response.Status = "completed"
	if e.Err != nil {
		response.Status, response.Error = "error", e.Err.Error()
	}
	if e.Run.Stopping {
		response.Status, response.Error = "cancelled", ""
	}
	e.Run.Finished, e.Run.Response = true, response
	c.SyncActiveTurn()
	c.Dirty = true
	c.SaveVersion++
	c.Progress = "Saving response…"
	e.Run.Cancel()
	m.persist()
	m.queueChatSave(c)
	return m.syncPersistence()
}

func (m *model) stopChat() {
	if m.run == nil || m.run.Stopping || m.run.Finished {
		return
	}
	m.run.Stopping = true
	m.run.Cancel()
	m.run.Chat.Progress = "Stopping…"
	m.persist()
}

// After the event loop exits, join the producer before closing its turn or store.
func (m *model) drainChat() {
	m.quitting = true
	m.stopChat()
	m.drainPersistence()
	for m.run != nil && !m.run.Finished {
		run := m.run
		m.updateChatEvent(appchat.WaitChat(run)().(appchat.ChatEvent))
		m.drainPersistence()
	}
}

func (m *model) chatCommand(command string) {
	if m.externalAgentSelected() {
		if command == "newchat" {
			if m.nativeAgent != nil && !m.nativeAgent.Finished {
				m.status = "Close the native session with :X first."
				return
			}
			m.nativeFresh = true
		}
		m.focusChat(true)
		if command == "openchat" {
			m.status = "Use /resume in " + m.settings.MainAgent.Label() + " for its conversation history."
		}
		return
	}
	if command == "newchat" && m.historyView != "" {
		m.historyView = ""
		m.loadChat()
	}
	if command == "chat" {
		m.chat().History = false
		m.chat().Models = false
		m.focusChat(true)
		return
	}
	c := m.chat()
	c.Models = false
	if m.run != nil && m.run.Chat == c {
		m.status = "Stop this response with :X first."
		return
	}
	if c.Restore != nil && command == "openchat" {
		m.status = "Wait for saved conversation restoration to finish."
		return
	}
	if command == "openchat" {
		choices, err := m.history.Conversations(m.conversationScope())
		if err != nil {
			m.status = err.Error()
			return
		}
		c.Choices = []history.Conversation{{ID: "history-scopes", Title: "Choose history scope…"}}
		for i := len(choices) - 1; i >= 0; i-- {
			c.Choices = append(c.Choices, choices[i])
		}
		c.History = true
		c.Cursor = 0
	} else {
		if command == "newchat" {
			if c.Restore != nil {
				c.Restore.Cancel()
			}
			*c = appchat.ChatState{}
			m.chatIDs[m.conversationScope()] = ""
		}
		c.History = false
	}
	m.focusChat(command != "openchat")
}

func (m *model) focusChat(typing bool) {
	m.closeHelp()
	if typing {
		m.chat().History, m.chat().Models = false, false
	}
	m.rightHidden = false
	m.focus = 2
	m.chatEditing = typing && !m.externalAgentSelected()
	if m.externalAgentSelected() && typing {
		m.nativeRequested = true
	}
	m.status = ""
	m.persist()
}

func (m *model) restoreChatPrompt() {
	if m.draft().Text != "" {
		m.status = "Clear the draft before restoring the last message."
		return
	}
	c := m.chat()
	if len(c.Turns) == 0 {
		m.status = "No previous message in this conversation."
		return
	}
	prompt := c.Turns[len(c.Turns)-1].Request.Prompt
	if c.ActiveTurn != nil {
		prompt = c.ActiveTurn.Request.Prompt
	} else if c.Newer {
		page, err := m.history.TurnPage(context.Background(), c.Conversation.WorkspaceID, c.Conversation.ID, 0, 1, appchat.ChatCacheBytes)
		if err != nil || len(page.Turns) == 0 {
			m.status = "Could not restore the latest saved question."
			return
		}
		prompt = page.Turns[0].Request.Prompt
	}
	*m.draft() = textview.NewField(prompt)
	m.draft().Literal = true
	m.focusChat(true)
	m.status = "Message restored. Enter sends a new request."
}

func (m *model) updateChat(k tea.KeyPressMsg) tea.Cmd {
	if m.externalAgentSelected() {
		return m.nativeAgentKey(k)
	}
	c := m.chat()
	key := k.String()
	if c.Models {
		m.updateModelPicker(key)
		return nil
	}
	if c.History {
		choices := m.filteredConversations()
		c.Cursor = max(0, min(c.Cursor, len(choices)-1))
		switch key {
		case "esc":
			c.History = false
		case "up":
			c.Cursor = max(0, c.Cursor-1)
		case "down":
			c.Cursor = max(0, min(len(choices)-1, c.Cursor+1))
		case "enter":
			if len(choices) > 0 {
				m.chooseHistory(choices[c.Cursor])
			}
		}
		return nil
	}
	switch key {
	case "enter":
		if m.chatEditing {
			text := m.draft().Text
			if m.settings.MainAgent.Effective() == backend.Plous && !m.draft().Literal && (text == "/model" || text == "/m") {
				*m.draft() = textview.NewField("")
				m.openModelPicker()
				return nil
			}
			if text == "//model" || text == "//m" {
				*m.draft() = textview.NewField(text[1:])
				m.draft().Literal = true
			}
			return m.sendChat()
		}
		return m.openChatSources()
	case "[", "]":
		if m.chatEditing {
			m.draft().Key(k, true)
		} else if key == "[" {
			m.selectChatReply(-1)
		} else {
			m.selectChatReply(1)
		}
	case "shift+enter", "ctrl+j":
		if m.chatEditing {
			m.draft().Insert("\n", true)
		}
	case "esc":
		m.chatEditing = false
	case "pgup", "pgdown":
		m.scrollChat(key)
	default:
		if m.chatEditing {
			m.draft().Key(k, true)
		} else {
			m.scrollChat(key)
		}
	}
	return nil
}

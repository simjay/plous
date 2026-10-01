package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/agent"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

type chatState struct {
	conversation   workspace.Conversation
	summary        *workspace.ReasoningEntry
	progress       string
	turns          []workspace.Turn
	dirty          bool
	history        bool
	models         bool
	modelCursor    int
	choices        []workspace.Conversation
	cursor, scroll int
	reading        bool
	viewRows       int
	viewHeight     int
	viewWidth      int
	anchor         chatRow
	err            string
}
type chatRun struct {
	chat     *chatState
	cancel   context.CancelFunc
	events   chan chatEvent
	terminal chatEvent
}
type chatEvent struct {
	run      *chatRun
	text     string
	status   string
	progress bool
	done     bool
	result   ai.Result
	err      error
}

func (m *model) chat() *chatState {
	if c := m.chats[m.active]; c != nil {
		return c
	}
	return &chatState{}
}
func (m *model) loadChat() {
	if m.chats == nil {
		m.chats = map[string]*chatState{}
	}
	if m.chats[m.active] != nil {
		return
	}
	c := &chatState{}
	m.chats[m.active] = c
	if m.store == nil {
		return
	}
	choices, err := m.store.Conversations(m.active)
	if err != nil {
		c.err = err.Error()
		return
	}
	id, selected := m.chatIDs[m.active]
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
func (m *model) openConversation(c *chatState, conversation workspace.Conversation) {
	turns, err := m.store.Turns(conversation.WorkspaceID, conversation.ID)
	if err != nil {
		c.err = err.Error()
		return
	}
	// A previous process cannot still own these requests under the store lock.
	for i := range turns {
		if turns[i].Response.Status == "streaming" {
			turns[i].Response.Status = "interrupted"
			turns[i].Response.Error = "Plous stopped before this response finished. No automatic retry."
			if err := m.store.SaveResponse(conversation.WorkspaceID, conversation.ID, turns[i].Request.ID, turns[i].Response); err != nil {
				c.err = err.Error()
				return
			}
		}
	}
	summary, err := m.store.LatestCompaction(conversation.WorkspaceID, conversation.ID)
	if err != nil {
		c.err = err.Error()
		return
	}
	*c = chatState{conversation: conversation, turns: turns, summary: summary}
	if m.chatIDs == nil {
		m.chatIDs = map[string]string{}
	}
	m.chatIDs[conversation.WorkspaceID] = conversation.ID
	m.persist()
}
func (m *model) flushChats() error {
	for _, c := range m.chats {
		if c.dirty && len(c.turns) > 0 {
			t := c.turns[len(c.turns)-1]
			if err := m.store.SaveResponse(c.conversation.WorkspaceID, c.conversation.ID, t.Request.ID, t.Response); err != nil {
				return err
			}
			c.dirty = false
		}
	}
	return nil
}
func (m *model) contextText() string {
	if m.current() == nil {
		return ""
	}
	// Capture identities and filters only; resource bodies are retrieved through tools.
	value := struct {
		Workspace     string        `json:"workspace"`
		Revision      uint64        `json:"revision"`
		Tool          string        `json:"tool,omitempty"`
		Connection    string        `json:"connection,omitempty"`
		KubeContext   string        `json:"kube_context,omitempty"`
		Namespace     string        `json:"namespace,omitempty"`
		Page          assistantPage `json:"page"`
		ReadNamespace string        `json:"read_namespace,omitempty"`
	}{Workspace: m.current().Name, Revision: m.current().Revision, Tool: m.tool}
	if c := m.currentConnection(); c != nil {
		value.Tool = c.Tool
		value.Connection = c.Name
		value.KubeContext = c.Settings["context"]
		value.Namespace = c.Settings["namespace"]
	}
	value.Page = m.assistantPageContext()
	if scope := m.assistantKubernetesScope(); scope != nil {
		value.ReadNamespace = scope.Namespace
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}
func (m *model) sendChat() tea.Cmd {
	c := m.chat()
	text := strings.TrimSpace(m.draft().text)
	if text == "" {
		return nil
	}
	if m.run != nil {
		m.status = "A response is running. Use :X to stop it."
		return nil
	}
	if c.err != "" {
		m.status = "Chat storage error: " + c.err
		return nil
	}
	key, err := m.store.Credential(m.settings.Agent)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if err := m.settings.Agent.Ready(key); err != nil {
		m.status = err.Error()
		return nil
	}
	if err := m.flushChats(); err != nil {
		m.status = err.Error()
		return nil
	}
	if c.conversation.ID == "" {
		title := strings.Split(text, "\n")[0]
		runes := []rune(title)
		if len(runes) > 60 {
			title = string(runes[:60]) + "…"
		}
		conversation, err := m.store.NewConversation(m.active, title)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		c.conversation = conversation
		m.chatIDs[m.active] = c.conversation.ID
	}
	summary, err := m.store.LatestCompaction(c.conversation.WorkspaceID, c.conversation.ID)
	if err != nil {
		m.status = "Could not load conversation summary: " + err.Error()
		return nil
	}
	c.summary = summary
	messages, contextText := m.chatMessages(text, false)
	if len(text) > 256*1024 {
		m.status = "Message exceeds the local size limit."
		return nil
	}

	request := workspace.Request{Role: "chat", SettingsRevision: m.settings.Revision, Agent: m.settings.Agent, Prompt: text, Context: contextText}
	turn, err := m.store.StartTurn(m.active, c.conversation.ID, request)
	if err != nil {
		m.status = "Could not save request: " + err.Error()
		return nil
	}
	history := append([]workspace.Turn(nil), c.turns...)
	c.turns = append(c.turns, turn)
	c.progress = "Thinking…"
	c.scroll, c.reading = 0, false
	*m.draft() = newEditor("")
	m.chatEditing = true
	m.persist()
	m.status = ""
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	run := &chatRun{chat: c, cancel: cancel, events: make(chan chatEvent, 8)}
	m.run = run
	kube := m.assistantKubernetesScope()
	workspaceID, conversationID := m.active, c.conversation.ID
	// Only the UI goroutine owns chat state. The worker sends immutable snapshots.
	go func() {
		defer cancel()
		defer close(run.events)
		result, err := agent.Run(ctx, agent.Options{Store: m.store, WorkspaceID: workspaceID, ConversationID: conversationID, TurnID: turn.Request.ID, Config: request.Agent, Key: key, System: messages[0].Text, Prompt: text, History: history, Kubernetes: kube, Progress: func(status string) {
			select {
			case run.events <- chatEvent{run: run, progress: true, status: status}:
			case <-ctx.Done():
			}
		}, Emit: func(text string) error {
			select {
			case run.events <- chatEvent{run: run, text: text}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}})
		// Closing events publishes the terminal result even when the output buffer is full.
		run.terminal = chatEvent{run: run, done: true, result: result, err: err}
	}()
	return waitChat(run)
}
func waitChat(run *chatRun) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-run.events
		if !ok {
			if run.terminal.done {
				return run.terminal
			}
			return chatEvent{run: run, done: true, err: fmt.Errorf("AI stream ended unexpectedly")}
		}
		return event
	}
}
func (m *model) updateChatEvent(e chatEvent) tea.Cmd {
	if m.run != e.run {
		return nil
	}
	c := e.run.chat
	if e.progress {
		c.progress = e.status
		return waitChat(e.run)
	}
	t := &c.turns[len(c.turns)-1]
	if !e.done {
		t.Response.Text = e.text
		c.dirty = true
		m.persist()
		return waitChat(e.run)
	}
	if e.result.Text != "" {
		t.Response.Text = e.result.Text
	}
	t.Response.Usage = e.result.Usage
	t.Response.Finish = e.result.Finish
	t.Response.Status = "completed"
	if e.err != nil {
		t.Response.Status = "error"
		t.Response.Error = e.err.Error()
	}
	c.dirty = true
	c.progress = ""
	if summary, err := m.store.LatestCompaction(c.conversation.WorkspaceID, c.conversation.ID); err == nil {
		c.summary = summary
	}
	e.run.cancel()
	m.run = nil
	m.persist()
	if err := m.flushChats(); err != nil {
		m.status = "Could not save response: " + err.Error()
	}
	return nil
}
func (m *model) stopChat() {
	if m.run == nil {
		return
	}
	c := m.run.chat
	m.run.cancel()
	m.run = nil
	t := &c.turns[len(c.turns)-1]
	t.Response.Status = "cancelled"
	c.progress = ""
	if summary, err := m.store.LatestCompaction(c.conversation.WorkspaceID, c.conversation.ID); err == nil {
		c.summary = summary
	}
	c.dirty = true
	m.persist()
}
func (m *model) chatCommand(command string) {
	if command == "chat" {
		m.chat().history = false
		m.chat().models = false
		m.focusChat(true)
		return
	}
	c := m.chat()
	c.models = false
	if m.run != nil && m.run.chat == c {
		m.status = "Stop this response with :X first."
		return
	}
	if err := m.flushChats(); err != nil {
		m.status = err.Error()
		return
	}
	if command == "openchat" {
		choices, err := m.store.Conversations(m.active)
		if err != nil {
			m.status = err.Error()
			return
		}
		c.choices = nil
		for i := len(choices) - 1; i >= 0; i-- {
			c.choices = append(c.choices, choices[i])
		}
		c.history = true
		c.cursor = 0
	} else {
		if command == "newchat" {
			*c = chatState{}
			m.chatIDs[m.active] = ""
		}
		c.history = false
	}
	m.focusChat(command != "openchat")
}

func (m *model) focusChat(typing bool) {
	m.closeHelp()
	if typing {
		m.chat().history, m.chat().models = false, false
	}
	m.rightHidden = false
	m.focus = 2
	m.chatEditing = typing
	m.status = ""
	m.persist()
}

func (m *model) restoreChatPrompt() {
	if m.draft().text != "" {
		m.status = "Clear the draft before restoring the last message."
		return
	}
	c := m.chat()
	if len(c.turns) == 0 {
		m.status = "No previous message in this conversation."
		return
	}
	*m.draft() = newEditor(c.turns[len(c.turns)-1].Request.Prompt)
	m.draft().literal = true
	m.focusChat(true)
	m.status = "Message restored. Enter sends a new request."
}

func (m *model) updateChat(k tea.KeyPressMsg) tea.Cmd {
	c := m.chat()
	key := k.String()
	if c.models {
		m.updateModelPicker(key)
		return nil
	}
	if c.history {
		choices := m.filteredConversations()
		c.cursor = max(0, min(c.cursor, len(choices)-1))
		switch key {
		case "esc":
			c.history = false
		case "up":
			c.cursor = max(0, c.cursor-1)
		case "down":
			c.cursor = max(0, min(len(choices)-1, c.cursor+1))
		case "enter":
			if len(choices) > 0 {
				if err := m.flushChats(); err != nil {
					m.status = err.Error()
					return nil
				}
				m.openConversation(c, choices[c.cursor])
			}
		}
		return nil
	}
	switch key {
	case "enter":
		if m.chatEditing {
			text := m.draft().text
			if !m.draft().literal && (text == "/model" || text == "/m") {
				*m.draft() = newEditor("")
				m.openModelPicker()
				return nil
			}
			if text == "//model" || text == "//m" {
				*m.draft() = newEditor(text[1:])
				m.draft().literal = true
			}
			return m.sendChat()
		}
	case "shift+enter", "ctrl+j":
		if m.chatEditing {
			m.draft().insert("\n", true)
		}
	case "esc":
		m.chatEditing = false
	case "pgup", "pgdown":
		m.scrollChat(key)
	default:
		if m.chatEditing {
			m.draft().key(k, true)
		} else {
			m.scrollChat(key)
		}
	}
	return nil
}

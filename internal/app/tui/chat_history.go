package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai/history"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) replaceTranscript(page history.HistoryPage) {
	c := m.chat()
	c.Turns, c.Older, c.Newer = page.Turns, page.HasOlder, page.HasNewer
	c.SyncActiveTurn()
	c.Oversized = len(c.Turns) == 1 && appchat.TurnTextBytes(c.Turns[0]) > appchat.ChatCacheBytes
	c.ViewWidth = 0
	c.Evidence = nil
}

func (m *model) chatPage(direction int) bool {
	c := m.chat()
	if m.history == nil || c.Conversation.ID == "" || len(c.Turns) == 0 {
		return false
	}
	var page history.HistoryPage
	var err error
	if direction < 0 {
		page, err = m.history.TurnPage(context.Background(), c.Conversation.WorkspaceID, c.Conversation.ID, c.Turns[0].Sequence, appchat.ChatCacheTurns, appchat.ChatCacheBytes)
	} else {
		page, err = m.history.TurnPageAfter(context.Background(), c.Conversation.WorkspaceID, c.Conversation.ID, c.Turns[len(c.Turns)-1].Sequence, appchat.ChatCacheTurns, appchat.ChatCacheBytes)
	}
	if err != nil {
		m.status = "Could not load saved messages: " + err.Error()
		return false
	}
	if len(page.Turns) == 0 {
		return false
	}
	m.replaceTranscript(page)
	c.Reading, c.ReplySelected = true, true
	c.Reply = 0
	if direction < 0 {
		c.Reply = len(c.Turns) - 1
	}
	c.ReplyJump = true
	return true
}

func (m *model) chatEndpoint(oldest bool) bool {
	c := m.chat()
	if m.history == nil || c.Conversation.ID == "" {
		return false
	}
	var page history.HistoryPage
	var err error
	if oldest {
		page, err = m.history.TurnPageAfter(context.Background(), c.Conversation.WorkspaceID, c.Conversation.ID, 0, appchat.ChatCacheTurns, appchat.ChatCacheBytes)
	} else {
		page, err = m.history.TurnPage(context.Background(), c.Conversation.WorkspaceID, c.Conversation.ID, 0, appchat.ChatCacheTurns, appchat.ChatCacheBytes)
	}
	if err != nil {
		m.status = "Could not load saved messages: " + err.Error()
		return false
	}
	m.replaceTranscript(page)
	c.Scroll, c.ReplyJump = 0, false
	c.Reading, c.ReplySelected = oldest, oldest
	c.Reply = 0
	return true
}

func (m *model) evictInactiveChats() {
	for id, c := range m.chats {
		if id == m.conversationScope() || c.Evicted {
			continue
		}
		if c.Restore != nil {
			c.Restore.Cancel()
			c.Restore = nil
		}
		if c.Dirty && c.ActiveTurn == nil && (m.run == nil || m.run.Chat != c) {
			continue
		}
		if len(c.Turns) > 0 {
			c.ResumeSequence = c.Turns[0].Sequence
			if m.run != nil && m.run.Chat == c && c.ActiveTurn == nil {
				turn := c.Turns[len(c.Turns)-1]
				c.ActiveTurn = &turn
			}
		}
		c.Turns, c.Messages, c.Evidence, c.Rendered = nil, nil, nil, nil
		c.ReplyLines, c.Questions, c.Choices = nil, nil, nil
		c.Anchor.Text = ""
		c.Evicted = true
	}
}

func (m *model) rehydrateChat(c *appchat.ChatState) {
	if m.history == nil || c.Conversation.ID == "" {
		c.Evicted = false
		return
	}
	m.beginChatRestore(c)
}

func (m *model) boundStreamingWindow(c *appchat.ChatState) {
	bytes := 0
	for _, t := range c.Turns {
		bytes += appchat.TurnTextBytes(t)
	}
	if bytes <= appchat.ChatCacheBytes || len(c.Turns) <= 1 {
		return
	}
	if !c.Reading {
		c.TrimTranscript()
		return
	}
	selected := c.SelectedReply()
	turn := c.Turns[selected]
	if selected < len(c.ReplyLines) {
		start := c.ReplyLines[selected] - strings.Count(appchat.ChatMessage(turn.Request.Prompt, true), "\n") - 3
		c.Anchor.Line = max(0, c.Anchor.Line-start)
	}
	c.Older = c.Older || selected > 0
	c.Newer = c.Newer || selected < len(c.Turns)-1
	c.Turns = []history.Turn{turn}
	c.Reply, c.ReplySelected, c.ReplyJump = 0, true, false
	c.ViewWidth = -1
}

func (m *model) openChatReply() {
	if s := m.chatSources; m.chatSourcesVisible() {
		if s.Loading {
			m.status = "Wait for the saved source to finish loading."
			return
		}
		if s.Reply == nil {
			s.Reply = appchat.NewReplyViewer(s.Evidence.Turn)
		}
		s.Document, s.ShowReply = nil, true
		return
	}
	c := m.chat()
	if c.Restore != nil || len(c.Turns) == 0 || c.History || c.Models {
		m.status = "Select a reply to read."
		return
	}
	turn := c.Turns[c.SelectedReply()]
	unsaved := turn.Response.Status == "streaming" && m.run != nil && m.run.Chat == c && m.run.Finished && c.ActiveTurn != nil && c.ActiveTurn.Request.ID == turn.Request.ID
	if turn.Response.Status == "streaming" && !unsaved {
		m.status = "Read reply is available when this response finishes."
		return
	}
	m.closeHelp()
	m.restoreView()
	m.focusChat(false)
	// Sources loading is scheduled by Update; the reader can open immediately.
	e := c.Evidence
	if unsaved {
		e = &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn, Loaded: true, Err: errors.New("this response has not been saved; its displayed text can be copied")}
	} else if e == nil || e.Turn.Request.ID != turn.Request.ID {
		e = &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn}
	}
	m.sourcesTravel = overlayTravel{}
	m.chatSources = &appchat.ChatSources{Chat: c, Evidence: e, Reply: appchat.NewReplyViewer(turn), ShowReply: true, List: len(e.Records) > 1, Follow: !c.Reading, Tail: c.Turns[len(c.Turns)-1].Response.Text}
	if unsaved {
		m.chatSources.Reply.Name = "Unsaved response to: " + ui.OneLine(turn.Request.Prompt)
	}
	c.Reading = true
	m.status = ""
}

func (m *model) selectChatReply(step int) {
	c := m.chat()
	if len(c.Turns) == 0 {
		return
	}
	if step < 0 && c.SelectedReply() == 0 && c.Older || step > 0 && c.SelectedReply() == len(c.Turns)-1 && c.Newer {
		m.chatPage(step)
		return
	}
	c.Reply = max(0, min(c.SelectedReply()+step, len(c.Turns)-1))
	c.ReplySelected, c.ReplyJump, c.Reading = true, true, true
	c.ViewWidth = 0
	m.status = ""
}

func (m *model) syncChatEvidence() tea.Cmd {
	c := m.chat()
	if m.focus != 2 || c.History || c.Models || len(c.Turns) == 0 || m.history == nil {
		return nil
	}
	turn := c.Turns[c.SelectedReply()]
	if turn.Request.ID == "" || turn.Response.Status == "streaming" {
		return nil
	}
	if e := c.Evidence; e != nil && e.Conversation.ID == c.Conversation.ID && e.Turn.Request.ID == turn.Request.ID && e.Turn.Response.Status == turn.Response.Status {
		return nil
	}
	e := &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn}
	c.Evidence = e
	if s := m.chatSources; s != nil && s.Chat == c && s.Evidence.Turn.Request.ID == turn.Request.ID {
		s.Evidence = e
	}
	store := m.history
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		records, err := store.TurnEvidence(ctx, e.Conversation.WorkspaceID, e.Conversation.ID, e.Turn.Request.ID)
		return appchat.ChatEvidenceResult{Evidence: e, Records: records, Err: err}
	}
}

func (m *model) chatReplyHint(index int) string {
	c := m.chat()
	if index != c.SelectedReply() || c.Turns[index].Response.Status == "streaming" {
		return "\n"
	}
	label, action, value := "Sources", "View sources", ""
	if e := c.Evidence; e != nil && e.Turn.Request.ID == c.Turns[index].Request.ID && e.Loaded {
		if e.Err != nil {
			label, action = "Sources unavailable", "View source details"
		} else if len(e.Records) == 0 {
			label, action = "Reply details", "View reply details"
		} else {
			value = fmt.Sprintf("· %d", len(e.Records))
		}
	}
	if m.focus == 2 && !m.chatEditing && m.searches[2] == nil {
		return textview.ActionChip(textview.Action{Mouse: "sources/" + c.Turns[index].Request.ID, Key: "Enter", Label: action, Value: value}, max(1, c.ContentWidth), ui.WorkspaceTheme()) + "\n" +
			textview.ActionChip(textview.Action{Mouse: "readreply/" + c.Turns[index].Request.ID, Key: ":A", Label: "Read reply"}, max(1, c.ContentWidth), ui.WorkspaceTheme())
	}
	return ui.MouseTarget(ui.Muted.Render(strings.TrimSpace(label+" "+value)), "sources/"+c.Turns[index].Request.ID) + "\n" +
		ui.MouseTarget(ui.Muted.Render("Read reply"), "readreply/"+c.Turns[index].Request.ID)
}

func (m *model) openChatSources() tea.Cmd {
	c := m.chat()
	if len(c.Turns) == 0 {
		return nil
	}
	if c.Turns[c.SelectedReply()].Response.Status == "streaming" {
		m.status = "Sources will be available when this reply finishes."
		return nil
	}
	cmd := m.syncChatEvidence()
	if c.Evidence == nil || c.Evidence.Turn.Request.ID != c.Turns[c.SelectedReply()].Request.ID {
		m.status = "No saved source records are available for this reply."
		return cmd
	}
	m.sourcesTravel = overlayTravel{}
	m.chatSources = &appchat.ChatSources{Chat: c, Evidence: c.Evidence, List: len(c.Evidence.Records) > 1, Follow: !c.Reading, Tail: c.Turns[len(c.Turns)-1].Response.Text}
	c.Reading = true
	m.status = ""
	return cmd
}

func (m *model) closeChatSources() {
	if s := m.chatSources; s != nil {
		c := s.Chat
		if s.Follow && len(c.Turns) > 0 && c.Turns[len(c.Turns)-1].Response.Text == s.Tail {
			c.Reading = false
		}
		if m.chatSourcesVisible() {
			m.focus = 2
		}
		m.chatSources = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) chatSourcesKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.chatSources
	key := msg.String()
	searching := s.Document != nil && s.Document.Searching || s.ShowReply && s.Reply.Reader.Searching
	if !searching {
		if key == ":" {
			m.beginCommand()
			return nil
		}
		if m.updateShortcut(msg) {
			return nil
		}
	}
	m.shortcutPrefix, m.status = "", ""
	if s.ShowReply {
		if !searching && key == "enter" {
			s.ShowReply = false
			return nil
		}
		used, copied := s.Reply.Key(key, msg.Text)
		if !used && key == "esc" {
			m.closeChatSources()
		}
		return m.copyWorkspaceText(copied)
	}
	if s.Document != nil {
		event := s.Document.Key(key, msg.Text)
		if event.Action == "back" {
			s.Document = nil
		}
		return m.copyWorkspaceText(event.Copy)
	}
	if key == "esc" {
		if !s.List && len(s.Evidence.Records) > 1 && !s.Loading {
			s.List = true
		} else if s.Reply != nil {
			s.ShowReply = true
		} else {
			m.closeChatSources()
		}
		return nil
	}
	if !s.Evidence.Loaded || s.Loading {
		return nil
	}
	if s.List {
		switch key {
		case "up", "k":
			s.Index = max(0, s.Index-1)
		case "down", "j":
			s.Index = min(len(s.Evidence.Records)-1, s.Index+1)
		case "enter":
			s.List = false
			s.Scroll = 0
		}
		return nil
	}
	if key == "enter" && len(s.Evidence.Records) > 0 {
		return m.openChatSourceDocument()
	}
	switch key {
	case "up", "k":
		s.Scroll--
	case "down", "j":
		s.Scroll++
	case "pgup":
		s.Scroll -= max(1, s.Page-1)
	case "pgdown":
		s.Scroll += max(1, s.Page-1)
	case "home":
		s.Scroll = 0
	case "end":
		s.Scroll = s.Last
	}
	s.Scroll = max(0, min(s.Last, s.Scroll))
	return nil
}

func (m *model) openChatSourceDocument() tea.Cmd {
	return m.chatSources.OpenDocument()
}

func (m *model) updateChatSourceDocument(result appchat.ChatSourceDocumentResult) tea.Cmd {
	if m.chatSources == result.Sources {
		m.chatSources.ApplyDocument(result)
	}
	return nil
}

func (m *model) chatSourcesBody(width, height int) string { return m.chatSources.Body(width, height) }

func (m *model) renderChatSourcesOverlay() string {
	base := *m
	base.chatSources = nil
	width, height := min(96, m.width-4), min(18, m.height-4)
	if m.chatSources.Document != nil || m.chatSources.ShowReply {
		width, height = min(110, m.width-4), min(38, m.height-4)
	} else if m.chatSources.List {
		height = min(m.height-4, min(24, max(10, len(m.chatSources.Evidence.Records)+6)))
	}
	available := height - 3
	if m.command != nil {
		available -= 3
	}
	body := textview.Fit(m.chatSourcesBody(width-4, available), width-4, available)
	if m.command != nil {
		body += "\n" + m.commandBox(width-4)
	}
	title := "Sources"
	if e := m.chatSources.Evidence; e.Loaded && len(e.Records) == 0 && e.Err == nil {
		title = "Reply details"
	}
	if m.chatSources.ShowReply {
		title = "Read reply"
	}
	screen := strings.Split(ui.CenteredOverlay(base.render(), textview.MouseScope(helpBox(title, body, width, height, true), "sources")), "\n")
	screen[len(screen)-1] = textview.MouseScope(m.footer(), "sources")
	return strings.Join(screen, "\n")
}

func (m *model) chatSourcesHint() string {
	s := m.chatSources
	if s.ShowReply {
		if s.Reply.Reader.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · v Select · y Copy · R Source · Enter Sources · Esc Back · hh Help"
	}
	if s.Document != nil {
		if s.Document.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · y Copy · Esc " + documentBackHint(s.Document, "Source details") + " · hh Help"
	}
	if s.List {
		return "↑↓ Choose source · Enter Open · Esc Reply · hh Help"
	}
	if len(s.Evidence.Records) == 0 || !s.Evidence.Loaded || s.Evidence.Err != nil {
		return "↑↓ Scroll · Esc Reply · hh Help"
	}
	return "Enter View captured result · ↑↓ Scroll · Esc Back · hh Help"
}

const historyScopeChoice = "scope:"

func (m *model) historyScopeChoices() {
	scopes, err := m.store.Scopes()
	if err != nil {
		m.status = err.Error()
		return
	}
	c := m.chat()
	c.Choices = []history.Conversation{{ID: historyScopeChoice, Title: "General chat"}}
	for _, scope := range scopes {
		label := scope.Connection.Name
		if scope.Kind == "current_context" {
			label = "Current Workspace / Monitor / " + label
		} else if scope.Kind == "current_directory" {
			label = "Archived Code / " + scope.Connection.Settings["path"]
		} else {
			for _, w := range m.workspaces {
				if w.ID == scope.WorkspaceID {
					label = w.Name + " / " + label
					break
				}
			}
		}
		c.Choices = append(c.Choices, history.Conversation{ID: historyScopeChoice + scope.ID, WorkspaceID: scope.ID, Title: label})
	}
	c.History, c.Cursor = true, 0
	m.searches[2] = nil
}

func (m *model) chooseHistory(choice history.Conversation) {
	if choice.ID == "history-scopes" {
		m.historyScopeChoices()
		return
	}
	if strings.HasPrefix(choice.ID, historyScopeChoice) {
		if choice.WorkspaceID != m.conversationScope() {
			m.historyViewName = choice.Title
			m.historyView = choice.WorkspaceID
			if m.historyView == "" {
				m.historyView = "@general"
			}
			m.loadChat()
		}
		choices, err := m.history.Conversations(choice.WorkspaceID)
		if err != nil {
			m.status = err.Error()
			return
		}
		c := m.chat()
		c.Choices = []history.Conversation{{ID: "history-scopes", Title: "Choose history scope…"}}
		for i := len(choices) - 1; i >= 0; i-- {
			c.Choices = append(c.Choices, choices[i])
		}
		c.Cursor = 0
		m.searches[2] = nil
		return
	}
	if choice.WorkspaceID != m.conversationScope() {
		m.historyViewName = m.historyScopeName(choice.WorkspaceID)
		m.historyView = choice.WorkspaceID
		if m.historyView == "" {
			m.historyView = "@general"
		}
		m.loadChat()
	}
	m.searches[2] = nil
	m.openConversation(m.chat(), choice)
}

func (m *model) historyScopeName(id string) string {
	if id == "" {
		return "General chat"
	}
	scopes, _ := m.store.Scopes()
	for _, scope := range scopes {
		if scope.ID == id {
			return scope.Connection.Name
		}
	}
	return "Saved connection"
}

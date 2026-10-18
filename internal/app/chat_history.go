package app

import (
	"context"
	"github.com/simjay/plous/internal/ai/history"
	appchat "github.com/simjay/plous/internal/app/chat"

	"strings"
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

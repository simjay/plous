package app

import (
	tea "charm.land/bubbletea/v2"
	appchat "github.com/simjay/plous/internal/app/chat"
)

func (m *model) beginChatRestore(c *appchat.ChatState)              { c.BeginRestore() }
func (m *model) syncChatRestore() tea.Cmd                           { return m.chatRestore.Start(m.chat(), m.history) }
func (m *model) updateChatRestore(result appchat.ChatRestoreResult) { m.chatRestore.Apply(result) }
func (m *model) drainChatRestores()                                 { m.chatRestore.Stop(m.chats) }

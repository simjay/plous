package app

import (
	"strings"

	"github.com/simjay/plous/internal/ai/history"
)

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
			label = "Current Workspace / Code / " + scope.Connection.Settings["path"]
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

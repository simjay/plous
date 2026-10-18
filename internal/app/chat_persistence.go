package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	appchat "github.com/simjay/plous/internal/app/chat"
)

func (m *model) queueChatSaves() {
	for _, c := range m.chats {
		if c.Dirty && !c.SaveFailed {
			m.queueChatSave(c)
		}
	}
}

func (m *model) queueChatSave(c *appchat.ChatState) {
	if !c.Dirty || c.ActiveTurn == nil && len(c.Turns) == 0 {
		return
	}
	t := c.ActiveTurn
	if t == nil {
		t = &c.Turns[len(c.Turns)-1]
	}
	turn, conversation, version := *t, c.Conversation, c.SaveVersion
	run := m.run
	closing := run != nil && run.Chat == c && run.Finished
	if closing {
		turn.Response = run.Response
	}
	store := m.history
	m.enqueuePersistence(&persistenceJob{key: "response/" + conversation.ID, write: func() any {
		result := appchat.ChatSaveResult{Err: store.SaveResponse(conversation.WorkspaceID, conversation.ID, turn.Request.ID, turn.Response)}
		if closing {
			outcome, err := store.TurnOutcome(conversation.WorkspaceID, conversation.ID, turn.Request.ID)
			result.Closed, result.Status = outcome.Closed, outcome.Status
			if result.Err == nil && err == nil && !outcome.Closed {
				result.Err = errors.New("response closure was not acknowledged")
			}
			if result.Err == nil && err != nil {
				result.Err = err
			}
			if result.Closed {
				result.Messages, result.ContextErr = store.AgentMessages(context.Background(), conversation.WorkspaceID, conversation.ID)
			}
		}
		return result
	}, apply: func(value any) tea.Cmd {
		result := value.(appchat.ChatSaveResult)
		if c.Conversation.ID != conversation.ID {
			return nil
		}
		if result.Err != nil && !result.Closed {
			m.persistenceErr = result.Err
			c.SaveFailed = c.SaveVersion == version
			m.persist()
			c.Err = result.Err.Error()
			m.status = "Could not save response: " + result.Err.Error()
			if closing {
				c.Progress = "Response not saved. :A reads and copies the unsaved text."
			}
			return nil
		}
		if c.SaveVersion == version {
			c.Dirty = false
		}
		c.Err, c.SaveFailed = "", false
		if !closing {
			return nil
		}
		if c.ActiveTurn != nil && c.ActiveTurn.Request.ID == turn.Request.ID {
			c.ActiveTurn.Response = turn.Response
			c.ActiveTurn.Response.Status = result.Status
			c.SyncActiveTurn()
		}
		c.Progress, c.ActiveTurn = "", nil
		if c.Restore != nil {
			c.Restore.Cancel()
			c.Restore = nil
			if !c.Evicted {
				m.beginChatRestore(c)
			}
		}
		if m.run == run {
			m.run = nil
		}
		if !c.Evicted {
			c.Messages = result.Messages
		}
		if result.ContextErr != nil {
			c.Err = result.ContextErr.Error()
			m.status = "Reply saved. Restart Plous to restore chat context: " + result.ContextErr.Error()
		} else if result.Err != nil {
			m.status = "Reply saved. " + result.Err.Error()
		}
		m.evictInactiveChats()
		m.persist()
		return nil
	}})
}

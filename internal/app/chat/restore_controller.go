package chat

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/simjay/plous/internal/ai/history"
)

func (c *ChatState) BeginRestore() {
	if c.Restore != nil {
		c.Restore.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.Restore = &ChatRestore{Ctx: ctx, Cancel: cancel, Done: make(chan struct{})}
	c.Evicted, c.Err = false, ""
}

func (rctl *RestoreController) Start(c *ChatState, store *history.Store) tea.Cmd {
	r := c.Restore
	if r == nil || r.Started || store == nil {
		return nil
	}
	if worker := rctl.Worker; worker != nil {
		select {
		case <-worker.Done:
		default:
			return nil
		}
	}
	rctl.Worker = r
	r.Started = true
	conversation := c.Conversation
	reading, sequence, showHistory := c.Reading, c.ResumeSequence, c.History
	go func() {
		result := ChatRestoreResult{Chat: c, Restore: r}
		defer func() { r.Output = result; close(r.Done) }()
		// Catch-up is asynchronous; a derived-index failure must not prevent restoration.
		_, _ = store.SearchIndex(conversation.WorkspaceID, conversation.ID, history.SearchOptions{})
		if reading && sequence > 0 {
			result.Page, result.Err = store.TurnPageAfter(r.Ctx, conversation.WorkspaceID, conversation.ID, sequence-1, ChatCacheTurns, ChatCacheBytes)
		} else {
			result.Page, result.Err = store.TurnPage(r.Ctx, conversation.WorkspaceID, conversation.ID, 0, ChatCacheTurns, ChatCacheBytes)
		}
		if result.Err != nil {
			return
		}
		if result.Err = r.Ctx.Err(); result.Err != nil {
			return
		}
		result.Messages, result.Err = store.AgentMessages(r.Ctx, conversation.WorkspaceID, conversation.ID)
		if result.Err != nil {
			return
		}
		if showHistory {
			choices, err := store.Conversations(conversation.WorkspaceID)
			result.Err = err
			for i := len(choices) - 1; i >= 0; i-- {
				result.Choices = append(result.Choices, choices[i])
			}
		}
	}()
	return func() tea.Msg { <-r.Done; return r.Output }
}

func (rctl *RestoreController) Apply(result ChatRestoreResult) {
	c, r := result.Chat, result.Restore
	if rctl.Worker == r {
		rctl.Worker = nil
	}
	if c.Restore != r || r.Ctx.Err() != nil {
		return
	}
	r.Cancel()
	c.Restore = nil
	if result.Err != nil {
		c.Err = result.Err.Error()
		return
	}
	c.Turns, c.Older, c.Newer = result.Page.Turns, result.Page.HasOlder, result.Page.HasNewer
	c.Messages = result.Messages
	if c.History {
		c.Choices = result.Choices
	}
	c.SyncActiveTurn()
	c.TrimTranscript()
}

func (rctl *RestoreController) Stop(chats map[string]*ChatState) {
	if r := rctl.Worker; r != nil {
		r.Cancel()
		<-r.Done
		rctl.Worker = nil
	}
	for _, c := range chats {
		if r := c.Restore; r != nil {
			r.Cancel()
			c.Restore = nil
		}
	}
}

// RestoreController serializes history reads and owns their cancellation.
type RestoreController struct{ Worker *ChatRestore }

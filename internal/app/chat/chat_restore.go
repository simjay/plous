package chat

import (
	"context"
	"github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

type ChatRestore struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	Started bool
	Done    chan struct{}
	Output  ChatRestoreResult
}

type ChatRestoreResult struct {
	Chat     *ChatState
	Restore  *ChatRestore
	Page     history.HistoryPage
	Messages []aimodel.RequestMessage
	Choices  []history.Conversation
	Err      error
}

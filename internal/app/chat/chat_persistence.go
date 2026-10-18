package chat

import (
	aimodel "github.com/simjay/plous/internal/ai/model"
)

type ChatSaveResult struct {
	Err        error
	Closed     bool
	Status     string
	Messages   []aimodel.RequestMessage
	ContextErr error
}

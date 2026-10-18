package chat

import (
	"github.com/simjay/plous/internal/ai/history"
)

const (
	ChatCacheTurns = 48
	ChatCacheBytes = 4 << 20
)

func TurnTextBytes(t history.Turn) int {
	return history.TurnBytes(t)
}

func (c *ChatState) TrimTranscript() {
	bytes := 0
	first := len(c.Turns)
	for first > 0 {
		size := TurnTextBytes(c.Turns[first-1])
		if first < len(c.Turns) && (bytes+size > ChatCacheBytes || len(c.Turns)-first >= ChatCacheTurns) {
			break
		}
		bytes += size
		first--
	}
	if first > 0 {
		c.Turns = append([]history.Turn(nil), c.Turns[first:]...)
		c.Older = true
	}
	c.Oversized = bytes > ChatCacheBytes
}

func (c *ChatState) SyncActiveTurn() {
	if c.ActiveTurn == nil {
		return
	}
	for i := range c.Turns {
		if c.Turns[i].Request.ID == c.ActiveTurn.Request.ID {
			c.Turns[i] = *c.ActiveTurn
			return
		}
	}
	c.Newer = true
}

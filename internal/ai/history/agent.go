package history

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
)

// RecentMessages reads completed exchanges before the active turn. Current input,
// execution metadata and incomplete replies do not consume the context allowance.
func (s *Store) RecentMessages(ctx context.Context, w, id, turn string, limit, maxBytes int) ([]agent.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 100 || maxBytes < 1 || maxBytes > MaxRecordBytes {
		return nil, errors.New("invalid recent conversation limits")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return nil, err
	}
	if h.meta.Active != turn || h.active == nil {
		return nil, errors.New("recent context does not belong to active turn")
	}
	turns, err := h.orderedTurns(h.active.Sequence, true, limit)
	if err != nil {
		return nil, err
	}
	var messages []agent.Message
	for _, location := range turns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if location.Status != "completed" {
			continue
		}
		t, err := h.loadTurn(location.ID)
		if err != nil {
			return nil, err
		}
		size := len(t.Request.Prompt) + len(t.Response.Text)
		if size > maxBytes {
			continue
		}
		maxBytes -= size
		messages = append([]agent.Message{{Role: "user", Text: t.Request.Prompt}, {Role: "assistant", Text: t.Response.Text}}, messages...)
	}
	return messages, nil
}

type CallRecord struct {
	MessageID string            `json:"message_id"`
	Status    string            `json:"status"`
	ID        string            `json:"id"`
	Time      time.Time         `json:"time"`
	Kind      string            `json:"kind"`
	CallID    string            `json:"call_id"`
	Component string            `json:"component"`
	Usage     aimodel.CallUsage `json:"usage"`
	ElapsedMS int64             `json:"elapsed_ms"`
	Failed    bool              `json:"failed"`
}

func (s *Store) SaveUsage(w, id, turn string, record CallRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closing {
		return ErrClosed
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return err
	}
	if h.meta.Active != turn {
		return errors.New("usage does not belong to active turn")
	}
	record.ID, record.Time, record.Kind = chronologicalID(), time.Now().UTC(), "usage"
	record.MessageID = turn
	record.Status = "completed"
	if record.Failed {
		record.Status = "error"
	}
	_, err = h.append("execution", record, turn, 0)
	return err
}

func (h *historyState) completionBoundaries() historyOffsets {
	b := historyOffsets{"reasoning": h.meta.Offsets["reasoning"]}
	b["execution"] = h.meta.Offsets["execution"]
	return b
}
func (s *Store) SaveObservation(w, id, turn, tool string, args json.RawMessage, result observation.Result) (ReasoningEntry, error) {
	if err := result.Validate(); err != nil {
		return ReasoningEntry{}, err
	}
	return s.AppendReasoning(w, id, ReasoningEntry{MessageID: turn, Kind: "tool", Tool: tool, Input: args, Status: result.Outcome, Projection: &result})
}

// SaveMessages checkpoints the native provider conversation outside the searchable corpus.
func (s *Store) SaveMessages(w, id, turn string, messages []aimodel.RequestMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closing {
		return ErrClosed
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return err
	}
	if h.meta.Active != turn {
		return errors.New("checkpoint does not belong to active turn")
	}
	e := ReasoningEntry{ID: chronologicalID(), MessageID: turn, TurnID: turn, Kind: "messages", Status: "completed", Messages: messages}
	e.Time = h.active.Request.Time
	_, err = h.append("execution", e, turn, 0)
	return err
}

// AgentMessages restores the latest completed native exchange checkpoint.
func (s *Store) AgentMessages(ctx context.Context, w, id string) ([]aimodel.RequestMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return nil, err
	}
	return h.agentMessages(ctx)
}
func (h *historyState) agentMessages(ctx context.Context) ([]aimodel.RequestMessage, error) {
	anchor := uint64(0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		turns, err := h.orderedTurns(anchor, true, 100)
		if err != nil {
			return nil, err
		}
		for _, t := range turns {
			if t.Status != "completed" {
				continue
			}
			var latest []aimodel.RequestMessage
			after := uint64(0)
			for after < h.meta.Sequence {
				rows, next, err := h.turnRecords(ctx, t.ID, after, h.meta.Sequence, 100, MaxRecordBytes)
				if err != nil {
					return nil, err
				}
				for _, row := range rows {
					if row.Stream != "execution" {
						continue
					}
					var e ReasoningEntry
					if err := json.Unmarshal(row.Data, &e); err != nil {
						return nil, err
					}
					if e.Kind == "messages" {
						latest = e.Messages
					}
				}
				if next <= after {
					break
				}
				after = next
			}
			if latest != nil {
				return latest, nil
			}
		}
		if len(turns) < 100 {
			return nil, nil
		}
		anchor = turns[len(turns)-1].Sequence
	}
}

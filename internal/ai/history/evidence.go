package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TurnEvidence reads the observations for one reply using its saved turn links.
func (s *Store) TurnEvidence(ctx context.Context, w, conversation, turn string) ([]ReasoningEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !idPattern.MatchString(turn) {
		return nil, errors.New("invalid reply identity")
	}
	h, err := s.conversationHistory(w, conversation)
	if err != nil {
		return nil, err
	}
	savedTurn, err := h.turn(turn)
	if err != nil {
		return nil, err
	}
	if savedTurn.Bytes > HistoryTurnBytes {
		return nil, ErrHistoryBudget
	}
	var result []ReasoningEntry
	inherited := map[string]bool{}
	delivered := map[string]bool{}
	var after uint64
	size := 0
	for after < h.meta.Sequence {
		records, next, err := h.turnRecords(ctx, turn, after, h.meta.Sequence, 100, min(4*MaxRecordBytes, max(1, HistoryTurnBytes-size)))
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			size += len(record.Data) + 1
			if size > HistoryTurnBytes {
				return nil, ErrHistoryBudget
			}
			if record.Stream == "execution" {
				var checkpoint ReasoningEntry
				if err := json.Unmarshal(record.Data, &checkpoint); err != nil {
					return nil, err
				}
				for _, message := range checkpoint.Messages {
					if message.Role != "tool" {
						continue
					}
					var reply struct {
						Ref string `json:"ref"`
					}
					if json.Unmarshal([]byte(message.Content), &reply) == nil && reply.Ref != "" {
						inherited[reply.Ref] = true
					}
				}
			}
			if record.Stream != "reasoning" {
				continue
			}
			var e ReasoningEntry
			if err = json.Unmarshal(record.Data, &e); err != nil {
				return nil, err
			}
			if e.Kind != "tool" || e.Projection == nil {
				continue
			}
			if err := e.Projection.Validate(); err != nil {
				return nil, err
			}
			data, err := json.Marshal(e.Projection)
			if err != nil {
				return nil, err
			}
			e.Content = string(data)
			e.Time = e.Projection.ObservedAt
			e.Ref = record.Ref
			delivered[e.Ref] = true
			result = append(result, e)
		}
		after = next
	}
	// Native checkpoints bind earlier delivered facts to this reply without
	// copying the observation into another reasoning record.
	refs := make([]string, 0, len(inherited))
	for ref := range inherited {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if delivered[ref] {
			continue
		}
		parts := strings.SplitN(ref, ":", 3)
		if len(parts) != 3 || parts[0] != "record" || parts[1] != conversation {
			continue
		}
		original, err := h.readID(ctx, parts[2])
		if err != nil {
			return nil, err
		}
		if original.Sequence > savedTurn.Sequence || original.Stream != "reasoning" {
			continue
		}
		var e ReasoningEntry
		if err = json.Unmarshal(original.Data, &e); err != nil {
			return nil, err
		}
		if e.Projection == nil {
			continue
		}
		encoded, err := json.Marshal(e.Projection)
		if err != nil {
			return nil, err
		}
		size += len(encoded)
		if size > HistoryTurnBytes {
			return nil, ErrHistoryBudget
		}
		e.Content, e.Ref, e.Time = string(encoded), original.Ref, e.Projection.ObservedAt
		result = append(result, e)
	}
	// Follow only host-written history receipts, never resource text or model prose.
	// The same captured history directory enforces the connection boundary.
	for i := 0; i < len(result); i++ {
		receipt := result[i]
		if receipt.Projection == nil || receipt.Projection.Schema != "history.read.v1" {
			continue
		}
		var refs []string
		for _, row := range receipt.Projection.Rows {
			if row.Identity.Kind == "SavedEvidence" || row.Identity.Kind == "SavedExcerpt" {
				refs = append(refs, row.Identity.Name)
			}
		}
		for name, value := range receipt.Projection.Facts {
			if strings.HasPrefix(name, "source_") && value.Text != nil {
				refs = append(refs, *value.Text)
			}
		}
		sort.Strings(refs)
		for _, ref := range refs {
			if delivered[ref] {
				continue
			}
			parts := strings.SplitN(ref, ":", 3)
			if len(parts) != 3 || parts[0] != "record" || (w == "" && parts[1] != conversation) {
				return nil, errors.New("saved history citation is outside the reply scope")
			}
			origin, err := s.conversationHistory(w, parts[1])
			if err != nil {
				return nil, err
			}
			raw, err := origin.readID(ctx, parts[2])
			if err != nil {
				return nil, err
			}
			var entry ReasoningEntry
			switch raw.Stream {
			case "reasoning":
				if err = json.Unmarshal(raw.Data, &entry); err != nil {
					return nil, err
				}
				if entry.Projection == nil {
					return nil, errors.New("saved history citation is not projected evidence")
				}
				encoded, err := json.Marshal(entry.Projection)
				if err != nil {
					return nil, err
				}
				entry.Content, entry.Time = string(encoded), entry.Projection.ObservedAt
			case "conversation":
				var message ConversationRecord
				if err = json.Unmarshal(raw.Data, &message); err != nil {
					return nil, err
				}
				text, status, at := message.Text, message.Status, message.Time
				if message.Role == "assistant" {
					t, err := origin.loadTurn(raw.TurnID)
					if err != nil {
						return nil, err
					}
					if t.EndSequence == 0 {
						return nil, errors.New("saved citation has no settled answer")
					}
					text, status, at = t.Response.Text, t.Response.Status, t.Response.Updated
				} else if message.Role != "user" {
					return nil, errors.New("saved citation is not a message")
				}
				encoded, err := json.Marshal(map[string]any{"ref": ref, "role": message.Role, "status": status, "time": at, "text": text})
				if err != nil {
					return nil, err
				}
				entry = ReasoningEntry{ID: raw.ID, Kind: "tool", Tool: "history_read", Status: "completed", Time: at,
					Content: string(encoded)}
			default:
				return nil, fmt.Errorf("saved citation refers to unsupported stream %q", raw.Stream)
			}
			size += len(entry.Content)
			if size > HistoryTurnBytes {
				return nil, ErrHistoryBudget
			}
			entry.Ref = ref
			delivered[ref] = true
			result = append(result, entry)
		}
	}
	return result, nil
}

package history

import (
	"encoding/json"
	"errors"
	"time"
)

// SaveUIResult keeps presentation receipts outside searchable resource evidence.
func (s *Store) SaveUIResult(w, id, turn, tool string, args, result json.RawMessage) error {
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
		return errors.New("UI result does not belong to active turn")
	}
	record := struct {
		ID        string          `json:"id"`
		Time      time.Time       `json:"time"`
		Kind      string          `json:"kind"`
		MessageID string          `json:"message_id"`
		Status    string          `json:"status"`
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Result    json.RawMessage `json:"result"`
	}{chronologicalID(), time.Now().UTC(), "ui", turn, "completed", tool, args, result}
	_, err = h.append("execution", record, turn, 0)
	return err
}

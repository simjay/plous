package history

import (
	"context"
	"github.com/simjay/plous/internal/integrations/observation"
	"time"
)

func appendObservationFixture(s *Store, w, id string, entry ReasoningEntry) (ReasoningEntry, error) {
	tool := entry.Tool
	if tool == "" {
		tool = "source_files_read"
	}
	result := observation.New("fixture.text.v1", tool, time.Now().UTC())
	result.Facts["text"] = observation.Text(entry.Content)
	return s.SaveObservation(w, id, entry.MessageID, tool, entry.Input, result)
}

func (s *Store) TurnRecords(ctx context.Context, w, id, turn string, maxSequence uint64) ([]StoredRecord, error) {
	var result []StoredRecord
	var after uint64
	for after < maxSequence {
		records, next, err := s.ReadTurnRecords(ctx, w, id, turn, after, maxSequence, 100, 4*MaxRecordBytes)
		if err != nil {
			return nil, err
		}
		result = append(result, records...)
		if next <= after {
			break
		}
		after = next
	}
	return result, nil
}

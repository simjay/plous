package history

import (
	"context"
	"errors"
	bolt "go.etcd.io/bbolt"
	"os"
	"path/filepath"
	"testing"
)

func TestTurnEvidenceRejectsTurnBeyondStorageBudget(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "Oversized reply")
	h := s.histories[":"+c.ID]
	location, err := h.turn(turn.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	location.Bytes = HistoryTurnBytes + 1
	if err := h.db.Update(func(tx *bolt.Tx) error {
		return putHistoryJSON(historyBucket(tx, c.ID).Bucket([]byte("turns")), []byte(turn.Request.ID), location)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID); !errors.Is(err, ErrHistoryBudget) {
		t.Fatal("oversized saved turn was not bounded", err)
	}
}

func TestTurnEvidenceRejectsWrongScopeAndUnsafeFiles(t *testing.T) {
	s, c := newRecordStore(t)
	w, err := addWorkspaceFixture(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TurnEvidence(context.Background(), w.ID, c.ID, "turn"); err == nil {
		t.Fatal("read crossed the conversation's workspace")
	}
	if _, err := s.TurnEvidence(context.Background(), "", c.ID, "../turn"); err == nil {
		t.Fatal("accepted invalid reply identity")
	}
	path := filepath.Join(s.root, "conversations", c.ID, "reasoning", "000001.jsonl")
	if err := os.WriteFile(path, []byte("broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TurnEvidence(context.Background(), "", c.ID, "turn"); err == nil {
		t.Fatal("accepted corrupt completed record")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TurnEvidence(context.Background(), "", c.ID, "turn"); err == nil {
		t.Fatal("source viewer followed a symlink")
	}
}

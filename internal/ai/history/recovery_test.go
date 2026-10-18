package history

import (
	"context"
	bolt "go.etcd.io/bbolt"
	"strings"
	"testing"
)

func TestIndependentShardRecoveryRebuildsCrossStreamOrder(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "question")
	h := s.histories[":"+c.ID]
	h.shardTarget = 1800
	var refs []string
	for i := 0; i < 6; i++ {
		entry, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Status: "completed", Content: strings.Repeat("e", 1400)})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, entry.Ref)
		if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: strings.Repeat("a", i*900)}); err != nil {
			t.Fatal(err)
		}
	}
	requireHistoryAnswer(t, s, c, turn, "completed")
	if len(h.manifest.Streams["conversation"]) < 2 || len(h.manifest.Streams["reasoning"]) < 2 {
		t.Fatal(h.manifest)
	}
	if err := h.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte(c.ID)) }); err != nil {
		t.Fatal(err)
	}
	s = closeAndReopenHistory(t, s)
	turns, err := s.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Status != "completed" {
		t.Fatal(turns, err)
	}
	h = s.histories[":"+c.ID]
	for _, ref := range refs {
		if _, err = s.ResolveRecord(context.Background(), "", c.ID, ref, RecordBoundary{Sequence: h.meta.Sequence}); err != nil {
			t.Fatal(ref, err)
		}
	}
}

func TestTurnOutcomeDistinguishesFailureBeforeClosure(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "question")
	h := s.histories[":"+c.ID]
	injectHistoryFailure(h, "short", 1)
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "unsaved answer"}); err == nil {
		t.Fatal("expected injected write error")
	}
	outcome, err := s.TurnOutcome("", c.ID, turn.Request.ID)
	if err != nil || outcome.Closed || outcome.Status != "streaming" || outcome.EndSequence != 0 {
		t.Fatal("unconfirmed closure reported complete", outcome, err)
	}
}

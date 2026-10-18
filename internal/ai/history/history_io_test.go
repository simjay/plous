package history

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type failingHistoryFile struct {
	*os.File
	shortWrite bool
	syncErr    error
	syncCalls  *int
}

func (f *failingHistoryFile) WriteAt(data []byte, offset int64) (int, error) {
	if f.shortWrite {
		return f.File.WriteAt(data[:len(data)/2], offset)
	}
	return f.File.WriteAt(data, offset)
}
func (f *failingHistoryFile) Sync() error {
	if f.syncCalls != nil {
		*f.syncCalls++
	}
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.File.Sync()
}
func injectHistoryFailure(h *historyState, mode string, nth int) *int {
	opened := new(int)
	h.openAppend = func(path string) (historyAppendFile, error) {
		f, err := openRecordFile(path, true)
		if err != nil {
			return nil, err
		}
		*opened++
		if *opened != nth {
			return f, nil
		}
		failed := &failingHistoryFile{File: f, shortWrite: mode == "short"}
		if mode == "sync" {
			failed.syncErr = syscall.EIO
		}
		return failed, nil
	}
	return opened
}
func TestHistoryAppendIOFailuresPoisonAndRecover(t *testing.T) {
	for _, phase := range []string{"reasoning", "answer", "closing"} {
		for _, mode := range []string{"short", "sync"} {
			t.Run(phase+"-"+mode, func(t *testing.T) {
				s, c := newRecordStore(t)
				turn := requireHistoryTurn(t, s, c, "question")
				if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: "acknowledged partial"}); err != nil {
					t.Fatal(err)
				}
				h := s.histories[":"+c.ID]
				before := h.meta.Sequence
				nth := 1
				if phase == "closing" {
					nth = 2
				}
				opened := injectHistoryFailure(h, mode, nth)
				var err error
				if phase == "reasoning" {
					var entry ReasoningEntry
					entry, err = appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Tool: "kubernetes_resources_read", Status: "completed", Content: "uncertain observation"})
					if entry.Ref != "" {
						t.Fatal("failed append published a reference", entry.Ref)
					}
				} else {
					err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "final answer"})
				}
				expected := error(io.ErrShortWrite)
				if mode == "sync" {
					expected = syscall.EIO
				}
				if !errors.Is(err, expected) {
					t.Fatalf("append error=%v want %v", err, expected)
				}
				expectedSequence := before
				if phase == "closing" {
					expectedSequence++
				}
				if h.meta.Sequence != expectedSequence {
					t.Fatal("failed append advanced durable lookup", h.meta.Sequence, expectedSequence)
				}
				if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "error", Text: "retry"}); !errors.Is(err, expected) {
					t.Fatal("poisoned writer accepted a response", err)
				}
				if _, err = appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Status: "completed"}); !errors.Is(err, expected) {
					t.Fatal("poisoned writer accepted a callback", err)
				}
				if _, err = s.StartTurn("", c.ID, Request{Prompt: "another question"}); !errors.Is(err, expected) {
					t.Fatal("poisoned writer admitted another turn", err)
				}
				if *opened != nth {
					t.Fatal("writer performed I/O after its failure", *opened, nth)
				}
				s = closeAndReopenHistory(t, s)
				turns, err := s.Turns("", c.ID)
				if err != nil || len(turns) != 1 {
					t.Fatal(turns, err)
				}
				text, status := "acknowledged partial", "interrupted"
				if phase == "closing" || phase == "answer" && mode == "sync" {
					text = "final answer"
				}
				if phase == "closing" && mode == "sync" {
					status = "completed"
				}
				if turns[0].Response.Text != text || turns[0].Response.Status != status {
					t.Fatal("recovery lost acknowledged text or misclassified closure", turns[0])
				}
				active, err := s.AgentMessages(t.Context(), "", c.ID)
				if err != nil || len(active) != 0 {
					t.Fatal("transcript recovery rewrote native model context", active, err)
				}
				bounds, err := s.RecordBoundaries("", c.ID)
				if err != nil {
					t.Fatal(err)
				}
				records, err := s.TurnRecords(context.Background(), "", c.ID, turn.Request.ID, bounds[0].Sequence)
				if err != nil {
					t.Fatal(err)
				}
				observations, answers, ends := 0, 0, 0
				for _, record := range records {
					if strings.Contains(string(record.Data), "uncertain observation") {
						observations++
					}
					if strings.Contains(string(record.Data), `"role":"assistant"`) && !strings.Contains(string(record.Data), `"status":"streaming"`) {
						answers++
					}
					if strings.Contains(string(record.Data), `"kind":"turn_end"`) {
						ends++
					}
				}
				expectedObservations := 0
				if phase == "reasoning" && mode == "sync" {
					expectedObservations = 1
				}
				if observations != expectedObservations || answers != 1 || ends != 1 {
					t.Fatal("recovery duplicated or discarded complete records", observations, answers, ends)
				}
			})
		}
	}
}
func TestHistoryRecoverySyncsCompleteTailsBeforeLookup(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "question")
	h := s.histories[":"+c.ID]
	injectHistoryFailure(h, "sync", 1)
	if _, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Tool: "kubernetes_resources_read", Status: "completed", Content: "complete but not synced"}); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	before := h.meta.Sequence
	recovered := &historyState{dir: h.dir, manifest: h.manifest, db: h.db, meta: h.meta, shardTarget: HistoryShardBytes}
	syncCalls := 0
	recovered.openAppend = func(path string) (historyAppendFile, error) {
		f, err := openRecordFile(path, true)
		if err != nil {
			return nil, err
		}
		wrapped := &failingHistoryFile{File: f, syncCalls: &syncCalls}
		if filepath.Base(filepath.Dir(path)) == "reasoning" {
			wrapped.syncErr = syscall.EIO
		}
		return wrapped, nil
	}
	if err := recovered.recover(); !errors.Is(err, syscall.EIO) {
		t.Fatal("recovery did not surface failed source sync", err)
	}
	if syncCalls != 1 || recovered.meta.Sequence != before {
		t.Fatal("recovery published lookup before synchronizing its complete tail", syncCalls, recovered.meta.Sequence, before)
	}
	s.histories[":"+c.ID] = recovered
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "not allowed"}); !errors.Is(err, syscall.EIO) {
		t.Fatal("failed recovery did not poison writer", err)
	}
	s = closeAndReopenHistory(t, s)
	turns, err := s.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Status != "interrupted" {
		t.Fatal(turns, err)
	}
	bounds, err := s.RecordBoundaries("", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.TurnRecords(context.Background(), "", c.ID, turn.Request.ID, bounds[0].Sequence)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || strings.Contains(string(entry.Data), "complete but not synced")
	}
	if !found {
		t.Fatal("recovery discarded a complete failed-sync entry")
	}
}
func TestHistoryHealthyRecoveryDoesNotSyncUnchangedStreams(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "question")
	requireHistoryAnswer(t, s, c, turn, "completed")
	h := s.histories[":"+c.ID]
	syncCalls := 0
	h.openAppend = func(path string) (historyAppendFile, error) {
		f, err := openRecordFile(path, true)
		if err != nil {
			return nil, err
		}
		return &failingHistoryFile{File: f, syncCalls: &syncCalls}, nil
	}
	if err := h.recover(); err != nil {
		t.Fatal(err)
	}
	if syncCalls != 0 {
		t.Fatal("healthy recovery unnecessarily synchronized source streams", syncCalls)
	}
}

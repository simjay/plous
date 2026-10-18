package history

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRecordStore(t *testing.T) (*Store, Conversation) {
	t.Helper()
	s, err := openTestHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := s.NewConversation("", "Test")
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func TestJSONLBoundariesAndRejectedInput(t *testing.T) {
	s, c := newRecordStore(t)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "before boundary"})
	if err != nil {
		t.Fatal(err)
	}
	boundaries, err := s.RecordBoundaries("", c.ID)
	if err != nil || len(boundaries) != 1 {
		t.Fatal(boundaries, err)
	}
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "after boundary"}); err != nil {
		t.Fatal(err)
	}
	lines, next, err := s.ReadRecords(context.Background(), "", c.ID, "conversation", 0, int64(boundaries[0].Sequence), 5, 1024)
	if err != nil || len(lines) != 1 || next != int64(boundaries[0].Sequence) || bytes.Contains(lines[0].Data, []byte("after boundary")) {
		t.Fatal(lines, next, err)
	}
	if _, _, err := s.ReadRecords(context.Background(), "", c.ID, "conversation", -1, int64(boundaries[0].Sequence), 5, 1024); err == nil {
		t.Fatal("read accepted a negative sequence cursor")
	}
	if _, _, err := s.ReadRecords(context.Background(), "", c.ID, "../credentials", 0, 0, 5, 1024); err == nil {
		t.Fatal("read accepted an arbitrary source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.ReadRecords(ctx, "", c.ID, "conversation", 0, int64(boundaries[0].Sequence), 5, 1024); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	if _, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: "missing", Content: "summary"}); err == nil {
		t.Fatal("observation accepted a missing turn")
	}
	if _, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{Kind: "tool", Status: "completed", Content: strings.Repeat("x", MaxRecordBytes)}); err == nil {
		t.Fatal("oversized record saved")
	}
}

func TestJSONLRejectsSymlinksAndCompletedCorruption(t *testing.T) {
	s, c := newRecordStore(t)
	path := filepath.Join(s.root, "conversations", c.ID, "reasoning", "000001.jsonl")
	original := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(original, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, path); err != nil {
		t.Fatal(err)
	}
	if _, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{Kind: "tool", Status: "completed"}); err == nil {
		t.Fatal("append followed a symlink")
	}
	if _, err := s.RecordBoundaries("", c.ID); err == nil {
		t.Fatal("snapshot followed a symlink")
	}
	content, _ := os.ReadFile(original)
	if string(content) != "outside" {
		t.Fatal("symlink target changed")
	}
	os.Remove(path)
	if err := os.WriteFile(path, []byte("broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Turns("", c.ID); err == nil {
		t.Fatal("completed corrupt record was accepted")
	}
}

func TestConversationRecordsAreCompleteJSONLines(t *testing.T) {
	s, c := newRecordStore(t)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "question\nwith newlines"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []Response{{Status: "streaming", Text: "partial"}, {Status: "completed", Text: "full\nanswer"}} {
		if err := s.SaveResponse("", c.ID, turn.Request.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(s.root, "conversations", c.ID, "conversation", "000001.jsonl"))
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 4 {
		t.Fatal("expected user, partial assistant, final answer, and closing entry")
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal("record is not standalone JSON")
		}
	}
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "full\nanswer"}); err != nil {
		t.Fatal("identical retry should be idempotent", err)
	}
	after, _ := os.ReadFile(filepath.Join(s.root, "conversations", c.ID, "conversation", "000001.jsonl"))
	if !bytes.Equal(data, after) {
		t.Fatal("identical retry appended a duplicate response")
	}
}

func TestStreamingRecordsStoreOnlyAddedTextAndRecoverResets(t *testing.T) {
	s, c := newRecordStore(t)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "question"})
	if err != nil {
		t.Fatal(err)
	}
	chunk := strings.Repeat("x", 1024)
	text := ""
	for range 40 {
		text += chunk
		if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(s.root, "conversations", c.ID, "conversation", "000001.jsonl")
	data, _ := os.ReadFile(path)
	if len(data) > 70*1024 {
		t.Fatalf("streaming storage grew faster than the answer: %d bytes", len(data))
	}
	turns, err := s.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Text != text {
		t.Fatal("streaming deltas did not rebuild the answer", err)
	}
	// The outer agent can clear intermediate commentary before its final answer.
	for _, response := range []Response{{Status: "streaming", Text: ""}, {Status: "streaming", Text: "new answer"}, {Status: "completed", Text: "new answer"}} {
		if err := s.SaveResponse("", c.ID, turn.Request.ID, response); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	reopened, err := openTestHistory(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	turns, err = reopened.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Text != "new answer" || turns[0].Response.Status != "completed" {
		t.Fatal("reset/final response was not restored after restart", turns, err)
	}
}

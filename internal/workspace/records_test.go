package workspace

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
	s, err := Open(t.TempDir())
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

func TestJSONLRecoveryAndCompactionPreserveOriginals(t *testing.T) {
	s, c := newRecordStore(t)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "original question"})
	if err != nil {
		t.Fatal(err)
	}
	response := Response{Status: "completed", Text: "original answer"}
	if err := s.SaveResponse("", c.ID, turn.Request.ID, response); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.root, "conversations", c.ID)
	path := filepath.Join(dir, "conversation.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.AppendReasoning("", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "compaction", Status: "completed", Content: "summary", CoveredTurnID: turn.Request.ID})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestCompaction("", c.ID)
	if err != nil || latest == nil || latest.ID != entry.ID || latest.CoveredTurnID != turn.Request.ID {
		t.Fatal(latest, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("compaction changed original Q&A")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString(`{"role":"user","text":"incomplete`)
	file.Close()
	if turns, err := s.Turns("", c.ID); err != nil || len(turns) != 1 || turns[0].Response.Text != "original answer" {
		t.Fatal(turns, err)
	}
	if _, err := s.StartTurn("", c.ID, Request{Prompt: "after recovery"}); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(path)
	if !bytes.HasPrefix(after, before) || bytes.Contains(after, []byte("incomplete")) {
		t.Fatal("recovery changed a complete record or retained an unpublished tail")
	}
	s.Close()
	reopened, err := Open(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	turns, err := reopened.Turns("", c.ID)
	if err != nil || len(turns) != 2 || turns[1].Request.Prompt != "after recovery" {
		t.Fatal(turns, err)
	}
	latest, err = reopened.LatestCompaction("", c.ID)
	if err != nil || latest == nil || latest.Content != "summary" {
		t.Fatal(latest, err)
	}
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
	lines, next, err := s.ReadRecords(context.Background(), "", c.ID, "conversation", 0, boundaries[0].Messages, 5, 1024)
	if err != nil || len(lines) != 1 || next != boundaries[0].Messages || bytes.Contains(lines[0].Data, []byte("after boundary")) {
		t.Fatal(lines, next, err)
	}
	if _, _, err := s.ReadRecords(context.Background(), "", c.ID, "conversation", 1, boundaries[0].Messages, 5, 1024); err == nil {
		t.Fatal("read accepted a cursor inside a record")
	}
	if _, _, err := s.ReadRecords(context.Background(), "", c.ID, "../credentials", 0, 0, 5, 1024); err == nil {
		t.Fatal("read accepted an arbitrary source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.ReadRecords(ctx, "", c.ID, "conversation", 0, boundaries[0].Messages, 5, 1024); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	if _, err := s.AppendReasoning("", c.ID, ReasoningEntry{Kind: "compaction", Status: "completed", Content: "summary", CoveredTurnID: "missing"}); err == nil {
		t.Fatal("compaction covered a missing turn")
	}
	if _, err := s.AppendReasoning("", c.ID, ReasoningEntry{Kind: "tool", Status: "completed", Content: strings.Repeat("x", MaxRecordBytes)}); err == nil {
		t.Fatal("oversized record saved")
	}
}

func TestJSONLRejectsSymlinksAndCompletedCorruption(t *testing.T) {
	s, c := newRecordStore(t)
	path := filepath.Join(s.root, "conversations", c.ID, "reasoning.jsonl")
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
	if _, err := s.AppendReasoning("", c.ID, ReasoningEntry{Kind: "tool", Status: "completed"}); err == nil {
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
	data, _ := os.ReadFile(filepath.Join(s.root, "conversations", c.ID, "conversation.jsonl"))
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 3 {
		t.Fatal("expected original user, partial assistant, and final assistant records")
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal("record is not standalone JSON")
		}
	}
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "full\nanswer"}); err != nil {
		t.Fatal("identical retry should be idempotent", err)
	}
	after, _ := os.ReadFile(filepath.Join(s.root, "conversations", c.ID, "conversation.jsonl"))
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
	path := filepath.Join(s.root, "conversations", c.ID, "conversation.jsonl")
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
	reopened, err := Open(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	turns, err = reopened.Turns("", c.ID)
	if err != nil || len(turns) != 1 || turns[0].Response.Text != "new answer" || turns[0].Response.Status != "completed" {
		t.Fatal("reset/final response was not restored after restart", turns, err)
	}
}

func TestCompactionCannotRegressCoveredHistory(t *testing.T) {
	s, c := newRecordStore(t)
	var turns []Turn
	for _, prompt := range []string{"one", "two"} {
		turn, err := s.StartTurn("", c.ID, Request{Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: prompt}); err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}
	for _, index := range []int{1, 0} {
		if _, err := s.AppendReasoning("", c.ID, ReasoningEntry{Kind: "compaction", Status: "completed", Content: turns[index].Request.Prompt, CoveredTurnID: turns[index].Request.ID}); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.LatestCompaction("", c.ID)
	if err != nil || latest == nil || latest.CoveredTurnID != turns[1].Request.ID || latest.Content != "two" {
		t.Fatal("late older compaction regressed the checkpoint", latest, err)
	}
	path := filepath.Join(s.root, "conversations", c.ID, "reasoning.jsonl")
	if err := appendJSONL(path, ReasoningEntry{Kind: "compaction", Status: "completed", Content: "invalid", CoveredTurnID: "z-missing"}); err != nil {
		t.Fatal(err)
	}
	latest, err = s.LatestCompaction("", c.ID)
	if err != nil || latest == nil || latest.Content != "two" {
		t.Fatal("invalid checkpoint replaced a valid summary", latest, err)
	}
}

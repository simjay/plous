package history

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func closeAndReopenHistory(t *testing.T, s *Store) *Store {
	t.Helper()
	root := s.root
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := openTestHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { next.Close() })
	return next
}
func requireHistoryTurn(t *testing.T, s *Store, c Conversation, prompt string) Turn {
	t.Helper()
	turn, err := s.StartTurn(c.WorkspaceID, c.ID, Request{Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}
func requireHistoryAnswer(t *testing.T, s *Store, c Conversation, turn Turn, status string) {
	t.Helper()
	if err := s.SaveResponse(c.WorkspaceID, c.ID, turn.Request.ID, Response{Status: status, Text: turn.Request.Prompt + " answer"}); err != nil {
		t.Fatal(err)
	}
}
func TestHistoryIndependentRotationAndStableReferences(t *testing.T) {
	for _, stream := range []string{"conversation", "reasoning"} {
		t.Run(stream, func(t *testing.T) {
			s, c := newRecordStore(t)
			turn := requireHistoryTurn(t, s, c, "first")
			h := s.histories[":"+c.ID]
			h.shardTarget = 5000
			if stream == "conversation" {
				if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: strings.Repeat("q", 6000)}); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Tool: "kubernetes_resources_read", Status: "completed", Content: strings.Repeat("r", 6000)}); err != nil {
					t.Fatal(err)
				}
			}
			requireHistoryAnswer(t, s, c, turn, "completed")
			if len(h.manifest.Streams[stream]) != 2 {
				t.Fatal("stream did not rotate", h.manifest)
			}
			other := "reasoning"
			if stream == other {
				other = "conversation"
			}
			if len(h.manifest.Streams[other]) != 1 {
				t.Fatal("unrelated stream rotated")
			}
			second := requireHistoryTurn(t, s, c, "second")
			requireHistoryAnswer(t, s, c, second, "completed")
			s = closeAndReopenHistory(t, s)
			page, err := s.TurnPage(context.Background(), "", c.ID, 0, 1, 4096)
			if err != nil || len(page.Turns) != 1 || page.Turns[0].Request.ID != second.Request.ID || !page.HasOlder {
				t.Fatal(page, err)
			}
			h = s.histories[":"+c.ID]
			record, err := s.ResolveRecord(context.Background(), "", c.ID, turn.UserRef, RecordBoundary{Sequence: h.meta.Sequence})
			if err != nil || record.ID != turn.Request.ID {
				t.Fatal(record, err)
			}
		})
	}
}
func TestHistoryClosingRecordIsOnlyCompletionAuthority(t *testing.T) {
	for _, withAnswer := range []bool{false, true} {
		t.Run(map[bool]string{false: "streaming", true: "saved-answer"}[withAnswer], func(t *testing.T) {
			s, c := newRecordStore(t)
			turn := requireHistoryTurn(t, s, c, "question")
			if withAnswer {
				h := s.histories[":"+c.ID]
				record := ConversationRecord{ID: chronologicalID(), TurnID: turn.Request.ID, Role: "assistant", Time: time.Now().UTC(), Response: Response{Status: "completed", Text: "saved final"}}
				if _, err := h.append("conversation", record, turn.Request.ID, 1); err != nil {
					t.Fatal(err)
				}
			} else if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: "saved partial"}); err != nil {
				t.Fatal(err)
			}
			s = closeAndReopenHistory(t, s)
			turns, err := s.Turns("", c.ID)
			if err != nil || len(turns) != 1 || turns[0].Response.Status != "interrupted" || turns[0].Response.Text == "" {
				t.Fatal(turns, err)
			}
			h := s.histories[":"+c.ID]
			records, err := s.TurnRecords(context.Background(), "", c.ID, turn.Request.ID, h.meta.Sequence)
			if err != nil {
				t.Fatal(err)
			}
			answers, ends := 0, 0
			for _, r := range records {
				var message ConversationRecord
				_ = json.Unmarshal(r.Data, &message)
				if message.Kind == "turn_end" {
					ends++
				}
				if message.Role == "assistant" && message.Status != "streaming" {
					answers++
				}
			}
			if answers != 1 || ends != 1 {
				t.Fatal("recovery duplicated answer or closure", answers, ends)
			}
			s = closeAndReopenHistory(t, s)
			again, err := s.Turns("", c.ID)
			if err != nil || again[0].AnswerRef != turns[0].AnswerRef || again[0].EndSequence != turns[0].EndSequence {
				t.Fatal("recovery is not idempotent", again, err)
			}
		})
	}
}
func TestHistoryReserveClosesExhaustedInvestigation(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "large investigation")
	content := strings.Repeat("x", 6<<20)
	var exceeded bool
	for i := 0; i < 12; i++ {
		_, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Status: "completed", Content: content})
		if errors.Is(err, ErrHistoryBudget) {
			exceeded = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !exceeded {
		t.Fatal("turn had no storage ceiling")
	}
	answer := strings.Repeat("\x00", 1<<20)
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "error", Text: answer, Error: ErrHistoryBudget.Error()}); err != nil {
		t.Fatal("reserved answer could not be saved", err)
	}
	h := s.histories[":"+c.ID]
	loc, err := h.turn(turn.Request.ID)
	if err != nil || loc.Bytes > HistoryTurnBytes || loc.Status != "error" {
		t.Fatal(loc, err)
	}
	if _, err = appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Status: "completed"}); err == nil {
		t.Fatal("late event accepted")
	}
}
func TestHistoryDoesNotEraseCompleteCorruption(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		t.Run(strconv.FormatBool(sealed), func(t *testing.T) {
			s, c := newRecordStore(t)
			turn := requireHistoryTurn(t, s, c, "original")
			h := s.histories[":"+c.ID]
			if sealed {
				h.shardTarget = 1
			}
			requireHistoryAnswer(t, s, c, turn, "completed")
			path := filepath.Join(h.dir, "execution", "000001.jsonl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			damaged := bytes.Replace(data, []byte("original"), []byte("modified"), 1)
			if bytes.Equal(data, damaged) {
				t.Fatal("fixture did not damage record")
			}
			if err = os.WriteFile(path, damaged, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Turns("", c.ID); err == nil {
				t.Fatal("changed original record accepted")
			}
			if err = s.RepairHistoryLookups(""); sealed && err == nil {
				t.Fatal("sealed checksum corruption ignored")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, damaged) {
				t.Fatal("complete corrupt record was truncated or overwritten")
			}
		})
	}
}

func TestHistoryCapturedExchangeCannotReadFutureParts(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "question")
	if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "streaming", Text: "captured"}); err != nil {
		t.Fatal(err)
	}
	bounds, err := s.RecordBoundaries("", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "captured future-secret"}); err != nil {
		t.Fatal(err)
	}
	records, next, err := s.ReadTurnRecords(context.Background(), "", c.ID, turn.Request.ID, 0, bounds[0].Sequence, 100, 4096)
	if err != nil || next != bounds[0].Sequence {
		t.Fatal("captured read did not reach its saved boundary", records, next, err)
	}
	var replies []ConversationRecord
	for _, record := range records {
		if record.Stream != "conversation" {
			continue
		}
		var message ConversationRecord
		if err := json.Unmarshal(record.Data, &message); err != nil {
			t.Fatal(err)
		}
		if message.Role == "assistant" {
			replies = append(replies, message)
		}
	}
	if len(replies) != 1 || replies[0].Text != "captured" || replies[0].Status != "streaming" {
		t.Fatal("read crossed captured sequence", replies)
	}
	if status, err := s.TurnStatus("", c.ID, turn.Request.ID, bounds[0].Sequence); err != nil || status != "streaming" {
		t.Fatal("status crossed captured sequence", status, err)
	}
}
func TestHistoryWorkspaceBoundaryIncludesNewerExistingChats(t *testing.T) {
	s, oldGeneral := newRecordStore(t)
	w, err := addWorkspaceFixture(s)
	if err != nil {
		t.Fatal(err)
	}
	older, err := s.NewConversation(w.ID, "older")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.NewConversation(w.ID, "newer")
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := s.RecordBoundaries(w.ID, older.ID)
	if err != nil || len(bounds) != 2 {
		t.Fatal("returning to an older chat hid existing workspace history", bounds, err)
	}
	found := false
	for _, b := range bounds {
		found = found || b.Conversation.ID == newer.ID
	}
	if !found {
		t.Fatal("newer existing chat was omitted")
	}
	if _, err = s.NewConversation("", "newer general"); err != nil {
		t.Fatal(err)
	}
	general, err := s.RecordBoundaries("", oldGeneral.ID)
	if err != nil || len(general) != 1 || general[0].Conversation.ID != oldGeneral.ID {
		t.Fatal("general chat scope expanded", general, err)
	}
}

func TestHistoryChangesAreBoundedAndWorkspaceScoped(t *testing.T) {
	s, c := newRecordStore(t)
	changed, err := s.HistoryChanges("")
	if err != nil {
		t.Fatal(err)
	}
	turn := requireHistoryTurn(t, s, c, "question")
	select {
	case <-changed:
	default:
		t.Fatal("durable question did not notify search")
	}
	for i := 0; i < 3; i++ {
		if _, err = appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Kind: "tool", Status: "completed", Content: "observation"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(changed) != 1 {
		t.Fatal("notifications were not coalesced", len(changed))
	}
	<-changed
	requireHistoryAnswer(t, s, c, turn, "completed")
	select {
	case <-changed:
	default:
		t.Fatal("durable answer did not notify search")
	}
	w, err := addWorkspaceFixture(s)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.NewConversation(w.ID, "other")
	if err != nil {
		t.Fatal(err)
	}
	otherTurn := requireHistoryTurn(t, s, other, "separate question")
	requireHistoryAnswer(t, s, other, otherTurn, "completed")
	if len(changed) != 0 {
		t.Fatal("unrelated workspace notified this search")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartTurn("", c.ID, Request{Prompt: "closed"}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestHistoryRecentOpenDoesNotReadUnrelatedSealedBodies(t *testing.T) {
	s, c := newRecordStore(t)
	var latest Turn
	for i := 0; i < 3; i++ {
		turn := requireHistoryTurn(t, s, c, strconv.Itoa(i))
		s.histories[":"+c.ID].shardTarget = 1
		requireHistoryAnswer(t, s, c, turn, "completed")
		latest = turn
	}
	path := filepath.Join(s.root, "conversations", c.ID, "execution", "000001.jsonl")
	s = closeAndReopenHistory(t, s)
	if err := os.WriteFile(path, []byte("changed old body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	page, err := s.TurnPage(context.Background(), "", c.ID, 0, 1, 4096)
	if err != nil || len(page.Turns) != 1 || page.Turns[0].Request.ID != latest.Request.ID {
		t.Fatal("recent opening read unrelated old content", page, err)
	}
	if _, err = s.TurnPageAfter(context.Background(), "", c.ID, 0, 1, 4096); err == nil {
		t.Fatal("directly requesting corrupt old content succeeded")
	}
}

func TestHistoryFailedRepairWithoutPreviousLookupCanClose(t *testing.T) {
	s, c := newRecordStore(t)
	path := filepath.Join(s.root, "conversations", c.ID, "reasoning", "000001.jsonl")
	if err := os.WriteFile(path, []byte("malformed complete record\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(s.historyDBs) != 0 {
		t.Fatal("fixture unexpectedly opened a lookup")
	}
	if err := s.RepairHistoryLookups(""); err == nil {
		t.Fatal("repair accepted malformed history")
	}
	if len(s.historyDBs) != 0 {
		t.Fatal("failed repair retained an unopened lookup")
	}
	if err := s.Close(); err != nil {
		t.Fatal("store could not close after failed repair", err)
	}
}

func TestHistoryCommitBroadcastsToEverySubscriber(t *testing.T) {
	store, conversation := newRecordStore(t)
	first, err := store.HistoryChanges("")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.HistoryChanges("")
	if err != nil {
		t.Fatal(err)
	}
	requireHistoryTurn(t, store, conversation, "broadcast")
	for _, subscriber := range []<-chan struct{}{first, second} {
		select {
		case <-subscriber:
		default:
			t.Fatal("durable commit was consumed by another subscriber")
		}
	}
}

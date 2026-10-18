package history

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecentMessagesBoundCompletedTextBeforeActiveTurn(t *testing.T) {
	s, err := openTestHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.NewConversation("", "context")
	if err != nil {
		t.Fatal(err)
	}
	for _, exchange := range []struct{ prompt, reply, status string }{
		{"first", "one", "completed"},
		{"unfinished", "partial", "interrupted"},
		{"second", "two", "completed"},
	} {
		turn, err := s.StartTurn("", c.ID, Request{Prompt: exchange.prompt, Context: strings.Repeat("metadata", 20000)})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveResponse("", c.ID, turn.Request.ID, Response{Text: exchange.reply, Status: exchange.status}); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.StartTurn("", c.ID, Request{Prompt: strings.Repeat("current", 40000)})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := s.RecentMessages(t.Context(), "", c.ID, active.Request.ID, 16, 128<<10)
	if err != nil || len(messages) != 4 {
		t.Fatalf("context=%v error=%v", messages, err)
	}
	for i, want := range []string{"first", "one", "second", "two"} {
		if messages[i].Text != want {
			t.Fatalf("message %d: %v", i, messages[i])
		}
	}
	messages, err = s.RecentMessages(t.Context(), "", c.ID, active.Request.ID, 16, 9)
	if err != nil || len(messages) != 2 || messages[0].Text != "second" {
		t.Fatal(messages, err)
	}
	if _, err := s.RecentMessages(t.Context(), "", c.ID, "another-turn", 16, 128<<10); err == nil {
		t.Fatal("accepted another active turn")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.RecentMessages(ctx, "", c.ID, active.Request.ID, 16, 128<<10); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

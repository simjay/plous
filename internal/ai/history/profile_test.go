package history_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/workspace"
)

func TestProfileLockOutlivesConversationResources(t *testing.T) {
	root := t.TempDir()
	profile, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { profile.Close() })
	saved, err := history.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := saved.NewConversation("", "Profile lifetime")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := workspace.Open(root); err == nil {
		other.Close()
		t.Fatal("a second profile acquired the history owner's lock")
	}
	readDuringShutdown := false
	if err := profile.AddCloser(func() error {
		_, err := saved.TurnPage(context.Background(), "", conversation.ID, 0, 1, 4096)
		readDuringShutdown = err == nil
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Close(); err != nil || !readDuringShutdown {
		t.Fatal("history closed before its consumers", err)
	}
	if _, err := saved.NewConversation("", "closed"); !errors.Is(err, history.ErrClosed) {
		t.Fatal("history remained writable after profile close", err)
	}
	if _, err := history.Open(profile); !errors.Is(err, workspace.ErrClosed) {
		t.Fatal("closed profile admitted history resources", err)
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := history.Open(reopened)
	if err != nil {
		t.Fatal(err)
	}
	conversations, err := restored.Conversations("")
	if err != nil || len(conversations) != 1 || conversations[0].ID != conversation.ID {
		t.Fatal("profile restart changed conversation identity", conversations, err)
	}
}

func TestProfileLockStaysHeldWhileHistoryConsumersDrain(t *testing.T) {
	root := t.TempDir()
	profile, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { profile.Close() })
	saved, err := history.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := saved.NewConversation("", "Shutdown boundary")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := saved.StartTurn("", conversation.ID, history.Request{Prompt: "Keep this original request"})
	if err != nil {
		t.Fatal(err)
	}
	if err := saved.SaveResponse("", conversation.ID, turn.Request.ID, history.Response{Status: "completed", Text: "Saved answer"}); err != nil {
		t.Fatal(err)
	}
	draining, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	if err := saved.AddCloser(func() error {
		close(draining)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- profile.Close() }()
	select {
	case <-draining:
	case <-time.After(5 * time.Second):
		t.Fatal("profile shutdown did not drain history consumers")
	}
	if other, err := workspace.Open(root); err == nil {
		other.Close()
		t.Fatal("profile lock was released before history consumers finished")
	}
	if _, err := saved.StartTurn("", conversation.ID, history.Request{Prompt: "Too late"}); !errors.Is(err, history.ErrClosed) {
		t.Fatal("closing history accepted a new turn", err)
	}
	page, err := saved.TurnPage(context.Background(), "", conversation.ID, 0, 1, 4096)
	if err != nil || len(page.Turns) != 1 || page.Turns[0].Response.Text != "Saved answer" {
		t.Fatal("draining consumers lost access to saved records", page, err)
	}
	unblock()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile shutdown did not finish after history consumers drained")
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := history.Open(reopened)
	if err != nil {
		t.Fatal(err)
	}
	page, err = restored.TurnPage(context.Background(), "", conversation.ID, 0, 1, 4096)
	if err != nil || len(page.Turns) != 1 || page.Turns[0].Request.ID != turn.Request.ID || page.Turns[0].Request.Prompt != turn.Request.Prompt || page.Turns[0].Response.Status != "completed" || page.Turns[0].Response.Text != "Saved answer" {
		t.Fatal("profile shutdown changed a completed exchange", page, err)
	}
}

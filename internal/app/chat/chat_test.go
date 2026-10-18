package chat

import (
	"context"

	ai "github.com/simjay/plous/internal/ai/model"

	"testing"
)

func TestChatTimeoutPreservesTerminalResultAfterFullEventBuffer(t *testing.T) {
	run := &ChatRun{Events: make(chan ChatEvent, 1)}
	run.Events <- ChatEvent{Run: run, Text: "checkpoint"}
	run.Terminal = ChatEvent{Run: run, Done: true, Result: ai.Result{Text: "last output", Usage: ai.Usage{Input: 30}}, Err: context.DeadlineExceeded}
	close(run.Events)
	first := WaitChat(run)().(ChatEvent)
	final := WaitChat(run)().(ChatEvent)
	if first.Text != "checkpoint" || !final.Done || final.Result.Text != "last output" || final.Result.Usage.Input != 30 || final.Err != context.DeadlineExceeded {
		t.Fatalf("terminal result lost: first=%+v final=%+v", first, final)
	}
}

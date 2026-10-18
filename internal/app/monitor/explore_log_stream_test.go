package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestLogBatchPreservesOutputAndFinalState(t *testing.T) {
	for _, explicitDone := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "done"}[explicitDone], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := &ExploreLogRun{Ctx: ctx, Cancel: cancel, Events: make(chan ExploreLogEvent, 8)}
			failure := errors.New("stream failed")
			run.Events <- ExploreLogEvent{Connected: true, WorkloadStatus: &kubernetes.WorkloadLogStatus{Active: 2}}
			run.Events <- ExploreLogEvent{Text: "[pod/api] first\n"}
			run.Events <- ExploreLogEvent{Text: "[pod/proxy] second\n"}
			run.Events <- ExploreLogEvent{WorkloadStatus: &kubernetes.WorkloadLogStatus{Finished: 1, Failed: 1}}
			run.Events <- ExploreLogEvent{Text: "last\n", Err: failure, Done: explicitDone}
			close(run.Events)
			got := WaitExploreLog(run)().(ExploreLogEvent)
			if got.Run != run || got.Text != "[pod/api] first\n[pod/proxy] second\nlast\n" || !got.Connected || !got.Done || got.Err != failure {
				t.Fatalf("lost output or stream state: %+v", got)
			}
			if got.WorkloadStatus.Active != 0 || got.WorkloadStatus.Finished != 1 || got.WorkloadStatus.Failed != 1 {
				t.Fatalf("lost latest membership: %+v", got.WorkloadStatus)
			}
		})
	}
}

func TestLogBatchesBoundBusyStreamsWithoutLosingLines(t *testing.T) {
	for _, line := range []string{"x\n", strings.Repeat("界", 1024) + "\n"} {
		ctx, cancel := context.WithCancel(context.Background())
		run := &ExploreLogRun{Ctx: ctx, Cancel: cancel, Events: make(chan ExploreLogEvent, 32)}
		t.Cleanup(cancel)
		const count = 5000
		go func() {
			defer close(run.Events)
			for range count {
				select {
				case run.Events <- ExploreLogEvent{Text: line}:
				case <-ctx.Done():
					return
				}
			}
		}()
		lines, updates := 0, 0
		for {
			event := WaitExploreLog(run)().(ExploreLogEvent)
			updates++
			lines += strings.Count(event.Text, "\n")
			if len(event.Text) > 64*1024+len(line) || strings.Count(event.Text, "\n") > 1024 || strings.ReplaceAll(event.Text, line, "") != "" {
				t.Fatal("batch exceeded bounds or changed source text")
			}
			if event.Done {
				break
			}
		}
		if lines != count || updates < 2 || updates >= count/2 {
			t.Fatalf("received %d lines in %d updates", lines, updates)
		}
	}
}

func TestLogBatchFlushesIdleStreamAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &ExploreLogRun{Ctx: ctx, Cancel: cancel, Events: make(chan ExploreLogEvent, 1)}
	run.Events <- ExploreLogEvent{Text: "sparse output\n"}
	result := make(chan ExploreLogEvent, 1)
	go func() { result <- WaitExploreLog(run)().(ExploreLogEvent) }()
	select {
	case got := <-result:
		if got.Done || got.Text != "sparse output\n" {
			t.Fatalf("lost sparse output: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("idle live stream did not flush")
	}
	go func() { result <- WaitExploreLog(run)().(ExploreLogEvent) }()
	cancel()
	select {
	case got := <-result:
		if !got.Done || got.Text != "" {
			t.Fatalf("cancelled stream returned output: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not release the waiting command")
	}
	// Cancellation also wins when old output is ready to receive.
	run.Events <- ExploreLogEvent{Text: "stale\n"}
	if got := WaitExploreLog(run)().(ExploreLogEvent); !got.Done || got.Text != "" {
		t.Fatalf("cancelled backlog returned output: %+v", got)
	}
}

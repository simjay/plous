package app

import (
	"context"
	"fmt"
	"github.com/simjay/plous/internal/app/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
	"testing"
)

func BenchmarkPodLogBacklog(b *testing.B) {
	const count = 1000
	line := "[fixture/api] 2026-10-09T21:00:00Z " + strings.Repeat("received request ", 8) + "\n"
	for _, batched := range []bool{false, true} {
		b.Run(map[bool]string{false: "per_line", true: "batched"}[batched], func(b *testing.B) {
			b.SetBytes(int64(count * len(line)))
			b.ReportAllocs()
			updates := 0
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				run := &monitor.ExploreLogRun{Ctx: ctx, Cancel: cancel, Events: make(chan monitor.ExploreLogEvent, count)}
				for range count {
					run.Events <- monitor.ExploreLogEvent{Run: run, Text: line}
				}
				close(run.Events)
				m := &model{explore: &monitor.ExploreState{MergedLogs: true, Log: run, DetailValid: true, Detail: &unstructured.Unstructured{}}}
				for m.explore.Log != nil {
					var event monitor.ExploreLogEvent
					if batched {
						event = monitor.WaitExploreLog(run)().(monitor.ExploreLogEvent)
					} else {
						var ok bool
						event, ok = <-run.Events
						event.Run, event.Done = run, !ok
					}
					m.updateExploreMessage(event)
					m.exploreLogBody(160, 40)
					updates++
				}
				cancel()
			}
			b.ReportMetric(float64(updates)/float64(b.N), "updates/op")
		})
	}
}

func BenchmarkYAMLPreparation(b *testing.B) {
	data := map[string]any{}
	for i := range 1000 {
		data[fmt.Sprintf("key-%04d", i)] = strings.Repeat("fixture ", 8)
	}
	m := &model{explore: &monitor.ExploreState{Detail: &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "fixture", "uid": "fixture", "resourceVersion": "1"}, "data": data,
	}}}}
	m.syncYAMLDocument()
	b.ReportAllocs()
	for b.Loop() {
		m.syncYAMLDocument()
	}
}

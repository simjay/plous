package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func BenchmarkKubernetesTable(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			m := &model{explore: &monitor.ExploreState{Listed: true, Resource: kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}, Preference: monitor.ExplorePreference{Namespace: "apps"}}}
			for i := 0; i < count; i++ {
				m.explore.Collection.Objects = append(m.explore.Collection.Objects, unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": fmt.Sprintf("pod-%05d", i), "namespace": "apps", "uid": fmt.Sprint(i)}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "fixture:1"}}}, "status": map[string]any{"phase": "Running"}}})
			}
			m.exploreTableBody(100, 30)
			b.ReportAllocs()
			for b.Loop() {
				m.exploreTableBody(100, 30)
			}
		})
	}
}

func BenchmarkHelmInspector(b *testing.B) {
	for _, count := range []int{10, 100} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var manifest strings.Builder
			for i := 0; i < count; i++ {
				fmt.Fprintf(&manifest, "---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web-%03d, namespace: apps}\nspec: {replicas: 2, template: {spec: {containers: [{name: web, image: fixture:1}]}}}\n", i)
			}
			h := &monitor.HelmState{Tab: 5, Detail: &helmtool.Snapshot{Name: "fixture", Namespace: "apps", Version: 1, Manifest: manifest.String(), Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "1"}}}}
			m := &model{}
			m.helmDetailBody(h, 100, 30)
			b.ReportAllocs()
			for b.Loop() {
				m.helmDetailBody(h, 100, 30)
			}
		})
	}
}

func BenchmarkHelmCards(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			h := &monitor.HelmState{Preference: monitor.HelmPreference{Section: "Charts"}}
			for i := 0; i < count; i++ {
				h.Releases = append(h.Releases, helmtool.Release{Name: fmt.Sprintf("release-%04d", i), Namespace: "apps", Chart: fmt.Sprintf("chart-%02d", i%10), Version: "1.0", Revision: 1, Status: "deployed"})
			}
			monitor.HelmChartCards(h, 100, 30)
			b.ReportAllocs()
			for b.Loop() {
				monitor.HelmChartCards(h, 100, 30)
			}
		})
	}
}

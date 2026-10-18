package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func logPod(name string) unstructured.Unstructured {
	pod := unstructured.Unstructured{Object: testPod(name, name+"-uid")}
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "app", "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}}}, "status", "containerStatuses")
	return pod
}

func TestWorkloadLogsMembershipFailureAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var phase atomic.Int32
	var mu sync.Mutex
	stopped := map[string]bool{}
	logs := func(ctx context.Context, o LogOptions, emit func(string)) error {
		if o.Pod == "broken" {
			return errors.New("failed source")
		}
		o.OnConnect()
		emit("2026-09-30T12:00:00Z hello\n")
		<-ctx.Done()
		mu.Lock()
		stopped[o.Pod] = true
		mu.Unlock()
		return ctx.Err()
	}
	targets := func(context.Context) ([]unstructured.Unstructured, error) {
		if phase.Load() == 0 {
			return []unstructured.Unstructured{logPod("first"), logPod("broken")}, nil
		}
		return []unstructured.Unstructured{logPod("next")}, nil
	}
	var text strings.Builder
	sawFailure, sawFirst := false, false
	err := workloadLogs(ctx, LogOptions{Follow: true}, time.Millisecond, targets, logs, func(event WorkloadLogEvent) {
		text.WriteString(event.Text)
		if strings.Contains(event.Text, "[first/app]") {
			sawFirst = true
		}
		if event.Status != nil && event.Status.Failed == 1 {
			sawFailure = true
		}
		if sawFirst && sawFailure {
			phase.Store(1)
		}
		if strings.Contains(event.Text, "[next/app]") {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || !sawFirst || !sawFailure || !strings.Contains(text.String(), "[next/app]") {
		t.Fatalf("err=%v first=%v failed=%v output=%s", err, sawFirst, sawFailure, text.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if !stopped["first"] || !stopped["next"] {
		t.Fatalf("streams survived removal/close: %v", stopped)
	}
}

func TestWorkloadPreviousLogsForwardOptionsAndFinish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	since := int64(60)
	timestamps := true
	options := LogOptions{Container: "init", Previous: true, SinceSeconds: &since, Timestamps: &timestamps}
	pod := logPod("one")
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "init"}}, "spec", "initContainers")
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "init", "restartCount": int64(2)}}, "status", "initContainerStatuses")
	var output string
	err := workloadLogs(ctx, options, time.Millisecond, func(context.Context) ([]unstructured.Unstructured, error) {
		return []unstructured.Unstructured{pod}, nil
	}, func(ctx context.Context, o LogOptions, emit func(string)) error {
		if o.Container != "init" || o.UID != pod.GetUID() || !o.Previous || o.Follow || o.SinceSeconds != &since || o.Timestamps != &timestamps {
			t.Errorf("lost source options: %#v", o)
		}
		emit("previous output\n")
		return nil
	}, func(event WorkloadLogEvent) { output += event.Text })
	if err != nil || output != "[one/init] previous output\n" {
		t.Fatalf("err=%v text=%q", err, output)
	}
}

func TestSinglePodMergedLogsSelectApplicationOrExplicitContainers(t *testing.T) {
	pod := logPod("one")
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "app"}, map[string]any{"name": "proxy"}}, "spec", "containers")
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "init"}}, "spec", "initContainers")
	_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "debug"}}, "spec", "ephemeralContainers")
	for field, names := range map[string][]string{"containerStatuses": {"app", "proxy"}, "initContainerStatuses": {"init"}, "ephemeralContainerStatuses": {"debug"}} {
		var states []any
		for _, name := range names {
			states = append(states, map[string]any{"name": name, "restartCount": int64(1)})
		}
		_ = unstructured.SetNestedSlice(pod.Object, states, "status", field)
	}
	for _, selected := range []string{"", "init", "debug"} {
		t.Run("container="+selected, func(t *testing.T) {
			var mu sync.Mutex
			requested := map[string]int{}
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/log") {
					name := r.URL.Query().Get("container")
					mu.Lock()
					requested[name]++
					mu.Unlock()
					fmt.Fprint(w, "2026-09-30T12:00:00Z first\nsecond\n")
					return
				}
				reply(w, pod.Object)
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var output strings.Builder
			err := client.WorkloadLogs(ctx, kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, &pod, nil, LogOptions{Previous: true, Container: selected}, func(event WorkloadLogEvent) { output.WriteString(event.Text) })
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"app", "proxy"}
			if selected != "" {
				want = []string{selected}
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requested) != len(want) {
				t.Fatalf("unexpected sources: %v", requested)
			}
			for _, name := range want {
				prefix := "[one/" + name + "] "
				if requested[name] != 1 || !strings.Contains(output.String(), prefix+"2026-09-30T12:00:00Z first\n") || !strings.Contains(output.String(), prefix+"second\n") {
					t.Fatalf("lost source identity or content: %v %q", requested, output.String())
				}
			}
		})
	}
}

func TestWorkloadLogBoundAndExplicitPartialCoverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pods := []unstructured.Unstructured{}
	for i := range MaxWorkloadLogSources + 5 {
		pods = append(pods, logPod(fmt.Sprintf("pod-%02d", i)))
	}
	var started atomic.Int32
	skipped := 0
	err := workloadLogs(ctx, LogOptions{}, time.Millisecond, func(context.Context) ([]unstructured.Unstructured, error) { return pods, nil }, func(ctx context.Context, o LogOptions, emit func(string)) error {
		started.Add(1)
		return nil
	}, func(event WorkloadLogEvent) {
		if event.Status != nil {
			skipped = event.Status.Skipped
			if event.Status.Finished == MaxWorkloadLogSources {
				cancel()
			}
		}
	})
	if !errors.Is(err, context.Canceled) || started.Load() != MaxWorkloadLogSources || skipped != 5 {
		t.Fatalf("err=%v streams=%d skipped=%d", err, started.Load(), skipped)
	}
}

func TestWorkloadLogsWaitForContainersAndRestartOncePerInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var phase atomic.Int32
	var calls atomic.Int32
	sawWaiting := false
	err := workloadLogs(ctx, LogOptions{}, time.Millisecond, func(context.Context) ([]unstructured.Unstructured, error) {
		pod := logPod("one")
		status := map[string]any{"name": "app", "restartCount": int64(phase.Load() - 1), "state": map[string]any{"running": map[string]any{}}}
		if phase.Load() == 0 {
			status = map[string]any{"name": "app", "restartCount": int64(0), "state": map[string]any{"waiting": map[string]any{"reason": "ContainerCreating"}}}
		}
		_ = unstructured.SetNestedSlice(pod.Object, []any{status}, "status", "containerStatuses")
		return []unstructured.Unstructured{pod}, nil
	}, func(ctx context.Context, o LogOptions, emit func(string)) error { calls.Add(1); return nil }, func(event WorkloadLogEvent) {
		if event.Status == nil {
			return
		}
		if event.Status.Waiting == 1 {
			sawWaiting = true
			phase.Store(1)
		}
		if event.Status.Finished == 1 {
			if phase.Load() == 1 {
				phase.Store(2)
			} else {
				cancel()
			}
		}
	})
	if !errors.Is(err, context.Canceled) || !sawWaiting || calls.Load() != 2 {
		t.Fatalf("err=%v waiting=%v calls=%d", err, sawWaiting, calls.Load())
	}
}

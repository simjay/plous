package monitor

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWorkloadLogProjectionOwnershipIdentityAndBounds(t *testing.T) {
	for _, condition := range []string{"complete", "denied", "missing-container", "replaced-before", "replaced-after", "source-limit", "byte-limit"} {
		t.Run(condition, func(t *testing.T) {
			count := 2
			if condition == "source-limit" {
				count = MaxAgentLogSources + 1
			}
			pods := make([]any, 0, count+1)
			byName := map[string]unstructured.Unstructured{}
			for i := 0; i <= count; i++ {
				pod := logPod(fmt.Sprintf("pod-%02d", i))
				uid := "set-uid"
				if i == count {
					uid = "different-owner"
				}
				_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"apiVersion": "apps/v1", "kind": "StatefulSet", "name": "web", "uid": uid, "controller": true}}, "metadata", "ownerReferences")
				_ = unstructured.SetNestedStringMap(pod.Object, map[string]string{"app": "web"}, "metadata", "labels")
				if condition == "missing-container" && i == 0 {
					_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "other"}}, "spec", "containers")
				}
				pods = append(pods, pod.Object)
				byName[pod.GetName()] = pod
			}
			var logReads, podReads atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1":
					agentTestDiscovery(w)
				case "/apis/apps":
					reply(w, map[string]any{"preferredVersion": map[string]any{"groupVersion": "apps/v1"}})
				case "/apis/apps/v1":
					reply(w, map[string]any{"resources": []any{map[string]any{"name": "statefulsets", "kind": "StatefulSet", "namespaced": true, "verbs": []string{"get", "list"}}}})
				case "/apis/apps/v1/namespaces/shop/statefulsets/web":
					reply(w, map[string]any{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": map[string]any{"name": "web", "namespace": "shop", "uid": "set-uid"}})
				case "/api/v1/namespaces/shop/pods":
					reply(w, map[string]any{"apiVersion": "v1", "kind": "PodList", "items": pods})
				default:
					name := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/shop/pods/")
					if strings.HasSuffix(name, "/log") {
						name = strings.TrimSuffix(name, "/log")
						logReads.Add(1)
						if name == fmt.Sprintf("pod-%02d", count) {
							t.Error("collected logs from overlapping labels without ownership")
						}
						if r.URL.Query().Get("container") != "app" || r.URL.Query().Get("follow") != "false" {
							t.Error("unbounded or wrong-container logs", r.URL)
						}
						if condition == "denied" && name == "pod-00" {
							apiError(w, 403, "Forbidden")
							return
						}
						fmt.Fprint(w, "hello\n")
						return
					}
					pod, ok := byName[name]
					if !ok {
						t.Errorf("unexpected read %s", r.URL)
						w.WriteHeader(404)
						return
					}
					pod = *pod.DeepCopy()
					if name == "pod-00" {
						n := podReads.Add(1)
						if condition == "replaced-before" || condition == "replaced-after" && n > 1 {
							pod.SetUID("replacement")
						}
					}
					reply(w, pod.Object)
				}
			})
			projector, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: "logs", Resource: "statefulsets.apps", Name: "web", Container: "app"}
			if condition == "byte-limit" {
				command.MaxBytes = 3
			}
			got, err := projector.Execute(t.Context(), command)
			if err != nil || got.Outcome != "partial" || got.Coverage.Complete {
				t.Fatal(got, err)
			}
			if got.Facts["owned_pods"].String() != fmt.Sprint(count) {
				t.Fatal("ownership count", got)
			}
			wantObserved, wantFailed, wantSkipped := count, 0, 0
			switch condition {
			case "denied", "missing-container", "replaced-before", "replaced-after":
				wantObserved--
				wantFailed = 1
			case "source-limit":
				wantObserved = MaxAgentLogSources
				wantSkipped = 1
			case "byte-limit":
				wantObserved = 1
				wantSkipped = count - 1
			}
			for key, want := range map[string]int{"sources_observed": wantObserved, "sources_failed": wantFailed, "sources_skipped": wantSkipped} {
				if got.Facts[key].String() != fmt.Sprint(want) {
					t.Fatalf("%s: %s want %d", key, got.Facts[key].String(), want)
				}
			}
			if wantFailed > 0 && got.Rows[0].Facts["excerpt"].Text != nil {
				t.Fatal("failed identity leaked log excerpt", got.Rows[0])
			}
			if int(logReads.Load()) > MaxAgentLogSources {
				t.Fatal("exceeded log source limit")
			}
			if condition == "byte-limit" && got.Rows[0].Facts["excerpt"].String() != "hel" {
				t.Fatal("byte limit ignored", got)
			}
		})
	}
}

package monitor

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func usageDiscovery(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1":
		agentTestDiscovery(w)
	case "/apis/metrics.k8s.io":
		reply(w, map[string]any{"preferredVersion": map[string]any{"groupVersion": "metrics.k8s.io/v1beta1"}})
	case "/apis/metrics.k8s.io/v1beta1":
		reply(w, map[string]any{"resources": []any{map[string]any{"name": "pods", "kind": "PodMetrics", "namespaced": true, "verbs": []string{"get", "list"}}}})
	default:
		return false
	}
	return true
}

func TestUsageProjectionMatchesCurrentPodAndBudgets(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second)
	pod := logPod("web")
	pod.Object["spec"].(map[string]any)["containers"] = []any{map[string]any{"name": "app", "resources": map[string]any{"requests": map[string]any{"cpu": "250m"}, "limits": map[string]any{"memory": "128Mi"}}}}
	metrics := usageFixture("web", "125m", "10Mi", now)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if usageDiscovery(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/v1/namespaces/shop/pods/web":
			reply(w, pod.Object)
		case "/apis/metrics.k8s.io/v1beta1/namespaces/shop/pods/web":
			reply(w, metrics.Object)
		default:
			t.Errorf("unexpected acquisition %s", r.URL)
			w.WriteHeader(404)
		}
	})
	executor, _ := NewProjector(client, agentTestScope())
	got, err := executor.Execute(t.Context(), Command{Operation: "usage", Resource: "pods", Name: "web"})
	if err != nil || !got.Coverage.Complete || len(got.Rows) != 1 {
		t.Fatal(got, err)
	}
	facts := got.Rows[0].Facts
	for key, want := range map[string]int64{"cpu_nanocores": 125000000, "memory_bytes": 10485760, "container.app.requests.cpu_nanocores": 250000000, "container.app.limits.memory_bytes": 134217728} {
		if facts[key].Number == nil || *facts[key].Number != want {
			t.Fatalf("%s: %v", key, facts[key])
		}
	}
	if !facts["container.app.limits.cpu_nanocores"].Unknown || got.Rows[0].Identity.UID != string(pod.GetUID()) {
		t.Fatal(facts, got.Rows[0].Identity)
	}
	r := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	sample, err := client.ObjectUsage(t.Context(), r, &pod, []kubecommon.Resource{{Group: "metrics.k8s.io", Version: "v1beta1", Name: "pods", Kind: "PodMetrics", Namespaced: true}}, got.ObservedAt)
	if err != nil || sample.CPU != .125 || sample.Memory != 10485760 || !sample.Coverage.Complete {
		t.Fatal(sample, err)
	}
	bars := UsageBudgets(r, pod, sample)[0].Bars
	if *bars[0].Value != .125 || *bars[0].Total != .25 || bars[1].Total != nil {
		t.Fatal(bars)
	}
}

func TestUsageProjectionUnavailableNeverReportsZero(t *testing.T) {
	for _, kind := range []string{"stale", "wrong-container", "wrong-kind", "denied"} {
		t.Run(kind, func(t *testing.T) {
			sample := usageFixture("web", "0", "0", time.Now().Add(-time.Second))
			switch kind {
			case "stale":
				sample.Object["timestamp"] = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
			case "wrong-container":
				sample.Object["containers"].([]any)[0].(map[string]any)["name"] = "other"
			case "wrong-kind":
				sample.SetKind("Secret")
			}
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if usageDiscovery(w, r) {
					return
				}
				if strings.HasPrefix(r.URL.Path, "/api/") {
					reply(w, logPod("web").Object)
					return
				}
				if kind == "denied" {
					apiError(w, 403, "Forbidden")
					return
				}
				reply(w, sample.Object)
			})
			executor, _ := NewProjector(client, agentTestScope())
			got, err := executor.Execute(t.Context(), Command{Operation: "usage", Resource: "pods", Name: "web"})
			if got.Coverage.Complete || got.Outcome == "completed" {
				t.Fatal(got, err)
			}
			if len(got.Rows) > 0 && !got.Rows[0].Facts["cpu_nanocores"].Unknown {
				t.Fatal("unavailable sample became zero", got)
			}
			if err != nil && kind != "wrong-kind" {
				t.Fatal(err)
			}
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUsageScopeDeniedBeforeAcquisition(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	executor, _ := NewProjector(client, agentTestScope())
	for _, c := range []Command{{Operation: "usage", Resource: "secrets", Name: "web"}, {Operation: "usage", Resource: "pods"}, {Operation: "usage", Resource: "pods", Name: "web", Namespace: "other"}} {
		if _, err := executor.Execute(t.Context(), c); err == nil {
			t.Fatal("allowed", c)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("denials contacted API")
	}
}

func TestQuotaFactsAndBarsDistinguishConfiguredFromReported(t *testing.T) {
	q := summaryFixture(t, `{"spec":{"hard":{"requests.cpu":"4","pods":"10"}},"status":{"hard":{"requests.cpu":"2"},"used":{"requests.cpu":"1500m","pods":"0"}}}`)
	facts := map[string]observation.Value{}
	projectQuotaBudgets(q.Object, facts)
	if *facts["quota.requests.cpu.hard"].Number != 2000000000 || *facts["quota.requests.cpu.used"].Number != 1500000000 || !facts["quota.pods.hard"].Unknown || *facts["quota.pods.used"].Number != 0 {
		t.Fatal(facts)
	}
	bars := StatusCharts(kubecommon.Resource{Name: "resourcequotas"}, q)[0].Bars
	if bars[0].Total != nil || *bars[0].Value != 0 || *bars[1].Total != 2 || *bars[1].Value != 1.5 {
		t.Fatal(bars)
	}
}

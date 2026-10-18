package monitor

import (
	"context"
	"math"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func usageFixture(name, cpu, memory string, at time.Time) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1", "kind": "PodMetrics",
		"metadata":  map[string]any{"name": name, "namespace": "shop"},
		"timestamp": at.Format(time.RFC3339Nano), "window": "15s",
		"containers": []any{map[string]any{"name": "app", "usage": map[string]any{"cpu": cpu, "memory": memory}}},
	}}
}

func TestUsageQuantitiesFreshnessAndReplacement(t *testing.T) {
	now := time.Now()
	object := usageFixture("one", "125000000n", "1024Ki", now)
	containers := object.Object["containers"].([]any)
	object.Object["containers"] = append(containers, map[string]any{"name": "proxy", "usage": map[string]any{"cpu": "75m", "memory": "2Mi"}})
	u, err := parseUsage(object, false)
	if err != nil || math.Abs(u.CPU-.2) > 1e-9 || u.Memory != 3*1024*1024 {
		t.Fatalf("usage=%+v err=%v", u, err)
	}
	pod := logPod("one")
	if !u.Matches(pod, now) {
		t.Fatal("fresh matching sample rejected")
	}
	pod.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Second)))
	if u.Matches(pod, now) {
		t.Fatal("sample window predates the replacement Pod")
	}
	pod.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Hour)))
	u.UID = "different"
	if u.Matches(pod, now) {
		t.Fatal("wrong UID accepted")
	}
	u.UID = ""
	if u.Matches(pod, now.Add(UsageMaxAge+time.Second)) || u.Matches(pod, now.Add(-time.Minute)) {
		t.Fatal("stale or future sample accepted")
	}
	u.Containers = []string{"proxy"}
	if u.Matches(pod, now) {
		t.Fatal("missing running container accepted")
	}
	for _, quantity := range []string{"", "bad", "-1", "1e9999"} {
		if _, err := parseUsage(usageFixture("one", quantity, "1Mi", now), false); err == nil {
			t.Errorf("accepted invalid CPU %q", quantity)
		}
	}
}

func TestUsageListPaginatesAndPreservesMissingSamples(t *testing.T) {
	now := time.Now()
	pages := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/apis/metrics.k8s.io/v1/namespaces/shop/pods" || r.URL.Query().Get("limit") != "500" {
			t.Errorf("unexpected metrics request %s %s", r.Method, r.URL)
		}
		pages++
		page := objectList("PodMetrics", "", usageFixture("one", "100m", "10Mi", now).Object, usageFixture("missing", "", "10Mi", now).Object)
		page["apiVersion"] = "metrics.k8s.io/v1"
		if r.URL.Query().Get("continue") == "" {
			page["metadata"].(map[string]any)["continue"] = "next"
		} else {
			page["items"] = []any{usageFixture("two", "0", "0", now).Object}
		}
		reply(w, page)
	})
	metrics := kubecommon.Resource{Group: "metrics.k8s.io", Version: "v1", Name: "pods", Namespaced: true}
	samples, err := client.UsageList(context.Background(), metrics, "shop")
	if err != nil || pages != 2 || len(samples) != 2 || samples["shop/one"].CPU != .1 {
		t.Fatalf("samples=%+v pages=%d err=%v", samples, pages, err)
	}
	columns, rows := Table(kubecommon.Resource{Name: "pods", Namespaced: true}, Collection{Objects: []unstructured.Unstructured{logPod("one"), logPod("two"), logPod("missing")}, Usage: samples}, now)
	cpu := slices.Index(columns, "CPU")
	if rows[0].Cells[cpu] != "100.0m" || rows[1].Cells[cpu] != "0.0m" || rows[2].Cells[cpu] != "N/A" {
		t.Fatal(rows)
	}
	columns, rows = Table(kubecommon.Resource{Name: "pods"}, Collection{Objects: []unstructured.Unstructured{logPod("one")}, Usage: samples}, now.Add(2*time.Minute))
	if rows[0].Cells[slices.Index(columns, "CPU")] != "N/A" {
		t.Fatal("stale metrics still displayed")
	}
	if slices.Index(columns, "Age") != 6 || slices.Index(columns, "CPU") != 7 {
		t.Fatal("usage columns changed an existing saved sort index")
	}
}

func TestObjectUsageFollowsOwnersAndRejectsPartialTotals(t *testing.T) {
	now := time.Now()
	deployment := summaryFixture(t, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"shop","uid":"deployment"}}`)
	rs := summaryFixture(t, `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"web-rs","namespace":"shop","uid":"rs","ownerReferences":[{"apiVersion":"apps/v1","kind":"Deployment","name":"web","uid":"deployment","controller":true}]}}`)
	owned := logPod("one")
	owned.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-rs", UID: "rs", Controller: boolPointer(true)}})
	second := owned.DeepCopy()
	second.SetName("two")
	second.SetUID("two-uid")
	unrelated := logPod("unrelated")
	completed := owned.DeepCopy()
	completed.SetName("completed")
	completed.Object["status"].(map[string]any)["phase"] = "Succeeded"
	resources := []kubecommon.Resource{
		{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true},
		{Group: "apps", Version: "v1", Name: "replicasets", Kind: "ReplicaSet", Namespaced: true},
		{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true},
		{Group: "metrics.k8s.io", Version: "v1beta1", Name: "pods", Kind: "PodMetrics", Namespaced: true},
	}
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial"}[partial], func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("unexpected write %s", r.Method)
				}
				switch r.URL.Path {
				case "/apis/apps/v1/namespaces/shop/deployments/web":
					reply(w, deployment.Object)
				case "/apis/apps/v1/namespaces/shop/replicasets":
					reply(w, objectList("ReplicaSet", "1", rs.Object))
				case "/api/v1/namespaces/shop/pods":
					reply(w, objectList("Pod", "1", owned.Object, second.Object, unrelated.Object, completed.Object))
				case "/apis/metrics.k8s.io/v1beta1/namespaces/shop/pods":
					items := []map[string]any{usageFixture("one", "100m", "10Mi", now).Object, usageFixture("unrelated", "9", "9Gi", now).Object}
					if !partial {
						items = append(items, usageFixture("two", "200m", "20Mi", now).Object)
					}
					page := objectList("PodMetrics", "1", items...)
					page["apiVersion"] = "metrics.k8s.io/v1beta1"
					reply(w, page)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					apiError(w, 404, "NotFound")
				}
			})
			u, err := client.ObjectUsage(context.Background(), resources[0], &deployment, resources, now)
			projector, _ := NewProjector(client, agentTestScope())
			guardedResources := slices.Clone(resources)
			for i := range guardedResources {
				guardedResources[i].Verbs = []string{"get", "list"}
			}
			guarded, guardedErr := objectUsage(t.Context(), &projectedObjectReader{projector: projector}, guardedResources[0], &deployment, guardedResources, now)
			if !reflect.DeepEqual(u, guarded) || (err == nil) != (guardedErr == nil) {
				t.Fatalf("UI and guarded measurement acquisition differ: %+v / %+v, %v / %v", u, guarded, err, guardedErr)
			}

			if partial {
				if err == nil || !u.Timestamp.IsZero() || u.Coverage.Observed != 1 || u.Expected != 2 {
					t.Fatalf("incomplete total presented as valid: %+v %v", u, err)
				}
			} else if err != nil || math.Abs(u.CPU-.3) > 1e-9 || u.Memory != 30*1024*1024 || u.Coverage.Observed != 2 || !u.Coverage.Complete {
				t.Fatalf("incorrect aggregate: %+v %v", u, err)
			}
		})
	}
}

func TestNodeUsageAndDeniedMetrics(t *testing.T) {
	now := time.Now()
	node := summaryFixture(t, `{"apiVersion":"v1","kind":"Node","metadata":{"name":"worker","uid":"node"}}`)
	r := kubecommon.Resource{Name: "nodes", Version: "v1", Kind: "Node"}
	metrics := kubecommon.Resource{Group: "metrics.k8s.io", Version: "v1beta1", Name: "nodes", Kind: "NodeMetrics"}
	for _, denied := range []bool{false, true} {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/nodes/worker" {
				reply(w, node.Object)
				return
			}
			if r.URL.Path != "/apis/metrics.k8s.io/v1beta1/nodes/worker" {
				t.Errorf("unexpected request %s", r.URL)
			}
			if denied {
				apiError(w, 403, "Forbidden")
				return
			}
			body := usageFixture("worker", "1", "1Gi", now)
			body.SetNamespace("")
			body.Object["usage"] = map[string]any{"cpu": "2", "memory": "1Gi"}
			reply(w, body.Object)
		})
		u, err := client.ObjectUsage(context.Background(), r, &node, []kubecommon.Resource{metrics}, now)
		if denied {
			if err == nil || !strings.Contains(UsageErrorText(err), "RBAC") || strings.Contains(UsageErrorText(err), "server-private-value") {
				t.Fatal(err)
			}
		} else if err != nil || u.CPU != 2 || u.Memory != 1024*1024*1024 {
			t.Fatalf("%+v %v", u, err)
		}
	}
	if HasUsage(kubecommon.Resource{Name: "pods", Group: "example.io"}) || HasUsage(kubecommon.Resource{Name: "services"}) {
		t.Fatal("unrelated resource inherited usage")
	}
}

func boolPointer(value bool) *bool { return &value }

func TestObjectUsageIdentifiesChangedAndRepeatedSnapshots(t *testing.T) {
	now := time.Now()
	workload := summaryFixture(t, `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"web","namespace":"shop","uid":"rs"}}`)
	one := logPod("one")
	one.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web", UID: "rs", Controller: boolPointer(true)}})
	two := one.DeepCopy()
	two.SetName("two")
	two.SetUID("two")
	resources := []kubecommon.Resource{
		{Group: "apps", Version: "v1", Name: "replicasets", Kind: "ReplicaSet", Namespaced: true},
		{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true},
		{Group: "metrics.k8s.io", Version: "v1beta1", Name: "pods", Kind: "PodMetrics", Namespaced: true},
	}
	round := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/shop/replicasets/web":
			reply(w, workload.Object)
		case "/api/v1/namespaces/shop/pods":
			reply(w, objectList("Pod", "1", one.Object, two.Object))
		case "/apis/metrics.k8s.io/v1beta1/namespaces/shop/pods":
			cpu, at := "200m", now.Add(-15*time.Second)
			if round > 0 {
				cpu, at = "1", now
			}
			page := objectList("PodMetrics", "1", usageFixture("one", "100m", "1Mi", now.Add(-30*time.Second)).Object, usageFixture("two", cpu, "1Mi", at).Object)
			page["apiVersion"] = "metrics.k8s.io/v1beta1"
			reply(w, page)
		default:
			t.Errorf("unexpected path %s", r.URL)
			apiError(w, 404, "NotFound")
		}
	})
	first, err := client.ObjectUsage(context.Background(), resources[0], &workload, resources, now)
	if err != nil {
		t.Fatal(err)
	}
	round++
	second, err := client.ObjectUsage(context.Background(), resources[0], &workload, resources, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first=%vm second=%vm same timestamp=%v", first.CPU*1000, second.CPU*1000, first.Timestamp.Equal(second.Timestamp))
	if first.Snapshot == "" || first.Snapshot == second.Snapshot || !second.Collected.After(first.Collected) || !first.Timestamp.Equal(second.Timestamp) {
		t.Fatal("changed Pod measurements lost their snapshot identity", first, second)
	}
	if !second.SourceEnd.Equal(now) {
		t.Fatal("missing source time range", second)
	}
	repeated, err := client.ObjectUsage(context.Background(), resources[0], &workload, resources, now.Add(2*time.Second))
	if err != nil || repeated.Snapshot != second.Snapshot {
		t.Fatal("unchanged constituent samples must deduplicate", repeated, err)
	}
	if second.Fresh(now.Add(UsageMaxAge)) {
		t.Fatal("collection time masked stale constituent metrics")
	}
}

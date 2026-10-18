package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type overviewFixture struct {
	collections map[string]Collection
	denied      map[string]bool
	metrics     map[string]Usage
}

func (f overviewFixture) List(_ context.Context, r kubecommon.Resource, ns, selector string) (Collection, error) {
	if ns != "" || selector != "" {
		panic("Overview broadened or filtered its declared cluster population")
	}
	if f.denied[r.Key()] {
		return Collection{}, errors.New("denied")
	}
	return f.collections[r.Key()], nil
}
func (f overviewFixture) UsageList(context.Context, kubecommon.Resource, string) (map[string]Usage, error) {
	return f.metrics, nil
}

func readOverview(ctx context.Context, reader overviewReader, catalog []kubecommon.Resource) Overview {
	result := readOverviewInventory(ctx, reader, catalog)
	readOverviewUsage(ctx, reader, catalog).Apply(&result)
	return result
}
func overviewCatalog() []kubecommon.Resource {
	return []kubecommon.Resource{
		{Version: "v1", Name: "nodes", Kind: "Node"}, {Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true},
		{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true},
		{Group: "apps", Version: "v1", Name: "statefulsets", Kind: "StatefulSet", Namespaced: true},
		{Group: "apps", Version: "v1", Name: "daemonsets", Kind: "DaemonSet", Namespaced: true},
		{Group: "autoscaling", Version: "v2", Name: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler", Namespaced: true},
		{Group: "metrics.k8s.io", Version: "v1beta1", Name: "nodes", Kind: "NodeMetrics"},
	}
}
func TestOverviewSeparatesUsageCapacityAndDeclaredResources(t *testing.T) {
	now := time.Now()
	node := summaryFixture(t, `{"apiVersion":"v1","kind":"Node","metadata":{"name":"n1"},"status":{"allocatable":{"cpu":"4","memory":"8Gi"},"conditions":[{"type":"Ready","status":"True"}]}}`)
	pod := summaryFixture(t, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"web","namespace":"shop"},"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"500m","memory":"1Gi"},"limits":{"cpu":"2","memory":"2Gi"}}}]},"status":{"phase":"Running"}}`)
	completed := *pod.DeepCopy()
	completed.Object["status"] = map[string]any{"phase": "Succeeded"}
	f := overviewFixture{collections: map[string]Collection{"nodes": {Objects: []unstructured.Unstructured{node}}, "pods": {Objects: []unstructured.Unstructured{pod, completed}}}, metrics: map[string]Usage{"/n1": {CPU: 1, Memory: 2 << 30, Timestamp: now, Window: 15 * time.Second}}}
	o := readOverview(context.Background(), f, overviewCatalog())
	if o.CPU.Used == nil || *o.CPU.Used != 1 || *o.CPU.Allocatable != 4 || *o.CPU.Requests != .5 || *o.CPU.Limits != 2 {
		t.Fatalf("mixed measurement bases: %+v", o.CPU)
	}
	if o.ActivePods != 1 || o.NodesReady != 1 || o.PodsRunning != 1 || o.MetricsObserved != 1 {
		t.Fatalf("bad populations: %+v", o)
	}
	withoutMetrics := readOverview(context.Background(), f, overviewCatalog()[:6])
	if withoutMetrics.CPU.Used != nil || withoutMetrics.CPU.Allocatable == nil || *withoutMetrics.CPU.Allocatable != 4 || withoutMetrics.MetricsError == "" {
		t.Fatal("missing metrics erased readable capacity or became measured consumption", withoutMetrics.CPU)
	}
	f.metrics["/n1"] = Usage{CPU: 0, Memory: 0, Timestamp: now, Window: 15 * time.Second}
	o = readOverview(context.Background(), f, overviewCatalog())
	if o.CPU.Used == nil || *o.CPU.Used != 0 {
		t.Fatal("measured zero must remain zero")
	}
	for _, at := range []time.Time{now.Add(-5 * time.Minute), now.Add(time.Minute)} {
		f.metrics["/n1"] = Usage{CPU: 1, Memory: 1, Timestamp: at}
		o = readOverview(context.Background(), f, overviewCatalog())
		if o.CPU.Used != nil || o.MetricsError == "" || o.CPU.Requests == nil {
			t.Fatal("invalid metrics became consumption or erased spec data")
		}
	}
	f.metrics = nil
	f.denied = map[string]bool{"pods": true}
	o = readOverview(context.Background(), f, overviewCatalog())
	if o.CPU.Requests != nil || o.CPU.Used != nil || o.Reads["pods"].Error == "" {
		t.Fatal("missing data became zero")
	}
	f.denied = nil
	f.collections["nodes"] = Collection{Objects: []unstructured.Unstructured{node}, Truncated: true}
	f.metrics = map[string]Usage{"/n1": {CPU: 1, Memory: 1, Timestamp: now}}
	o = readOverview(context.Background(), f, overviewCatalog())
	if o.CPU.Used != nil || o.CPU.Allocatable != nil || !o.Reads["nodes"].Partial {
		t.Fatal("partial nodes became a cluster total")
	}
}
func TestOverviewLinksWarningsAndUsesControllerTimes(t *testing.T) {
	pod := summaryFixture(t, `{"metadata":{"name":"web","namespace":"shop","uid":"pod-1"},"status":{"phase":"Running","containerStatuses":[{"name":"web","state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`)
	deploy := summaryFixture(t, `{"metadata":{"name":"web","namespace":"shop","uid":"deploy-1","annotations":{"deployment.kubernetes.io/revision":"4"}},"status":{"conditions":[{"type":"Progressing","status":"False","reason":"ProgressDeadlineExceeded","lastUpdateTime":"2026-10-13T12:00:00Z"}]}}`)
	f := overviewFixture{collections: map[string]Collection{"pods": {Objects: []unstructured.Unstructured{pod}}, "deployments.apps": {Objects: []unstructured.Unstructured{deploy}}}}
	o := readOverview(context.Background(), f, overviewCatalog())
	if len(o.Changes) != 1 || o.Changes[0].Revision != "4" || o.Changes[0].At.UTC().Format(time.RFC3339) != "2026-10-13T12:00:00Z" {
		t.Fatalf("fabricated or lost rollout evidence: %+v", o.Changes)
	}
	if len(o.Attention) < 2 {
		t.Fatal(o.Attention)
	}
	for _, entry := range o.Attention {
		if entry.Name != "web" || entry.Namespace != "shop" || entry.UID == "" {
			t.Fatal("lost inspector identity", entry)
		}
	}
}

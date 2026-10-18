package monitor

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestOverviewRestartChangesRequireComparableContainers(t *testing.T) {
	pod := summaryFixture(t, `{"metadata":{"name":"web","namespace":"shop","uid":"pod-1"},"spec":{"containers":[{"name":"web"}],"initContainers":[{"name":"setup"}]},"status":{"phase":"Running","containerStatuses":[{"name":"web","restartCount":2}],"initContainerStatuses":[{"name":"setup","restartCount":4}]}}`)
	f := overviewFixture{collections: map[string]Collection{"pods": {Objects: []unstructured.Unstructured{pod}}}}
	before := readOverview(context.Background(), f, overviewCatalog())
	for _, tc := range []struct {
		name   string
		mutate func(*unstructured.Unstructured, *overviewFixture)
		want   *int64
	}{
		{"same containers", func(p *unstructured.Unstructured, _ *overviewFixture) {
			p.Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)["restartCount"] = int64(5)
		}, new(int64(3))},
		{"replacement Pod", func(p *unstructured.Unstructured, _ *overviewFixture) { p.SetUID("pod-2") }, nil},
		{"counter reset", func(p *unstructured.Unstructured, _ *overviewFixture) {
			p.Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)["restartCount"] = int64(1)
		}, nil},
		{"missing status", func(p *unstructured.Unstructured, _ *overviewFixture) {
			delete(p.Object["status"].(map[string]any), "containerStatuses")
		}, nil},
		{"container removed", func(p *unstructured.Unstructured, _ *overviewFixture) {
			delete(p.Object["spec"].(map[string]any), "initContainers")
		}, nil},
		{"partial inventory", func(_ *unstructured.Unstructured, f *overviewFixture) {
			c := f.collections["pods"]
			c.Truncated = true
			f.collections["pods"] = c
		}, nil},
		{"denied inventory", func(_ *unstructured.Unstructured, f *overviewFixture) { f.denied = map[string]bool{"pods": true} }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := *pod.DeepCopy()
			fixture := overviewFixture{collections: map[string]Collection{"pods": {Objects: []unstructured.Unstructured{current}}}}
			tc.mutate(&current, &fixture)
			c := fixture.collections["pods"]
			c.Objects[0] = current
			fixture.collections["pods"] = c
			after := readOverview(context.Background(), fixture, overviewCatalog())
			r := after.Reads["pods"]
			r.ReadAt = before.Reads["pods"].ReadAt.Add(15 * time.Second)
			after.Reads["pods"] = r
			got := CompareOverview(before, after).Restarts
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Fatalf("restart change=%v, want %v", got, tc.want)
			}
		})
	}
	after := before
	after.Reads = map[string]OverviewRead{"pods": before.Reads["pods"]}
	for _, gap := range []time.Duration{0, -time.Second, 46 * time.Second} {
		r := before.Reads["pods"]
		r.ReadAt = r.ReadAt.Add(gap)
		after.Reads["pods"] = r
		if CompareOverview(before, after).Restarts != nil {
			t.Fatalf("compared invalid interval %s", gap)
		}
	}
}

func TestOverviewReplicaChangesUseHPAIdentity(t *testing.T) {
	hpa := summaryFixture(t, `{"metadata":{"name":"web","uid":"hpa-1"},"status":{"currentReplicas":2,"desiredReplicas":4}}`)
	f := overviewFixture{collections: map[string]Collection{"horizontalpodautoscalers.autoscaling": {Objects: []unstructured.Unstructured{hpa}}}}
	before := readOverview(context.Background(), f, overviewCatalog())
	for _, uid := range []string{"hpa-1", "hpa-2"} {
		current := *hpa.DeepCopy()
		current.SetUID(types.UID(uid))
		current.Object["status"].(map[string]any)["currentReplicas"] = int64(4)
		f.collections["horizontalpodautoscalers.autoscaling"] = Collection{Objects: []unstructured.Unstructured{current}}
		after := readOverview(context.Background(), f, overviewCatalog())
		got := CompareOverview(before, after).Replicas
		if uid == "hpa-1" && (got == nil || *got != 2) {
			t.Fatal("lost same-HPA scaling change", got)
		}
		if uid == "hpa-2" && got != nil {
			t.Fatal("new HPA misrepresented as scaling", got)
		}
	}
}

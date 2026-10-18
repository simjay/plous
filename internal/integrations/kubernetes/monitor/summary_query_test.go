package monitor

import (
	"context"
	"net/http"
	"strings"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestDeclaredPodResourcesHandleInitSidecarsOverheadAndPodLevelLimits(t *testing.T) {
	pod := summaryFixture(t, `{"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"500m","memory":"128Mi"},"limits":{"memory":"512Mi"}}}],"initContainers":[{"name":"proxy","restartPolicy":"Always","resources":{"requests":{"cpu":"100m","memory":"32Mi"},"limits":{"memory":"64Mi"}}},{"name":"setup","resources":{"requests":{"cpu":"2","memory":"256Mi"},"limits":{"memory":"1Gi"}}}],"overhead":{"cpu":"50m","memory":"16Mi"}}}`)
	requests, limits := declaredPodResources(pod.Object, "requests"), declaredPodResources(pod.Object, "limits")
	for key, want := range map[string]string{"cpu": "2150m", "memory": "304Mi"} {
		if got := quantityText(requests, key); got != want {
			t.Errorf("requests %s=%s want %s", key, got, want)
		}
	}
	if quantityText(limits, "memory") != "1104Mi" || quantityText(limits, "cpu") != "Not specified" {
		t.Fatal(limits)
	}
	pod.Object["spec"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"cpu": "3"}, "limits": map[string]any{"memory": "2Gi"}}
	if got := quantityText(declaredPodResources(pod.Object, "requests"), "cpu"); got != "3050m" {
		t.Fatal(got)
	}
	if got := quantityText(declaredPodResources(pod.Object, "limits"), "memory"); got != "2064Mi" {
		t.Fatal(got)
	}
}

func TestNodeSummarySupplementDeclaresCoverageAndExcludesCompletedPods(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pods" {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		active := summaryFixture(t, `{"metadata":{"name":"app","namespace":"shop"},"spec":{"nodeName":"worker","containers":[{"resources":{"requests":{"cpu":"200m"}}}]},"status":{"phase":"Running"}}`)
		completed := active.DeepCopy()
		completed.SetName("finished")
		completed.Object["status"].(map[string]any)["phase"] = "Succeeded"
		elsewhere := active.DeepCopy()
		elsewhere.SetName("other-node")
		elsewhere.Object["spec"].(map[string]any)["nodeName"] = "other"
		reply(w, objectList("Pod", "1", active.Object, completed.Object, elsewhere.Object))
	})
	node := summaryFixture(t, `{"metadata":{"name":"worker"},"status":{"allocatable":{"cpu":"4"}}}`)
	sections, warning := client.SummarySupplement(context.Background(), kubecommon.Resource{Name: "nodes"}, node, []kubecommon.Resource{{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}})
	if warning != "" {
		t.Fatal(warning)
	}
	totals := findSummarySection(t, sections, "Declared Pod resources")
	if len(totals.Rows) != 1 || strings.Join(totals.Rows[0], "|") != "cpu|200m|Not specified|4" {
		t.Fatal(totals)
	}
	if coverage := findSummaryField(t, totals, "Coverage"); !strings.Contains(coverage, "1 non-terminal") || !strings.Contains(coverage, "all namespaces") {
		t.Fatal(coverage)
	}
	if pods := findSummarySection(t, sections, "Assigned Pods"); len(pods.Rows) != 2 {
		t.Fatal(pods)
	}
}

func TestSummarySupplementDeniedReadDoesNotBecomeEmptyQuota(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) { apiError(w, 403, "Forbidden") })
	namespace := summaryFixture(t, `{"metadata":{"name":"shop"}}`)
	sections, warning := client.SummarySupplement(context.Background(), kubecommon.Resource{Name: "namespaces"}, namespace, []kubecommon.Resource{{Name: "resourcequotas", Kind: "ResourceQuota", Version: "v1", Namespaced: true}})
	if len(sections) != 0 || !strings.Contains(strings.ToLower(warning), "denied") {
		t.Fatalf("%#v %s", sections, warning)
	}
}

func TestAllocationAndTrustSelectorsUseExactReferences(t *testing.T) {
	claim := summaryFixture(t, `{"status":{"allocation":{"devices":{"results":[{"driver":"gpu.example.io","pool":"node-a","device":"gpu-0"}]}}}}`)
	slice := summaryFixture(t, `{"spec":{"driver":"gpu.example.io","pool":{"name":"node-a"},"devices":[{"name":"gpu-0"}]}}`)
	if !allocationUsesSlice(claim.Object, slice.Object) {
		t.Fatal("exact device not matched")
	}
	slice.Object["spec"].(map[string]any)["driver"] = "unrelated.example.io"
	if allocationUsesSlice(claim.Object, slice.Object) {
		t.Fatal("matched same device name from another driver")
	}
	if selectorMatches(nil, map[string]string{"app": "checkout"}, false) {
		t.Fatal("absent trust-bundle selector matched everything")
	}
	if !selectorMatches(map[string]any{}, map[string]string{"app": "checkout"}, false) {
		t.Fatal("explicit empty selector did not match")
	}
}

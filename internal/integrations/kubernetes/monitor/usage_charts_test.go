package monitor

import (
	"encoding/json"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestUsageContainerBreakdownAndBudgetScopes(t *testing.T) {
	metrics := usageFixture("one", "100m", "2Mi", time.Now())
	metrics.Object["containers"] = append(metrics.Object["containers"].([]any), map[string]any{"name": "proxy", "usage": map[string]any{"cpu": "50m", "memory": "1Mi"}})
	u, err := parseUsage(metrics, false)
	if err != nil || len(u.Breakdown) != 2 || u.Breakdown[0].CPU != .1 || u.Breakdown[1].Memory != 1048576 {
		t.Fatalf("%+v %v", u, err)
	}
	pod := summaryFixture(t, `{"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"200m","memory":"4Mi"},"limits":{"memory":"8Mi"}}},{"name":"proxy"}]}}`)
	sections := UsageBudgets(kubecommon.Resource{Name: "pods"}, pod, u)
	if len(sections) != 1 || len(sections[0].Bars) != 8 {
		t.Fatal(sections)
	}
	bars := sections[0].Bars
	if *bars[0].Value != .1 || *bars[0].Total != .2 || bars[1].Total != nil || !strings.Contains(bars[1].Detail, "not specified") || *bars[3].Total != 8*1048576 {
		t.Fatal(bars)
	}
	if s := UsageBudgets(kubecommon.Resource{Name: "deployments", Group: "apps"}, pod, u); len(s) != 0 {
		t.Fatal("workload template became aggregate budget")
	}
	node := summaryFixture(t, `{"status":{"allocatable":{"cpu":"4","memory":"8Gi"}}}`)
	bars = UsageBudgets(kubecommon.Resource{Name: "nodes"}, node, u)[0].Bars
	if *bars[0].Total != 4 || *bars[1].Total != 8*1024*1024*1024 {
		t.Fatal(bars)
	}
}

func TestUsageReadinessKeepsDesiredSeparateAndMissingStatusUnknown(t *testing.T) {
	r := kubecommon.Resource{Name: "deployments", Group: "apps"}
	object := summaryFixture(t, `{"metadata":{"generation":3},"spec":{"replicas":5},"status":{"replicas":4,"readyReplicas":3,"updatedReplicas":2,"availableReplicas":3,"observedGeneration":2}}`)
	sections := StatusCharts(r, object)
	bar := sections[0].Bars[0]
	if *bar.Value != 3 || *bar.Total != 4 || !strings.Contains(bar.Detail, "1 not ready · desired 5") || len(sections[0].Fields) != 2 {
		t.Fatal(sections)
	}
	if len(StatusCharts(r, summaryFixture(t, `{"spec":{"replicas":5}}`))) != 0 {
		t.Fatal("unknown status became zero")
	}
	zero := StatusCharts(r, summaryFixture(t, `{"status":{"replicas":4}}`))
	if *zero[0].Bars[0].Value != 0 {
		t.Fatal("omitted zero count not handled")
	}
	if len(StatusCharts(kubecommon.Resource{Name: "deployments", Group: "example.io"}, object)) != 0 {
		t.Fatal("custom resource inherited Kubernetes semantics")
	}
}

func TestUsageQuotaUsesReportedHardAndKeepsUnknownValues(t *testing.T) {
	object := summaryFixture(t, `{"spec":{"hard":{"requests.cpu":"10"}},"status":{"hard":{"requests.cpu":"4","pods":"0","requests.memory":"4Gi"},"used":{"requests.cpu":"5","pods":"0","count/jobs.batch":"3"}}}`)
	sections := StatusCharts(kubecommon.Resource{Name: "resourcequotas"}, object)
	seen := map[string]DetailBar{}
	for _, bar := range sections[0].Bars {
		seen[bar.Label] = bar
	}
	if *seen["requests.cpu"].Total != 4 || *seen["requests.cpu"].Value != 5 {
		t.Fatal("configured quota replaced reported accounting", seen)
	}
	if seen["requests.memory"].Value != nil || seen["count/jobs.batch"].Total != nil || *seen["pods"].Total != 0 {
		t.Fatal("missing values were fabricated", seen)
	}
}

func TestUsageZeroDeploymentRetainsReadiness(t *testing.T) {
	desired := int32(3)
	deployment := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "pending", Generation: 1}, Spec: appsv1.DeploymentSpec{Replicas: &desired}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1}}
	data, err := json.Marshal(deployment)
	if err != nil {
		t.Fatal(err)
	}
	object := summaryFixture(t, string(data))
	t.Log(string(data))
	charts := StatusCharts(kubecommon.Resource{Name: "deployments", Group: "apps"}, object)
	if len(charts) == 0 {
		t.Error("observed zero replicas has no readiness summary")
	}
}

package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"testing"
)

func TestOverviewWarnsForPodReadinessGate(t *testing.T) {
	pod := summaryFixture(t, `{"metadata":{"name":"gated","namespace":"shop"},"spec":{"containers":[{"name":"app"}],"readinessGates":[{"conditionType":"example.com/ready"}]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"False","reason":"ReadinessGatesNotReady"}],"containerStatuses":[{"name":"app","ready":true,"state":{"running":{}}}]}}`)
	var o Overview
	o.addObject(kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}, pod)
	if len(o.Attention) == 0 {
		t.Error("Running Pod with Ready=False is absent from Needs Attention")
	}
}

func TestOverviewUsesReportedTerminationAndScalingEvidence(t *testing.T) {
	pod := summaryFixture(t, `{"metadata":{"name":"worker-1","namespace":"shop","ownerReferences":[{"apiVersion":"batch/v1","kind":"Job","name":"weekly","uid":"job-1","controller":true}]},"status":{"phase":"Failed","containerStatuses":[{"name":"worker","ready":false,"state":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-10-13T12:00:00Z"}}}]}}`)
	var o Overview
	resource := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	o.addObject(resource, pod)
	if len(o.Attention) != 1 || o.Attention[0].At.IsZero() || o.Attention[0].GroupKey != "shop/job-1/OOMKilled" || o.Attention[0].Severity != 2 {
		t.Fatal(o.Attention)
	}
	done := summaryFixture(t, `{"metadata":{"name":"finished"},"status":{"phase":"Succeeded"}}`)
	o.addObject(resource, done)
	if o.PodsSucceeded != 1 || o.PodsFailed != 1 || len(o.Attention) != 1 {
		t.Fatal("completed Pods corrupted accounting or became failures", o)
	}
	hpa := summaryFixture(t, `{"metadata":{"name":"web"},"status":{"currentReplicas":3,"desiredReplicas":5,"lastScaleTime":"2026-10-13T12:10:00Z"}}`)
	o.addObject(kubecommon.Resource{Group: "autoscaling", Version: "v2", Name: "horizontalpodautoscalers"}, hpa)
	if o.HPAUp != 1 || o.HPACurrent != 3 || o.HPADesired != 5 || o.HPALastScale.IsZero() {
		t.Fatal("scaling evidence lost", o)
	}
}

func TestOverviewNodePressureCountsNodesOnce(t *testing.T) {
	node := summaryFixture(t, `{"metadata":{"name":"worker"},"spec":{"unschedulable":true},"status":{"conditions":[{"type":"Ready","status":"True"},{"type":"DiskPressure","status":"True"},{"type":"MemoryPressure","status":"True"}]}}`)
	var o Overview
	o.addObject(kubecommon.Resource{Version: "v1", Name: "nodes", Kind: "Node"}, node)
	if o.NodesReady != 1 || o.NodesCordoned != 1 || o.NodesPressure != 1 {
		t.Fatal("node conditions counted incorrectly", o)
	}
}

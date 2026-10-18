package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCustomResourceWithNativeNameUsesGenericPresentation(t *testing.T) {
	r := kubecommon.Resource{Group: "example.io", Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	object := unstructured.Unstructured{Object: testPod("custom", "custom")}
	columns, rows := Table(r, Collection{Objects: []unstructured.Unstructured{object}}, time.Now())
	if HasPresenter(r) || len(columns) != 4 || len(rows[0].Cells) != 4 || strings.Contains(summaryText(r, object), "Pod IP:") {
		t.Fatal("custom resource inherited core Pod fields")
	}
	var client *Client
	references, warning := client.Related(context.Background(), r, object, []kubecommon.Resource{{Name: "nodes", Kind: "Node"}})
	if len(references) != 0 || warning != "" {
		t.Fatal("custom resource inherited core Pod relationships")
	}
}

func TestPodStatePrioritizesInitFailureOverContainerWaiting(t *testing.T) {
	o := testPod("checkout", "pod")
	o["spec"].(map[string]any)["initContainers"] = []any{map[string]any{"name": "migrate"}}
	o["status"] = map[string]any{
		"phase":                 "Pending",
		"initContainerStatuses": []any{map[string]any{"name": "migrate", "restartCount": int64(2), "state": map[string]any{"terminated": map[string]any{"exitCode": int64(1), "reason": "Error"}}}},
		"containerStatuses":     []any{map[string]any{"name": "app", "state": map[string]any{"waiting": map[string]any{"reason": "PodInitializing"}}}},
	}
	ready, total, restarts, status := PodState(o)
	if ready != 0 || total != 1 || restarts != 2 || status != "Init:Error" {
		t.Fatalf("ready=%d/%d restarts=%d status=%s", ready, total, restarts, status)
	}
	o["metadata"].(map[string]any)["deletionTimestamp"] = "2026-09-29T12:00:00Z"
	_, _, _, status = PodState(o)
	if status != "Terminating" {
		t.Fatal("deletion did not override previous state")
	}
}

func TestPodStateIncludesRestartableInitContainersInReadiness(t *testing.T) {
	o := testPod("checkout", "pod")
	o["spec"].(map[string]any)["initContainers"] = []any{map[string]any{"name": "proxy", "restartPolicy": "Always"}}
	o["status"] = map[string]any{
		"phase":                 "Running",
		"initContainerStatuses": []any{map[string]any{"name": "proxy", "ready": true, "started": true, "restartCount": int64(1)}},
		"containerStatuses":     []any{map[string]any{"name": "app", "ready": true, "restartCount": int64(2)}},
	}
	ready, total, restarts, status := PodState(o)
	if ready != 2 || total != 2 || restarts != 3 || status != "Running" {
		t.Fatalf("ready=%d/%d restarts=%d status=%s", ready, total, restarts, status)
	}
}

package kubernetes

import (
	"encoding/json"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func progressWorkload(t *testing.T, kind, name, group string) (Resource, *unstructured.Unstructured) {
	t.Helper()
	r := Resource{Name: name, Kind: kind, Group: group, Version: "v1", Namespaced: true}
	o := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"sample","namespace":"test","uid":"original","generation":2},"spec":{"replicas":3,"template":{"metadata":{"annotations":{"restart":"now"}}}},"status":{"observedGeneration":2,"replicas":3,"readyReplicas":3,"updatedReplicas":3,"availableReplicas":3}}`), o); err != nil {
		t.Fatal(err)
	}
	return r, o
}

func TestOperationProgressRequiresObservedGenerationAndActualConvergence(t *testing.T) {
	r, o := progressWorkload(t, "Deployment", "deployments", "apps")
	target := NewOperationTarget(r, "restart", o, o, 0)
	cases := []struct {
		name, field string
		value       int64
		summary     string
	}{
		{"old status", "observedGeneration", 1, "Waiting for the controller"},
		{"new replicas", "updatedReplicas", 2, "Rollout in progress"},
		{"old replicas", "replicas", 4, "Rollout in progress"},
		{"availability", "availableReplicas", 2, "Rollout in progress"},
		{"readiness", "readyReplicas", 2, "Rollout in progress"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			current := o.DeepCopy()
			_ = unstructured.SetNestedField(current.Object, tt.value, "status", tt.field)
			p := target.Progress(current, nil)
			if !p.Watching() || p.Summary != tt.summary {
				t.Fatalf("%+v", p)
			}
		})
	}
	if p := target.Progress(o, nil); p.State != "complete" {
		t.Fatalf("%+v", p)
	}
	current := o.DeepCopy()
	current.SetGeneration(3)
	if p := target.Progress(current, nil); p.State == "complete" {
		t.Fatal("new generation used old status")
	}
}

func TestOperationProgressReportsFailureReplacementAndSupersession(t *testing.T) {
	r, o := progressWorkload(t, "Deployment", "deployments", "apps")
	target := NewOperationTarget(r, "restart", o, o, 0)
	_ = unstructured.SetNestedSlice(o.Object, []any{map[string]any{"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded", "message": "new ReplicaSet unavailable"}}, "status", "conditions")
	p := target.Progress(o, nil)
	if p.State != "attention" || !strings.Contains(p.Details, "ProgressDeadlineExceeded") {
		t.Fatalf("%+v", p)
	}
	o.SetUID("replacement")
	if p := target.Progress(o, nil); p.Summary != "Object replaced" {
		t.Fatalf("%+v", p)
	}
	o.SetUID("original")
	_ = unstructured.SetNestedField(o.Object, int64(4), "spec", "replicas")
	if p := target.Progress(o, nil); p.Summary != "Desired state changed again" {
		t.Fatalf("%+v", p)
	}
	p = target.Progress(nil, apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "sample", nil))
	if p.State != "unavailable" || p.Watching() {
		t.Fatalf("%+v", p)
	}
}

func TestScaleProgressIncludingZeroAndUnsupportedCustomReadiness(t *testing.T) {
	for _, resource := range []struct{ name, kind, group string }{{"deployments", "Deployment", "apps"}, {"statefulsets", "StatefulSet", "apps"}, {"replicasets", "ReplicaSet", "apps"}, {"replicationcontrollers", "ReplicationController", ""}} {
		t.Run(resource.name, func(t *testing.T) {
			r, o := progressWorkload(t, resource.kind, resource.name, resource.group)
			target := NewOperationTarget(r, "scale", o, nil, 0)
			_ = unstructured.SetNestedField(o.Object, int64(0), "spec", "replicas")
			if p := target.Progress(o, nil); !p.Watching() {
				t.Fatalf("old replicas: %+v", p)
			}
			for _, field := range []string{"replicas", "readyReplicas"} {
				_ = unstructured.SetNestedField(o.Object, int64(0), "status", field)
			}
			if p := target.Progress(o, nil); p.State != "complete" {
				t.Fatalf("zero: %+v", p)
			}
		})
	}
	r, o := progressWorkload(t, "Deployment", "deployments", "example.io")
	unstructured.RemoveNestedField(o.Object, "spec", "replicas")
	_ = unstructured.SetNestedField(o.Object, int64(3), "spec", "workers")
	if p := NewOperationTarget(r, "scale", o, nil, 3).Progress(o, nil); p.State != "attention" {
		t.Fatalf("invented custom health: %+v", p)
	}
}

func TestStatefulSetProgressRespectsRevisionsPartitionAndOnDelete(t *testing.T) {
	r, o := progressWorkload(t, "StatefulSet", "statefulsets", "apps")
	_ = unstructured.SetNestedField(o.Object, "RollingUpdate", "spec", "updateStrategy", "type")
	_ = unstructured.SetNestedField(o.Object, "new", "status", "updateRevision")
	_ = unstructured.SetNestedField(o.Object, "old", "status", "currentRevision")
	target := NewOperationTarget(r, "restart", o, o, 0)
	if p := target.Progress(o, nil); !p.Watching() {
		t.Fatalf("old revision: %+v", p)
	}
	_ = unstructured.SetNestedField(o.Object, "new", "status", "currentRevision")
	if p := target.Progress(o, nil); p.State != "complete" {
		t.Fatalf("%+v", p)
	}
	_ = unstructured.SetNestedField(o.Object, int64(2), "spec", "updateStrategy", "rollingUpdate", "partition")
	_ = unstructured.SetNestedField(o.Object, int64(1), "status", "updatedReplicas")
	_ = unstructured.SetNestedField(o.Object, "old", "status", "currentRevision")
	target = NewOperationTarget(r, "restart", o, o, 0)
	if p := target.Progress(o, nil); p.Summary != "Partitioned rollout complete" {
		t.Fatalf("%+v", p)
	}
	// Nonzero ordinals use the absolute partition, not replicas minus partition.
	_ = unstructured.SetNestedField(o.Object, int64(5), "spec", "ordinals", "start")
	target = NewOperationTarget(r, "restart", o, o, 0)
	if p := target.Progress(o, nil); !p.Watching() {
		t.Fatalf("nonzero ordinals: %+v", p)
	}
	_ = unstructured.SetNestedField(o.Object, "OnDelete", "spec", "updateStrategy", "type")
	target = NewOperationTarget(r, "restart", o, o, 0)
	if p := target.Progress(o, nil); p.Summary != "Manual Pod replacement required" {
		t.Fatalf("%+v", p)
	}
}

func TestDaemonSetProgressUsesScheduledAndAvailableCounts(t *testing.T) {
	r, o := progressWorkload(t, "DaemonSet", "daemonsets", "apps")
	for _, field := range []string{"desiredNumberScheduled", "numberReady", "updatedNumberScheduled", "numberAvailable"} {
		_ = unstructured.SetNestedField(o.Object, int64(2), "status", field)
	}
	target := NewOperationTarget(r, "restart", o, o, 0)
	if p := target.Progress(o, nil); p.State != "complete" {
		t.Fatalf("%+v", p)
	}
	_ = unstructured.SetNestedField(o.Object, int64(1), "status", "numberAvailable")
	if p := target.Progress(o, nil); !p.Watching() {
		t.Fatalf("%+v", p)
	}
	_ = unstructured.SetNestedField(o.Object, int64(0), "status", "desiredNumberScheduled")
	if p := target.Progress(o, nil); p.Summary != "No eligible nodes" {
		t.Fatalf("%+v", p)
	}
}

func TestDeletionProgressPreservesFinalizersAndHandlesReplacement(t *testing.T) {
	r, o := progressWorkload(t, "ConfigMap", "configmaps", "")
	target := NewOperationTarget(r, "delete", o, nil, 0)
	stamp := metav1.Now()
	o.SetDeletionTimestamp(&stamp)
	o.SetFinalizers([]string{"test.example/hold"})
	p := target.Progress(o, nil)
	if !p.Watching() || !strings.Contains(p.Details, "test.example/hold") || len(o.GetFinalizers()) != 1 {
		t.Fatalf("%+v", p)
	}
	if p := target.Progress(nil, apierrors.NewNotFound(schema.GroupResource{Resource: r.Name}, "sample")); p.State != "complete" {
		t.Fatalf("%+v", p)
	}
	apiMissing := &apierrors.StatusError{ErrStatus: metav1.Status{Code: 404, Reason: metav1.StatusReasonNotFound}}
	if p := target.Progress(nil, apiMissing); p.State == "complete" {
		t.Fatal("unavailable API reported as object deletion")
	}
	o.SetUID("replacement")
	if p := target.Progress(o, nil); p.Summary != "Original object deleted" || !strings.Contains(p.Details, "replacement") {
		t.Fatalf("%+v", p)
	}
}

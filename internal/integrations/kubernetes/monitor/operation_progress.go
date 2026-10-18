package monitor

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// OperationTarget captures the accepted change so a later reconciliation cannot
// silently turn an unrelated object's healthy status into this request's success.
type OperationTarget struct {
	Resource  kubecommon.Resource
	Operation string
	Object    *unstructured.Unstructured
}

type OperationProgress struct {
	State   string
	Summary string
	Details string
}

func (p OperationProgress) Watching() bool { return p.State == "waiting" }

func NewOperationTarget(r kubecommon.Resource, operation string, original, response *unstructured.Unstructured, replicas int64) OperationTarget {
	expected := original.DeepCopy()
	if operation == "restart" && response != nil {
		expected = response.DeepCopy()
	}
	if operation == "scale" {
		_ = unstructured.SetNestedField(expected.Object, replicas, "spec", "replicas")
	}
	return OperationTarget{Resource: r, Operation: operation, Object: expected}
}

func (t OperationTarget) Progress(current *unstructured.Unstructured, err error) OperationProgress {
	result := func(state, summary, details string) OperationProgress {
		return OperationProgress{state, summary, details}
	}
	var status apierrors.APIStatus
	objectMissing := apierrors.IsNotFound(err) && errors.As(err, &status) && t.Object != nil
	if objectMissing {
		details := status.Status().Details
		objectMissing = details != nil && details.Name == t.Object.GetName() && details.Kind == t.Resource.Name && details.Group == t.Resource.Group
	}
	if t.Operation == "delete" && objectMissing {
		return result("complete", "Object deleted", "The original object is no longer present. Dependent resources may still be terminating.")
	}
	if err != nil {
		return result("unavailable", "Status unavailable", OperationErrorText(err)+"\nThe request was accepted. Checking again only reads its status.")
	}
	if current == nil || t.Object == nil || t.Object.GetUID() == "" {
		return result("unavailable", "Status unavailable", "The accepted object's identity could not be verified.")
	}
	if current.GetUID() != t.Object.GetUID() {
		if t.Operation == "delete" {
			return result("complete", "Original object deleted", "A replacement now uses this name. It has not been changed by this operation.")
		}
		return result("attention", "Object replaced", "A different object now uses this name. Reopen its details to inspect it.")
	}
	if t.Operation == "delete" {
		details := "Waiting for the original object to disappear."
		if stamp := current.GetDeletionTimestamp(); stamp != nil {
			details += "\nDeletion started: " + stamp.Format("15:04:05 MST")
		}
		if seconds := current.GetDeletionGracePeriodSeconds(); seconds != nil {
			details += fmt.Sprintf("\nGrace period: %d seconds", *seconds)
		}
		if finalizers := current.GetFinalizers(); len(finalizers) > 0 {
			details += "\nWaiting on finalizers: " + strings.Join(finalizers, ", ")
		}
		return result("waiting", "Deletion in progress", details)
	}
	if current.GetDeletionTimestamp() != nil {
		return result("attention", "Object is being deleted", "The accepted change can no longer be tracked to completion.")
	}
	key := t.Resource.Key()
	switch key {
	case "deployments.apps", "statefulsets.apps", "replicasets.apps", "replicationcontrollers", "daemonsets.apps":
	default:
		return result("attention", "Scale request accepted", "This API has no supported readiness check. Inspect its reported status in Describe or YAML.")
	}
	if !reflect.DeepEqual(current.Object["spec"], t.Object.Object["spec"]) {
		return result("attention", "Desired state changed again", "Another update or an autoscaler changed this object after the request. Inspect its current state.")
	}
	number := func(section, field string) int64 {
		value, _, _ := unstructured.NestedInt64(current.Object, section, field)
		return value
	}
	desired, ready := number("spec", "replicas"), number("status", "readyReplicas")

	details := fmt.Sprintf("Ready: %d / %d · Current replicas: %d", ready, desired, number("status", "replicas"))
	if key == "daemonsets.apps" {
		desired, ready = number("status", "desiredNumberScheduled"), number("status", "numberReady")
		details = fmt.Sprintf("Ready: %d / %d · Updated: %d · Available: %d", ready, desired, number("status", "updatedNumberScheduled"), number("status", "numberAvailable"))
	}
	generation, observed := current.GetGeneration(), number("status", "observedGeneration")
	if generation == 0 || observed < generation || generation < t.Object.GetGeneration() {
		return result("waiting", "Waiting for the controller", details+"\nThe controller has not reported the accepted generation yet.")
	}
	if key == "deployments.apps" {
		paused, _, _ := unstructured.NestedBool(current.Object, "spec", "paused")
		if paused {
			return result("attention", "Deployment is paused", details+"\nResume it before expecting rollout completion.")
		}
		conditions, _, _ := unstructured.NestedSlice(current.Object, "status", "conditions")
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			failed := condition["type"] == "Progressing" && condition["status"] == "False" && condition["reason"] == "ProgressDeadlineExceeded"
			failed = failed || condition["type"] == "ReplicaFailure" && condition["status"] == "True"
			if failed {
				return result("attention", "Controller reports a failure", details+"\n"+Value(condition, "reason")+": "+Value(condition, "message"))
			}
		}
	}
	if t.Operation == "scale" {
		if ready == desired && number("status", "replicas") == desired {
			return result("complete", "Replica target reached", details)
		}
		return result("waiting", "Scaling in progress", details)
	}
	strategy := Value(current.Object, "spec", "updateStrategy", "type")
	if strategy == "OnDelete" {
		return result("attention", "Manual Pod replacement required", details+"\nOnDelete updates the template without replacing existing Pods.")
	}
	switch key {
	case "deployments.apps":
		updated, available := number("status", "updatedReplicas"), number("status", "availableReplicas")
		details += fmt.Sprintf("\nUpdated: %d · Available: %d", updated, available)
		if updated == desired && ready == desired && number("status", "replicas") == desired && available >= desired {
			return result("complete", "Rollout complete", details)
		}
	case "daemonsets.apps":
		if desired == 0 {
			return result("attention", "No eligible nodes", "The template was updated, but this DaemonSet currently targets no nodes.")
		}
		if number("status", "updatedNumberScheduled") == desired && ready == desired && number("status", "numberAvailable") >= desired {
			return result("complete", "Rollout complete", details)
		}
	case "statefulsets.apps":
		partition, _, _ := unstructured.NestedInt64(current.Object, "spec", "updateStrategy", "rollingUpdate", "partition")
		start, _, _ := unstructured.NestedInt64(current.Object, "spec", "ordinals", "start")
		target := min(desired, max(int64(0), start+desired-partition))
		updated := number("status", "updatedReplicas")
		details += fmt.Sprintf("\nUpdated: %d / %d eligible · Partition: %d", updated, target, partition)
		update, _, _ := unstructured.NestedString(current.Object, "status", "updateRevision")
		revision, _, _ := unstructured.NestedString(current.Object, "status", "currentRevision")
		available := number("spec", "minReadySeconds") == 0 || number("status", "availableReplicas") >= desired
		if ready == desired && number("status", "replicas") == desired && updated >= target && update != "" && available {
			if target < desired {
				return result("complete", "Partitioned rollout complete", details+"\nPods below the partition keep their existing revision.")
			}
			if update == revision {
				return result("complete", "Rollout complete", details)
			}
		}
	}
	return result("waiting", "Rollout in progress", details)
}

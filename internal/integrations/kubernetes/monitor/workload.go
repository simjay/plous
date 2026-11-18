package monitor

import (
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type workloadStatusFields struct {
	desired, current, ready, available, updated string
	extra                                       []string
}

func workloadFields(kind string) workloadStatusFields {
	fields := workloadStatusFields{current: "replicas", ready: "readyReplicas", available: "availableReplicas"}
	switch kind {
	case "deployments.apps":
		fields.updated, fields.extra = "updatedReplicas", []string{"unavailableReplicas"}
	case "statefulsets.apps":
		fields.updated, fields.extra = "updatedReplicas", []string{"currentReplicas"}
	case "daemonsets.apps":
		fields = workloadStatusFields{
			desired: "desiredNumberScheduled", current: "currentNumberScheduled",
			ready: "numberReady", available: "numberAvailable", updated: "updatedNumberScheduled",
			extra: []string{"numberUnavailable", "numberMisscheduled"},
		}
	}
	return fields
}

type workloadCounts struct {
	desired, current, ready, available, updated *int64
	generation, observed                        *int64
}

func readWorkload(fields workloadStatusFields, object map[string]any) workloadCounts {
	status, _ := object["status"].(map[string]any)
	replicas, _, _ := unstructured.NestedFieldNoCopy(object, "spec", "replicas")
	generation, _, _ := unstructured.NestedFieldNoCopy(object, "metadata", "generation")
	desired := reportedInteger(replicas)
	if fields.desired != "" {
		desired = statusCount(status, fields.desired)
	}
	return workloadCounts{
		desired: desired, current: statusCount(status, fields.current),
		ready: statusCount(status, fields.ready), available: statusCount(status, fields.available),
		updated:    statusCount(status, fields.updated),
		generation: reportedInteger(generation), observed: statusCount(status, "observedGeneration"),
	}
}

// Kubernetes omits zero-valued status counters. Missing status and malformed
// counters leave the value unknown.
func statusCount(status map[string]any, field string) *int64 {
	if status == nil || field == "" {
		return nil
	}
	value, exists := status[field]
	if !exists {
		return new(int64(0))
	}
	count := reportedInteger(value)
	if count == nil || *count < 0 {
		return nil
	}
	return count
}

func (c workloadCounts) generationObserved() *bool {
	if c.generation == nil || c.observed == nil {
		return nil
	}
	return new(*c.observed >= *c.generation)
}

func (c workloadCounts) currentUnready() *int64 {
	if c.ready == nil || c.current == nil || *c.ready > *c.current {
		return nil
	}
	return new(*c.current - *c.ready)
}

func replicaShortfall(desired, count *int64) *int64 {
	if desired == nil || *desired < 0 || count == nil {
		return nil
	}
	return new(max(0, *desired-*count))
}

func formatReplicaCount(count *int64, missing string) string {
	if count == nil {
		return missing
	}
	return strconv.FormatInt(*count, 10)
}

func (c workloadCounts) readyRatio() string {
	return formatReplicaCount(c.ready, "?") + "/" + formatReplicaCount(c.desired, "?")
}

func workloadReadyColumn(kind string) tableColumn {
	fields := workloadFields(kind)
	return computed("Ready", func(o map[string]any) string {
		return readWorkload(fields, o).readyRatio()
	})
}

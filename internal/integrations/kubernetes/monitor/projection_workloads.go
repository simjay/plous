package monitor

import (
	"fmt"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"maps"
)

func projectWorkloads(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	if r.Version != "v1" {
		return fmt.Errorf("unsupported workload API version")
	}
	maps.Copy(f, workloadFacts(r.Key(), o))
	return nil
}

func workloadFacts(kind string, o map[string]any) map[string]observation.Value {
	f := map[string]observation.Value{}
	fields := []string{"replicas", "readyReplicas", "availableReplicas", "observedGeneration"}
	switch kind {
	case "deployments.apps":
		fields = append(fields, "updatedReplicas", "unavailableReplicas")
	case "statefulsets.apps":
		fields = append(fields, "updatedReplicas", "currentReplicas")
	case "daemonsets.apps":
		fields = []string{"observedGeneration", "desiredNumberScheduled", "currentNumberScheduled", "numberReady", "numberAvailable", "updatedNumberScheduled", "numberUnavailable", "numberMisscheduled"}
	}
	for _, field := range fields {
		f[field] = workloadCount(o, "status", field)
	}
	f["desired_replicas"] = projectedValue(o, "spec", "replicas")
	current, ready, available, updated := "replicas", "readyReplicas", "availableReplicas", "updatedReplicas"
	if kind == "daemonsets.apps" {
		f["desired_replicas"] = f["desiredNumberScheduled"]
		current, ready, available, updated = "currentNumberScheduled", "numberReady", "numberAvailable", "updatedNumberScheduled"
	}
	f["current_replicas"], f["ready_replicas"] = f[current], f[ready]
	f["current_unready_replicas"] = observation.Unknown()
	if n, total := f[ready], f[current]; n.Number != nil && total.Number != nil && *n.Number <= *total.Number {
		f["current_unready_replicas"] = observation.Number(*total.Number - *n.Number)
	}
	desired := f["desired_replicas"]
	for name, field := range map[string]string{"unready_replicas": ready, "unavailable_replicas": available, "not_updated_replicas": updated} {
		value := observation.Unknown()
		if n := f[field]; desired.Number != nil && *desired.Number >= 0 && n.Number != nil && *n.Number >= 0 {
			value = observation.Number(max(0, *desired.Number-*n.Number))
		}
		f[name] = value
	}
	f["generation"] = projectedValue(o, "metadata", "generation")
	f["generation_observed"] = observation.Unknown()
	if g, seen := f["generation"], f["observedGeneration"]; g.Number != nil && seen.Number != nil {
		f["generation_observed"] = observation.Boolean(*seen.Number >= *g.Number)
	}
	f["ready_ratio"] = observation.Unknown()
	if n := f[ready]; n.Number != nil && desired.Number != nil {
		f["ready_ratio"] = observation.Text(fmt.Sprintf("%d/%d", *n.Number, *desired.Number))
	}
	return f
}

// Workload status counters are non-pointer integer fields with omitempty in
// the Kubernetes v1 API. An omitted counter in a present status means zero.
// An absent status or malformed counter does not establish a value.
func workloadCount(o map[string]any, section, field string) observation.Value {
	status, ok := o[section].(map[string]any)
	if !ok {
		return observation.Unknown()
	}
	if _, exists := status[field]; !exists {
		return observation.Number(0)
	}
	v := projectedValue(status, field)
	if v.Number == nil || *v.Number < 0 {
		return observation.Unknown()
	}
	return v
}

func workloadReadyColumn(kind string) tableColumn {
	return computed("Ready", func(o map[string]any) string {
		f := workloadFacts(kind, o)
		cell := func(v observation.Value) string {
			if v.Number == nil {
				return "?"
			}
			return v.String()
		}
		return cell(f["ready_replicas"]) + "/" + cell(f["desired_replicas"])
	})
}
func projectJobs(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	for _, field := range []string{"active", "succeeded", "failed", "startTime", "completionTime", "lastScheduleTime", "lastSuccessfulTime"} {
		f[field] = projectedValue(o, "status", field)
	}
	for _, field := range []string{"completions", "parallelism", "backoffLimit", "schedule", "suspend", "concurrencyPolicy"} {
		f[field] = projectedValue(o, "spec", field)
	}
	return nil
}

package monitor

import (
	"errors"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectWorkloads(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	if r.Version != "v1" {
		return errors.New("unsupported workload API version")
	}
	fields := workloadFields(r.Key())
	counts := readWorkload(fields, o)
	for name, count := range map[string]*int64{
		"desired_replicas":         counts.desired,
		"current_replicas":         counts.current,
		"ready_replicas":           counts.ready,
		"current_unready_replicas": counts.currentUnready(),
		"unready_replicas":         replicaShortfall(counts.desired, counts.ready),
		"unavailable_replicas":     replicaShortfall(counts.desired, counts.available),
		"not_updated_replicas":     replicaShortfall(counts.desired, counts.updated),
		"generation":               counts.generation,
		"observedGeneration":       counts.observed,
		fields.current:             counts.current,
		fields.ready:               counts.ready,
		fields.available:           counts.available,
	} {
		f[name] = workloadCountFact(count)
	}
	if fields.updated != "" {
		f[fields.updated] = workloadCountFact(counts.updated)
	}
	if fields.desired != "" {
		f[fields.desired] = workloadCountFact(counts.desired)
	}
	status, _ := o["status"].(map[string]any)
	for _, name := range fields.extra {
		f[name] = workloadCountFact(statusCount(status, name))
	}
	f["generation_observed"] = observation.Unknown()
	if observed := counts.generationObserved(); observed != nil {
		f["generation_observed"] = observation.Boolean(*observed)
	}
	f["ready_ratio"] = observation.Unknown()
	if counts.ready != nil && counts.desired != nil {
		f["ready_ratio"] = observation.Text(counts.readyRatio())
	}
	return nil
}

func workloadCountFact(count *int64) observation.Value {
	if count == nil {
		return observation.Unknown()
	}
	return observation.Number(*count)
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

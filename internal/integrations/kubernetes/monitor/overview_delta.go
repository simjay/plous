package monitor

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// OverviewDelta compares complete reads of the same objects over a short interval.
type OverviewDelta struct {
	Restarts, Replicas           *int64
	RestartsSince, RestartsUntil time.Time
	ReplicasSince, ReplicasUntil time.Time
}

func CompareOverview(before, after Overview) OverviewDelta {
	d := OverviewDelta{}
	if comparableOverviewRead(before.Reads["pods"], after.Reads["pods"]) {
		d.Restarts = overviewCounterDelta(before.RestartsByContainer, after.RestartsByContainer, true)
		d.RestartsSince, d.RestartsUntil = before.Reads["pods"].ReadAt, after.Reads["pods"].ReadAt
	}
	key := "horizontalpodautoscalers.autoscaling"
	if comparableOverviewRead(before.Reads[key], after.Reads[key]) {
		d.Replicas = overviewCounterDelta(before.ReplicasByHPA, after.ReplicasByHPA, false)
		d.ReplicasSince, d.ReplicasUntil = before.Reads[key].ReadAt, after.Reads[key].ReadAt
	}
	return d
}

func comparableOverviewRead(before, after OverviewRead) bool {
	duration := after.ReadAt.Sub(before.ReadAt)
	return !before.ReadAt.IsZero() && duration > 0 && duration <= 45*time.Second && before.Error == "" && after.Error == "" && !before.Partial && !after.Partial
}

func overviewCounterDelta(before, after map[string]int64, monotonic bool) *int64 {
	if before == nil || after == nil || len(before) != len(after) {
		return nil
	}
	var delta int64
	for key, value := range after {
		previous, found := before[key]
		if !found || value < 0 || previous < 0 || monotonic && value < previous {
			return nil
		}
		delta += value - previous
	}
	return &delta
}

func (o *Overview) addRestartCounters(pod unstructured.Unstructured) {
	if o.RestartsByContainer == nil {
		return
	}
	uid := string(pod.GetUID())
	if uid == "" {
		o.RestartsByContainer = nil
		return
	}
	for _, fields := range [][2]string{{"containers", "containerStatuses"}, {"initContainers", "initContainerStatuses"}} {
		statuses := map[string]int64{}
		for _, status := range Maps(pod.Object, "status", fields[1]) {
			n, exists, err := unstructured.NestedInt64(status, "restartCount")
			if exists && err == nil && n >= 0 {
				statuses[Value(status, "name")] = n
			}
		}
		containers := Maps(pod.Object, "spec", fields[0])
		if fields[0] == "containers" && len(containers) == 0 {
			o.RestartsByContainer = nil
			return
		}
		for _, container := range containers {
			name := Value(container, "name")
			n, found := statuses[name]
			if name == "" || !found {
				o.RestartsByContainer = nil
				return
			}
			o.RestartsByContainer[uid+"/"+fields[0]+"/"+name] = n
		}
	}
}

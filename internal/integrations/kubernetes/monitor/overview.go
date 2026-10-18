package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// OverviewRead describes one bounded inventory, including unsuccessful reads.
type OverviewRead struct {
	Resource kubecommon.Resource
	Count    int
	ReadAt   time.Time
	Error    string
	Partial  bool
}

type OverviewEntry struct {
	Resource                  kubecommon.Resource
	Namespace, Name, UID      string
	At                        time.Time
	Reason, Outcome, Revision string
	GroupKey, GroupTitle      string
	Summary, Cause            string
	Severity                  int
}

type OverviewMetric struct {
	Used, Allocatable, Requests, Limits *float64
	RequestsMissing, LimitsMissing      int
}

type Overview struct {
	ReadAt                                                                     time.Time
	Reads                                                                      map[string]OverviewRead
	CPU, Memory                                                                OverviewMetric
	MetricsAt, MetricsEnd, MetricsReadAt                                       time.Time
	MetricsObserved, MetricsExpected                                           int
	MetricsError                                                               string
	NodesPressureUnknown                                                       int
	NodesCordoned, NodesPressure                                               int
	NodesReady, NodesUnknown                                                   int
	PodsRunning, PodsPending, PodsFailed, PodsSucceeded, PodsUnknown, Restarts int
	ActivePods                                                                 int
	HPACurrent, HPADesired                                                     int64
	HPAUnknown                                                                 int
	HPAUp, HPADown, HPAStable                                                  int
	HPALastScale                                                               time.Time
	Changes, Attention                                                         []OverviewEntry
	RestartsByContainer, ReplicasByHPA                                         map[string]int64
}

type overviewReader interface {
	List(context.Context, kubecommon.Resource, string, string) (Collection, error)
	UsageList(context.Context, kubecommon.Resource, string) (map[string]Usage, error)
}

func (c *Client) OverviewInventory(ctx context.Context, catalog []kubecommon.Resource) Overview {
	return readOverviewInventory(ctx, c, catalog)
}

func readOverviewInventory(ctx context.Context, reader overviewReader, catalog []kubecommon.Resource) Overview {
	keys := []string{"nodes", "pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "horizontalpodautoscalers.autoscaling"}
	reads := make([]OverviewRead, len(keys))
	collections := make([]Collection, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		r, ok := Find(catalog, key)
		reads[i].Resource = r
		if !ok {
			reads[i].Error = "API unavailable or not discovered"
			continue
		}
		wg.Go(func() {
			collection, err := reader.List(ctx, r, "", "")
			reads[i].ReadAt = time.Now()
			if err != nil {
				reads[i].Error = ErrorText(err)
				return
			}
			collections[i] = collection
			reads[i].Count, reads[i].Partial = len(collection.Objects), collection.Truncated
		})
	}
	wg.Wait()
	result := Overview{ReadAt: time.Now(), Reads: map[string]OverviewRead{}}
	if reads[1].Error == "" && !reads[1].Partial {
		result.RestartsByContainer = map[string]int64{}
	}
	if reads[5].Error == "" && !reads[5].Partial {
		result.ReplicasByHPA = map[string]int64{}
	}
	for i, key := range keys {
		result.Reads[key] = reads[i]
		for _, object := range collections[i].Objects {
			result.addObject(reads[i].Resource, object)
		}
	}
	nodes, pods := collections[0], collections[1]
	if reads[0].Error == "" && !reads[0].Partial {
		result.CPU.Allocatable = overviewQuantitySum(nodes.Objects, "status", "allocatable", "cpu")
		result.Memory.Allocatable = overviewQuantitySum(nodes.Objects, "status", "allocatable", "memory")
	}
	if reads[1].Error == "" {
		for _, pod := range pods.Objects {
			if terminalPod(pod) {
				continue
			}
			result.ActivePods++
			for i, key := range []string{"cpu", "memory"} {
				metric := &result.CPU
				if i == 1 {
					metric = &result.Memory
				}
				requests := quantityValue(declaredPodResources(pod.Object, "requests"), key)
				limits := quantityValue(declaredPodResources(pod.Object, "limits"), key)
				if requests == nil {
					metric.RequestsMissing++
				} else {
					addOverviewNumber(&metric.Requests, *requests)
				}
				if limits == nil {
					metric.LimitsMissing++
				} else {
					addOverviewNumber(&metric.Limits, *limits)
				}
			}
		}
	}
	return result
}

func overviewQuantitySum(objects []unstructured.Unstructured, path ...string) *float64 {
	if len(objects) == 0 {
		return nil
	}
	total := 0.0
	for _, object := range objects {
		value := quantityNumber(Value(object.Object, path...))
		if value == nil {
			return nil
		}
		total += *value
	}
	return &total
}
func addOverviewNumber(total **float64, value float64) {
	if *total == nil {
		*total = new(float64)
	}
	**total += value
}
func overviewTime(value string) time.Time { t, _ := time.Parse(time.RFC3339Nano, value); return t }

func (o *Overview) addObject(r kubecommon.Resource, object unstructured.Unstructured) {
	entry := OverviewEntry{Resource: r, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID())}
	warn := func(reason string, severity int, at time.Time) {
		e := entry
		e.Reason, e.Severity, e.At = reason, severity, at
		o.Attention = append(o.Attention, e)
	}
	m := object.Object
	switch r.Key() {
	case "nodes":
		if Value(m, "spec", "unschedulable") == "true" {
			o.NodesCordoned++
		}
		pressure := false
		pressureKnown := 0
		ready := ""
		for _, c := range Maps(m, "status", "conditions") {
			kind, status := Value(c, "type"), Value(c, "status")
			if kind == "DiskPressure" || kind == "MemoryPressure" || kind == "PIDPressure" {
				if status == "True" {
					pressure = true
				}
				if status == "True" || status == "False" {
					pressureKnown++
				}
			}
			if kind == "Ready" {
				ready = status
			}
			if kind == "Ready" && status != "True" || kind != "Ready" && status == "True" && (kind == "DiskPressure" || kind == "MemoryPressure" || kind == "PIDPressure" || kind == "NetworkUnavailable") {
				warn(kind+"="+status+": "+Value(c, "reason"), 2, overviewTime(Value(c, "lastTransitionTime")))
			}
		}
		if pressure {
			o.NodesPressure++
		} else if pressureKnown < 3 {
			o.NodesPressureUnknown++
		}
		if ready == "True" {
			o.NodesReady++
		} else if ready == "" || ready == "Unknown" {
			o.NodesUnknown++
			if ready == "" {
				warn("Ready condition not reported", 1, time.Time{})
			}
		}
	case "pods":
		phase := Value(m, "status", "phase")
		switch phase {
		case "Running":
			o.PodsRunning++
		case "Pending":
			o.PodsPending++
		case "Failed":
			o.PodsFailed++
		case "Succeeded":
			o.PodsSucceeded++
		default:
			o.PodsUnknown++
		}
		facts := podFacts(m)
		o.Restarts += facts.Restarts
		o.addRestartCounters(object)
		severity := 0
		if phase == "Failed" || facts.Status == "CrashLoopBackOff" || facts.Status == "ImagePullBackOff" || facts.Status == "ErrImagePull" || facts.Status == "CreateContainerConfigError" {
			severity = 2
		} else if phase == "Pending" || phase == "Unknown" || phase == "" || phase == "Running" && (facts.Ready.Boolean == nil || !*facts.Ready.Boolean || facts.ContainersReady < facts.ContainersTotal) {
			severity = 1
		}
		if severity > 0 {
			reason, cause, at := overviewPodProblem(m, facts.Status)
			entry.Cause = cause
			if cause == "CrashLoopBackOff" || cause == "ImagePullBackOff" || cause == "ErrImagePull" || cause == "CreateContainerConfigError" || cause == "OOMKilled" {
				severity = 2
			}
			entry.Summary = map[string]string{"ReadinessGatesNotReady": "Readiness gate blocked", "ContainersNotReady": "Containers not ready", "Unschedulable": "Cannot schedule Pod"}[cause]
			for _, owner := range object.GetOwnerReferences() {
				if owner.Controller != nil && *owner.Controller && owner.UID != "" {
					entry.GroupKey = entry.Namespace + "/" + string(owner.UID) + "/" + cause
					entry.GroupTitle = owner.Kind + " " + owner.Name
					break
				}
			}
			warn(reason, severity, at)
		}
	case "deployments.apps", "statefulsets.apps", "daemonsets.apps":
		facts := workloadFacts(r.Key(), m)
		seen := facts["generation_observed"]
		previousGeneration := seen.Boolean != nil && !*seen.Boolean
		if previousGeneration {
			warn("Awaiting controller status for current generation", 1, time.Time{})
		} else if unready := facts["unready_replicas"]; unready.Number != nil && *unready.Number > 0 {
			warn(fmt.Sprintf("%d desired replicas not ready", *unready.Number), 1, time.Time{})
		} else if facts["ready_replicas"].Unknown {
			warn("Workload readiness not reported", 1, time.Time{})
		}
		var latest map[string]any
		var at time.Time
		for _, condition := range Maps(m, "status", "conditions") {
			kind, status := Value(condition, "type"), Value(condition, "status")
			changed := overviewTime(Value(condition, "lastUpdateTime"))
			if changed.IsZero() {
				changed = overviewTime(Value(condition, "lastTransitionTime"))
			}
			if kind == "Progressing" && changed.After(at) {
				latest, at = condition, changed
			}
			if (kind == "Available" || kind == "Progressing") && status == "False" || kind == "ReplicaFailure" && status == "True" {
				warn(Value(condition, "reason")+": "+Value(condition, "message"), 2, changed)
			}
		}
		// Controller condition updates are observable evidence, not Git activity.
		if latest != nil {
			entry.At, entry.Reason, entry.Outcome = at, "Rollout condition", Value(latest, "reason")
			entry.Reason += ": " + entry.Outcome
			switch Value(latest, "status") {
			case "False":
				entry.Outcome = "Failed"
			case "True":
				entry.Outcome = "Progressing"
				if Value(latest, "reason") == "NewReplicaSetAvailable" {
					entry.Outcome = "Available"
				}
			default:
				entry.Outcome = "Unknown"
			}
			if previousGeneration {
				entry.Reason += " from previous generation"
				entry.Outcome = "Previous status"
			} else {
				entry.Revision = object.GetAnnotations()["deployment.kubernetes.io/revision"]
			}
			o.Changes = append(o.Changes, entry)
		}
	case "horizontalpodautoscalers.autoscaling":
		current, c, _ := unstructured.NestedInt64(m, "status", "currentReplicas")
		desired, d, _ := unstructured.NestedInt64(m, "status", "desiredReplicas")
		if o.ReplicasByHPA != nil {
			if entry.UID == "" || !c || current < 0 {
				o.ReplicasByHPA = nil
			} else {
				o.ReplicasByHPA[entry.UID] = current
			}
		}
		if c && d {
			o.HPACurrent += current
			o.HPADesired += desired
			switch {
			case desired > current:
				o.HPAUp++
			case desired < current:
				o.HPADown++
			default:
				o.HPAStable++
			}
		} else {
			o.HPAUnknown++
		}
		if at := overviewTime(Value(m, "status", "lastScaleTime")); at.After(o.HPALastScale) {
			o.HPALastScale = at
		}
		for _, condition := range Maps(m, "status", "conditions") {
			if (Value(condition, "type") == "AbleToScale" || Value(condition, "type") == "ScalingActive") && Value(condition, "status") == "False" {
				warn(Value(condition, "reason")+": "+Value(condition, "message"), 1, overviewTime(Value(condition, "lastTransitionTime")))
			}
		}
	}
}

func overviewPodProblem(object map[string]any, fallback string) (reason, cause string, at time.Time) {
	for _, key := range []string{"initContainerStatuses", "containerStatuses"} {
		for _, container := range Maps(object, "status", key) {
			for _, state := range []string{"waiting", "terminated"} {
				value := Value(container, "state", state, "reason")
				if value == "" || value == "Completed" {
					continue
				}
				cause = value
				reason = value + " · " + Value(container, "name")
				if state == "terminated" {
					reason += " · exit " + Value(container, "state", state, "exitCode")
					at = overviewTime(Value(container, "state", state, "finishedAt"))
				}
				if message := Value(container, "state", state, "message"); message != "" {
					reason += ": " + message
				}
				return
			}
		}
	}
	for _, condition := range Maps(object, "status", "conditions") {
		if (Value(condition, "type") == "Ready" || Value(condition, "type") == "PodScheduled") && Value(condition, "status") != "True" {
			cause = Value(condition, "reason")
			if cause == "" {
				cause = Value(condition, "type") + "=" + Value(condition, "status")
			}
			reason = cause
			if message := Value(condition, "message"); message != "" {
				reason += ": " + message
			}
			return reason, cause, overviewTime(Value(condition, "lastTransitionTime"))
		}
	}
	cause = Value(object, "status", "reason")
	if cause == "" {
		cause = fallback
	}
	reason = cause
	if message := Value(object, "status", "message"); message != "" {
		reason += ": " + message
	} else if cause == "Running" {
		reason = "Pod readiness not reported"
	}
	return
}

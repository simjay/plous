package monitor

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const UsageMaxAge = 90 * time.Second

// Usage retains the source interval so repeated, stale, or pre-creation samples
// cannot be presented as a new observation of a replacement object.
type Usage struct {
	CPU, Memory   float64
	Timestamp     time.Time
	IntervalStart time.Time
	Window        time.Duration
	UID           types.UID
	Containers    []string
	Breakdown     []ContainerUsage
	// Aggregates retain collection time and the range of source sample times.
	Collected  time.Time
	SourceEnd  time.Time
	Snapshot   string
	Aggregated bool
	Identity   observation.Identity
	Coverage   observation.Coverage
	Expected   int
}

type ContainerUsage struct {
	Name        string
	CPU, Memory float64
}

func (u Usage) Fresh(now time.Time) bool {
	return !u.Timestamp.IsZero() && !u.Timestamp.After(now.Add(5*time.Second)) && now.Sub(u.Timestamp) <= UsageMaxAge
}

func (u Usage) Matches(object unstructured.Unstructured, now time.Time) bool {
	created := object.GetCreationTimestamp().Time
	if !u.Fresh(now) || u.UID != "" && u.UID != object.GetUID() || !created.IsZero() && u.Timestamp.Add(-u.Window).Before(created) {
		return false
	}
	for _, name := range RunningContainers(object) {
		if !slices.Contains(u.Containers, name) {
			return false
		}
	}
	return true
}

func UsageKey(object unstructured.Unstructured) string {
	return object.GetNamespace() + "/" + object.GetName()
}

func HasUsage(r kubecommon.Resource) bool { return r.Key() == "nodes" || HasPodTargets(r) }
func hasUsageResource(key string) bool {
	name, group, _ := strings.Cut(key, ".")
	return HasUsage(kubecommon.Resource{Name: name, Group: group})
}

func UsageResource(r kubecommon.Resource, resources []kubecommon.Resource) (kubecommon.Resource, bool) {
	name := "pods"
	if r.Key() == "nodes" {
		name = "nodes"
	}
	for _, candidate := range resources {
		if candidate.Key() == name+".metrics.k8s.io" {
			return candidate, true
		}
	}
	return kubecommon.Resource{}, false
}

func parseUsage(object unstructured.Unstructured, node bool) (Usage, error) {
	u := Usage{UID: object.GetUID()}
	var err error
	u.Timestamp, err = time.Parse(time.RFC3339Nano, Value(object.Object, "timestamp"))
	if err != nil {
		return Usage{}, operationError("Metrics timestamp is unavailable.")
	}
	u.Window, err = time.ParseDuration(Value(object.Object, "window"))
	if err != nil || u.Window <= 0 {
		return Usage{}, operationError("Metrics interval is unavailable.")
	}
	u.IntervalStart = u.Timestamp.Add(-u.Window)
	containers := Maps(object.Object, "containers")
	if node {
		containers = []map[string]any{object.Object}
	}
	if len(containers) == 0 {
		return Usage{}, operationError("Container metrics are not available yet.")
	}
	for _, container := range containers {
		if !node {
			name := Value(container, "name")
			if name == "" || slices.Contains(u.Containers, name) {
				return Usage{}, operationError("Container metrics are incomplete.")
			}
			u.Containers = append(u.Containers, name)
		}
		cpu, cpuErr := resource.ParseQuantity(Value(container, "usage", "cpu"))
		memory, memoryErr := resource.ParseQuantity(Value(container, "usage", "memory"))
		if cpuErr != nil || memoryErr != nil || cpu.Sign() < 0 || memory.Sign() < 0 {
			return Usage{}, operationError("CPU or memory metrics are unavailable.")
		}
		u.CPU += cpu.AsApproximateFloat64()
		u.Memory += memory.AsApproximateFloat64()
		if !node {
			u.Breakdown = append(u.Breakdown, ContainerUsage{Name: Value(container, "name"), CPU: cpu.AsApproximateFloat64(), Memory: memory.AsApproximateFloat64()})
		}
	}
	if math.IsInf(u.CPU, 0) || math.IsInf(u.Memory, 0) || math.IsNaN(u.CPU) || math.IsNaN(u.Memory) {
		return Usage{}, operationError("CPU or memory metrics are invalid.")
	}
	return u, nil
}

// UsageList reads one bounded metrics collection. Invalid individual samples
// remain absent rather than becoming a misleading zero in the table.
func (c *Client) UsageList(ctx context.Context, metrics kubecommon.Resource, namespace string) (map[string]Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := map[string]Usage{}
	continuation, count := "", 0
	for {
		list, err := c.resource(metrics, namespace).List(ctx, metav1.ListOptions{Limit: 500, Continue: continuation})
		if err != nil {
			return nil, err
		}
		count += len(list.Items)
		if count > MaxObjects || count == MaxObjects && list.GetContinue() != "" {
			return nil, operationError("Too many metrics. Narrow the namespace scope.")
		}
		for key, usage := range usageSamples(list.Items, metrics.Name == "nodes") {
			result[key] = usage
		}
		continuation = list.GetContinue()
		if continuation == "" {
			return result, nil
		}
	}
}

type usageReader interface {
	objectReader
	UsageList(context.Context, kubecommon.Resource, string) (map[string]Usage, error)
}

// ObjectUsage aggregates active Pods reached through controller UIDs.
// Incomplete coverage never yields a numeric workload total.
func (c *Client) ObjectUsage(ctx context.Context, r kubecommon.Resource, object *unstructured.Unstructured, resources []kubecommon.Resource, now time.Time) (Usage, error) {
	return objectUsage(ctx, c, r, object, resources, now)
}

func objectUsage(ctx context.Context, c usageReader, r kubecommon.Resource, object *unstructured.Unstructured, resources []kubecommon.Resource, now time.Time) (Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	base := Usage{Identity: usageIdentity(r, *object), Aggregated: r.Key() != "pods" && r.Key() != "nodes", Collected: now}
	if !HasUsage(r) {
		return base, operationError("This resource has no live CPU or memory usage.")
	}
	metrics, ok := UsageResource(r, resources)
	if !ok {
		return base, operationError("Metrics API unavailable. A resource metrics provider such as Metrics Server is required.")
	}
	if r.Key() == "nodes" || r.Key() == "pods" {
		current, err := c.Get(ctx, r, object.GetNamespace(), object.GetName())
		if err != nil {
			return base, err
		}
		if object.GetUID() == "" || current.GetUID() != object.GetUID() {
			return base, TargetChangedError()
		}
		if r.Key() == "pods" && terminalPod(*current) {
			return base, operationError("Completed Pods have no live usage.")
		}
		reported, err := c.Get(ctx, metrics, object.GetNamespace(), object.GetName())
		if err != nil {
			return base, err
		}
		u, err := parseUsage(*reported, r.Key() == "nodes")
		if err == nil && !u.Matches(*current, now) {
			err = operationError("Metrics are stale or do not match this object yet.")
		}
		u.Collected, u.SourceEnd = now, u.Timestamp
		u.Identity = base.Identity
		u.Coverage = observation.Coverage{Complete: err == nil, Observed: 1}
		if err == nil {
			u.Coverage.Total = new(1)
		} else {
			u.Coverage.Observed = 0
		}
		return u, err
	}
	pods, err := podTargets(ctx, c, r, object, resources)
	if err != nil {
		return base, err
	}
	active := pods[:0]
	for _, pod := range pods {
		if !terminalPod(pod) {
			active = append(active, pod)
		}
	}
	if len(active) == 0 {
		base.Coverage = observation.Coverage{Complete: true, Total: new(0)}
		return base, operationError("No active Pods to measure.")
	}
	base.Expected = len(active)
	samples, err := c.UsageList(ctx, metrics, object.GetNamespace())
	if err != nil {
		return base, err
	}
	total, measured := base, 0
	identity := sha256.New()
	for _, pod := range active {
		u, found := samples[UsageKey(pod)]
		if !found || !u.Matches(pod, now) {
			continue
		}
		measured++
		fmt.Fprintf(identity, "%q/%q/%q/%d/%g/%g\n", UsageKey(pod), pod.GetUID(), u.Timestamp.UTC().Format(time.RFC3339Nano), u.Window, u.CPU, u.Memory)
		total.CPU += u.CPU
		total.Memory += u.Memory
		if total.Timestamp.IsZero() || u.Timestamp.Before(total.Timestamp) {
			total.Timestamp = u.Timestamp
		}
		if total.IntervalStart.IsZero() || u.IntervalStart.Before(total.IntervalStart) {
			total.IntervalStart = u.IntervalStart
		}
		if u.Timestamp.After(total.SourceEnd) {
			total.SourceEnd = u.Timestamp
		}
	}
	total.Coverage = observation.Coverage{Observed: measured}
	if measured != len(active) {
		return Usage{Aggregated: true, Identity: total.Identity, Coverage: total.Coverage, Expected: total.Expected, Collected: now}, operationError("Waiting for complete, fresh Pod metrics.")
	}
	total.Collected, total.Snapshot = now, fmt.Sprintf("%x", identity.Sum(nil))
	total.Coverage.Complete, total.Coverage.Total = true, new(measured)
	return total, nil
}

func terminalPod(object unstructured.Unstructured) bool {
	phase := Value(object.Object, "status", "phase")
	return phase == "Succeeded" || phase == "Failed"
}

func CPUUsage(value float64) string { return fmt.Sprintf("%.1fm", value*1000) }

func MemoryUsage(value float64) string {
	if value >= 1024*1024*1024 {
		return fmt.Sprintf("%.2fGi", value/(1024*1024*1024))
	}
	return fmt.Sprintf("%.1fMi", value/(1024*1024))
}

func controllerName(object unstructured.Unstructured) string {
	for _, owner := range object.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller {
			return owner.Kind + "/" + owner.Name
		}
	}
	return "none"
}

func UsageErrorText(err error) string {
	var local operationError
	if errors.As(err, &local) {
		return string(local)
	}
	if apierrors.IsNotFound(err) {
		return "Metrics are not available yet, or the Metrics API is unavailable."
	}
	return ErrorText(err)
}

func usageIdentity(r kubecommon.Resource, object unstructured.Unstructured) observation.Identity {
	return observation.Identity{API: r.GVR().GroupVersion().String(), Kind: r.Kind, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID()), Revision: object.GetResourceVersion()}
}

func usageSamples(objects []unstructured.Unstructured, node bool) map[string]Usage {
	result := map[string]Usage{}
	for _, object := range objects {
		if u, err := parseUsage(object, node); err == nil {
			result[UsageKey(object)] = u
		}
	}
	return result
}

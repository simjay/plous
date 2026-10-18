package monitor

import (
	"fmt"
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A nil quantity means unreported or invalid, never zero.
type resourceBudget struct {
	subject, metric, basis string
	used                   *float64
	total                  *resource.Quantity
}

func reportedQuantity(text string) *resource.Quantity {
	q, err := resource.ParseQuantity(text)
	if err != nil || q.Sign() < 0 {
		return nil
	}
	return &q
}
func quantityNumber(text string) *float64 { return quantityFloat(reportedQuantity(text)) }
func quantityFloat(q *resource.Quantity) *float64 {
	if q == nil {
		return nil
	}
	return new(q.AsApproximateFloat64())
}
func quantityLabel(q *resource.Quantity, missing string) string {
	if q == nil {
		return missing
	}
	return q.String()
}

func usageBudgets(r kubecommon.Resource, object unstructured.Unstructured, usage Usage) []resourceBudget {
	var result []resourceBudget
	add := func(subject, metric, basis string, used float64, spec map[string]any, path ...string) {
		result = append(result, resourceBudget{subject, metric, basis, new(used), reportedQuantity(Value(spec, path...))})
	}
	if r.Key() == "nodes" {
		add("", "cpu", "allocatable", usage.CPU, object.Object, "status", "allocatable", "cpu")
		add("", "memory", "allocatable", usage.Memory, object.Object, "status", "allocatable", "memory")
	} else if r.Key() == "pods" {
		containers := append(Maps(object.Object, "spec", "containers"), Maps(object.Object, "spec", "initContainers")...)
		containers = append(containers, Maps(object.Object, "spec", "ephemeralContainers")...)
		for _, sample := range usage.Breakdown {
			for _, c := range containers {
				if Value(c, "name") != sample.Name {
					continue
				}
				for _, metric := range []string{"cpu", "memory"} {
					value := sample.CPU
					if metric == "memory" {
						value = sample.Memory
					}
					for _, basis := range []string{"requests", "limits"} {
						add(sample.Name, metric, basis, value, c, "resources", basis, metric)
					}
				}
			}
		}
	}
	return result
}

func quotaBudgets(o map[string]any) []resourceBudget {
	hard, used := nestedMap(o, "status", "hard"), nestedMap(o, "status", "used")
	var result []resourceBudget
	for _, key := range unionKeys(hard, used, nestedMap(o, "spec", "hard")) {
		result = append(result, resourceBudget{metric: key, basis: "hard", used: quantityNumber(scalar(used[key])), total: reportedQuantity(scalar(hard[key]))})
	}
	return result
}

func projectUsageBudgets(r kubecommon.Resource, object unstructured.Unstructured, usage Usage, facts map[string]observation.Value) {
	for _, budget := range usageBudgets(r, object, usage) {
		unit, suffix := 1.0, "bytes"
		if budget.metric == "cpu" {
			unit, suffix = 1e9, "nanocores"
		}
		prefix := "node"
		if budget.subject != "" {
			prefix = "container." + budget.subject
		}
		key := fmt.Sprintf("%s.%s.%s_%s", prefix, budget.basis, budget.metric, suffix)
		facts[key] = observation.Unknown()
		if budget.total != nil {
			facts[key] = measuredNumber(budget.total.AsApproximateFloat64(), unit)
		}
	}
}

func projectQuotaBudgets(o map[string]any, facts map[string]observation.Value) {
	for _, b := range quotaBudgets(o) {
		unit, suffix := 1.0, "units"
		switch b.metric {
		case "cpu", "requests.cpu", "limits.cpu":
			unit, suffix = 1e9, "nanocores"
		case "memory", "requests.memory", "limits.memory", "storage", "requests.storage":
			suffix = "bytes"
		}
		prefix := "quota." + b.metric
		facts[prefix+".unit"] = observation.Text(suffix)
		facts[prefix+".used"], facts[prefix+".hard"] = observation.Unknown(), observation.Unknown()
		if b.used != nil {
			facts[prefix+".used"] = measuredNumber(*b.used, unit)
		}
		if b.total != nil {
			facts[prefix+".hard"] = measuredNumber(b.total.AsApproximateFloat64(), unit)
		}
	}
}

type quantities = map[string]resource.Quantity

func quantityValue(values quantities, key string) *float64 {
	if value, ok := values[key]; ok && value.Sign() >= 0 {
		return new(value.AsApproximateFloat64())
	}
	return nil
}

func quantitiesAt(o map[string]any, path ...string) quantities {
	result := quantities{}
	for key, value := range nestedMap(o, path...) {
		if q, err := resource.ParseQuantity(scalar(value)); err == nil {
			result[key] = q
		}
	}
	return result
}
func addQuantities(target, values quantities) {
	for key, value := range values {
		current := target[key]
		current.Add(value)
		target[key] = current
	}
}
func maxQuantities(target, values quantities) {
	for key, value := range values {
		current, ok := target[key]
		if !ok || value.Cmp(current) > 0 {
			target[key] = value.DeepCopy()
		}
	}
}
func quantityText(values quantities, key string) string {
	if value, ok := values[key]; ok {
		return value.String()
	}
	return "Not specified"
}
func declaredPodResources(o map[string]any, kind string) quantities {
	total, sidecars, initMaximum := quantities{}, quantities{}, quantities{}
	for _, container := range Maps(o, "spec", "containers") {
		addQuantities(total, quantitiesAt(container, "resources", kind))
	}
	for _, container := range Maps(o, "spec", "initContainers") {
		values := quantitiesAt(container, "resources", kind)
		if Value(container, "restartPolicy") == "Always" {
			addQuantities(total, values)
			addQuantities(sidecars, values)
			values = quantities{}
			addQuantities(values, sidecars)
		} else {
			addQuantities(values, sidecars)
		}
		maxQuantities(initMaximum, values)
	}
	maxQuantities(total, initMaximum)
	for key, value := range quantitiesAt(o, "spec", "resources", kind) {
		if key == "cpu" || key == "memory" || strings.HasPrefix(key, "hugepages-") {
			total[key] = value
		}
	}
	for key, value := range quantitiesAt(o, "spec", "overhead") {
		current, exists := total[key]
		if kind == "limits" && (!exists || current.IsZero()) {
			continue
		}
		current.Add(value)
		total[key] = current
	}
	return total
}

type podReservation struct {
	object   unstructured.Unstructured
	requests quantities
}

// Reservations use current specs of non-terminal assigned Pods. The caller
// supplies collection coverage and must not describe a partial list as a total.
func nodeReservations(node string, pods []unstructured.Unstructured) ([]podReservation, map[string]quantities, int) {
	totals := map[string]quantities{"requests": {}, "limits": {}}
	var assigned []podReservation
	counted := 0
	for _, pod := range pods {
		if Value(pod.Object, "spec", "nodeName") != node {
			continue
		}
		requests := declaredPodResources(pod.Object, "requests")
		assigned = append(assigned, podReservation{pod, requests})
		if terminalPod(pod) {
			continue
		}
		counted++
		addQuantities(totals["requests"], requests)
		addQuantities(totals["limits"], declaredPodResources(pod.Object, "limits"))
	}
	return assigned, totals, counted
}

func projectPodReservations(o map[string]any, facts map[string]observation.Value) {
	for _, basis := range []string{"requests", "limits"} {
		values := declaredPodResources(o, basis)
		for _, metric := range []string{"cpu", "memory"} {
			unit, suffix := 1.0, "bytes"
			if metric == "cpu" {
				unit, suffix = 1e9, "nanocores"
			}
			key := "declared." + basis + "." + metric + "_" + suffix
			facts[key] = observation.Unknown()
			if value := quantityValue(values, metric); value != nil {
				facts[key] = measuredNumber(*value, unit)
			}
		}
	}
}

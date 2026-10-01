package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SummarySupplement loads cross-object facts only for views that need them.
// Returned warnings describe unavailable coverage separately from empty data.
func (c *Client) SummarySupplement(ctx context.Context, r Resource, object unstructured.Unstructured, resources []Resource) ([]DetailSection, string) {
	if r.Key() == "services" {
		return c.serviceReadinessSummary(ctx, object, resources)
	}
	if r.Key() != "nodes" && r.Key() != "namespaces" {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	key, ns := "pods", ""
	if r.Key() == "namespaces" {
		key, ns = "resourcequotas", object.GetName()
	}
	target, ok := findResourceIdentity(resources, key)
	if !ok {
		return nil, key + " API unavailable; summary coverage is unavailable."
	}
	collection, err := c.List(ctx, target, ns, "")
	if err != nil {
		return nil, ResourceTitle(target) + ": " + ErrorText(err)
	}
	warning := ""
	if collection.Truncated {
		warning = fmt.Sprintf("Summary is partial: only %d %s were returned.", len(collection.Objects), ResourceTitle(target))
	}
	if r.Key() == "namespaces" {
		sections := []DetailSection{{Title: "Namespace quotas", Fields: []DetailField{{"Quota objects", fmt.Sprint(len(collection.Objects))}, {"Scope", ns}}}}
		for _, quota := range collection.Objects {
			s := quotaSummary(quota.Object)
			for i := range s {
				s[i].Title = quota.GetName() + " · " + s[i].Title
			}
			sections = append(sections, s...)
		}
		return sections, warning
	}
	totals := map[string]map[string]resource.Quantity{"requests": {}, "limits": {}}
	pods := DetailSection{Title: "Assigned Pods", Columns: []string{"Namespace", "Pod", "Phase", "CPU requests", "Memory requests"}}
	counted := 0
	for _, pod := range collection.Objects {
		if Value(pod.Object, "spec", "nodeName") != object.GetName() {
			continue
		}
		phase := Value(pod.Object, "status", "phase")
		requests := declaredPodResources(pod.Object, "requests")
		pods.Rows = append(pods.Rows, []string{pod.GetNamespace(), pod.GetName(), fallback(phase, "Not reported"), quantityText(requests, "cpu"), quantityText(requests, "memory")})
		if phase == "Succeeded" || phase == "Failed" {
			continue
		}
		counted++
		addQuantities(totals["requests"], requests)
		addQuantities(totals["limits"], declaredPodResources(pod.Object, "limits"))
	}
	aggregate := DetailSection{Title: "Declared Pod resources", Columns: []string{"Resource", "Requests", "Limits", "Allocatable"}, Fields: []DetailField{{"Coverage", fmt.Sprintf("%d non-terminal Pods assigned to this Node; queried all namespaces", counted)}, {"Basis", "Current Pod specifications, including init containers, sidecars and overhead. Excludes measured usage and runtime allocation changes."}}}
	keys := map[string]any{}
	for key := range totals["requests"] {
		keys[key] = true
	}
	for key := range totals["limits"] {
		keys[key] = true
	}
	for _, key := range sortedKeys(keys) {
		aggregate.Rows = append(aggregate.Rows, []string{key, quantityText(totals["requests"], key), quantityText(totals["limits"], key), Value(object.Object, "status", "allocatable", key)})
	}
	return []DetailSection{aggregate, pods}, warning
}

type quantities = map[string]resource.Quantity

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

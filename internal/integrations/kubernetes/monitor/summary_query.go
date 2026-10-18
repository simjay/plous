package monitor

import (
	"context"
	"fmt"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SummarySupplement loads cross-object facts only for views that need them.
// Returned warnings describe unavailable coverage separately from empty data.
func (c *Client) SummarySupplement(ctx context.Context, r kubecommon.Resource, object unstructured.Unstructured, resources []kubecommon.Resource) ([]DetailSection, string) {
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
	assigned, totals, counted := nodeReservations(object.GetName(), collection.Objects)
	pods := DetailSection{Title: "Assigned Pods", Columns: []string{"Namespace", "Pod", "Phase", "CPU requests", "Memory requests"}}
	for _, p := range assigned {
		pods.Rows = append(pods.Rows, []string{p.object.GetNamespace(), p.object.GetName(), fallback(Value(p.object.Object, "status", "phase"), "Not reported"), quantityText(p.requests, "cpu"), quantityText(p.requests, "memory")})
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
		if key == "cpu" || key == "memory" {
			request, capacity := quantityText(totals["requests"], key), Value(object.Object, "status", "allocatable", key)
			aggregate.Bars = append(aggregate.Bars, DetailBar{Label: key + " reservations", Metric: key, Detail: request + " requests / " + capacity + " allocatable", Value: quantityValue(totals["requests"], key), Total: quantityNumber(capacity)})
		}
	}
	return []DetailSection{aggregate, pods}, warning
}

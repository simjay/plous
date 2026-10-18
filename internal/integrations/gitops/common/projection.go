package common

import (
	"fmt"
	"strings"
	"time"

	"github.com/simjay/plous/internal/integrations/observation"
)

// ProjectFacts retains the same controller interpretation used by Monitor.
func ProjectFacts(item Item, facts map[string]observation.Value) {
	for key, value := range map[string]string{"status": item.Status, "reason": item.Reason, "sync": item.Sync, "health": item.Health, "destination.name": item.Destination.Name, "destination.server": item.Destination.Server, "destination.namespace": item.Destination.Namespace} {
		if value != "" {
			facts[key] = observation.Text(value)
		}
	}
	facts["inventory_reported"] = observation.Boolean(item.InventoryReported)
	if !item.ObservedAt.IsZero() {
		facts["controller_observed_at"] = observation.Text(item.ObservedAt.Format(time.RFC3339Nano))
	}
	for _, field := range item.Fields {
		facts["detail."+strings.ReplaceAll(strings.ToLower(field.Label), " ", "_")] = observation.Text(field.Value)
	}
	ref := func(prefix string, r Reference) {
		for key, value := range map[string]string{"group": r.Group, "kind": r.Kind, "namespace": r.Namespace, "name": r.Name} {
			facts[prefix+key] = observation.Text(value)
		}
	}
	for n, source := range item.Sources {
		prefix := fmt.Sprintf("source.%d.", n)
		ref(prefix, source.Ref)
		for key, value := range map[string]string{"url": source.URL, "path": source.Path, "revision": source.Revision, "chart": source.Chart} {
			if value != "" {
				facts[prefix+key] = observation.Text(value)
			}
		}
	}
	for n, dependency := range item.Dependencies {
		ref(fmt.Sprintf("dependency.%d.", n), dependency)
	}
	for n, resource := range item.Resources {
		prefix := fmt.Sprintf("managed.%d.", n)
		ref(prefix, resource.Ref)
		for key, value := range map[string]string{"sync": resource.Sync, "health": resource.Health, "message": resource.Message} {
			if value != "" {
				facts[prefix+key] = observation.Text(value)
			}
		}
	}
	facts["managed_resources"] = observation.Unknown()
	if item.InventoryReported {
		facts["managed_resources"] = observation.Number(int64(len(item.Resources)))
	}
}

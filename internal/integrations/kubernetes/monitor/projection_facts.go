package monitor

import (
	"fmt"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectedValue(o map[string]any, path ...string) observation.Value {
	var value any = o
	for _, part := range path {
		m, ok := value.(map[string]any)
		if !ok {
			return observation.Unknown()
		}
		value, ok = m[part]
		if !ok || value == nil {
			return observation.Unknown()
		}
	}
	switch v := value.(type) {
	case string:
		if len(v) > 2048 {
			v = string([]rune(v)[:min(len([]rune(v)), 512)]) + " [excerpt]"
		}
		return observation.Text(v)
	case bool:
		return observation.Boolean(v)
	case float64:
		if v == float64(int64(v)) {
			return observation.Number(int64(v))
		}
		return observation.Unknown()
	case int64:
		return observation.Number(v)
	case int:
		return observation.Number(int64(v))
	}
	return observation.Unknown()
}
func projectIdentity(r kubecommon.Resource, o map[string]any) observation.Identity {
	return observation.Identity{API: r.Group + "/" + r.Version, Kind: r.Kind, Name: Value(o, "metadata", "name"), Namespace: Value(o, "metadata", "namespace"), UID: Value(o, "metadata", "uid"), Revision: Value(o, "metadata", "resourceVersion")}
}
func projectConditions(o map[string]any, facts map[string]observation.Value) {
	for _, c := range Maps(o, "status", "conditions") {
		name := Value(c, "type")
		if name == "" {
			continue
		}
		facts["condition."+name] = projectedValue(c, "status")
		for _, field := range []string{"reason", "message", "lastTransitionTime", "observedGeneration"} {
			if v := projectedValue(c, field); !v.Unknown {
				facts["condition."+name+"."+field] = v
			}
		}
	}
}
func projectResource(r kubecommon.Resource, o map[string]any, now time.Time) (observation.Row, error) {
	row := observation.Row{Identity: projectIdentity(r, o), Facts: map[string]observation.Value{}}
	if kind := Value(o, "kind"); kind != "" && kind != r.Kind {
		return row, fmt.Errorf("Kubernetes returned a different resource kind")
	}
	api := r.Version
	if r.Group != "" {
		api = r.Group + "/" + r.Version
	}
	if actual := Value(o, "apiVersion"); actual != "" && actual != api {
		return row, fmt.Errorf("Kubernetes returned a different API version")
	}
	if row.Identity.Name == "" {
		return row, fmt.Errorf("%s returned an object without a name", r.Key())
	}
	f := row.Facts
	projectStringMap(o, f, "label.", "metadata", "labels")
	f["created_at"] = projectedValue(o, "metadata", "creationTimestamp")
	if created, err := time.Parse(time.RFC3339, Value(o, "metadata", "creationTimestamp")); err == nil {
		f["age_seconds"] = observation.Number(int64(now.Sub(created).Seconds()))
	}
	projectConditions(o, f)
	f["generation"] = projectedValue(o, "metadata", "generation")
	projectDiagnosticFields(r, o, f, now)
	if r.Key() == "resourcequotas" {
		projectQuotaBudgets(o, f)
	}
	if project := resourceProjections[r.Key()]; project != nil {
		if err := project(r, o, f); err != nil {
			return row, err
		}
	} else if _, supported := f["diagnostic_coverage"]; !supported {
		f["projection_coverage"] = observation.Text("identity and reported conditions only")
	}

	return row, nil
}

func projectStringMap(o map[string]any, facts map[string]observation.Value, prefix string, path ...string) {
	var value any = o
	for _, key := range path {
		m, ok := value.(map[string]any)
		if !ok {
			return
		}
		value = m[key]
	}
	m, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, value := range m {
		if text, ok := value.(string); ok {
			facts[prefix+key] = observation.Text(text)
		}
	}
}

var resourceProjections = map[string]func(kubecommon.Resource, map[string]any, map[string]observation.Value) error{"pods": projectPods, "deployments.apps": projectWorkloads, "statefulsets.apps": projectWorkloads, "replicasets.apps": projectWorkloads, "replicationcontrollers": projectWorkloads, "daemonsets.apps": projectWorkloads, "jobs.batch": projectJobs, "cronjobs.batch": projectJobs, "services": projectServices, "persistentvolumeclaims": projectVolumes, "persistentvolumes": projectVolumes, "events": projectEvents, "events.events.k8s.io": projectEvents, "nodes": projectNodes}

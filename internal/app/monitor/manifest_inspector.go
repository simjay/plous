package monitor

import (
	"fmt"
	"sort"
	"strings"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
)

func ResourceFacts(r manifest.Resource, width int) []string {
	object, _ := manifest.Redact(r.Object).(map[string]any)
	path := func(keys ...string) any {
		var value any = object
		for _, key := range keys {
			item, _ := value.(map[string]any)
			value = item[key]
		}
		return value
	}
	var fields []kubernetes.DetailField
	add := func(label string, value any) {
		if value != nil {
			fields = append(fields, kubernetes.DetailField{Label: label, Value: ResourceFact(value)})
		}
	}
	var pod any
	switch r.Kind {
	case "Deployment", "StatefulSet", "ReplicaSet", "DaemonSet", "Job":
		add("Replicas", path("spec", "replicas"))
		pod = path("spec", "template", "spec")
	case "CronJob":
		add("Schedule", path("spec", "schedule"))
		pod = path("spec", "jobTemplate", "spec", "template", "spec")
	case "Pod":
		pod = path("spec")
	case "Service":
		add("Type", path("spec", "type"))
		ports, _ := path("spec", "ports").([]any)
		var labels []string
		for _, port := range ports {
			p, _ := port.(map[string]any)
			target, protocol := p["targetPort"], p["protocol"]
			if target == nil {
				target = p["port"]
			}
			if protocol == nil {
				protocol = "TCP"
			}
			labels = append(labels, fmt.Sprintf("%v → %v/%v", p["port"], target, protocol))
		}
		add("Ports", strings.Join(labels, ", "))
		add("Pod labels", path("spec", "selector"))
	case "ConfigMap", "Secret":
		keys := map[string]bool{}
		for _, field := range []string{"data", "binaryData", "stringData"} {
			values, _ := r.Object[field].(map[string]any)
			for key := range values {
				keys[key] = true
			}
		}
		var names []string
		for key := range keys {
			names = append(names, key)
		}
		sort.Strings(names)
		add("Keys", strings.Join(names, ", "))
	case "HorizontalPodAutoscaler":
		add("Scales", path("spec", "scaleTargetRef"))
		add("Min replicas", path("spec", "minReplicas"))
		add("Max replicas", path("spec", "maxReplicas"))
	case "RoleBinding", "ClusterRoleBinding":
		add("Role", path("roleRef"))
	case "PersistentVolumeClaim":
		add("Storage class", path("spec", "storageClassName"))
		add("Storage", path("spec", "resources", "requests", "storage"))
	}
	if spec, ok := pod.(map[string]any); ok {
		add("Service account", spec["serviceAccountName"])
		var images, ports, refs []string
		for _, kind := range []string{"initContainers", "containers"} {
			containers, _ := spec[kind].([]any)
			for _, item := range containers {
				c, _ := item.(map[string]any)
				images = append(images, fmt.Sprintf("%v: %v", c["name"], c["image"]))
				ps, _ := c["ports"].([]any)
				for _, item := range ps {
					p, _ := item.(map[string]any)
					ports = append(ports, fmt.Sprint(p["containerPort"]))
				}
			}
		}
		refs = ResourceNamedReferences(r)
		if len(images) > 0 {
			add("Images", strings.Join(images, "; "))
		}
		if len(ports) > 0 {
			add("Ports", strings.Join(ports, ", "))
		}
		if len(refs) > 0 {
			sort.Strings(refs)
			add("References", strings.Join(refs, "; "))
		}
	}
	return RenderSummaryFields(fields, width)
}

func ResourceFact(value any) string {
	switch value.(type) {
	case map[string]any, []any:
		return ui.OneLine(strings.TrimSpace(manifest.YAML(value, false)))
	default:
		return ui.OneLine(fmt.Sprint(value))
	}
}

// Read only object names from references; literal env values and secret data stay masked.
func ResourceNamedReferences(r manifest.Resource) []string {
	var refs []string
	add := func(node map[string]any, key, field string) {
		if ref, ok := node[key].(map[string]any); ok {
			if name, ok := ref[field].(string); ok {
				refs = append(refs, key+": "+ui.OneLine(name))
			}
		}
	}
	value := any(r.Object)
	keys := []string{"spec", "template", "spec"}
	if r.Kind == "Pod" {
		keys = []string{"spec"}
	}
	if r.Kind == "CronJob" {
		keys = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	for _, key := range keys {
		node, _ := value.(map[string]any)
		value = node[key]
	}
	spec, _ := value.(map[string]any)
	for _, kind := range []string{"containers", "initContainers"} {
		containers, _ := spec[kind].([]any)
		for _, item := range containers {
			c, _ := item.(map[string]any)
			for _, field := range []string{"env", "envFrom"} {
				entries, _ := c[field].([]any)
				for _, item := range entries {
					entry, _ := item.(map[string]any)
					if field == "env" {
						entry, _ = entry["valueFrom"].(map[string]any)
					}
					for _, key := range []string{"secretKeyRef", "secretRef", "configMapKeyRef", "configMapRef"} {
						add(entry, key, "name")
					}
				}
			}
		}
	}
	volumes, _ := spec["volumes"].([]any)
	for _, item := range volumes {
		volume, _ := item.(map[string]any)
		add(volume, "secret", "secretName")
		add(volume, "configMap", "name")
		add(volume, "persistentVolumeClaim", "claimName")
	}
	sort.Strings(refs)
	return refs
}

func ResourceGroup(r manifest.Resource) string {
	switch r.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Pod", "Job", "CronJob":
		return "Workloads"
	case "Service", "Ingress", "NetworkPolicy", "Gateway", "HTTPRoute":
		return "Networking"
	case "ConfigMap", "Secret":
		return "Configuration"
	case "PersistentVolumeClaim", "PersistentVolume", "StorageClass":
		return "Storage"
	case "HorizontalPodAutoscaler", "PodDisruptionBudget":
		return "Scaling & availability"
	case "ServiceAccount", "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding":
		return "Access"
	default:
		return "Other resources"
	}
}

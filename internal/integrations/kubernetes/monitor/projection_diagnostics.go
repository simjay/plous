package monitor

import (
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// These are configuration and relationship fields, not alternative status
// calculations. Readiness, workload counts and usage keep their existing owners.
func projectDiagnosticFields(r kubecommon.Resource, o map[string]any, f map[string]observation.Value, now time.Time) {
	selected := map[string]any{}
	if metadata, ok := o["metadata"].(map[string]any); ok {
		selected["metadata"] = map[string]any{"ownerReferences": metadata["ownerReferences"]}
	}
	supported := true
	switch r.Key() {
	case "pods", "deployments.apps", "statefulsets.apps", "replicasets.apps", "replicationcontrollers", "daemonsets.apps", "jobs.batch", "cronjobs.batch", "services", "persistentvolumeclaims", "persistentvolumes", "nodes", "ingresses.networking.k8s.io", "networkpolicies.networking.k8s.io", "horizontalpodautoscalers.autoscaling", "poddisruptionbudgets.policy", "resourcequotas", "limitranges", "gateways.gateway.networking.k8s.io", "gatewayclasses.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io", "grpcroutes.gateway.networking.k8s.io", "tcproutes.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "udproutes.gateway.networking.k8s.io", "referencegrants.gateway.networking.k8s.io":
		selected["spec"] = o["spec"]
		switch r.Key() {
		case "horizontalpodautoscalers.autoscaling", "poddisruptionbudgets.policy", "resourcequotas":
			selected["status"] = o["status"]
		}
	case "endpoints":
		selected["subsets"] = o["subsets"]
	case "endpointslices.discovery.k8s.io":
		for _, key := range []string{"addressType", "endpoints", "ports"} {
			selected[key] = o[key]
		}
	case "roles.rbac.authorization.k8s.io", "clusterroles.rbac.authorization.k8s.io":
		selected["rules"], selected["aggregationRule"] = o["rules"], o["aggregationRule"]
	case "rolebindings.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io":
		selected["subjects"], selected["roleRef"] = o["subjects"], o["roleRef"]
	case "pods.metrics.k8s.io", "nodes.metrics.k8s.io":
		for _, key := range []string{"timestamp", "window", "containers", "usage"} {
			selected[key] = o[key]
		}
		usage, err := parseUsage(unstructured.Unstructured{Object: o}, r.Name == "nodes")
		f["cpu_nanocores"], f["memory_bytes"], f["fresh"] = observation.Unknown(), observation.Unknown(), observation.Unknown()
		f["sample_scope"] = observation.Text("reported metrics sample; not joined to current object creation or container identity")
		if err == nil {
			projectMeasurementValues(usage, f)
			f["fresh"] = observation.Boolean(usage.Fresh(now))
		} else {
			f["usage_unavailable"] = observation.Text(err.Error())
		}
	default:
		supported = false
	}
	fields, complete := manifest.Fields(selected)
	for n, field := range fields {
		if n >= 256 {
			complete = false
			break
		}
		f[field.Pointer] = field.Value
	}
	if supported {
		f["diagnostic_coverage"] = observation.Text("selected configuration and relationships; sensitive fields masked")
	}
	if !complete {
		f["diagnostic_coverage"] = observation.Text("partial: diagnostic field limit reached or oversized values omitted")
	}
}

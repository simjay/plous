package monitor

import (
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

func (p *relationCollector) optionalRelationships(r kubecommon.Resource, object unstructured.Unstructured) {
	o, ns, name := object.Object, object.GetNamespace(), object.GetName()
	switch r.Key() {
	case "deployments.apps", "cronjobs.batch", "statefulsets.apps", "daemonsets.apps":
		if p.client == nil || object.GetUID() == "" {
			break
		}
		if _, ok := findResourceIdentity(p.resources, "pods"); !ok {
			break
		}
		pods, err := p.client.PodTargets(p.ctx, r, &object, p.resources)
		if err != nil {
			p.warnings = append(p.warnings, "Descendant Pods: "+OperationErrorText(err))
			break
		}
		for _, pod := range pods {
			p.named("pods", pod.GetNamespace(), pod.GetName(), string(pod.GetUID()), "Descendant by controller UID")
			if r.Key() == "statefulsets.apps" {
				for _, volume := range Maps(pod.Object, "spec", "volumes") {
					p.named("persistentvolumeclaims", pod.GetNamespace(), Value(volume, "persistentVolumeClaim", "claimName"), "", "Owned Pod volume claim")
				}
			}
			if r.Key() == "daemonsets.apps" {
				p.named("nodes", "", Value(pod.Object, "spec", "nodeName"), "", "Owned Pod assigned node")
			}
		}
	case "resourceclaims.resource.k8s.io":
		p.query("resourceslices.resource.k8s.io", "", "Exact allocated driver/pool/device", func(slice unstructured.Unstructured) bool { return allocationUsesSlice(o, slice.Object) })
	case "resourceclaimtemplates.resource.k8s.io":
		p.query("resourceclaims.resource.k8s.io", ns, "Owned generated claim", func(claim unstructured.Unstructured) bool { return hasOwner(claim, object) })
	case "resourceslices.resource.k8s.io":
		p.query("resourceclaims.resource.k8s.io", "", "Exact allocated driver/pool/device", func(claim unstructured.Unstructured) bool { return allocationUsesSlice(claim.Object, o) })
	case "deviceclasses.resource.k8s.io":
		for _, key := range []string{"resourceclaims.resource.k8s.io", "resourceclaimtemplates.resource.k8s.io"} {
			p.query(key, "", "Device class reference", func(candidate unstructured.Unstructured) bool {
				spec := nestedMap(candidate.Object, "spec")
				if strings.HasPrefix(key, "resourceclaimtemplates.") {
					spec = nestedMap(candidate.Object, "spec", "spec")
				}
				for _, class := range strings.Split(claimClassNames(spec), ", ") {
					if class == name {
						return true
					}
				}
				return false
			})
		}
	case "clustertrustbundles.certificates.k8s.io":
		p.query("pods", "", "Projected trust bundle reference", func(pod unstructured.Unstructured) bool {
			for _, volume := range Maps(pod.Object, "spec", "volumes") {
				for _, source := range Maps(volume, "projected", "sources") {
					ref := nestedMap(source, "clusterTrustBundle")
					if Value(ref, "name") == name {
						return true
					}
					if Value(ref, "signerName") != "" && Value(ref, "signerName") == Value(o, "spec", "signerName") && selectorMatches(nestedMap(ref, "labelSelector"), object.GetLabels(), false) {
						return true
					}
				}
			}
			return false
		})
	case "podsecuritypolicies.policy":
		for _, key := range []string{"roles.rbac.authorization.k8s.io", "clusterroles.rbac.authorization.k8s.io"} {
			p.query(key, "", "Rule grants use of this historical policy", func(role unstructured.Unstructured) bool {
				for _, rule := range Maps(role.Object, "rules") {
					if listContains(rule, "apiGroups", "policy") && listContains(rule, "resources", "podsecuritypolicies") && listContains(rule, "verbs", "use") {
						names, _, _ := unstructured.NestedStringSlice(rule, "resourceNames")
						if len(names) == 0 {
							return true
						}
						for _, candidate := range names {
							if candidate == name {
								return true
							}
						}
					}
				}
				return false
			})
		}
	case "runtimeclasses.node.k8s.io":
		selector, _, _ := unstructured.NestedStringMap(o, "scheduling", "nodeSelector")
		if len(selector) > 0 {
			p.query("nodes", "", "Runtime node selector match", func(node unstructured.Unstructured) bool {
				return labels.SelectorFromSet(selector).Matches(labels.Set(node.GetLabels()))
			})
		}
	case "csistoragecapacities.storage.k8s.io":
		selector := nestedMap(o, "nodeTopology")
		if selector != nil {
			p.query("nodes", "", "Storage topology selector match", func(node unstructured.Unstructured) bool { return selectorMatches(selector, node.GetLabels(), false) })
		}
	case "networkpolicies.networking.k8s.io":
		var selectors []map[string]any
		for _, direction := range []struct{ key, peer string }{{"ingress", "from"}, {"egress", "to"}} {
			for _, rule := range Maps(o, "spec", direction.key) {
				for _, peer := range Maps(rule, direction.peer) {
					if selector := nestedMap(peer, "namespaceSelector"); selector != nil {
						selectors = append(selectors, selector)
					}
				}
			}
		}
		if len(selectors) > 0 {
			p.query("namespaces", "", "Policy namespace selector match", func(candidate unstructured.Unstructured) bool {
				for _, selector := range selectors {
					if selectorMatches(selector, candidate.GetLabels(), true) {
						return true
					}
				}
				return false
			})
		}
	case "clusterroles.rbac.authorization.k8s.io":
		selectors := Maps(o, "aggregationRule", "clusterRoleSelectors")
		if len(selectors) > 0 {
			p.query("clusterroles.rbac.authorization.k8s.io", "", "Aggregation selector match", func(candidate unstructured.Unstructured) bool {
				if candidate.GetName() == name {
					return false
				}
				for _, selector := range selectors {
					if selectorMatches(selector, candidate.GetLabels(), true) {
						return true
					}
				}
				return false
			})
		}
	case "referencegrants.gateway.networking.k8s.io":
		for _, to := range Maps(o, "spec", "to") {
			if Value(to, "name") != "" {
				p.reference(to, ns, "", "", "Permitted target reference")
			}
		}
		for _, from := range Maps(o, "spec", "from") {
			for _, target := range p.resources {
				if target.Group != Value(from, "group") || target.Kind != Value(from, "kind") {
					continue
				}
				if !strings.HasSuffix(target.Key(), ".gateway.networking.k8s.io") {
					continue
				}
				p.query(target.Key(), Value(from, "namespace"), "Cross-namespace reference permitted by grant", func(candidate unstructured.Unstructured) bool {
					for _, ref := range gatewayCrossNamespaceRefs(candidate) {
						if fallback(Value(ref, "namespace"), candidate.GetNamespace()) != ns {
							continue
						}
						for _, to := range Maps(o, "spec", "to") {
							if Value(ref, "group") == Value(to, "group") && Value(ref, "kind") == Value(to, "kind") && (Value(to, "name") == "" || Value(ref, "name") == Value(to, "name")) {
								return true
							}
						}
					}
					return false
				})
			}
		}
	case "gateways.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io", "grpcroutes.gateway.networking.k8s.io", "tcproutes.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "udproutes.gateway.networking.k8s.io":
		references := gatewayCrossNamespaceRefs(object)
		namespaces := map[string]bool{}
		for _, ref := range references {
			targetNS := Value(ref, "namespace")
			if targetNS != "" && targetNS != ns {
				namespaces[targetNS] = true
			}
		}
		for targetNS := range namespaces {
			p.query("referencegrants.gateway.networking.k8s.io", targetNS, "Permits cross-namespace reference", func(grant unstructured.Unstructured) bool {
				fromMatches := false
				for _, from := range Maps(grant.Object, "spec", "from") {
					if Value(from, "namespace") == ns && Value(from, "group") == r.Group && Value(from, "kind") == r.Kind {
						fromMatches = true
					}
				}
				if !fromMatches {
					return false
				}
				for _, ref := range references {
					if Value(ref, "namespace") != targetNS {
						continue
					}
					for _, to := range Maps(grant.Object, "spec", "to") {
						if Value(to, "group") == Value(ref, "group") && Value(to, "kind") == Value(ref, "kind") && (Value(to, "name") == "" || Value(to, "name") == Value(ref, "name")) {
							return true
						}
					}
				}
				return false
			})
		}
	}
}
func allocationUsesSlice(claim, slice map[string]any) bool {
	driver, pool := Value(slice, "spec", "driver"), Value(slice, "spec", "pool", "name")
	if driver == "" || pool == "" {
		return false
	}
	devices := map[string]bool{}
	for _, device := range Maps(slice, "spec", "devices") {
		devices[Value(device, "name")] = true
	}
	for _, allocation := range Maps(claim, "status", "allocation", "devices", "results") {
		if Value(allocation, "driver") == driver && Value(allocation, "pool") == pool && devices[Value(allocation, "device")] {
			return true
		}
	}
	return false
}
func selectorMatches(selector map[string]any, values map[string]string, nilMatches bool) bool {
	if selector == nil {
		return nilMatches
	}
	var typed metav1.LabelSelector
	if runtime.DefaultUnstructuredConverter.FromUnstructured(selector, &typed) != nil {
		return false
	}
	parsed, err := metav1.LabelSelectorAsSelector(&typed)
	return err == nil && parsed.Matches(labels.Set(values))
}
func listContains(o map[string]any, key, want string) bool {
	values, _, _ := unstructured.NestedStringSlice(o, key)
	for _, value := range values {
		if value == want || value == "*" {
			return true
		}
	}
	return false
}
func gatewayCrossNamespaceRefs(object unstructured.Unstructured) []map[string]any {
	var refs []map[string]any
	add := func(ref map[string]any, kind string) {
		if Value(ref, "name") == "" {
			return
		}
		copy := map[string]any{}
		for key, value := range ref {
			copy[key] = value
		}
		if _, ok := copy["kind"]; !ok {
			copy["kind"] = kind
		}
		if _, ok := copy["group"]; !ok {
			copy["group"] = ""
		}
		refs = append(refs, copy)
	}
	for _, listener := range Maps(object.Object, "spec", "listeners") {
		for _, ref := range Maps(listener, "tls", "certificateRefs") {
			add(ref, "Secret")
		}
	}
	for _, rule := range Maps(object.Object, "spec", "rules") {
		for _, ref := range Maps(rule, "backendRefs") {
			add(ref, "Service")
		}
		for _, filter := range Maps(rule, "filters") {
			add(nestedMap(filter, "requestMirror", "backendRef"), "Service")
		}
	}
	return refs
}

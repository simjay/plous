package kubernetes

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type relationCollector struct {
	client    *Client
	ctx       context.Context
	resources []Resource
	add       func(Resource, string, string, string, string)
	warnings  []string
}

func (p *relationCollector) named(key, ns, name, uid, relation string) {
	if name == "" {
		return
	}
	r, ok := findResourceIdentity(p.resources, key)
	if !ok {
		p.warnings = append(p.warnings, key+" API unavailable")
		return
	}
	if !r.Namespaced {
		ns = ""
	}
	p.add(r, ns, name, uid, relation)
}

func (p *relationCollector) boundVolume(claim unstructured.Unstructured) {
	name := Value(claim.Object, "spec", "volumeName")
	if name == "" {
		return
	}
	r, ok := findResourceIdentity(p.resources, "persistentvolumes")
	if !ok {
		p.warnings = append(p.warnings, "persistentvolumes API unavailable")
		return
	}
	if p.client == nil {
		p.add(r, "", name, "", "Selected volume · binding not verified")
		return
	}
	volume, err := p.client.Get(p.ctx, r, "", name)
	if err != nil {
		p.add(r, "", name, "", "Selected volume · binding not verified")
		p.warnings = append(p.warnings, "Volume binding: "+ErrorText(err))
		return
	}
	ref := nestedMap(volume.Object, "spec", "claimRef")
	if Value(ref, "name") != claim.GetName() || Value(ref, "namespace") != claim.GetNamespace() || string(claim.GetUID()) == "" || Value(ref, "uid") != string(claim.GetUID()) {
		p.warnings = append(p.warnings, "Selected volume "+name+" does not report a binding to this claim UID.")
		return
	}
	p.add(r, "", name, string(volume.GetUID()), "Bound volume · verified claim UID")
}

func (p *relationCollector) reference(ref map[string]any, ns, defaultGroup, defaultKind, relation string) {
	name := Value(ref, "name")
	if name == "" {
		return
	}
	group := defaultGroup
	kind := fallback(Value(ref, "kind"), defaultKind)
	if value, exists := ref["group"]; exists {
		group = scalar(value)
	}
	if value, exists := ref["apiGroup"]; exists {
		group = scalar(value)
	}
	if version := Value(ref, "apiVersion"); version != "" {
		gv, err := schema.ParseGroupVersion(version)
		if err != nil {
			return
		}
		group = gv.Group
	}
	if explicitNS := Value(ref, "namespace"); explicitNS != "" {
		ns = explicitNS
	}
	for _, r := range p.resources {
		if r.Group == group && r.Kind == kind {
			if !r.Namespaced {
				ns = ""
			}
			p.add(r, ns, name, Value(ref, "uid"), relation)
			return
		}
	}
	p.warnings = append(p.warnings, fallback(group, "core")+"/"+kind+" API unavailable")
}
func (p *relationCollector) query(key, ns, relation string, match func(unstructured.Unstructured) bool) {
	p.queryRelation(key, ns, func(object unstructured.Unstructured) string {
		if match(object) {
			return relation
		}
		return ""
	})
}

func (p *relationCollector) queryRelation(key, ns string, relation func(unstructured.Unstructured) string) {
	if p.ctx.Err() != nil {
		return
	}
	r, ok := findResourceIdentity(p.resources, key)
	if !ok || p.client == nil {
		return
	}
	if !r.Namespaced {
		ns = ""
	}
	collection, err := p.client.List(p.ctx, r, ns, "")
	if err != nil {
		p.warnings = append(p.warnings, ResourceTitle(r)+": "+ErrorText(err))
		return
	}
	scope := "cluster"
	if r.Namespaced {
		scope = ns
		if scope == "" {
			scope = "all namespaces"
		}
	}
	if collection.Truncated {
		p.warnings = append(p.warnings, ResourceTitle(r)+" related results limited to "+fmt.Sprint(MaxObjects)+" objects in "+scope)
	}
	for _, object := range collection.Objects {
		if label := relation(object); label != "" {
			p.add(r, object.GetNamespace(), object.GetName(), string(object.GetUID()), label+" · "+scope)
		}
	}
}
func (p *relationCollector) selectorPods(ns string, selector map[string]any, emptyMatches bool) {
	if selector == nil && !emptyMatches {
		return
	}
	var spec metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(selector, &spec); err != nil {
		p.warnings = append(p.warnings, "Pod selector could not be read")
		return
	}
	selection, err := metav1.LabelSelectorAsSelector(&spec)
	if err != nil {
		p.warnings = append(p.warnings, "Pod selector is invalid")
		return
	}
	p.query("pods", ns, "Selector match", func(candidate unstructured.Unstructured) bool {
		return selection.Matches(labels.Set(candidate.GetLabels()))
	})
}

func (c *Client) additionalRelated(ctx context.Context, r Resource, object unstructured.Unstructured, resources []Resource, add func(Resource, string, string, string, string)) []string {
	p := relationCollector{client: c, ctx: ctx, resources: resources, add: add}
	o, ns, name := object.Object, object.GetNamespace(), object.GetName()
	// Every namespaced object has an explicit namespace scope, not an owner link.
	if r.Namespaced {
		p.namedIfServed("namespaces", "", ns, "Namespace scope")
	}
	if !HasPresenter(r) {
		return p.warnings
	}
	switch r.Key() {
	case "pods":
		p.podReferences(nestedMap(o, "spec"), ns)
		p.query("services", ns, "Service selector match", func(candidate unstructured.Unstructured) bool {
			selector, _, _ := unstructured.NestedStringMap(candidate.Object, "spec", "selector")
			return len(selector) > 0 && labels.SelectorFromSet(selector).Matches(labels.Set(object.GetLabels()))
		})
	case "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers", "jobs.batch", "cronjobs.batch":
		spec := nestedMap(o, "spec", "template", "spec")
		if r.Key() == "cronjobs.batch" {
			spec = nestedMap(o, "spec", "jobTemplate", "spec", "template", "spec")
		}
		p.podReferences(spec, ns)
		if r.Key() == "statefulsets.apps" {
			p.named("services", ns, Value(o, "spec", "serviceName"), "", "Governing service")
		}
		for _, key := range []string{"horizontalpodautoscalers.autoscaling", "verticalpodautoscalers.autoscaling.k8s.io"} {
			targetPath := "scaleTargetRef"
			if strings.HasPrefix(key, "vertical") {
				targetPath = "targetRef"
			}
			p.query(key, ns, "Autoscaler target reference", func(candidate unstructured.Unstructured) bool {
				return matchesReference(nestedMap(candidate.Object, "spec", targetPath), r, object, ns)
			})
		}
		if r.Key() == "statefulsets.apps" || r.Key() == "daemonsets.apps" {
			p.query("controllerrevisions.apps", ns, "Owned revision", func(candidate unstructured.Unstructured) bool { return hasOwner(candidate, object) })
		}
	case "services":
		selector, _, _ := unstructured.NestedStringMap(o, "spec", "selector")
		if len(selector) > 0 {
			p.query("pods", ns, "Service selector match", func(candidate unstructured.Unstructured) bool {
				return labels.SelectorFromSet(selector).Matches(labels.Set(candidate.GetLabels()))
			})
		}
		p.query("ingresses.networking.k8s.io", ns, "Backend reference", func(candidate unstructured.Unstructured) bool {
			for _, ref := range ingressBackendRefs(candidate.Object) {
				if Value(ref, "kind") == "Service" && Value(ref, "name") == name {
					return true
				}
			}
			return false
		})
	case "ingresses.networking.k8s.io":
		p.named("ingressclasses.networking.k8s.io", "", Value(o, "spec", "ingressClassName"), "", "Ingress class")
		for _, ref := range ingressBackendRefs(o) {
			p.reference(ref, ns, "", "Service", "Backend reference")
		}
		for _, tls := range Maps(o, "spec", "tls") {
			p.named("secrets", ns, Value(tls, "secretName"), "", "TLS certificate reference")
		}
	case "ingressclasses.networking.k8s.io":
		p.query("ingresses.networking.k8s.io", "", "Ingress class reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "ingressClassName") == name
		})
		p.reference(nestedMap(o, "spec", "parameters"), "", "", "", "Parameters reference")
	case "endpoints":
		for _, subset := range Maps(o, "subsets") {
			for _, kind := range []string{"addresses", "notReadyAddresses"} {
				for _, address := range Maps(subset, kind) {
					p.reference(nestedMap(address, "targetRef"), ns, "", "Pod", "Endpoint target")
					p.named("nodes", "", Value(address, "nodeName"), "", "Endpoint node")
				}
			}
		}
	case "endpointslices.discovery.k8s.io":
		for _, endpoint := range Maps(o, "endpoints") {
			p.reference(nestedMap(endpoint, "targetRef"), ns, "", "Pod", "Endpoint target")
			p.named("nodes", "", Value(endpoint, "nodeName"), "", "Endpoint node")
		}
	case "networkpolicies.networking.k8s.io":
		p.selectorPods(ns, nestedMap(o, "spec", "podSelector"), true)
	case "poddisruptionbudgets.policy":
		selector := nestedMap(o, "spec", "selector")
		if selector != nil && (len(nestedMap(selector, "matchLabels")) > 0 || len(Maps(selector, "matchExpressions")) > 0 || r.Version != "v1beta1") {
			p.selectorPods(ns, selector, true)
		}
	case "horizontalpodautoscalers.autoscaling":
		p.reference(nestedMap(o, "spec", "scaleTargetRef"), ns, "", "", "Scale target")
	case "verticalpodautoscalers.autoscaling.k8s.io":
		p.reference(nestedMap(o, "spec", "targetRef"), ns, "", "", "Target workload")
	case "ipaddresses.networking.k8s.io":
		parent := nestedMap(o, "spec", "parentRef")
		key := Value(parent, "resource")
		if group := Value(parent, "group"); group != "" {
			key += "." + group
		}
		p.named(key, Value(parent, "namespace"), Value(parent, "name"), "", "Allocation parent")
	case "servicecidrs.networking.k8s.io":
		var ranges []netip.Prefix
		for _, text := range strings.Split(stringList(o, "spec", "cidrs"), ", ") {
			if prefix, err := netip.ParsePrefix(text); err == nil {
				ranges = append(ranges, prefix)
			}
		}
		p.query("ipaddresses.networking.k8s.io", "", "Address in CIDR", func(candidate unstructured.Unstructured) bool {
			ip, err := netip.ParseAddr(candidate.GetName())
			if err != nil {
				return false
			}
			for _, prefix := range ranges {
				if prefix.Contains(ip) {
					return true
				}
			}
			return false
		})
	case "gateways.gateway.networking.k8s.io":
		p.named("gatewayclasses.gateway.networking.k8s.io", "", Value(o, "spec", "gatewayClassName"), "", "Gateway class")
		p.reference(nestedMap(o, "spec", "infrastructure", "parametersRef"), ns, "", "", "Infrastructure parameters reference")
		for _, listener := range Maps(o, "spec", "listeners") {
			for _, cert := range Maps(listener, "tls", "certificateRefs") {
				p.reference(cert, ns, "", "Secret", "Listener TLS reference")
			}
		}
		for _, key := range gatewayRouteKeys {
			p.queryRelation(key, "", func(candidate unstructured.Unstructured) string {
				return gatewayRouteRelation(candidate, r, object)
			})
		}
	case "gatewayclasses.gateway.networking.k8s.io":
		p.reference(nestedMap(o, "spec", "parametersRef"), "", "", "", "Parameters reference")
		p.query("gateways.gateway.networking.k8s.io", "", "Gateway class reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "gatewayClassName") == name
		})
	case "httproutes.gateway.networking.k8s.io", "grpcroutes.gateway.networking.k8s.io", "tcproutes.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "udproutes.gateway.networking.k8s.io":
		for _, ref := range Maps(o, "spec", "parentRefs") {
			p.reference(ref, ns, "gateway.networking.k8s.io", "Gateway", "Route parent reference")
		}
		for _, rule := range Maps(o, "spec", "rules") {
			for _, ref := range Maps(rule, "backendRefs") {
				p.reference(ref, ns, "", "Service", "Route backend reference")
			}
			for _, filter := range Maps(rule, "filters") {
				if ref := nestedMap(filter, "requestMirror", "backendRef"); ref != nil {
					p.reference(ref, ns, "", "Service", "Mirror backend reference")
				}
				p.reference(nestedMap(filter, "extensionRef"), ns, "", "", "Extension filter reference")
			}
		}
	case "persistentvolumeclaims":
		p.boundVolume(object)
		p.reference(nestedMap(o, "spec", "dataSource"), ns, "", "", "Data source")
		p.reference(nestedMap(o, "spec", "dataSourceRef"), ns, "", "", "Data source reference")
		p.named("volumeattributesclasses.storage.k8s.io", "", Value(o, "spec", "volumeAttributesClassName"), "", "Volume attributes class")
		p.query("pods", ns, "Volume claim reference", func(candidate unstructured.Unstructured) bool {
			for _, volume := range Maps(candidate.Object, "spec", "volumes") {
				if Value(volume, "persistentVolumeClaim", "claimName") == name {
					return true
				}
			}
			return false
		})
	case "persistentvolumes":
		p.reference(nestedMap(o, "spec", "claimRef"), ns, "", "PersistentVolumeClaim", "Bound claim identity")
		p.named("csidrivers.storage.k8s.io", "", Value(o, "spec", "csi", "driver"), "", "CSI driver")
		p.query("volumeattachments.storage.k8s.io", "", "Attached volume reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "source", "persistentVolumeName") == name
		})
	case "storageclasses.storage.k8s.io":
		for _, key := range []string{"persistentvolumeclaims", "persistentvolumes"} {
			p.query(key, "", "Storage class reference", func(candidate unstructured.Unstructured) bool {
				return Value(candidate.Object, "spec", "storageClassName") == name
			})
		}
		p.namedIfServed("csidrivers.storage.k8s.io", "", Value(o, "provisioner"), "Provisioner identity")
	case "csidrivers.storage.k8s.io":
		p.query("csinodes.storage.k8s.io", "", "Registered CSI driver", func(candidate unstructured.Unstructured) bool {
			for _, driver := range Maps(candidate.Object, "spec", "drivers") {
				if Value(driver, "name") == name {
					return true
				}
			}
			return false
		})
		p.query("storageclasses.storage.k8s.io", "", "Provisioner identity", func(candidate unstructured.Unstructured) bool { return Value(candidate.Object, "provisioner") == name })
		p.query("persistentvolumes", "", "CSI driver reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "csi", "driver") == name
		})
		p.query("volumeattachments.storage.k8s.io", "", "Attacher identity", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "attacher") == name
		})
	case "csinodes.storage.k8s.io":
		p.namedIfServed("nodes", "", name, "CSI node name association")
		for _, driver := range Maps(o, "spec", "drivers") {
			p.named("csidrivers.storage.k8s.io", "", Value(driver, "name"), "", "Registered driver")
		}
	case "csistoragecapacities.storage.k8s.io":
		p.named("storageclasses.storage.k8s.io", "", Value(o, "storageClassName"), "", "Storage class")
	case "volumeattachments.storage.k8s.io":
		p.named("persistentvolumes", "", Value(o, "spec", "source", "persistentVolumeName"), "", "Source volume")
		p.named("nodes", "", Value(o, "spec", "nodeName"), "", "Attachment node")
		p.named("csidrivers.storage.k8s.io", "", Value(o, "spec", "attacher"), "", "Attacher identity")
	case "volumeattributesclasses.storage.k8s.io":
		p.named("csidrivers.storage.k8s.io", "", Value(o, "driverName"), "", "CSI driver")
		for _, key := range []string{"persistentvolumeclaims", "persistentvolumes"} {
			p.query(key, "", "Volume attributes class reference", func(candidate unstructured.Unstructured) bool {
				return Value(candidate.Object, "spec", "volumeAttributesClassName") == name
			})
		}
	case "volumesnapshots.snapshot.storage.k8s.io":
		p.named("persistentvolumeclaims", ns, Value(o, "spec", "source", "persistentVolumeClaimName"), "", "Source claim")
		p.named("volumesnapshotcontents.snapshot.storage.k8s.io", "", Value(o, "status", "boundVolumeSnapshotContentName"), "", "Bound snapshot content")
		p.named("volumesnapshotclasses.snapshot.storage.k8s.io", "", Value(o, "spec", "volumeSnapshotClassName"), "", "Snapshot class")
		p.query("persistentvolumeclaims", ns, "Snapshot data source reference", func(candidate unstructured.Unstructured) bool {
			for _, field := range []string{"dataSource", "dataSourceRef"} {
				if matchesReference(nestedMap(candidate.Object, "spec", field), r, object, candidate.GetNamespace()) {
					return true
				}
			}
			return false
		})
	case "volumesnapshotcontents.snapshot.storage.k8s.io":
		p.reference(nestedMap(o, "spec", "volumeSnapshotRef"), ns, "snapshot.storage.k8s.io", "VolumeSnapshot", "Bound snapshot identity")
		p.named("volumesnapshotclasses.snapshot.storage.k8s.io", "", Value(o, "spec", "volumeSnapshotClassName"), "", "Snapshot class")
		p.named("csidrivers.storage.k8s.io", "", Value(o, "spec", "driver"), "", "CSI driver")
	case "volumesnapshotclasses.snapshot.storage.k8s.io":
		p.named("csidrivers.storage.k8s.io", "", Value(o, "driver"), "", "CSI driver")
		for _, key := range []string{"volumesnapshots.snapshot.storage.k8s.io", "volumesnapshotcontents.snapshot.storage.k8s.io"} {
			p.query(key, "", "Snapshot class reference", func(candidate unstructured.Unstructured) bool {
				return Value(candidate.Object, "spec", "volumeSnapshotClassName") == name
			})
		}
	case "configmaps", "secrets":
		p.reversePodReferences(r, object)
	case "serviceaccounts":
		for _, field := range []string{"secrets", "imagePullSecrets"} {
			for _, ref := range Maps(o, field) {
				p.named("secrets", ns, Value(ref, "name"), "", "Secret reference")
			}
		}
		p.query("pods", ns, "Service account reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "serviceAccountName") == name
		})
		for _, key := range []string{"rolebindings.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io"} {
			p.query(key, "", "Service account subject", func(candidate unstructured.Unstructured) bool {
				for _, subject := range Maps(candidate.Object, "subjects") {
					if Value(subject, "kind") == "ServiceAccount" && Value(subject, "namespace") == ns && Value(subject, "name") == name {
						return true
					}
				}
				return false
			})
		}
	case "rolebindings.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io":
		p.reference(nestedMap(o, "roleRef"), ns, "rbac.authorization.k8s.io", "", "Role reference")
		for _, subject := range Maps(o, "subjects") {
			if Value(subject, "kind") == "ServiceAccount" {
				p.reference(subject, ns, "", "ServiceAccount", "Service account subject")
			}
		}
	case "roles.rbac.authorization.k8s.io", "clusterroles.rbac.authorization.k8s.io":
		scope := ns
		if r.Name == "clusterroles" {
			scope = ""
		}
		for _, key := range []string{"rolebindings.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io"} {
			if r.Name == "roles" && strings.HasPrefix(key, "cluster") {
				continue
			}
			p.query(key, scope, "Role reference", func(candidate unstructured.Unstructured) bool {
				return matchesReference(nestedMap(candidate.Object, "roleRef"), r, object, candidate.GetNamespace())
			})
		}
	case "mutatingwebhookconfigurations.admissionregistration.k8s.io", "validatingwebhookconfigurations.admissionregistration.k8s.io":
		for _, webhook := range Maps(o, "webhooks") {
			service := nestedMap(webhook, "clientConfig", "service")
			p.named("services", Value(service, "namespace"), Value(service, "name"), "", "Webhook service")
		}
	case "validatingadmissionpolicies.admissionregistration.k8s.io", "mutatingadmissionpolicies.admissionregistration.k8s.io":
		key := "validatingadmissionpolicybindings.admissionregistration.k8s.io"
		if r.Name == "mutatingadmissionpolicies" {
			key = "mutatingadmissionpolicybindings.admissionregistration.k8s.io"
		}
		p.query(key, "", "Admission policy reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "policyName") == name
		})
	case "validatingadmissionpolicybindings.admissionregistration.k8s.io", "mutatingadmissionpolicybindings.admissionregistration.k8s.io":
		key := "validatingadmissionpolicies.admissionregistration.k8s.io"
		if r.Name == "mutatingadmissionpolicybindings" {
			key = "mutatingadmissionpolicies.admissionregistration.k8s.io"
		}
		p.named(key, "", Value(o, "spec", "policyName"), "", "Admission policy")
	case "priorityclasses.scheduling.k8s.io", "runtimeclasses.node.k8s.io":
		field := "priorityClassName"
		if r.Name == "runtimeclasses" {
			field = "runtimeClassName"
		}
		p.query("pods", "", "Pod class reference", func(candidate unstructured.Unstructured) bool { return Value(candidate.Object, "spec", field) == name })
	case "nodes":
		p.namedIfServed("csinodes.storage.k8s.io", "", name, "CSI node name association")
		p.query("volumeattachments.storage.k8s.io", "", "Attachment node reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "nodeName") == name
		})
		p.query("leases.coordination.k8s.io", "kube-node-lease", "Owned node lease", func(candidate unstructured.Unstructured) bool { return hasOwner(candidate, object) })
	case "leases.coordination.k8s.io":
		p.query("leasecandidates.coordination.k8s.io", ns, "Lease name reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "leaseName") == name
		})
	case "leasecandidates.coordination.k8s.io":
		p.named("leases.coordination.k8s.io", ns, Value(o, "spec", "leaseName"), "", "Election lease")
	case "events", "events.events.k8s.io":
		for _, field := range []string{"involvedObject", "regarding", "related"} {
			p.reference(nestedMap(o, field), ns, "", "", "Event "+field)
		}
	case "flowschemas.flowcontrol.apiserver.k8s.io":
		p.named("prioritylevelconfigurations.flowcontrol.apiserver.k8s.io", "", Value(o, "spec", "priorityLevelConfiguration", "name"), "", "Priority level")
		for _, rule := range Maps(o, "spec", "rules") {
			for _, subject := range Maps(rule, "subjects") {
				if Value(subject, "kind") == "ServiceAccount" {
					sa := nestedMap(subject, "serviceAccount")
					if Value(sa, "name") != "*" {
						p.named("serviceaccounts", Value(sa, "namespace"), Value(sa, "name"), "", "Flow schema subject")
					}
				}
			}
		}
	case "prioritylevelconfigurations.flowcontrol.apiserver.k8s.io":
		p.query("flowschemas.flowcontrol.apiserver.k8s.io", "", "Priority level reference", func(candidate unstructured.Unstructured) bool {
			return Value(candidate.Object, "spec", "priorityLevelConfiguration", "name") == name
		})
	case "customresourcedefinitions.apiextensions.k8s.io":
		service := nestedMap(o, "spec", "conversion", "webhook", "clientConfig", "service")
		p.named("services", Value(service, "namespace"), Value(service, "name"), "", "Conversion webhook")
	case "apiservices.apiregistration.k8s.io":
		service := nestedMap(o, "spec", "service")
		p.named("services", Value(service, "namespace"), Value(service, "name"), "", "API backend service")
	case "resourceclaims.resource.k8s.io", "resourceclaimtemplates.resource.k8s.io":
		spec := nestedMap(o, "spec")
		if r.Name == "resourceclaimtemplates" {
			spec = nestedMap(o, "spec", "spec")
		}
		for _, class := range strings.Split(claimClassNames(spec), ", ") {
			p.named("deviceclasses.resource.k8s.io", "", class, "", "Requested device class")
		}
		field := "resourceClaimName"
		if r.Name == "resourceclaimtemplates" {
			field = "resourceClaimTemplateName"
		}
		p.query("pods", ns, "Resource claim reference", func(candidate unstructured.Unstructured) bool {
			for _, ref := range Maps(candidate.Object, "spec", "resourceClaims") {
				if Value(ref, field) == name {
					return true
				}
			}
			return false
		})
		for _, ref := range Maps(o, "status", "reservedFor") {
			key := Value(ref, "resource")
			if group := Value(ref, "apiGroup"); group != "" {
				key += "." + group
			}
			p.named(key, ns, Value(ref, "name"), Value(ref, "uid"), "Reserved consumer")
		}
	case "resourceslices.resource.k8s.io":
		p.named("nodes", "", Value(o, "spec", "nodeName"), "", "Explicit node scope")
		p.query("resourceslices.resource.k8s.io", "", "Same driver and pool", func(candidate unstructured.Unstructured) bool {
			return candidate.GetName() != name && Value(candidate.Object, "spec", "driver") == Value(o, "spec", "driver") && Value(candidate.Object, "spec", "pool", "name") == Value(o, "spec", "pool", "name")
		})
	case "namespaces":
		for _, key := range []string{"resourcequotas", "limitranges", "rolebindings.rbac.authorization.k8s.io"} {
			p.query(key, name, "Namespace scope", func(unstructured.Unstructured) bool { return true })
		}
	}
	p.optionalRelationships(r, object)
	p.admissionParameters(r, object)
	return p.warnings
}

var gatewayRouteKeys = []string{"httproutes.gateway.networking.k8s.io", "grpcroutes.gateway.networking.k8s.io", "tcproutes.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "udproutes.gateway.networking.k8s.io"}

func (p *relationCollector) namedIfServed(key, ns, name, relation string) {
	if _, ok := findResourceIdentity(p.resources, key); ok {
		p.named(key, ns, name, "", relation)
	}
}
func hasOwner(candidate, owner unstructured.Unstructured) bool {
	if owner.GetUID() == "" {
		return false
	}
	for _, ref := range candidate.GetOwnerReferences() {
		if ref.UID == owner.GetUID() {
			return true
		}
	}
	return false
}
func matchesReference(ref map[string]any, r Resource, object unstructured.Unstructured, ns string) bool {
	if Value(ref, "name") != object.GetName() || Value(ref, "kind") != r.Kind {
		return false
	}
	group := firstValue(ref, "apiGroup", "group")
	if version := Value(ref, "apiVersion"); version != "" {
		gv, err := schema.ParseGroupVersion(version)
		if err != nil {
			return false
		}
		group = gv.Group
	}
	if group != r.Group {
		return false
	}
	if r.Namespaced {
		ns = fallback(Value(ref, "namespace"), ns)
		return ns == object.GetNamespace()
	}
	return true
}
func gatewayReferenceMatches(ref map[string]any, ns, group, kind string, r Resource, object unstructured.Unstructured) bool {
	copy := map[string]any{}
	for key, value := range ref {
		copy[key] = value
	}
	if _, ok := copy["group"]; !ok {
		copy["group"] = group
	}
	if _, ok := copy["kind"]; !ok {
		copy["kind"] = kind
	}
	return matchesReference(copy, r, object, ns)
}
func ingressBackendRefs(o map[string]any) []map[string]any {
	var refs []map[string]any
	add := func(backend map[string]any) {
		if service := nestedMap(backend, "service"); service != nil {
			refs = append(refs, map[string]any{"kind": "Service", "name": Value(service, "name")})
		}
		if ref := nestedMap(backend, "resource"); ref != nil {
			refs = append(refs, ref)
		}
	}
	add(nestedMap(o, "spec", "defaultBackend"))
	for _, rule := range Maps(o, "spec", "rules") {
		for _, path := range Maps(rule, "http", "paths") {
			add(nestedMap(path, "backend"))
		}
	}
	return refs
}
func (p *relationCollector) podReferences(spec map[string]any, ns string) {
	p.named("serviceaccounts", ns, Value(spec, "serviceAccountName"), "", "Service account")
	p.named("runtimeclasses.node.k8s.io", "", Value(spec, "runtimeClassName"), "", "Runtime class")
	p.named("priorityclasses.scheduling.k8s.io", "", Value(spec, "priorityClassName"), "", "Priority class")
	for _, ref := range Maps(spec, "imagePullSecrets") {
		p.named("secrets", ns, Value(ref, "name"), "", "Image pull secret")
	}
	for _, claim := range Maps(spec, "resourceClaims") {
		p.named("resourceclaims.resource.k8s.io", ns, Value(claim, "resourceClaimName"), "", "Resource claim")
		p.named("resourceclaimtemplates.resource.k8s.io", ns, Value(claim, "resourceClaimTemplateName"), "", "Resource claim template")
	}
	for _, volume := range Maps(spec, "volumes") {
		p.named("persistentvolumeclaims", ns, Value(volume, "persistentVolumeClaim", "claimName"), "", "Volume claim")
		p.named("configmaps", ns, Value(volume, "configMap", "name"), "", "Volume configuration")
		p.named("secrets", ns, Value(volume, "secret", "secretName"), "", "Volume secret")
		for _, source := range Maps(volume, "projected", "sources") {
			p.named("configmaps", ns, Value(source, "configMap", "name"), "", "Projected configuration")
			p.named("secrets", ns, Value(source, "secret", "name"), "", "Projected secret")
			p.named("clustertrustbundles.certificates.k8s.io", "", Value(source, "clusterTrustBundle", "name"), "", "Projected trust bundle")
		}
	}
	for _, group := range []string{"containers", "initContainers", "ephemeralContainers"} {
		for _, container := range Maps(spec, group) {
			for _, env := range Maps(container, "envFrom") {
				p.named("configmaps", ns, Value(env, "configMapRef", "name"), "", "Environment configuration")
				p.named("secrets", ns, Value(env, "secretRef", "name"), "", "Environment secret")
			}
			for _, env := range Maps(container, "env") {
				p.named("configmaps", ns, Value(env, "valueFrom", "configMapKeyRef", "name"), "", "Environment configuration")
				p.named("secrets", ns, Value(env, "valueFrom", "secretKeyRef", "name"), "", "Environment secret")
			}
		}
	}
}
func (p *relationCollector) reversePodReferences(target Resource, object unstructured.Unstructured) {
	for _, key := range []string{"pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers", "jobs.batch", "cronjobs.batch"} {
		p.query(key, object.GetNamespace(), "Explicit Pod/template reference", func(candidate unstructured.Unstructured) bool {
			spec := nestedMap(candidate.Object, "spec", "template", "spec")
			if key == "pods" {
				spec = nestedMap(candidate.Object, "spec")
			}
			if key == "cronjobs.batch" {
				spec = nestedMap(candidate.Object, "spec", "jobTemplate", "spec", "template", "spec")
			}
			found := false
			probe := relationCollector{resources: []Resource{target}, add: func(r Resource, ns, name, uid, relation string) {
				if r.Key() == target.Key() && ns == object.GetNamespace() && name == object.GetName() {
					found = true
				}
			}}
			probe.podReferences(spec, object.GetNamespace())
			return found
		})
	}
	if target.Key() == "secrets" {
		p.query("ingresses.networking.k8s.io", object.GetNamespace(), "TLS secret reference", func(candidate unstructured.Unstructured) bool {
			for _, entry := range Maps(candidate.Object, "spec", "tls") {
				if Value(entry, "secretName") == object.GetName() {
					return true
				}
			}
			return false
		})
		p.query("serviceaccounts", object.GetNamespace(), "Service account secret reference", func(candidate unstructured.Unstructured) bool {
			for _, field := range []string{"secrets", "imagePullSecrets"} {
				for _, ref := range Maps(candidate.Object, field) {
					if Value(ref, "name") == object.GetName() {
						return true
					}
				}
			}
			return false
		})
	}
}

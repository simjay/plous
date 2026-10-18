package monitor

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DetailField struct {
	Label string
	Value string
}

type DetailSection struct {
	Title   string
	Fields  []DetailField
	Columns []string
	Rows    [][]string
	Bars    []DetailBar
}

// Summary uses API identity so an extension named Pod cannot inherit core Pod semantics.
func Summary(r kubecommon.Resource, object unstructured.Unstructured) []DetailSection {
	o := object.Object
	var sections []DetailSection
	switch r.Key() {
	case "pods":
		sections = podSummary(o)
	case "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers":
		sections = workloadSummary(r, o)
	case "jobs.batch", "cronjobs.batch":
		sections = jobSummary(r, o)
	case "services":
		sections = serviceSummary(o)
	case "ingresses.networking.k8s.io":
		sections = ingressSummary(o)
	case "endpointslices.discovery.k8s.io", "endpoints":
		sections = endpointSummary(r, o)
	case "networkpolicies.networking.k8s.io":
		sections = networkPolicySummary(o)
	case "ingressclasses.networking.k8s.io":
		sections = []DetailSection{detailFields("Ingress class", o, "Controller", "spec.controller", "Parameters kind", "spec.parameters.kind", "Parameters name", "spec.parameters.name", "Parameters namespace", "spec.parameters.namespace")}
	case "nodes":
		sections = nodeSummary(o)
	case "namespaces":
		sections = []DetailSection{detailFields("Namespace", o, "Phase", "status.phase")}
	case "persistentvolumeclaims", "persistentvolumes":
		sections = volumeSummary(r, o)
	case "storageclasses.storage.k8s.io":
		sections = []DetailSection{detailFields("Storage class", o, "Provisioner", "provisioner", "Reclaim policy", "reclaimPolicy", "Binding mode", "volumeBindingMode", "Allow expansion", "allowVolumeExpansion"), mapSection("Parameters", nestedMap(o, "parameters"), "Parameter", "Value")}
	case "configmaps", "secrets":
		sections = configSummary(r, o)
	case "horizontalpodautoscalers.autoscaling":
		sections = hpaSummary(o)
	case "resourcequotas":
		sections = quotaSummary(o)
	case "limitranges":
		sections = limitSummary(o)
	case "roles.rbac.authorization.k8s.io", "clusterroles.rbac.authorization.k8s.io":
		sections = []DetailSection{permissionRules(Maps(o, "rules"))}
		for i, selector := range Maps(o, "aggregationRule", "clusterRoleSelectors") {
			sections[0].Fields = append(sections[0].Fields, DetailField{fmt.Sprintf("Aggregate selector %d", i+1), labelSelector(selector)})
		}
	case "rolebindings.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io":
		sections = []DetailSection{detailFields("Role binding", o, "Role kind", "roleRef.kind", "Role name", "roleRef.name", "Role API group", "roleRef.apiGroup"), objectTable("Subjects", Maps(o, "subjects"), []string{"Kind", "Name", "Namespace", "API group"}, "kind", "name", "namespace", "apiGroup")}
	case "serviceaccounts":
		sections = serviceAccountSummary(o)
	case "poddisruptionbudgets.policy":
		sections = []DetailSection{detailFields("Disruption budget", o, "Minimum available", "spec.minAvailable", "Maximum unavailable", "spec.maxUnavailable", "Allowed disruptions", "status.disruptionsAllowed", "Current healthy", "status.currentHealthy", "Desired healthy", "status.desiredHealthy", "Expected Pods", "status.expectedPods", "Unhealthy eviction policy", "spec.unhealthyPodEvictionPolicy")}
		selector := nestedMap(o, "spec", "selector")
		selection := "none"
		if selector != nil {
			selection = labelSelector(selector)
		}
		sections[0].Fields = append(sections[0].Fields, DetailField{"Pod selector", selection})
	case "priorityclasses.scheduling.k8s.io":
		sections = []DetailSection{detailFields("Priority class", o, "Value", "value", "Global default", "globalDefault", "Preemption policy", "preemptionPolicy", "Description", "description")}
	case "runtimeclasses.node.k8s.io":
		sections = []DetailSection{detailFields("Runtime class", o, "Handler", "handler"), mapSection("Pod overhead", nestedMap(o, "overhead", "podFixed"), "Resource", "Quantity"), mapSection("Node selector", nestedMap(o, "scheduling", "nodeSelector"), "Label", "Value")}
	case "leases.coordination.k8s.io":
		sections = []DetailSection{detailFields("Lease", o, "Holder", "spec.holderIdentity", "Duration seconds", "spec.leaseDurationSeconds", "Acquired", "spec.acquireTime", "Renewed", "spec.renewTime", "Transitions", "spec.leaseTransitions", "Strategy", "spec.strategy", "Preferred holder", "spec.preferredHolder")}
	case "events", "events.events.k8s.io":
		sections = eventSummary(o)
	case "mutatingwebhookconfigurations.admissionregistration.k8s.io", "validatingwebhookconfigurations.admissionregistration.k8s.io":
		sections = webhookSummary(o)
	case "validatingadmissionpolicies.admissionregistration.k8s.io", "mutatingadmissionpolicies.admissionregistration.k8s.io", "validatingadmissionpolicybindings.admissionregistration.k8s.io", "mutatingadmissionpolicybindings.admissionregistration.k8s.io":
		sections = admissionSummary(o)
	case "customresourcedefinitions.apiextensions.k8s.io":
		sections = []DetailSection{detailFields("Resource definition", o, "API group", "spec.group", "Kind", "spec.names.kind", "Plural", "spec.names.plural", "Scope", "spec.scope"), objectTable("Versions", Maps(o, "spec", "versions"), []string{"Version", "Served", "Storage"}, "name", "served", "storage")}
	default:
		var native bool
		sections, native = extendedSummary(r, o)
		if !native {
			sections = []DetailSection{mapSection("Reported status", scalarMap(nestedMap(o, "status")), "Field", "Value"), {Title: "Resource details", Fields: []DetailField{{"YAML", "Open YAML for the complete resource specification."}}}}
		}
	}
	sections = append(sections, supplementalSummary(r, o)...)
	if len(sections) == 0 {
		sections = []DetailSection{{Title: "Overview"}}
	}
	identity := []DetailField{}
	if namespace := object.GetNamespace(); namespace != "" {
		identity = append(identity, DetailField{"Namespace", namespace})
	}
	identity = append(identity, DetailField{"Age", Age(object.GetCreationTimestamp().Time, time.Now())})
	for _, owner := range object.GetOwnerReferences() {
		identity = append(identity, DetailField{"Owner", owner.Kind + "/" + owner.Name})
	}
	// Identity follows operational fields, keeping failure information above metadata.
	sections[0].Fields = append(sections[0].Fields, identity...)
	if conditions := Maps(o, "status", "conditions"); len(conditions) > 0 {
		sections = append(sections, conditionsSection("Conditions", conditions))
	}
	metadata := detailFields("Metadata", o, "API version", "apiVersion", "Kind", "kind", "UID", "metadata.uid", "Created", "metadata.creationTimestamp", "Deletion requested", "metadata.deletionTimestamp")
	metadata.Fields = append(metadata.Fields, DetailField{"Name", object.GetName()}, DetailField{"Finalizers", fallback(strings.Join(object.GetFinalizers(), ", "), "none")}, DetailField{"Annotations", fmt.Sprintf("%d; inspect in YAML", len(object.GetAnnotations()))})
	sections = append(sections, mapSection("Labels", nestedMap(o, "metadata", "labels"), "Label", "Value"), metadata)
	result := sections[:0]
	for _, section := range sections {
		if len(section.Fields) > 0 || len(section.Rows) > 0 {
			result = append(result, section)
		}
	}
	return result
}

func podSummary(o map[string]any) []DetailSection {
	ready, total, restarts, status := PodState(o)
	overview := DetailSection{Title: "Pod overview", Fields: []DetailField{{"Status", status}, {"Ready", fmt.Sprintf("%d/%d", ready, total)}, {"Restarts", fmt.Sprint(restarts)}}}
	overview.Fields = append(overview.Fields, detailFields("", o, "Reason", "status.reason", "Node", "spec.nodeName", "Pod IP", "status.podIP", "QoS", "status.qosClass").Fields...)
	containers := DetailSection{Title: "Containers", Columns: []string{"Name", "Type", "Ready", "State", "Restarts", "Previous exit"}}
	var details []DetailSection
	for _, group := range []struct{ spec, status, label string }{{"containers", "containerStatuses", "app"}, {"initContainers", "initContainerStatuses", "init"}, {"ephemeralContainers", "ephemeralContainerStatuses", "ephemeral"}} {
		statuses := map[string]map[string]any{}
		for _, s := range Maps(o, "status", group.status) {
			statuses[Value(s, "name")] = s
		}
		for _, container := range Maps(o, "spec", group.spec) {
			name := Value(container, "name")
			s := statuses[name]
			kind := group.label
			if kind == "init" && Value(container, "restartPolicy") == "Always" {
				kind = "sidecar"
			}
			ready := fallback(Value(s, "ready"), "unknown")
			if kind == "init" && Value(s, "state", "terminated", "exitCode") == "0" {
				ready = "completed"
			}
			previous := termination(nestedMap(s, "lastState", "terminated"))
			containers.Rows = append(containers.Rows, []string{name, kind, ready, containerState(s), fallback(Value(s, "restartCount"), "0"), fallback(previous, "none")})
			detail := DetailSection{Title: "Container · " + name, Fields: []DetailField{{"Image", Value(container, "image")}}}
			if reason := firstValue(s, "state.waiting.message", "state.terminated.message"); reason != "" {
				detail.Fields = append(detail.Fields, DetailField{"Message", reason})
			}
			if previous != "" {
				detail.Fields = append(detail.Fields, DetailField{"Previous termination", previous})
				if message := Value(s, "lastState", "terminated", "message"); message != "" {
					detail.Fields = append(detail.Fields, DetailField{"Previous message", message})
				}
			}
			for _, field := range []struct{ key, label string }{{"requests", "Requests"}, {"limits", "Limits"}} {
				if value := pairs(nestedMap(container, "resources", field.key)); value != "" {
					detail.Fields = append(detail.Fields, DetailField{field.label, value})
				}
			}
			for _, probe := range []struct{ key, label string }{{"readinessProbe", "Readiness"}, {"livenessProbe", "Liveness"}, {"startupProbe", "Startup"}} {
				if p := nestedMap(container, probe.key); len(p) > 0 {
					detail.Fields = append(detail.Fields, DetailField{probe.label, probeText(p)})
				}
			}
			if ports := Maps(container, "ports"); len(ports) > 0 {
				var values []string
				for _, port := range ports {
					values = append(values, Value(port, "containerPort")+"/"+fallback(Value(port, "protocol"), "TCP"))
				}
				detail.Fields = append(detail.Fields, DetailField{"Ports", strings.Join(values, ", ")})
			}
			if mounts := Maps(container, "volumeMounts"); len(mounts) > 0 {
				var values []string
				for _, mount := range mounts {
					value := Value(mount, "name") + " → " + Value(mount, "mountPath")
					if Value(mount, "readOnly") == "true" {
						value += " (read-only)"
					}
					values = append(values, value)
				}
				detail.Fields = append(detail.Fields, DetailField{"Mounts", strings.Join(values, "; ")})
			}
			details = append(details, detail)
		}
	}
	sections := append([]DetailSection{overview, containers}, details...)
	if message := Value(o, "status", "message"); message != "" {
		sections = append(sections, DetailSection{Title: "Pod message", Fields: []DetailField{{"Message", message}}})
	}
	volumes := DetailSection{Title: "Volumes", Columns: []string{"Name", "Type", "Source"}}
	for _, volume := range Maps(o, "spec", "volumes") {
		kind, source := volumeSource(volume)
		volumes.Rows = append(volumes.Rows, []string{Value(volume, "name"), kind, source})
	}
	return append(sections, volumes)
}

func containerState(s map[string]any) string {
	if state := nestedMap(s, "state", "waiting"); len(state) > 0 {
		return fallback(Value(state, "reason"), "Waiting")
	}
	if state := nestedMap(s, "state", "terminated"); len(state) > 0 {
		return termination(state)
	}
	if _, ok := nestedMap(s, "state")["running"]; ok {
		return "Running"
	}
	return "Not reported"
}

func termination(state map[string]any) string {
	if len(state) == 0 {
		return ""
	}
	text := fallback(Value(state, "reason"), "Terminated")
	if code := Value(state, "exitCode"); code != "" {
		text += " (exit " + code + ")"
	}
	if signal := Value(state, "signal"); signal != "" && signal != "0" {
		text += ", signal " + signal
	}
	return text
}

func probeText(p map[string]any) string {
	var action string
	switch {
	case len(nestedMap(p, "httpGet")) > 0:
		h := nestedMap(p, "httpGet")
		action = fallback(Value(h, "scheme"), "HTTP") + " " + fallback(Value(h, "host"), "Pod IP") + ":" + Value(h, "port") + fallback(Value(h, "path"), "/")
	case len(nestedMap(p, "tcpSocket")) > 0:
		t := nestedMap(p, "tcpSocket")
		action = "TCP " + fallback(Value(t, "host"), "Pod IP") + ":" + Value(t, "port")
	case len(nestedMap(p, "grpc")) > 0:
		action = "gRPC :" + Value(p, "grpc", "port")
		if service := Value(p, "grpc", "service"); service != "" {
			action += " service " + service
		}
	case len(nestedMap(p, "exec")) > 0:
		command, _, _ := unstructured.NestedStringSlice(p, "exec", "command")
		action = "exec " + strings.Join(command, " ")
	default:
		action = "No handler reported"
	}
	return action + " · delay " + fallback(Value(p, "initialDelaySeconds"), "0") + "s · every " + fallback(Value(p, "periodSeconds"), "10") + "s · timeout " + fallback(Value(p, "timeoutSeconds"), "1") + "s · failures " + fallback(Value(p, "failureThreshold"), "3") + " · successes " + fallback(Value(p, "successThreshold"), "1")
}

func workloadSummary(r kubecommon.Resource, o map[string]any) []DetailSection {
	var overview DetailSection
	if r.Key() == "daemonsets.apps" {
		overview = detailFields("DaemonSet", o, "Desired scheduled", "status.desiredNumberScheduled", "Current scheduled", "status.currentNumberScheduled", "Ready", "status.numberReady", "Available", "status.numberAvailable", "Unavailable", "status.numberUnavailable", "Updated scheduled", "status.updatedNumberScheduled", "Misscheduled", "status.numberMisscheduled", "Update strategy", "spec.updateStrategy.type")
	} else {
		overview = detailFields(r.Kind, o, "Desired replicas", "spec.replicas", "Current replicas", "status.replicas", "Ready replicas", "status.readyReplicas", "Available replicas", "status.availableReplicas", "Updated replicas", "status.updatedReplicas", "Unavailable replicas", "status.unavailableReplicas", "Strategy", "spec.strategy.type", "Update strategy", "spec.updateStrategy.type", "Service", "spec.serviceName", "Pod management", "spec.podManagementPolicy", "Current revision", "status.currentRevision", "Update revision", "status.updateRevision", "Paused", "spec.paused")
	}
	// Report controller explanations before template configuration. Preserve the
	// condition's own meaning instead of treating every False as a failure.
	var reported []DetailField
	for _, condition := range Maps(o, "status", "conditions") {
		parts := []string{}
		for _, key := range []string{"status", "reason", "message"} {
			if value := Value(condition, key); value != "" {
				parts = append(parts, value)
			}
		}
		reported = append(reported, DetailField{Value(condition, "type"), strings.Join(parts, " · ")})
	}
	overview.Fields = append(reported, overview.Fields...)
	selector := nestedMap(o, "spec", "selector")
	if r.Key() == "replicationcontrollers" {
		overview.Fields = append(overview.Fields, DetailField{"Pod selector", pairs(selector)})
	} else {
		overview.Fields = append(overview.Fields, DetailField{"Pod selector", labelSelector(selector)})
	}
	return []DetailSection{overview, templateContainers(nestedMap(o, "spec", "template", "spec"))}
}

func templateContainers(spec map[string]any) DetailSection {
	section := DetailSection{Title: "Pod template containers", Columns: []string{"Name", "Type", "Image", "Requests", "Limits"}}
	for _, group := range []struct{ key, label string }{{"containers", "app"}, {"initContainers", "init"}} {
		for _, c := range Maps(spec, group.key) {
			section.Rows = append(section.Rows, []string{Value(c, "name"), group.label, Value(c, "image"), pairs(nestedMap(c, "resources", "requests")), pairs(nestedMap(c, "resources", "limits"))})
		}
	}
	return section
}

func jobSummary(r kubecommon.Resource, o map[string]any) []DetailSection {
	if r.Key() == "cronjobs.batch" {
		section := detailFields("CronJob", o, "Schedule", "spec.schedule", "Time zone", "spec.timeZone", "Suspended", "spec.suspend", "Concurrency policy", "spec.concurrencyPolicy", "Starting deadline seconds", "spec.startingDeadlineSeconds", "Last scheduled", "status.lastScheduleTime", "Last successful", "status.lastSuccessfulTime")
		section.Fields = append(section.Fields, DetailField{"Active jobs", fmt.Sprint(len(Maps(o, "status", "active")))})
		return []DetailSection{section, objectTable("Active jobs", Maps(o, "status", "active"), []string{"Kind", "Name", "Namespace"}, "kind", "name", "namespace"), templateContainers(nestedMap(o, "spec", "jobTemplate", "spec", "template", "spec"))}
	}
	return []DetailSection{detailFields("Job", o, "Completions", "spec.completions", "Parallelism", "spec.parallelism", "Active", "status.active", "Succeeded", "status.succeeded", "Failed", "status.failed", "Backoff limit", "spec.backoffLimit", "Completion mode", "spec.completionMode", "Suspended", "spec.suspend", "Started", "status.startTime", "Completed", "status.completionTime", "Deadline seconds", "spec.activeDeadlineSeconds"), templateContainers(nestedMap(o, "spec", "template", "spec"))}
}

func serviceSummary(o map[string]any) []DetailSection {
	section := detailFields("Service", o, "Type", "spec.type", "Cluster IP", "spec.clusterIP", "External name", "spec.externalName", "Session affinity", "spec.sessionAffinity", "External traffic policy", "spec.externalTrafficPolicy", "Internal traffic policy", "spec.internalTrafficPolicy", "Load balancer class", "spec.loadBalancerClass")
	section.Fields = append(section.Fields, DetailField{"Selector", fallback(pairs(nestedMap(o, "spec", "selector")), "none")})
	if ips := stringList(o, "spec", "externalIPs"); ips != "" {
		section.Fields = append(section.Fields, DetailField{"External IPs", ips})
	}
	return []DetailSection{section, objectTable("Ports", Maps(o, "spec", "ports"), []string{"Name", "Port", "Protocol", "Target port", "Node port"}, "name", "port", "protocol", "targetPort", "nodePort"), objectTable("Load balancer addresses", Maps(o, "status", "loadBalancer", "ingress"), []string{"IP", "Hostname"}, "ip", "hostname")}
}

func ingressSummary(o map[string]any) []DetailSection {
	section := detailFields("Ingress", o, "Ingress class", "spec.ingressClassName")
	if backend := nestedMap(o, "spec", "defaultBackend"); len(backend) > 0 {
		section.Fields = append(section.Fields, DetailField{"Default backend", ingressBackend(backend)})
	}
	routes := DetailSection{Title: "Routes", Columns: []string{"Host", "Path", "Path type", "Backend"}}
	for _, rule := range Maps(o, "spec", "rules") {
		for _, path := range Maps(rule, "http", "paths") {
			routes.Rows = append(routes.Rows, []string{fallback(Value(rule, "host"), "*"), Value(path, "path"), Value(path, "pathType"), ingressBackend(nestedMap(path, "backend"))})
		}
	}
	tls := DetailSection{Title: "TLS", Columns: []string{"Hosts", "Secret"}}
	for _, entry := range Maps(o, "spec", "tls") {
		tls.Rows = append(tls.Rows, []string{stringList(entry, "hosts"), Value(entry, "secretName")})
	}
	return []DetailSection{section, routes, tls, objectTable("Load balancer addresses", Maps(o, "status", "loadBalancer", "ingress"), []string{"IP", "Hostname"}, "ip", "hostname")}
}

func ingressBackend(o map[string]any) string {
	if service := nestedMap(o, "service"); len(service) > 0 {
		return Value(service, "name") + ":" + firstValue(service, "port.name", "port.number")
	}
	if resource := nestedMap(o, "resource"); len(resource) > 0 {
		return Value(resource, "kind") + "/" + Value(resource, "name")
	}
	return "Not reported"
}

func endpointSummary(r kubecommon.Resource, o map[string]any) []DetailSection {
	ports := DetailSection{Title: "Ports", Columns: []string{"Name", "Port", "Protocol"}}
	endpoints := DetailSection{Title: "Endpoints", Columns: []string{"Addresses", "Ready", "Serving", "Terminating", "Node", "Target"}}
	if r.Key() == "endpointslices.discovery.k8s.io" {
		for _, endpoint := range Maps(o, "endpoints") {
			endpoints.Rows = append(endpoints.Rows, []string{stringList(endpoint, "addresses"), fallback(Value(endpoint, "conditions", "ready"), "unknown"), fallback(Value(endpoint, "conditions", "serving"), "unknown"), fallback(Value(endpoint, "conditions", "terminating"), "unknown"), Value(endpoint, "nodeName"), objectReference(nestedMap(endpoint, "targetRef"))})
		}
		ports = objectTable("Ports", Maps(o, "ports"), ports.Columns, "name", "port", "protocol")
		overview := detailFields("EndpointSlice", o, "Address type", "addressType")
		if service := Value(o, "metadata", "labels", "kubernetes.io/service-name"); service != "" {
			overview.Fields = append(overview.Fields, DetailField{"Service", service})
		}
		return []DetailSection{overview, endpoints, ports}
	}
	for _, subset := range Maps(o, "subsets") {
		for _, group := range []struct{ key, ready string }{{"addresses", "true"}, {"notReadyAddresses", "false"}} {
			for _, endpoint := range Maps(subset, group.key) {
				endpoints.Rows = append(endpoints.Rows, []string{Value(endpoint, "ip"), group.ready, "", "", Value(endpoint, "nodeName"), objectReference(nestedMap(endpoint, "targetRef"))})
			}
		}
		ports.Rows = append(ports.Rows, objectTable("", Maps(subset, "ports"), ports.Columns, "name", "port", "protocol").Rows...)
	}
	return []DetailSection{{Title: "Endpoints", Fields: []DetailField{{"Addresses", fmt.Sprint(len(endpoints.Rows))}}}, endpoints, ports}
}

func networkPolicySummary(o map[string]any) []DetailSection {
	section := DetailSection{Title: "Network policy", Fields: []DetailField{{"Pod selector", labelSelector(nestedMap(o, "spec", "podSelector"))}, {"Policy types", stringList(o, "spec", "policyTypes")}}}
	sections := []DetailSection{section}
	for _, direction := range []struct{ key, peer, title string }{{"ingress", "from", "Ingress rules"}, {"egress", "to", "Egress rules"}} {
		rules := DetailSection{Title: direction.title, Columns: []string{"Rule", "Peers", "Ports"}}
		for i, rule := range Maps(o, "spec", direction.key) {
			var peers, ports []string
			for _, peer := range Maps(rule, direction.peer) {
				var terms []string
				for _, key := range []string{"podSelector", "namespaceSelector"} {
					if v, exists := peer[key]; exists {
						m, _ := v.(map[string]any)
						terms = append(terms, key+" "+labelSelector(m))
					}
				}
				if block := nestedMap(peer, "ipBlock"); len(block) > 0 {
					term := Value(block, "cidr")
					if except := stringList(block, "except"); except != "" {
						term += " except " + except
					}
					terms = append(terms, term)
				}
				peers = append(peers, fallback(strings.Join(terms, " AND "), "all"))
			}
			for _, port := range Maps(rule, "ports") {
				value := fallback(Value(port, "port"), "all")
				if end := Value(port, "endPort"); end != "" {
					value += "-" + end
				}
				ports = append(ports, value+"/"+fallback(Value(port, "protocol"), "TCP"))
			}
			rules.Rows = append(rules.Rows, []string{fmt.Sprint(i + 1), fallback(strings.Join(peers, " OR "), "all"), fallback(strings.Join(ports, ", "), "all")})
		}
		if len(rules.Rows) == 0 {
			rules.Fields = []DetailField{{"Rules", "none"}}
		}
		sections = append(sections, rules)
	}
	return sections
}

func nodeSummary(o map[string]any) []DetailSection {
	section := detailFields("Node", o, "Kubelet", "status.nodeInfo.kubeletVersion", "Container runtime", "status.nodeInfo.containerRuntimeVersion", "OS image", "status.nodeInfo.osImage", "Kernel", "status.nodeInfo.kernelVersion", "Architecture", "status.nodeInfo.architecture", "Scheduling disabled", "spec.unschedulable", "Provider ID", "spec.providerID", "Pod CIDR", "spec.podCIDR")
	for _, condition := range Maps(o, "status", "conditions") {
		if Value(condition, "type") == "Ready" {
			section.Fields = append([]DetailField{{"Ready", Value(condition, "status")}}, section.Fields...)
			break
		}
	}
	resources := DetailSection{Title: "Resources", Columns: []string{"Resource", "Capacity", "Allocatable"}}
	capacity, allocatable := nestedMap(o, "status", "capacity"), nestedMap(o, "status", "allocatable")
	for _, key := range unionKeys(capacity, allocatable) {
		resources.Rows = append(resources.Rows, []string{key, scalar(capacity[key]), scalar(allocatable[key])})
	}
	return []DetailSection{section, resources, objectTable("Addresses", Maps(o, "status", "addresses"), []string{"Type", "Address"}, "type", "address"), objectTable("Taints", Maps(o, "spec", "taints"), []string{"Key", "Value", "Effect"}, "key", "value", "effect")}
}

func volumeSummary(r kubecommon.Resource, o map[string]any) []DetailSection {
	section := detailFields(r.Kind, o, "Phase", "status.phase", "Reason", "status.reason", "Message", "status.message", "Storage class", "spec.storageClassName", "Volume mode", "spec.volumeMode", "Volume", "spec.volumeName", "Requested storage", "spec.resources.requests.storage", "Reclaim policy", "spec.persistentVolumeReclaimPolicy", "CSI driver", "spec.csi.driver", "Volume handle", "spec.csi.volumeHandle")
	section.Fields = append(section.Fields, DetailField{"Capacity", firstValue(o, "status.capacity.storage", "spec.capacity.storage")}, DetailField{"Access modes", stringList(o, "spec", "accessModes")})
	if claim := nestedMap(o, "spec", "claimRef"); len(claim) > 0 {
		section.Fields = append(section.Fields, DetailField{"Claim", objectReference(claim)})
	}
	if source := nestedMap(o, "spec", "dataSource"); len(source) > 0 {
		section.Fields = append(section.Fields, DetailField{"Data source", objectReference(source)})
	}
	return []DetailSection{section}
}

func configSummary(r kubecommon.Resource, o map[string]any) []DetailSection {
	section := detailFields(r.Kind, o, "Type", "type", "Immutable", "immutable")
	data := DetailSection{Title: "Data", Columns: []string{"Key", "Value"}}
	for _, field := range []string{"data", "binaryData", "stringData"} {
		values := nestedMap(o, field)
		for _, key := range sortedKeys(values) {
			value := scalar(values[key])
			if r.Key() == "secrets" {
				value = "<hidden>"
			} else if field == "binaryData" {
				decoded, err := base64.StdEncoding.DecodeString(value)
				if err == nil {
					value = fmt.Sprintf("<binary, %d bytes>", len(decoded))
				} else {
					value = "<binary>"
				}
			}
			data.Rows = append(data.Rows, []string{key, value})
		}
	}
	section.Fields = append(section.Fields, DetailField{"Keys", fmt.Sprint(len(data.Rows))})
	if r.Key() == "secrets" {
		section.Fields = append(section.Fields, DetailField{"Values", "Hidden. Press v in YAML to reveal."})
	}
	return []DetailSection{section, data}
}

func hpaSummary(o map[string]any) []DetailSection {
	section := detailFields("Horizontal Pod Autoscaler", o, "Target kind", "spec.scaleTargetRef.kind", "Target name", "spec.scaleTargetRef.name", "Target API", "spec.scaleTargetRef.apiVersion", "Minimum replicas", "spec.minReplicas", "Maximum replicas", "spec.maxReplicas", "Current replicas", "status.currentReplicas", "Desired replicas", "status.desiredReplicas", "Last scaled", "status.lastScaleTime", "Target CPU %", "spec.targetCPUUtilizationPercentage", "Current CPU %", "status.currentCPUUtilizationPercentage")
	return []DetailSection{section, hpaMetrics("Metric targets", Maps(o, "spec", "metrics"), "target"), hpaMetrics("Current metrics", Maps(o, "status", "currentMetrics"), "current")}
}

func hpaMetrics(title string, metrics []map[string]any, valueKey string) DetailSection {
	section := DetailSection{Title: title, Columns: []string{"Type", "Metric", "Container", "Value"}}
	for _, metric := range metrics {
		kind := Value(metric, "type")
		key := strings.ToLower(kind[:min(1, len(kind))]) + kind[min(1, len(kind)):]
		body := nestedMap(metric, key)
		name := firstValue(body, "name", "metric.name")
		valueObject := nestedMap(body, valueKey)
		value := firstValue(valueObject, "averageUtilization", "averageValue", "value")
		if Value(valueObject, "averageUtilization") != "" {
			value += "%"
		}
		section.Rows = append(section.Rows, []string{kind, name, Value(body, "container"), value})
	}
	return section
}

func quotaSummary(o map[string]any) []DetailSection {
	section := DetailSection{Title: "Resource quota", Columns: []string{"Resource", "Used", "Reported hard", "Configured hard"}}
	for _, budget := range quotaBudgets(o) {
		used := "not reported"
		if budget.used != nil {
			used = Value(o, "status", "used", budget.metric)
		}
		section.Rows = append(section.Rows, []string{budget.metric, used, quantityLabel(budget.total, "not reported"), fallback(Value(o, "spec", "hard", budget.metric), "not specified")})
	}
	if scopes := stringList(o, "spec", "scopes"); scopes != "" {
		section.Fields = append(section.Fields, DetailField{"Scopes", scopes})
	}
	return []DetailSection{section}
}

func limitSummary(o map[string]any) []DetailSection {
	section := DetailSection{Title: "Limit range", Columns: []string{"Type", "Minimum", "Maximum", "Default request", "Default limit", "Max limit/request"}}
	for _, limit := range Maps(o, "spec", "limits") {
		section.Rows = append(section.Rows, []string{Value(limit, "type"), pairs(nestedMap(limit, "min")), pairs(nestedMap(limit, "max")), pairs(nestedMap(limit, "defaultRequest")), pairs(nestedMap(limit, "default")), pairs(nestedMap(limit, "maxLimitRequestRatio"))})
	}
	return []DetailSection{section}
}

func permissionRules(rules []map[string]any) DetailSection {
	section := DetailSection{Title: "Permission rules", Columns: []string{"API groups", "Resources", "Verbs", "Resource names", "Non-resource URLs"}}
	for _, rule := range rules {
		groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
		for i, group := range groups {
			if group == "" {
				groups[i] = "core"
			}
		}
		section.Rows = append(section.Rows, []string{strings.Join(groups, ", "), stringList(rule, "resources"), stringList(rule, "verbs"), stringList(rule, "resourceNames"), stringList(rule, "nonResourceURLs")})
	}
	return section
}

func serviceAccountSummary(o map[string]any) []DetailSection {
	section := detailFields("Service account", o, "Automount token", "automountServiceAccountToken")
	return []DetailSection{section, objectTable("Image pull secrets", Maps(o, "imagePullSecrets"), []string{"Name"}, "name"), objectTable("Secret references", Maps(o, "secrets"), []string{"Name"}, "name")}
}

func eventSummary(o map[string]any) []DetailSection {
	section := detailFields("Event", o, "Type", "type", "Reason", "reason", "Action", "action", "Message", "message", "Note", "note", "Count", "count", "Series count", "series.count", "Event time", "eventTime", "First observed", "firstTimestamp", "Last observed", "lastTimestamp", "Reporting controller", "reportingController", "Reporting instance", "reportingInstance")
	for _, key := range []string{"involvedObject", "regarding", "related"} {
		if reference := nestedMap(o, key); len(reference) > 0 {
			section.Fields = append(section.Fields, DetailField{key, objectReference(reference)})
		}
	}
	return []DetailSection{section}
}

func webhookSummary(o map[string]any) []DetailSection {
	var sections []DetailSection
	for _, webhook := range Maps(o, "webhooks") {
		section := detailFields("Webhook · "+Value(webhook, "name"), webhook, "Failure policy", "failurePolicy", "Match policy", "matchPolicy", "Side effects", "sideEffects", "Timeout seconds", "timeoutSeconds", "Reinvocation policy", "reinvocationPolicy", "URL", "clientConfig.url", "Service", "clientConfig.service.name", "Service namespace", "clientConfig.service.namespace", "Service path", "clientConfig.service.path", "Service port", "clientConfig.service.port")
		section.Fields = append(section.Fields, DetailField{"Namespace selector", optionalSelector(nestedMap(webhook, "namespaceSelector"))}, DetailField{"Object selector", optionalSelector(nestedMap(webhook, "objectSelector"))})
		sections = append(sections, section, admissionRules(Maps(webhook, "rules")))
	}
	return sections
}

func admissionSummary(o map[string]any) []DetailSection {
	section := detailFields("Admission policy", o, "Policy name", "spec.policyName", "Failure policy", "spec.failurePolicy", "Reinvocation policy", "spec.reinvocationPolicy", "Parameter kind", "spec.paramKind.kind", "Parameter API", "spec.paramKind.apiVersion", "Parameter name", "spec.paramRef.name", "Parameter namespace", "spec.paramRef.namespace", "Parameter not found", "spec.paramRef.parameterNotFoundAction", "Match policy", "spec.matchConstraints.matchPolicy")
	if actions := stringList(o, "spec", "validationActions"); actions != "" {
		section.Fields = append(section.Fields, DetailField{"Validation actions", actions})
	}
	constraints := nestedMap(o, "spec", "matchConstraints")
	if constraints == nil {
		constraints = nestedMap(o, "spec", "matchResources")
	}
	section.Fields = append(section.Fields, DetailField{"Namespace selector", optionalSelector(nestedMap(constraints, "namespaceSelector"))}, DetailField{"Object selector", optionalSelector(nestedMap(constraints, "objectSelector"))})
	if matchPolicy := Value(constraints, "matchPolicy"); matchPolicy != "" && Value(o, "spec", "matchConstraints", "matchPolicy") == "" {
		section.Fields = append(section.Fields, DetailField{"Match policy", matchPolicy})
	}
	exclusions := admissionRules(Maps(constraints, "excludeResourceRules"))
	exclusions.Title = "Excluded resource rules"
	sections := []DetailSection{section, admissionRules(Maps(constraints, "resourceRules")), exclusions, objectTable("Validations", Maps(o, "spec", "validations"), []string{"Expression", "Message", "Reason"}, "expression", "message", "reason"), objectTable("Match conditions", Maps(o, "spec", "matchConditions"), []string{"Name", "Expression"}, "name", "expression"), objectTable("Variables", Maps(o, "spec", "variables"), []string{"Name", "Expression"}, "name", "expression")}
	mutations := DetailSection{Title: "Mutations", Columns: []string{"Patch type", "Expression"}}
	for _, mutation := range Maps(o, "spec", "mutations") {
		mutations.Rows = append(mutations.Rows, []string{Value(mutation, "patchType"), firstValue(mutation, "applyConfiguration.expression", "jsonPatch.expression")})
	}
	return append(sections, mutations)
}

func admissionRules(rules []map[string]any) DetailSection {
	section := DetailSection{Title: "Resource rules", Columns: []string{"Operations", "API groups", "Versions", "Resources", "Scope"}}
	for _, rule := range rules {
		section.Rows = append(section.Rows, []string{stringList(rule, "operations"), fallback(stringList(rule, "apiGroups"), "core"), stringList(rule, "apiVersions"), stringList(rule, "resources"), fallback(Value(rule, "scope"), "*")})
	}
	return section
}

func detailFields(title string, o map[string]any, fields ...string) DetailSection {
	section := DetailSection{Title: title}
	for i := 0; i+1 < len(fields); i += 2 {
		value, _, _ := unstructured.NestedFieldNoCopy(o, strings.Split(fields[i+1], ".")...)
		if text := scalar(value); text != "" {
			section.Fields = append(section.Fields, DetailField{fields[i], text})
		}
	}
	return section
}

func objectTable(title string, objects []map[string]any, columns []string, fields ...string) DetailSection {
	section := DetailSection{Title: title, Columns: columns}
	for _, object := range objects {
		row := make([]string, len(fields))
		for i, field := range fields {
			value, _, _ := unstructured.NestedFieldNoCopy(object, strings.Split(field, ".")...)
			row[i] = scalar(value)
		}
		section.Rows = append(section.Rows, row)
	}
	return section
}

func mapSection(title string, values map[string]any, columns ...string) DetailSection {
	section := DetailSection{Title: title, Columns: columns}
	for _, key := range sortedKeys(values) {
		if value := scalar(values[key]); value != "" {
			section.Rows = append(section.Rows, []string{key, value})
		}
	}
	return section
}

func nestedMap(o map[string]any, fields ...string) map[string]any {
	value, _, _ := unstructured.NestedMap(o, fields...)
	return value
}

func scalar(value any) string {
	switch value.(type) {
	case string, bool, int, int32, int64, float32, float64:
		return fmt.Sprint(value)
	}
	return ""
}

func scalarMap(values map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range values {
		if scalar(value) != "" {
			result[key] = value
		}
	}
	return result
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unionKeys(groups ...map[string]any) []string {
	merged := map[string]any{}
	for _, group := range groups {
		for key := range group {
			merged[key] = true
		}
	}
	return sortedKeys(merged)
}

func pairs(values map[string]any) string {
	var result []string
	for _, key := range sortedKeys(values) {
		if value := scalar(values[key]); value != "" {
			result = append(result, key+"="+value)
		}
	}
	return strings.Join(result, ", ")
}

func stringList(o map[string]any, fields ...string) string {
	values, _, _ := unstructured.NestedStringSlice(o, fields...)
	return strings.Join(values, ", ")
}

func firstValue(o map[string]any, paths ...string) string {
	for _, path := range paths {
		value, _, _ := unstructured.NestedFieldNoCopy(o, strings.Split(path, ".")...)
		if text := scalar(value); text != "" {
			return text
		}
	}
	return ""
}

func fallback(value, other string) string {
	if value == "" {
		return other
	}
	return value
}

func objectReference(o map[string]any) string {
	name := Value(o, "name")
	if namespace := Value(o, "namespace"); namespace != "" {
		name = namespace + "/" + name
	}
	if kind := Value(o, "kind"); kind != "" {
		name = kind + "/" + name
	}
	return name
}

func labelSelector(o map[string]any) string {
	if o == nil {
		return "not specified"
	}
	terms := []string{}
	if labels := pairs(nestedMap(o, "matchLabels")); labels != "" {
		terms = append(terms, labels)
	}
	for _, expression := range Maps(o, "matchExpressions") {
		key, operator := Value(expression, "key"), Value(expression, "operator")
		switch operator {
		case "Exists":
			terms = append(terms, key+" exists")
		case "DoesNotExist":
			terms = append(terms, key+" does not exist")
		default:
			terms = append(terms, key+" "+operator+" ("+stringList(expression, "values")+")")
		}
	}
	return fallback(strings.Join(terms, "; "), "all")
}

func volumeSource(volume map[string]any) (string, string) {
	for _, key := range sortedKeys(volume) {
		if key == "name" {
			continue
		}
		source, ok := volume[key].(map[string]any)
		if !ok {
			continue
		}
		switch key {
		case "secret":
			return key, Value(source, "secretName")
		case "configMap":
			return key, Value(source, "name")
		case "persistentVolumeClaim":
			return key, Value(source, "claimName")
		case "hostPath":
			return key, Value(source, "path")
		case "emptyDir":
			return key, "medium " + fallback(Value(source, "medium"), "node storage")
		case "projected":
			var projections []string
			for _, projection := range Maps(source, "sources") {
				for _, kind := range sortedKeys(projection) {
					p, _ := projection[kind].(map[string]any)
					projections = append(projections, strings.TrimSpace(kind+" "+Value(p, "name")))
				}
			}
			return key, strings.Join(projections, ", ")
		case "csi":
			return key, Value(source, "driver")
		case "nfs":
			return key, Value(source, "server") + ":" + Value(source, "path")
		default:
			return key, "See YAML for source configuration"
		}
	}
	return "unknown", ""
}

func optionalSelector(selector map[string]any) string {
	if selector == nil {
		return "all"
	}
	return labelSelector(selector)
}

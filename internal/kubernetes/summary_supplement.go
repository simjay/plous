package kubernetes

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

func supplementalSummary(r Resource, o map[string]any) []DetailSection {
	switch r.Key() {
	case "pods":
		return append(podSpecSections(nestedMap(o, "spec")), detailFields("Pod network", o, "Host IP", "status.hostIP", "Host network", "spec.hostNetwork", "DNS policy", "spec.dnsPolicy"))
	case "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers":
		sections := []DetailSection{detailFields("Rollout configuration", o, "Generation", "metadata.generation", "Observed generation", "status.observedGeneration", "Maximum surge", "spec.strategy.rollingUpdate.maxSurge", "Maximum unavailable", "spec.strategy.rollingUpdate.maxUnavailable", "Daemon maximum surge", "spec.updateStrategy.rollingUpdate.maxSurge", "Daemon maximum unavailable", "spec.updateStrategy.rollingUpdate.maxUnavailable", "Progress deadline seconds", "spec.progressDeadlineSeconds", "Minimum ready seconds", "spec.minReadySeconds", "Partition", "spec.updateStrategy.rollingUpdate.partition", "Start ordinal", "spec.ordinals.start", "PVC retention on delete", "spec.persistentVolumeClaimRetentionPolicy.whenDeleted", "PVC retention on scale", "spec.persistentVolumeClaimRetentionPolicy.whenScaled")}
		for _, claim := range Maps(o, "spec", "volumeClaimTemplates") {
			s := detailFields("Volume claim template · "+Value(claim, "metadata", "name"), claim, "Storage class", "spec.storageClassName", "Requested storage", "spec.resources.requests.storage", "Volume mode", "spec.volumeMode")
			s.Fields = append(s.Fields, DetailField{"Access modes", stringList(claim, "spec", "accessModes")})
			sections = append(sections, s)
		}
		return append(sections, podTemplateSections(nestedMap(o, "spec", "template"))...)
	case "jobs.batch":
		sections := []DetailSection{detailFields("Job execution", o, "Completed indexes", "status.completedIndexes", "Failed indexes", "status.failedIndexes", "Backoff per index", "spec.backoffLimitPerIndex", "Maximum failed indexes", "spec.maxFailedIndexes", "TTL after finish seconds", "spec.ttlSecondsAfterFinished"), structuredFields("Pod failure policy", nestedMap(o, "spec", "podFailurePolicy")), structuredFields("Success policy", nestedMap(o, "spec", "successPolicy"))}
		return append(sections, podTemplateSections(nestedMap(o, "spec", "template"))...)
	case "cronjobs.batch":
		sections := []DetailSection{detailFields("Job history", o, "Successful history limit", "spec.successfulJobsHistoryLimit", "Failed history limit", "spec.failedJobsHistoryLimit"), structuredFields("Job template settings", map[string]any{"parallelism": nestedMap(o, "spec", "jobTemplate", "spec")["parallelism"], "completions": nestedMap(o, "spec", "jobTemplate", "spec")["completions"], "backoffLimit": nestedMap(o, "spec", "jobTemplate", "spec")["backoffLimit"]})}
		return append(sections, podTemplateSections(nestedMap(o, "spec", "jobTemplate", "spec", "template"))...)
	case "horizontalpodautoscalers.autoscaling":
		return []DetailSection{structuredFields("Scaling behavior", nestedMap(o, "spec", "behavior"))}
	case "poddisruptionbudgets.policy":
		return []DetailSection{mapSection("Disrupted Pods", nestedMap(o, "status", "disruptedPods"), "Pod", "Disruption time")}
	case "services":
		return []DetailSection{{Title: "Service addressing", Fields: []DetailField{{"Cluster IPs", stringList(o, "spec", "clusterIPs")}, {"IP families", stringList(o, "spec", "ipFamilies")}}}, detailFields("Traffic configuration", o, "IP family policy", "spec.ipFamilyPolicy", "Allocate load-balancer node ports", "spec.allocateLoadBalancerNodePorts", "Health-check node port", "spec.healthCheckNodePort", "Publish not-ready addresses", "spec.publishNotReadyAddresses", "Traffic distribution", "spec.trafficDistribution", "Session affinity timeout seconds", "spec.sessionAffinityConfig.clientIP.timeoutSeconds")}
	case "ingressclasses.networking.k8s.io":
		return []DetailSection{{Title: "Ingress class selection", Fields: []DetailField{{"Default", fallback(Value(o, "metadata", "annotations", "ingressclass.kubernetes.io/is-default-class"), "not set")}}}, detailFields("Parameters reference", o, "Group", "spec.parameters.apiGroup", "Scope", "spec.parameters.scope")}
	case "namespaces":
		return []DetailSection{{Title: "Namespace finalization", Fields: []DetailField{{"Spec finalizers", fallback(stringList(o, "spec", "finalizers"), "none")}}}}
	case "runtimeclasses.node.k8s.io":
		return []DetailSection{tolerationsSection(Maps(o, "scheduling", "tolerations"))}
	case "persistentvolumeclaims", "persistentvolumes":
		sections := []DetailSection{detailFields("Volume attributes", o, "Requested class", "spec.volumeAttributesClassName", "Current class", "status.currentVolumeAttributesClassName", "Modification state", "status.modifyVolumeStatus.status", "Modification target", "status.modifyVolumeStatus.targetVolumeAttributesClassName", "Source group", "spec.dataSourceRef.apiGroup", "Source kind", "spec.dataSourceRef.kind", "Source namespace", "spec.dataSourceRef.namespace", "Source name", "spec.dataSourceRef.name"), structuredFields("Node affinity", nestedMap(o, "spec", "nodeAffinity"))}
		if options := stringList(o, "spec", "mountOptions"); options != "" {
			sections = append(sections, DetailSection{Title: "Mount options", Fields: []DetailField{{"Options", options}}})
		}
		return sections
	case "storageclasses.storage.k8s.io":
		return []DetailSection{{Title: "Storage class defaults", Fields: []DetailField{{"Default", fallback(Value(o, "metadata", "annotations", "storageclass.kubernetes.io/is-default-class"), "not set")}, {"Mount options", fallback(stringList(o, "mountOptions"), "none")}}}, structuredFields("Allowed topologies", map[string]any{"terms": mapsAsAny(Maps(o, "allowedTopologies"))})}
	case "configmaps", "secrets":
		section := DetailSection{Title: "Key sizes", Columns: []string{"Key", "Type", "Bytes"}}
		for _, field := range []string{"data", "binaryData", "stringData"} {
			for _, key := range sortedKeys(nestedMap(o, field)) {
				value := Value(o, field, key)
				if field == "binaryData" || (r.Name == "secrets" && field == "data") {
					value = decodeBase64(value)
				}
				section.Rows = append(section.Rows, []string{key, field, fmt.Sprint(len([]byte(value)))})
			}
		}
		return []DetailSection{section}
	case "serviceaccounts":
		if Value(o, "automountServiceAccountToken") == "" {
			return []DetailSection{{Title: "Token mounting", Fields: []DetailField{{"Automount", "Unset; the Pod setting and API default determine mounting."}}}}
		}
	case "roles.rbac.authorization.k8s.io", "rolebindings.rbac.authorization.k8s.io", "clusterroles.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io":
		scope := "Namespace " + Value(o, "metadata", "namespace")
		if !r.Namespaced {
			scope = "Cluster"
		}
		return []DetailSection{{Title: "Authorization scope", Fields: []DetailField{{"Scope", scope}, {"Auto-update", fallback(Value(o, "metadata", "annotations", "rbac.authorization.kubernetes.io/autoupdate"), "not set")}}}}
	case "mutatingwebhookconfigurations.admissionregistration.k8s.io", "validatingwebhookconfigurations.admissionregistration.k8s.io":
		var sections []DetailSection
		for _, hook := range Maps(o, "webhooks") {
			name := Value(hook, "name")
			sections = append(sections, DetailSection{Title: "Review versions · " + name, Fields: []DetailField{{"Versions", stringList(hook, "admissionReviewVersions")}}}, objectTable("Match conditions · "+name, Maps(hook, "matchConditions"), []string{"Name", "Expression"}, "name", "expression"), certificateSection("CA certificates · "+name, decodeBase64(Value(hook, "clientConfig", "caBundle"))))
		}
		return sections
	case "validatingadmissionpolicies.admissionregistration.k8s.io":
		return []DetailSection{objectTable("Audit annotations", Maps(o, "spec", "auditAnnotations"), []string{"Key", "Expression"}, "key", "valueExpression"), objectTable("Type-check warnings", Maps(o, "status", "typeChecking", "expressionWarnings"), []string{"Field", "Warning"}, "fieldRef", "warning")}
	case "validatingadmissionpolicybindings.admissionregistration.k8s.io", "mutatingadmissionpolicybindings.admissionregistration.k8s.io":
		if selector := nestedMap(o, "spec", "paramRef", "selector"); selector != nil {
			return []DetailSection{{Title: "Parameter selection", Fields: []DetailField{{"Selector", labelSelector(selector)}}}}
		}
	case "resourcequotas":
		section := DetailSection{Title: "Remaining quota", Columns: []string{"Resource", "Remaining"}}
		hard, used := nestedMap(o, "status", "hard"), nestedMap(o, "status", "used")
		for _, key := range sortedKeys(hard) {
			remaining := "Not reported"
			h, err := resource.ParseQuantity(scalar(hard[key]))
			u, err2 := resource.ParseQuantity(scalar(used[key]))
			if err == nil && err2 == nil {
				h.Sub(u)
				remaining = h.String()
			}
			section.Rows = append(section.Rows, []string{key, remaining})
		}
		return []DetailSection{section, structuredFields("Scope selector", nestedMap(o, "spec", "scopeSelector"))}
	case "events", "events.events.k8s.io":
		return []DetailSection{detailFields("Event reporting", o, "Source component", "source.component", "Source host", "source.host", "Series last observed", "series.lastObservedTime", "Deprecated first observed", "deprecatedFirstTimestamp", "Deprecated last observed", "deprecatedLastTimestamp", "Deprecated count", "deprecatedCount")}
	case "flowschemas.flowcontrol.apiserver.k8s.io", "prioritylevelconfigurations.flowcontrol.apiserver.k8s.io":
		return []DetailSection{{Title: "Bootstrap management", Fields: []DetailField{{"Auto-update", fallback(Value(o, "metadata", "annotations", "apf.kubernetes.io/autoupdate-spec"), "not set")}}}}
	case "customresourcedefinitions.apiextensions.k8s.io":
		s := detailFields("Conversion", o, "Strategy", "spec.conversion.strategy", "Webhook URL", "spec.conversion.webhook.clientConfig.url", "Webhook service", "spec.conversion.webhook.clientConfig.service.name", "Service namespace", "spec.conversion.webhook.clientConfig.service.namespace")
		s.Fields = append(s.Fields, DetailField{"Review versions", stringList(o, "spec", "conversion", "webhook", "conversionReviewVersions")})
		sections := []DetailSection{{Title: "Names", Fields: []DetailField{{"Singular", Value(o, "spec", "names", "singular")}, {"Short names", stringList(o, "spec", "names", "shortNames")}, {"Categories", stringList(o, "spec", "names", "categories")}, {"Stored versions", stringList(o, "status", "storedVersions")}}}, s}
		for _, version := range Maps(o, "spec", "versions") {
			name := Value(version, "name")
			subresources := nestedMap(version, "subresources")
			subresourceSection := structuredFields("Subresources · "+name, subresources)
			for _, subresource := range []string{"status", "scale"} {
				if value, exists := subresources[subresource]; exists && value != nil {
					subresourceSection.Fields = append(subresourceSection.Fields, DetailField{subresource, "Enabled"})
				}
			}
			sections = append(sections, objectTable("Printer columns · "+name, Maps(version, "additionalPrinterColumns"), []string{"Name", "Type", "JSON path", "Priority"}, "name", "type", "jsonPath", "priority"), subresourceSection)
			schema := nestedMap(version, "schema", "openAPIV3Schema")
			s := DetailSection{Title: "Schema · " + name, Columns: []string{"Property", "Type", "Description"}}
			properties := nestedMap(schema, "properties")
			for _, key := range sortedKeys(properties) {
				p, _ := properties[key].(map[string]any)
				s.Rows = append(s.Rows, []string{key, Value(p, "type"), Value(p, "description")})
			}
			s.Fields = append(s.Fields, DetailField{"Required", stringList(schema, "required")}, DetailField{"Complete schema", "Inspect YAML for nested schemas and validation rules."})
			sections = append(sections, s)
		}
		return sections
	}
	return nil
}

func tolerationsSection(tolerations []map[string]any) DetailSection {
	return objectTable("Tolerations", tolerations, []string{"Key", "Operator", "Value", "Effect", "Seconds"}, "key", "operator", "value", "effect", "tolerationSeconds")
}
func podSpecSections(spec map[string]any) []DetailSection {
	sections := []DetailSection{detailFields("Scheduling", spec, "Service account", "serviceAccountName", "Scheduler", "schedulerName", "Priority class", "priorityClassName", "Priority", "priority", "Runtime class", "runtimeClassName", "Restart policy", "restartPolicy", "Termination grace seconds", "terminationGracePeriodSeconds"), mapSection("Node selector", nestedMap(spec, "nodeSelector"), "Label", "Value"), tolerationsSection(Maps(spec, "tolerations")), structuredFields("Affinity", nestedMap(spec, "affinity")), structuredFields("Topology spread", map[string]any{"constraints": mapsAsAny(Maps(spec, "topologySpreadConstraints"))})}
	for _, group := range []string{"containers", "initContainers", "ephemeralContainers"} {
		for _, container := range Maps(spec, group) {
			s := DetailSection{Title: "Environment references · " + Value(container, "name"), Columns: []string{"Variable", "Source", "Reference"}}
			for _, env := range Maps(container, "env") {
				name := Value(env, "name")
				from := nestedMap(env, "valueFrom")
				if from == nil {
					s.Rows = append(s.Rows, []string{name, "Literal", "Inspect YAML"})
					continue
				}
				for _, kind := range sortedKeys(from) {
					ref, _ := from[kind].(map[string]any)
					reference := strings.TrimSpace(Value(ref, "name") + " " + Value(ref, "key"))
					if reference == "" {
						reference = firstValue(ref, "fieldPath", "resource")
					}
					s.Rows = append(s.Rows, []string{name, kind, reference})
				}
			}
			for _, from := range Maps(container, "envFrom") {
				for _, kind := range []string{"configMapRef", "secretRef"} {
					if ref := nestedMap(from, kind); ref != nil {
						s.Rows = append(s.Rows, []string{fallback(Value(from, "prefix"), "all keys"), kind, Value(ref, "name")})
					}
				}
			}
			sections = append(sections, s)
		}
	}
	sections = append(sections, objectTable("Image pull secrets", Maps(spec, "imagePullSecrets"), []string{"Name"}, "name"), objectTable("Resource claims", Maps(spec, "resourceClaims"), []string{"Name", "Claim", "Template"}, "name", "resourceClaimName", "resourceClaimTemplateName"))
	return sections
}

func podTemplateSections(template map[string]any) []DetailSection {
	spec := nestedMap(template, "spec")
	sections := podSpecSections(spec)
	sections = append(sections, mapSection("Pod template labels", nestedMap(template, "metadata", "labels"), "Label", "Value"))
	volumes := DetailSection{Title: "Pod template volumes", Columns: []string{"Name", "Type", "Source"}}
	for _, volume := range Maps(spec, "volumes") {
		kind, source := volumeSource(volume)
		volumes.Rows = append(volumes.Rows, []string{Value(volume, "name"), kind, source})
	}
	sections = append(sections, volumes)
	for _, group := range []string{"containers", "initContainers"} {
		for _, container := range Maps(spec, group) {
			name := Value(container, "name")
			section := DetailSection{Title: "Template probes · " + name}
			for _, probe := range []struct{ key, label string }{{"readinessProbe", "Readiness"}, {"livenessProbe", "Liveness"}, {"startupProbe", "Startup"}} {
				if values := nestedMap(container, probe.key); len(values) > 0 {
					section.Fields = append(section.Fields, DetailField{probe.label, probeText(values)})
				}
			}
			sections = append(sections, section, objectTable("Template mounts · "+name, Maps(container, "volumeMounts"), []string{"Volume", "Path", "Read-only", "Subpath"}, "name", "mountPath", "readOnly", "subPath"), objectTable("Template ports · "+name, Maps(container, "ports"), []string{"Name", "Container port", "Protocol", "Host port"}, "name", "containerPort", "protocol", "hostPort"))
		}
	}
	return sections
}

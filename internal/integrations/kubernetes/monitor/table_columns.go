package monitor

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type tableColumn struct {
	title string
	value func(map[string]any) string
}

func col(title, path string) tableColumn {
	return tableColumn{title, func(o map[string]any) string { return firstValue(o, path) }}
}
func computed(title string, value func(map[string]any) string) tableColumn {
	return tableColumn{title, value}
}
func listCol(title, path string) tableColumn {
	return computed(title, func(o map[string]any) string { return fallback(stringList(o, strings.Split(path, ".")...), "none") })
}
func countCol(title, path string) tableColumn {
	return computed(title, func(o map[string]any) string {
		v, _, _ := unstructured.NestedFieldNoCopy(o, strings.Split(path, ".")...)
		switch v := v.(type) {
		case []any:
			return fmt.Sprint(len(v))
		case map[string]any:
			return fmt.Sprint(len(v))
		}
		return "0"
	})
}
func ratioCol(title, numerator, denominator string) tableColumn {
	return computed(title, func(o map[string]any) string {
		return fallback(firstValue(o, numerator), "?") + "/" + fallback(firstValue(o, denominator), "?")
	})
}
func selectorCol(title, path string) tableColumn {
	return computed(title, func(o map[string]any) string { return labelSelector(nestedMap(o, strings.Split(path, ".")...)) })
}
func conditionCol(title, kind string) tableColumn {
	return computed(title, func(o map[string]any) string { return conditionValue(o, kind) })
}
func annotationCol(title, key string) tableColumn {
	return computed(title, func(o map[string]any) string { return fallback(Value(o, "metadata", "annotations", key), "not set") })
}
func referenceCol(title, path string) tableColumn {
	return computed(title, func(o map[string]any) string {
		reference := nestedMap(o, strings.Split(path, ".")...)
		if Value(reference, "name") == "" {
			return strings.TrimSpace(Value(reference, "kind") + " " + Value(reference, "apiVersion"))
		}
		return objectReference(reference)
	})
}

var nativeColumns = map[string][]tableColumn{
	"namespaces": {col("Phase", "status.phase")},
	"nodes": {conditionCol("Ready", "Ready"), computed("Scheduling", func(o map[string]any) string {
		if Value(o, "spec", "unschedulable") == "true" {
			return "Disabled"
		}
		return "Enabled"
	}), computed("Roles", nodeRoles), col("Version", "status.nodeInfo.kubeletVersion"), computed("Internal IP", func(o map[string]any) string {
		for _, a := range Maps(o, "status", "addresses") {
			if Value(a, "type") == "InternalIP" {
				return Value(a, "address")
			}
		}
		return ""
	})},
	"leases.coordination.k8s.io": {col("Holder", "spec.holderIdentity"), col("Renewed", "spec.renewTime"), col("Duration", "spec.leaseDurationSeconds"), col("Transitions", "spec.leaseTransitions")},
	"runtimeclasses.node.k8s.io": {col("Handler", "handler")},
	"pods": {computed("Ready", func(o map[string]any) string {
		ready, total, _, _ := PodState(o)
		return fmt.Sprintf("%d/%d", ready, total)
	}), computed("Status", func(o map[string]any) string { _, _, _, status := PodState(o); return status }), computed("Restarts", func(o map[string]any) string { _, _, restarts, _ := PodState(o); return fmt.Sprint(restarts) }), col("Node", "spec.nodeName"), col("Pod IP", "status.podIP")},
	"deployments.apps":                     {workloadReadyColumn("deployments.apps"), col("Updated", "status.updatedReplicas"), col("Available", "status.availableReplicas"), conditionCol("Progressing", "Progressing")},
	"statefulsets.apps":                    {workloadReadyColumn("statefulsets.apps"), col("Current", "status.currentReplicas"), col("Updated", "status.updatedReplicas")},
	"daemonsets.apps":                      {workloadReadyColumn("daemonsets.apps"), col("Current", "status.currentNumberScheduled"), col("Updated", "status.updatedNumberScheduled"), col("Available", "status.numberAvailable"), col("Misscheduled", "status.numberMisscheduled")},
	"replicasets.apps":                     {workloadReadyColumn("replicasets.apps"), col("Current", "status.replicas"), col("Available", "status.availableReplicas")},
	"replicationcontrollers":               {workloadReadyColumn("replicationcontrollers"), col("Current", "status.replicas"), col("Available", "status.availableReplicas")},
	"controllerrevisions.apps":             {col("Revision", "revision"), computed("Owner", ownerNames)},
	"jobs.batch":                           {ratioCol("Completions", "status.succeeded", "spec.completions"), col("Active", "status.active"), col("Failed", "status.failed"), computed("Conditions", conditionSummary), computed("Duration", jobDuration)},
	"cronjobs.batch":                       {col("Schedule", "spec.schedule"), col("Time zone", "spec.timeZone"), col("Suspend", "spec.suspend"), countCol("Active", "status.active"), col("Last schedule", "status.lastScheduleTime")},
	"horizontalpodautoscalers.autoscaling": {referenceCol("Target", "spec.scaleTargetRef"), col("Min", "spec.minReplicas"), col("Max", "spec.maxReplicas"), ratioCol("Current/Desired", "status.currentReplicas", "status.desiredReplicas"), computed("Metrics", hpaMetricCells)},
	"verticalpodautoscalers.autoscaling.k8s.io": {referenceCol("Target", "spec.targetRef"), col("Update mode", "spec.updatePolicy.updateMode"), computed("Recommendations", func(o map[string]any) string {
		if len(Maps(o, "status", "recommendation", "containerRecommendations")) == 0 {
			return "Not reported"
		}
		return fmt.Sprint(len(Maps(o, "status", "recommendation", "containerRecommendations")))
	})},
	"poddisruptionbudgets.policy": {col("Min available", "spec.minAvailable"), col("Max unavailable", "spec.maxUnavailable"), col("Allowed", "status.disruptionsAllowed"), ratioCol("Healthy", "status.currentHealthy", "status.desiredHealthy")},
	"services":                    {col("Type", "spec.type"), col("Cluster IP", "spec.clusterIP"), computed("Ports", servicePorts), computed("External IP", serviceAddresses), computed("Ready endpoints", func(map[string]any) string { return "Not queried" })},
	"endpoints": {computed("Ready", func(o map[string]any) string {
		n := 0
		for _, s := range Maps(o, "subsets") {
			n += len(Maps(s, "addresses"))
		}
		return fmt.Sprint(n)
	}), computed("Not ready", func(o map[string]any) string {
		n := 0
		for _, s := range Maps(o, "subsets") {
			n += len(Maps(s, "notReadyAddresses"))
		}
		return fmt.Sprint(n)
	}), computed("Ports", func(o map[string]any) string {
		var p []string
		for _, s := range Maps(o, "subsets") {
			p = append(p, portTexts(Maps(s, "ports"))...)
		}
		return strings.Join(p, ", ")
	})},
	"endpointslices.discovery.k8s.io": {col("Address type", "addressType"), computed("Service", func(o map[string]any) string { return Value(o, "metadata", "labels", "kubernetes.io/service-name") }), countCol("Endpoints", "endpoints"), computed("Ready", func(o map[string]any) string {
		n := 0
		unknown := 0
		for _, e := range Maps(o, "endpoints") {
			switch Value(e, "conditions", "ready") {
			case "true":
				n++
			case "":
				unknown++
			}
		}
		if unknown > 0 {
			return fmt.Sprintf("%d (+%d unknown)", n, unknown)
		}
		return fmt.Sprint(n)
	}), computed("Ports", func(o map[string]any) string { return strings.Join(portTexts(Maps(o, "ports")), ", ") })},
	"ingresses.networking.k8s.io":               {col("Class", "spec.ingressClassName"), computed("Hosts", func(o map[string]any) string { return joinedMapValues(Maps(o, "spec", "rules"), "host") }), computed("Addresses", loadBalancerAddresses), countCol("TLS", "spec.tls")},
	"ingressclasses.networking.k8s.io":          {col("Controller", "spec.controller"), annotationCol("Default", "ingressclass.kubernetes.io/is-default-class")},
	"networkpolicies.networking.k8s.io":         {selectorCol("Pod selector", "spec.podSelector"), listCol("Policy types", "spec.policyTypes"), countCol("Ingress rules", "spec.ingress"), countCol("Egress rules", "spec.egress")},
	"ipaddresses.networking.k8s.io":             {col("Parent resource", "spec.parentRef.resource"), col("Parent namespace", "spec.parentRef.namespace"), col("Parent name", "spec.parentRef.name")},
	"servicecidrs.networking.k8s.io":            {listCol("CIDRs", "spec.cidrs"), conditionCol("Ready", "Ready")},
	"gateways.gateway.networking.k8s.io":        {col("Class", "spec.gatewayClassName"), computed("Addresses", func(o map[string]any) string { return joinedMapValues(Maps(o, "status", "addresses"), "value") }), conditionCol("Programmed", "Programmed"), countCol("Listeners", "spec.listeners")},
	"gatewayclasses.gateway.networking.k8s.io":  {col("Controller", "spec.controllerName"), conditionCol("Accepted", "Accepted")},
	"httproutes.gateway.networking.k8s.io":      routeColumns(true),
	"grpcroutes.gateway.networking.k8s.io":      routeColumns(true),
	"tcproutes.gateway.networking.k8s.io":       routeColumns(false),
	"tlsroutes.gateway.networking.k8s.io":       routeColumns(true),
	"udproutes.gateway.networking.k8s.io":       routeColumns(false),
	"referencegrants.gateway.networking.k8s.io": {computed("From", func(o map[string]any) string { return grantReferences(Maps(o, "spec", "from")) }), computed("To", func(o map[string]any) string { return grantReferences(Maps(o, "spec", "to")) })},
	"persistentvolumeclaims":                    {col("Status", "status.phase"), col("Requested", "spec.resources.requests.storage"), col("Capacity", "status.capacity.storage"), listCol("Access", "spec.accessModes"), col("Storage class", "spec.storageClassName"), col("Volume", "spec.volumeName")},
	"persistentvolumes":                         {col("Status", "status.phase"), col("Capacity", "spec.capacity.storage"), listCol("Access", "spec.accessModes"), col("Reclaim", "spec.persistentVolumeReclaimPolicy"), col("Storage class", "spec.storageClassName"), referenceCol("Claim", "spec.claimRef")},
	"storageclasses.storage.k8s.io":             {col("Provisioner", "provisioner"), annotationCol("Default", "storageclass.kubernetes.io/is-default-class"), col("Reclaim", "reclaimPolicy"), col("Binding", "volumeBindingMode"), col("Expansion", "allowVolumeExpansion")},
	"csidrivers.storage.k8s.io":                 {col("Attach required", "spec.attachRequired"), col("Pod info", "spec.podInfoOnMount"), listCol("Lifecycle modes", "spec.volumeLifecycleModes"), col("Capacity tracking", "spec.storageCapacity")},
	"csinodes.storage.k8s.io":                   {computed("Drivers", func(o map[string]any) string { return joinedMapValues(Maps(o, "spec", "drivers"), "name") })},
	"csistoragecapacities.storage.k8s.io":       {col("Storage class", "storageClassName"), col("Capacity", "capacity"), col("Maximum volume", "maximumVolumeSize")},
	"volumeattachments.storage.k8s.io":          {col("Attacher", "spec.attacher"), col("Source volume", "spec.source.persistentVolumeName"), col("Node", "spec.nodeName"), col("Attached", "status.attached")},
	"volumeattributesclasses.storage.k8s.io":    {col("Driver", "driverName"), countCol("Parameters", "parameters")},
	"volumesnapshots.snapshot.storage.k8s.io": {col("Ready to use", "status.readyToUse"), computed("Source", func(o map[string]any) string {
		return firstValue(o, "spec.source.persistentVolumeClaimName", "spec.source.volumeSnapshotContentName")
	}), col("Restore size", "status.restoreSize"), col("Class", "spec.volumeSnapshotClassName"), col("Content", "status.boundVolumeSnapshotContentName")},
	"volumesnapshotcontents.snapshot.storage.k8s.io": {col("Ready to use", "status.readyToUse"), col("Driver", "spec.driver"), referenceCol("Snapshot", "spec.volumeSnapshotRef"), col("Deletion policy", "spec.deletionPolicy"), col("Restore bytes", "status.restoreSize")},
	"volumesnapshotclasses.snapshot.storage.k8s.io":  {col("Driver", "driver"), annotationCol("Default", "snapshot.storage.kubernetes.io/is-default-class"), col("Deletion policy", "deletionPolicy")},
	"configmaps":                                                     {countCol("Text keys", "data"), countCol("Binary keys", "binaryData"), col("Immutable", "immutable")},
	"secrets":                                                        {col("Type", "type"), countCol("Keys", "data"), col("Immutable", "immutable")},
	"serviceaccounts":                                                {computed("Automount", func(o map[string]any) string { return fallback(Value(o, "automountServiceAccountToken"), "inherited") }), countCol("Image-pull refs", "imagePullSecrets")},
	"roles.rbac.authorization.k8s.io":                                {countCol("Rules", "rules")},
	"clusterroles.rbac.authorization.k8s.io":                         {countCol("Rules", "rules"), countCol("Aggregation selectors", "aggregationRule.clusterRoleSelectors")},
	"rolebindings.rbac.authorization.k8s.io":                         {referenceCol("Role", "roleRef"), countCol("Subjects", "subjects")},
	"clusterrolebindings.rbac.authorization.k8s.io":                  {referenceCol("ClusterRole", "roleRef"), countCol("Subjects", "subjects")},
	"podsecuritypolicies.policy":                                     {col("Privileged", "spec.privileged"), col("RunAsUser rule", "spec.runAsUser.rule"), col("SELinux rule", "spec.seLinux.rule")},
	"mutatingwebhookconfigurations.admissionregistration.k8s.io":     {countCol("Webhooks", "webhooks")},
	"validatingwebhookconfigurations.admissionregistration.k8s.io":   {countCol("Webhooks", "webhooks")},
	"mutatingadmissionpolicies.admissionregistration.k8s.io":         {col("Failure policy", "spec.failurePolicy"), col("Reinvocation", "spec.reinvocationPolicy"), referenceCol("Parameter kind", "spec.paramKind")},
	"mutatingadmissionpolicybindings.admissionregistration.k8s.io":   {col("Policy", "spec.policyName"), referenceCol("Parameter", "spec.paramRef")},
	"validatingadmissionpolicies.admissionregistration.k8s.io":       {col("Failure policy", "spec.failurePolicy"), referenceCol("Parameter kind", "spec.paramKind"), computed("Conditions", conditionSummary)},
	"validatingadmissionpolicybindings.admissionregistration.k8s.io": {col("Policy", "spec.policyName"), listCol("Actions", "spec.validationActions"), referenceCol("Parameter", "spec.paramRef")},
	"limitranges":                       {computed("Types", func(o map[string]any) string { return joinedMapValues(Maps(o, "spec", "limits"), "type") }), countCol("Rules", "spec.limits")},
	"resourcequotas":                    {countCol("Resources", "spec.hard"), computed("Exceeded", quotaExceeded)},
	"priorityclasses.scheduling.k8s.io": {col("Value", "value"), col("Global default", "globalDefault"), col("Preemption", "preemptionPolicy")},
	"resourceclaims.resource.k8s.io": {computed("Allocation", func(o map[string]any) string {
		if nestedMap(o, "status", "allocation") == nil {
			return "Not reported"
		}
		return "Reported"
	}), countCol("Requests", "spec.devices.requests"), countCol("Reserved consumers", "status.reservedFor")},
	"resourceclaimtemplates.resource.k8s.io": {countCol("Requests", "spec.spec.devices.requests"), computed("Classes", func(o map[string]any) string { return claimClassNames(nestedMap(o, "spec", "spec")) })},
	"deviceclasses.resource.k8s.io":          {countCol("Selectors", "spec.selectors"), countCol("Configurations", "spec.config")},
	"resourceslices.resource.k8s.io": {col("Driver", "spec.driver"), col("Pool", "spec.pool.name"), computed("Node scope", func(o map[string]any) string {
		if Value(o, "spec", "allNodes") == "true" {
			return "All nodes"
		}
		if node := Value(o, "spec", "nodeName"); node != "" {
			return node
		}
		if nestedMap(o, "spec", "nodeSelector") != nil {
			return "Node selector"
		}
		return "Not reported"
	}), countCol("Devices", "spec.devices")},
	"events": {col("Type", "type"), col("Reason", "reason"), referenceCol("Object", "involvedObject"), col("Count", "count"), computed("Last observed", func(o map[string]any) string {
		return firstValue(o, "series.lastObservedTime", "lastTimestamp", "eventTime")
	}), col("Message", "message")},
	"events.events.k8s.io": {col("Type", "type"), col("Reason", "reason"), referenceCol("Regarding", "regarding"), computed("Count", func(o map[string]any) string { return firstValue(o, "series.count", "deprecatedCount") }), computed("Last observed", func(o map[string]any) string {
		return firstValue(o, "series.lastObservedTime", "eventTime", "deprecatedLastTimestamp")
	}), col("Note", "note")},
	"certificatesigningrequests.certificates.k8s.io": {col("Signer", "spec.signerName"), col("Requester", "spec.username"), computed("Conditions", conditionSummary), computed("Issued", func(o map[string]any) string {
		if Value(o, "status", "certificate") == "" {
			return "Not reported"
		}
		return "Yes"
	})},
	"clustertrustbundles.certificates.k8s.io": {col("Signer", "spec.signerName"), computed("Certificates", func(o map[string]any) string {
		return fmt.Sprint(len(parseCertificates(Value(o, "spec", "trustBundle"))))
	})},
	"flowschemas.flowcontrol.apiserver.k8s.io":                 {col("Priority level", "spec.priorityLevelConfiguration.name"), col("Precedence", "spec.matchingPrecedence"), col("Distinguisher", "spec.distinguisherMethod.type"), computed("Conditions", conditionSummary)},
	"prioritylevelconfigurations.flowcontrol.apiserver.k8s.io": {col("Type", "spec.type"), col("Nominal shares", "spec.limited.nominalConcurrencyShares"), col("Limit response", "spec.limited.limitResponse.type"), computed("Conditions", conditionSummary)},
	"leasecandidates.coordination.k8s.io":                      {col("Lease", "spec.leaseName"), col("Binary version", "spec.binaryVersion"), col("Emulation version", "spec.emulationVersion"), col("Renewed", "spec.renewTime")},
	"customresourcedefinitions.apiextensions.k8s.io": {col("Group", "spec.group"), col("Kind", "spec.names.kind"), col("Scope", "spec.scope"), conditionCol("Established", "Established"), computed("Served versions", func(o map[string]any) string {
		var versions []string
		for _, v := range Maps(o, "spec", "versions") {
			if Value(v, "served") == "true" {
				versions = append(versions, Value(v, "name"))
			}
		}
		return strings.Join(versions, ", ")
	})},
	"apiservices.apiregistration.k8s.io": {computed("Group/version", func(o map[string]any) string { return Value(o, "spec", "group") + "/" + Value(o, "spec", "version") }), computed("Service", func(o map[string]any) string {
		if nestedMap(o, "spec", "service") == nil {
			return "Local"
		}
		return objectReference(nestedMap(o, "spec", "service"))
	}), conditionCol("Available", "Available")},
}

func conditionValue(o map[string]any, kind string) string {
	for _, c := range Maps(o, "status", "conditions") {
		if Value(c, "type") == kind {
			return Value(c, "status")
		}
	}
	return ""
}
func conditionSummary(o map[string]any) string {
	var parts []string
	for _, c := range Maps(o, "status", "conditions") {
		parts = append(parts, Value(c, "type")+"="+Value(c, "status"))
	}
	return strings.Join(parts, ", ")
}
func joinedMapValues(objects []map[string]any, key string) string {
	var values []string
	for _, o := range objects {
		if v := Value(o, key); v != "" {
			values = append(values, v)
		}
	}
	return strings.Join(values, ", ")
}
func ownerNames(o map[string]any) string {
	var names []string
	for _, owner := range Maps(o, "metadata", "ownerReferences") {
		names = append(names, objectReference(owner))
	}
	return strings.Join(names, ", ")
}
func nodeRoles(o map[string]any) string {
	var roles []string
	for _, key := range sortedKeys(nestedMap(o, "metadata", "labels")) {
		if strings.HasPrefix(key, "node-role.kubernetes.io/") {
			roles = append(roles, strings.TrimPrefix(key, "node-role.kubernetes.io/"))
		}
	}
	return fallback(strings.Join(roles, ", "), "none")
}
func portTexts(ports []map[string]any) []string {
	var texts []string
	for _, p := range ports {
		texts = append(texts, Value(p, "port")+"/"+fallback(Value(p, "protocol"), "TCP"))
	}
	return texts
}
func servicePorts(o map[string]any) string {
	return strings.Join(portTexts(Maps(o, "spec", "ports")), ", ")
}
func loadBalancerAddresses(o map[string]any) string {
	var addresses []string
	for _, a := range Maps(o, "status", "loadBalancer", "ingress") {
		addresses = append(addresses, firstValue(a, "ip", "hostname"))
	}
	return strings.Join(addresses, ", ")
}
func serviceAddresses(o map[string]any) string {
	return strings.Trim(strings.Join([]string{stringList(o, "spec", "externalIPs"), loadBalancerAddresses(o)}, ", "), ", ")
}
func routeColumns(hosts bool) []tableColumn {
	columns := []tableColumn{}
	if hosts {
		columns = append(columns, listCol("Hostnames", "spec.hostnames"))
	} else {
		columns = append(columns, computed("Backends", func(o map[string]any) string {
			var names []string
			for _, rule := range Maps(o, "spec", "rules") {
				for _, backend := range Maps(rule, "backendRefs") {
					names = append(names, Value(backend, "name")+":"+Value(backend, "port"))
				}
			}
			return strings.Join(names, ", ")
		}))
	}
	return append(columns, computed("Parents", func(o map[string]any) string { return joinedMapValues(Maps(o, "spec", "parentRefs"), "name") }), computed("Parent conditions", routeConditionSummary))
}
func routeConditionSummary(o map[string]any) string {
	var parts []string
	for _, p := range Maps(o, "status", "parents") {
		var conditions []string
		for _, c := range Maps(p, "conditions") {
			if k := Value(c, "type"); k == "Accepted" || k == "ResolvedRefs" {
				conditions = append(conditions, k+"="+Value(c, "status"))
			}
		}
		parts = append(parts, Value(p, "parentRef", "name")+": "+strings.Join(conditions, ", "))
	}
	return strings.Join(parts, "; ")
}
func grantReferences(refs []map[string]any) string {
	var values []string
	for _, r := range refs {
		values = append(values, fallback(Value(r, "group"), "core")+"/"+Value(r, "kind")+" "+strings.Trim(Value(r, "namespace")+"/"+fallback(Value(r, "name"), "*"), "/"))
	}
	return strings.Join(values, "; ")
}
func quotaExceeded(o map[string]any) string {
	hard, used := nestedMap(o, "status", "hard"), nestedMap(o, "status", "used")
	if len(hard) == 0 || used == nil {
		return "Not reported"
	}
	exceeded := 0
	for k, v := range hard {
		h, err := resource.ParseQuantity(scalar(v))
		u, err2 := resource.ParseQuantity(scalar(used[k]))
		if err != nil || err2 != nil {
			return "Not reported"
		}
		if u.Cmp(h) > 0 {
			exceeded++
		}
	}
	return fmt.Sprint(exceeded)
}
func hpaMetricCells(o map[string]any) string {
	current, targets := hpaMetrics("", Maps(o, "status", "currentMetrics"), "current"), hpaMetrics("", Maps(o, "spec", "metrics"), "target")
	currentByName := map[string]string{}
	for _, row := range current.Rows {
		currentByName[strings.Join(row[:3], "/")] = row[3]
	}
	var parts []string
	for _, row := range targets.Rows {
		key := strings.Join(row[:3], "/")
		parts = append(parts, strings.Trim(row[1]+" "+row[2], " ")+" "+fallback(currentByName[key], "?")+"/"+fallback(row[3], "?"))
	}
	if len(parts) == 0 && Value(o, "spec", "targetCPUUtilizationPercentage") != "" {
		parts = append(parts, "cpu "+fallback(Value(o, "status", "currentCPUUtilizationPercentage"), "?")+"%/"+Value(o, "spec", "targetCPUUtilizationPercentage")+"%")
	}
	return strings.Join(parts, "; ")
}
func jobDuration(o map[string]any) string {
	start, err := time.Parse(time.RFC3339, Value(o, "status", "startTime"))
	if err != nil {
		return "Not reported"
	}
	end := time.Now()
	if completed := Value(o, "status", "completionTime"); completed != "" {
		if parsed, err := time.Parse(time.RFC3339, completed); err == nil {
			end = parsed
		}
	}
	return max(time.Duration(0), end.Sub(start)).Truncate(time.Second).String()
}

func claimClassNames(spec map[string]any) string {
	var classes []string
	for _, r := range Maps(spec, "devices", "requests") {
		if class := firstValue(r, "exactly.deviceClassName", "deviceClassName"); class != "" {
			classes = append(classes, class)
		}
		for _, sub := range Maps(r, "firstAvailable") {
			if class := Value(sub, "deviceClassName"); class != "" {
				classes = append(classes, class)
			}
		}
	}
	return strings.Join(classes, ", ")
}

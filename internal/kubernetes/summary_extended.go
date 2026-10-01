package kubernetes

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

func extendedSummary(r Resource, o map[string]any) ([]DetailSection, bool) {
	switch r.Key() {
	case "controllerrevisions.apps":
		return []DetailSection{detailFields("Controller revision", o, "Revision", "revision"), {Title: "Controller data", Fields: []DetailField{{"Payload", "Controller-owned history. Inspect YAML for the stored patch."}}}}, true
	case "verticalpodautoscalers.autoscaling.k8s.io":
		sections := []DetailSection{detailFields("Vertical Pod Autoscaler", o, "Target API", "spec.targetRef.apiVersion", "Target kind", "spec.targetRef.kind", "Target name", "spec.targetRef.name", "Update mode", "spec.updatePolicy.updateMode", "Minimum replicas", "spec.updatePolicy.minReplicas")}
		for _, p := range Maps(o, "spec", "resourcePolicy", "containerPolicies") {
			s := detailFields("Container policy · "+Value(p, "containerName"), p, "Mode", "mode", "Controlled values", "controlledValues")
			s.Fields = append(s.Fields, DetailField{"Controlled resources", stringList(p, "controlledResources")}, DetailField{"Minimum", pairs(nestedMap(p, "minAllowed"))}, DetailField{"Maximum", pairs(nestedMap(p, "maxAllowed"))})
			sections = append(sections, s)
		}
		for _, p := range Maps(o, "status", "recommendation", "containerRecommendations") {
			s := DetailSection{Title: "Recommendation · " + Value(p, "containerName"), Columns: []string{"Resource", "Target", "Lower bound", "Upper bound", "Uncapped target"}}
			keys := unionKeys(nestedMap(p, "target"), nestedMap(p, "lowerBound"))
			for _, key := range keys {
				s.Rows = append(s.Rows, []string{key, Value(p, "target", key), Value(p, "lowerBound", key), Value(p, "upperBound", key), Value(p, "uncappedTarget", key)})
			}
			sections = append(sections, s)
		}
		return sections, true
	case "ipaddresses.networking.k8s.io":
		return []DetailSection{detailFields("IP address", o, "Address", "metadata.name", "Parent group", "spec.parentRef.group", "Parent resource", "spec.parentRef.resource", "Parent namespace", "spec.parentRef.namespace", "Parent name", "spec.parentRef.name")}, true
	case "servicecidrs.networking.k8s.io":
		return []DetailSection{{Title: "Service CIDR", Fields: []DetailField{{"CIDRs", stringList(o, "spec", "cidrs")}}}}, true
	case "gatewayclasses.gateway.networking.k8s.io":
		return []DetailSection{detailFields("Gateway class", o, "Controller", "spec.controllerName", "Description", "spec.description", "Parameter group", "spec.parametersRef.group", "Parameter kind", "spec.parametersRef.kind", "Parameter namespace", "spec.parametersRef.namespace", "Parameter name", "spec.parametersRef.name"), objectTable("Supported features", Maps(o, "status", "supportedFeatures"), []string{"Name"}, "name")}, true
	case "gateways.gateway.networking.k8s.io":
		return gatewaySummary(o), true
	case "httproutes.gateway.networking.k8s.io", "grpcroutes.gateway.networking.k8s.io", "tcproutes.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "udproutes.gateway.networking.k8s.io":
		return routeSummary(r, o), true
	case "referencegrants.gateway.networking.k8s.io":
		return []DetailSection{{Title: "Reference grant", Fields: []DetailField{{"Purpose", "Permits the listed cross-namespace object references; does not grant network traffic access."}}}, objectTable("From", Maps(o, "spec", "from"), []string{"Group", "Kind", "Namespace"}, "group", "kind", "namespace"), objectTable("To", Maps(o, "spec", "to"), []string{"Group", "Kind", "Name (empty means all)"}, "group", "kind", "name")}, true
	case "csidrivers.storage.k8s.io":
		s := detailFields("CSI driver", o, "Attach required", "spec.attachRequired", "Pod information on mount", "spec.podInfoOnMount", "Storage capacity tracking", "spec.storageCapacity", "Filesystem group policy", "spec.fsGroupPolicy", "Requires republish", "spec.requiresRepublish", "SELinux mount", "spec.seLinuxMount")
		s.Fields = append(s.Fields, DetailField{"Volume lifecycle modes", stringList(o, "spec", "volumeLifecycleModes")})
		return []DetailSection{s, objectTable("Token requests", Maps(o, "spec", "tokenRequests"), []string{"Audience", "Expiration seconds"}, "audience", "expirationSeconds")}, true
	case "csinodes.storage.k8s.io":
		s := DetailSection{Title: "Registered drivers", Columns: []string{"Driver", "Node ID", "Topology keys", "Allocatable volumes"}}
		for _, d := range Maps(o, "spec", "drivers") {
			s.Rows = append(s.Rows, []string{Value(d, "name"), Value(d, "nodeID"), stringList(d, "topologyKeys"), Value(d, "allocatable", "count")})
		}
		return []DetailSection{s}, true
	case "csistoragecapacities.storage.k8s.io":
		s := detailFields("CSI storage capacity", o, "Storage class", "storageClassName", "Capacity", "capacity", "Maximum volume size", "maximumVolumeSize")
		s.Fields = append(s.Fields, DetailField{"Node topology", labelSelector(nestedMap(o, "nodeTopology"))})
		return []DetailSection{s}, true
	case "volumeattachments.storage.k8s.io":
		return []DetailSection{detailFields("Volume attachment", o, "Attacher", "spec.attacher", "Node", "spec.nodeName", "Persistent volume", "spec.source.persistentVolumeName", "Attached", "status.attached", "Attach error", "status.attachError.message", "Attach error time", "status.attachError.time", "Detach error", "status.detachError.message", "Detach error time", "status.detachError.time"), mapSection("Attachment metadata", nestedMap(o, "status", "attachmentMetadata"), "Key", "Value"), structuredFields("Inline source", nestedMap(o, "spec", "source", "inlineVolumeSpec"))}, true
	case "volumeattributesclasses.storage.k8s.io":
		return []DetailSection{detailFields("Volume attributes class", o, "Driver", "driverName"), mapSection("Parameters (immutable)", nestedMap(o, "parameters"), "Parameter", "Value")}, true
	case "volumesnapshots.snapshot.storage.k8s.io":
		return []DetailSection{detailFields("Volume snapshot", o, "Ready to use", "status.readyToUse", "Source PVC", "spec.source.persistentVolumeClaimName", "Source content", "spec.source.volumeSnapshotContentName", "Snapshot class", "spec.volumeSnapshotClassName", "Bound content", "status.boundVolumeSnapshotContentName", "Creation time", "status.creationTime", "Restore size", "status.restoreSize", "Error", "status.error.message", "Error time", "status.error.time")}, true
	case "volumesnapshotcontents.snapshot.storage.k8s.io":
		return []DetailSection{detailFields("Snapshot content", o, "Ready to use", "status.readyToUse", "Driver", "spec.driver", "Snapshot class", "spec.volumeSnapshotClassName", "Deletion policy", "spec.deletionPolicy", "Snapshot namespace", "spec.volumeSnapshotRef.namespace", "Snapshot", "spec.volumeSnapshotRef.name", "Snapshot UID", "spec.volumeSnapshotRef.uid", "Source volume handle", "spec.source.volumeHandle", "Source snapshot handle", "spec.source.snapshotHandle", "Snapshot handle", "status.snapshotHandle", "Created (Unix nanoseconds)", "status.creationTime", "Restore bytes", "status.restoreSize", "Error", "status.error.message", "Error time", "status.error.time")}, true
	case "volumesnapshotclasses.snapshot.storage.k8s.io":
		s := detailFields("Snapshot class", o, "Driver", "driver", "Deletion policy", "deletionPolicy")
		s.Fields = append(s.Fields, DetailField{"Default", fallback(Value(o, "metadata", "annotations", "snapshot.storage.kubernetes.io/is-default-class"), "not set")})
		return []DetailSection{s, mapSection("Parameters", nestedMap(o, "parameters"), "Parameter", "Value")}, true
	case "podsecuritypolicies.policy":
		s := detailFields("Pod Security Policy (historical API)", o, "Privileged", "spec.privileged", "Host network", "spec.hostNetwork", "Host PID", "spec.hostPID", "Host IPC", "spec.hostIPC", "Privilege escalation", "spec.allowPrivilegeEscalation", "Read-only root filesystem", "spec.readOnlyRootFilesystem", "Run as user rule", "spec.runAsUser.rule", "Run as group rule", "spec.runAsGroup.rule", "SELinux rule", "spec.seLinux.rule", "FS group rule", "spec.fsGroup.rule", "Supplemental groups rule", "spec.supplementalGroups.rule")
		for _, f := range []string{"allowedCapabilities", "defaultAddCapabilities", "requiredDropCapabilities", "volumes", "allowedProcMountTypes"} {
			s.Fields = append(s.Fields, DetailField{f, stringList(o, "spec", f)})
		}
		return []DetailSection{s, structuredFields("User and group ranges", map[string]any{"runAsUser": nestedMap(o, "spec", "runAsUser"), "runAsGroup": nestedMap(o, "spec", "runAsGroup"), "supplementalGroups": nestedMap(o, "spec", "supplementalGroups"), "fsGroup": nestedMap(o, "spec", "fsGroup")}), detailFields("SELinux options", o, "User", "spec.seLinux.seLinuxOptions.user", "Role", "spec.seLinux.seLinuxOptions.role", "Type", "spec.seLinux.seLinuxOptions.type", "Level", "spec.seLinux.seLinuxOptions.level"), objectTable("Allowed host paths", Maps(o, "spec", "allowedHostPaths"), []string{"Path prefix", "Read-only"}, "pathPrefix", "readOnly"), objectTable("Host ports", Maps(o, "spec", "hostPorts"), []string{"Minimum", "Maximum"}, "min", "max"), structuredFields("Runtime constraints", nestedMap(o, "spec", "runtimeClass"))}, true
	case "resourceclaims.resource.k8s.io", "resourceclaimtemplates.resource.k8s.io":
		spec := nestedMap(o, "spec")
		title := "Resource claim"
		if r.Name == "resourceclaimtemplates" {
			spec = nestedMap(o, "spec", "spec")
			title = "Resource claim template"
		}
		s := DetailSection{Title: title, Fields: []DetailField{{"Classes", fallback(claimClassNames(spec), "Not reported")}}}
		sections := []DetailSection{s}
		if r.Name == "resourceclaimtemplates" {
			sections = append(sections, structuredFields("Generated claim metadata", nestedMap(o, "spec", "metadata")))
		}
		for _, request := range Maps(spec, "devices", "requests") {
			sections = append(sections, structuredFields("Device request · "+Value(request, "name"), request))
		}
		sections = append(sections, structuredFields("Constraints", map[string]any{"constraints": mapsAsAny(Maps(spec, "devices", "constraints"))}), structuredFields("Device configuration", map[string]any{"config": mapsAsAny(Maps(spec, "devices", "config"))}))
		if r.Name == "resourceclaims" {
			sections = append(sections, objectTable("Allocated devices", Maps(o, "status", "allocation", "devices", "results"), []string{"Request", "Driver", "Pool", "Device", "Admin access"}, "request", "driver", "pool", "device", "adminAccess"), structuredFields("Allocation node selector", nestedMap(o, "status", "allocation", "nodeSelector")), objectTable("Reserved consumers", Maps(o, "status", "reservedFor"), []string{"API group", "Resource", "Name", "UID"}, "apiGroup", "resource", "name", "uid"), structuredFields("Reported device status", map[string]any{"devices": mapsAsAny(Maps(o, "status", "devices"))}))
		}
		return sections, true
	case "deviceclasses.resource.k8s.io":
		return []DetailSection{detailFields("Device class", o, "Extended resource", "spec.extendedResourceName"), objectTable("Device selectors", Maps(o, "spec", "selectors"), []string{"CEL expression"}, "cel.expression"), structuredFields("Driver configurations", map[string]any{"config": mapsAsAny(Maps(o, "spec", "config"))})}, true
	case "resourceslices.resource.k8s.io":
		sections := []DetailSection{detailFields("Resource slice", o, "Driver", "spec.driver", "Pool", "spec.pool.name", "Pool generation", "spec.pool.generation", "Slice count", "spec.pool.resourceSliceCount", "Node", "spec.nodeName", "All nodes", "spec.allNodes"), structuredFields("Node selector", nestedMap(o, "spec", "nodeSelector"))}
		for _, d := range Maps(o, "spec", "devices") {
			sections = append(sections, structuredFields("Device · "+Value(d, "name"), d))
		}
		sections = append(sections, structuredFields("Shared counters", map[string]any{"sharedCounters": mapsAsAny(Maps(o, "spec", "sharedCounters"))}))
		return sections, true
	case "certificatesigningrequests.certificates.k8s.io":
		return certificateRequestSummary(o), true
	case "clustertrustbundles.certificates.k8s.io":
		return []DetailSection{detailFields("Cluster trust bundle", o, "Signer", "spec.signerName"), certificateSection("Trust certificates", Value(o, "spec", "trustBundle"))}, true
	case "flowschemas.flowcontrol.apiserver.k8s.io":
		sections := []DetailSection{detailFields("Flow schema", o, "Priority level", "spec.priorityLevelConfiguration.name", "Matching precedence", "spec.matchingPrecedence", "Distinguisher", "spec.distinguisherMethod.type")}
		for i, rule := range Maps(o, "spec", "rules") {
			sections = append(sections, structuredFields(fmt.Sprintf("Matching rule %d", i+1), rule))
		}
		return sections, true
	case "prioritylevelconfigurations.flowcontrol.apiserver.k8s.io":
		return []DetailSection{detailFields("Priority level", o, "Type", "spec.type", "Nominal concurrency shares", "spec.limited.nominalConcurrencyShares", "Legacy assured shares", "spec.limited.assuredConcurrencyShares", "Lendable percent", "spec.limited.lendablePercent", "Borrowing limit percent", "spec.limited.borrowingLimitPercent", "Limit response", "spec.limited.limitResponse.type", "Queues", "spec.limited.limitResponse.queuing.queues", "Hand size", "spec.limited.limitResponse.queuing.handSize", "Queue length limit", "spec.limited.limitResponse.queuing.queueLengthLimit", "Exempt nominal shares", "spec.exempt.nominalConcurrencyShares", "Exempt lendable percent", "spec.exempt.lendablePercent")}, true
	case "leasecandidates.coordination.k8s.io":
		return []DetailSection{detailFields("Lease candidate", o, "Lease", "spec.leaseName", "Binary version", "spec.binaryVersion", "Emulation version", "spec.emulationVersion", "Strategy", "spec.strategy", "Legacy preferred strategy", "spec.preferredStrategy", "Ping time", "spec.pingTime", "Renew time", "spec.renewTime")}, true
	case "apiservices.apiregistration.k8s.io":
		s := detailFields("API service", o, "API group", "spec.group", "Version", "spec.version", "Group priority", "spec.groupPriorityMinimum", "Version priority", "spec.versionPriority", "Service namespace", "spec.service.namespace", "Service name", "spec.service.name", "Service port", "spec.service.port", "Skip TLS verification", "spec.insecureSkipTLSVerify")
		if nestedMap(o, "spec", "service") == nil {
			s.Fields = append(s.Fields, DetailField{"Handling", "Local"})
		}
		return []DetailSection{s, certificateSection("CA certificates", decodeBase64(Value(o, "spec", "caBundle")))}, true
	}
	return nil, false
}

func gatewaySummary(o map[string]any) []DetailSection {
	sections := []DetailSection{detailFields("Gateway", o, "Class", "spec.gatewayClassName"), objectTable("Requested addresses", Maps(o, "spec", "addresses"), []string{"Type", "Value"}, "type", "value"), objectTable("Reported addresses", Maps(o, "status", "addresses"), []string{"Type", "Value"}, "type", "value")}
	if parameters := nestedMap(o, "spec", "infrastructure", "parametersRef"); parameters != nil {
		section := detailFields("Infrastructure parameters", parameters, "Group", "group", "Kind", "kind", "Name", "name")
		section.Fields = append(section.Fields, DetailField{"Namespace", Value(o, "metadata", "namespace")})
		sections = append(sections, section)
	}
	for _, listener := range Maps(o, "spec", "listeners") {
		name := Value(listener, "name")
		s := detailFields("Listener · "+name, listener, "Protocol", "protocol", "Port", "port", "Hostname", "hostname", "TLS mode", "tls.mode", "Allowed route namespaces", "allowedRoutes.namespaces.from")
		s.Fields = append(s.Fields, DetailField{"Namespace selector", optionalSelector(nestedMap(listener, "allowedRoutes", "namespaces", "selector"))})
		sections = append(sections, s, objectTable("TLS certificates · "+name, Maps(listener, "tls", "certificateRefs"), []string{"Group", "Kind", "Namespace", "Name"}, "group", "kind", "namespace", "name"), objectTable("Allowed route kinds · "+name, Maps(listener, "allowedRoutes", "kinds"), []string{"Group", "Kind"}, "group", "kind"))
		for _, status := range Maps(o, "status", "listeners") {
			if Value(status, "name") == name {
				s := detailFields("Listener status · "+name, status, "Attached routes", "attachedRoutes")
				sections = append(sections, s, conditionsSection("Listener conditions · "+name, Maps(status, "conditions")))
			}
		}
	}
	return sections
}

func routeSummary(r Resource, o map[string]any) []DetailSection {
	sections := []DetailSection{{Title: r.Kind, Fields: []DetailField{{"Hostnames", fallback(stringList(o, "spec", "hostnames"), "Not specified")}}}, objectTable("Parents", Maps(o, "spec", "parentRefs"), []string{"Group", "Kind", "Namespace", "Name", "Section", "Port"}, "group", "kind", "namespace", "name", "sectionName", "port")}
	if r.Name == "tcproutes" || r.Name == "udproutes" {
		sections[0].Fields = nil
	}
	for i, rule := range Maps(o, "spec", "rules") {
		title := fmt.Sprintf("Rule %d", i+1)
		sections = append(sections, detailFields(title, rule, "Name", "name", "Request timeout", "timeouts.request", "Backend timeout", "timeouts.backendRequest"), structuredFields(title+" matches", map[string]any{"matches": mapsAsAny(Maps(rule, "matches"))}), structuredFields(title+" filters", map[string]any{"filters": mapsAsAny(Maps(rule, "filters"))}), objectTable(title+" backends", Maps(rule, "backendRefs"), []string{"Group", "Kind", "Namespace", "Name", "Port", "Weight"}, "group", "kind", "namespace", "name", "port", "weight"))
		for _, backend := range Maps(rule, "backendRefs") {
			sections = append(sections, structuredFields(title+" backend filters · "+Value(backend, "name"), map[string]any{"filters": mapsAsAny(Maps(backend, "filters"))}))
		}
	}
	for _, parent := range Maps(o, "status", "parents") {
		title := objectReference(nestedMap(parent, "parentRef"))
		s := detailFields("Parent status · "+title, parent, "Controller", "controllerName", "Section", "parentRef.sectionName", "Port", "parentRef.port")
		sections = append(sections, s, conditionsSection("Parent conditions · "+title, Maps(parent, "conditions")))
	}
	return sections
}

func conditionsSection(title string, conditions []map[string]any) DetailSection {
	return objectTable(title, conditions, []string{"Type", "Status", "Reason", "Message", "Transition"}, "type", "status", "reason", "message", "lastTransitionTime")
}
func mapsAsAny(values []map[string]any) []any {
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

// Only explicit native sections use this renderer. It retains nested rule and
// selector paths without treating arbitrary objects as strings or JSON blobs.
func structuredFields(title string, o map[string]any) DetailSection {
	s := DetailSection{Title: title, Columns: []string{"Field", "Value"}}
	var visit func(string, any)
	visit = func(path string, v any) {
		if len(s.Rows) >= 120 {
			return
		}
		switch value := v.(type) {
		case map[string]any:
			for _, k := range sortedKeys(value) {
				p := k
				if path != "" {
					p = path + " / " + k
				}
				visit(p, value[k])
			}
		case []any:
			for i, item := range value {
				visit(fmt.Sprintf("%s [%d]", path, i+1), item)
			}
		default:
			if value != nil {
				if text := scalar(value); text != "" {
					s.Rows = append(s.Rows, []string{path, text})
				}
			}
		}
	}
	visit("", o)
	if len(s.Rows) >= 120 {
		s.Fields = append(s.Fields, DetailField{"More fields", "Open YAML for the complete specification."})
	}
	return s
}
func decodeBase64(value string) string {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return value
	}
	return string(decoded)
}
func parseCertificates(value string) []*x509.Certificate {
	var certificates []*x509.Certificate
	data := []byte(value)
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			certificates = append(certificates, cert)
		}
	}
	return certificates
}
func certificateSection(title, value string) DetailSection {
	s := DetailSection{Title: title, Columns: []string{"Subject", "Issuer", "Valid from", "Valid until", "SHA256 fingerprint"}}
	for _, cert := range parseCertificates(value) {
		digest := sha256.Sum256(cert.Raw)
		s.Rows = append(s.Rows, []string{cert.Subject.String(), cert.Issuer.String(), cert.NotBefore.Format(time.RFC3339), cert.NotAfter.Format(time.RFC3339), fmt.Sprintf("%X", digest)})
	}
	if value != "" && len(s.Rows) == 0 {
		s.Fields = append(s.Fields, DetailField{"Certificate data", "Could not parse certificates; inspect YAML."})
	}
	return s
}
func certificateRequestSummary(o map[string]any) []DetailSection {
	s := detailFields("Certificate signing request", o, "Signer", "spec.signerName", "Requester", "spec.username", "Requester UID", "spec.uid", "Expiration seconds", "spec.expirationSeconds")
	s.Fields = append(s.Fields, DetailField{"Groups", stringList(o, "spec", "groups")}, DetailField{"Usages", stringList(o, "spec", "usages")})
	request := decodeBase64(Value(o, "spec", "request"))
	if block, _ := pem.Decode([]byte(request)); block != nil {
		if csr, err := x509.ParseCertificateRequest(block.Bytes); err == nil {
			s.Fields = append(s.Fields, DetailField{"Subject", csr.Subject.String()}, DetailField{"DNS names", strings.Join(csr.DNSNames, ", ")}, DetailField{"Email names", strings.Join(csr.EmailAddresses, ", ")}, DetailField{"Public key", csr.PublicKeyAlgorithm.String()}, DetailField{"Signature", csr.SignatureAlgorithm.String()})
			var ips, uris []string
			for _, ip := range csr.IPAddresses {
				ips = append(ips, ip.String())
			}
			for _, uri := range csr.URIs {
				uris = append(uris, uri.String())
			}
			s.Fields = append(s.Fields, DetailField{"IP names", strings.Join(ips, ", ")}, DetailField{"URI names", strings.Join(uris, ", ")})
		} else {
			s.Fields = append(s.Fields, DetailField{"Request", "Could not parse certificate request; inspect YAML."})
		}
	}
	return []DetailSection{s, certificateSection("Issued certificates", decodeBase64(Value(o, "status", "certificate")))}
}

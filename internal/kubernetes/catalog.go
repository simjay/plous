package kubernetes

import "cmp"

type navigationCategory struct {
	id, label, previousLabel string
}

var navigationCategories = []navigationCategory{
	{"overview", "Overview", "Overview"},
	{"cluster", "Cluster", "Cluster Architecture"},
	{"containers", "Containers", "Containers"},
	{"workloads", "Workloads", "Workloads"},
	{"network", "Network", "Services, Load Balancing, and Networking"},
	{"storage", "Storage", "Storage"},
	{"config", "Config", "Configuration"},
	{"security", "Security", "Security"},
	{"policies", "Policies", "Policies"},
	{"scheduling", "Scheduling", "Scheduling, Preemption and Eviction"},
	{"allocation", "Allocation", "Resource Management"},
	{"admin", "Admin", "Cluster Administration"},
	{"extensions", "Extensions", "Extending Kubernetes"},
	{"other", "Other APIs", "Other APIs"},
}

// Concept labels are presentation only. Persisted state uses the stable IDs.
var Categories = func() []string {
	labels := make([]string, len(navigationCategories))
	for i, category := range navigationCategories {
		labels[i] = category.label
	}
	return labels
}()

// CategoryID accepts saved labels from older sessions as well as current IDs.
func CategoryID(value string) string {
	for _, category := range navigationCategories {
		if value == category.id || value == category.label || value == category.previousLabel {
			return category.id
		}
	}
	return value
}

func categoryLabel(id string) string {
	for _, category := range navigationCategories {
		if category.id == id {
			return category.label
		}
	}
	return "Other APIs"
}

type catalogEntry struct {
	category string
	title    string
	order    int
}

// Match the full API identity. An extension named pods is not the core Pod API.
var resourceCatalog = map[string]catalogEntry{
	"namespaces":                 {"overview", "Namespaces", 0},
	"nodes":                      {"cluster", "Nodes", 0},
	"leases.coordination.k8s.io": {"cluster", "Leases", 1},
	"runtimeclasses.node.k8s.io": {"containers", "RuntimeClasses", 0},

	"pods":                                      {"workloads", "Pods", 0},
	"replicationcontrollers":                    {"workloads", "ReplicationControllers", 5},
	"deployments.apps":                          {"workloads", "Deployments", 1},
	"statefulsets.apps":                         {"workloads", "StatefulSets", 2},
	"daemonsets.apps":                           {"workloads", "DaemonSets", 3},
	"replicasets.apps":                          {"workloads", "ReplicaSets", 4},
	"controllerrevisions.apps":                  {"workloads", "ControllerRevisions", 11},
	"jobs.batch":                                {"workloads", "Jobs", 6},
	"cronjobs.batch":                            {"workloads", "CronJobs", 7},
	"horizontalpodautoscalers.autoscaling":      {"workloads", "HorizontalPodAutoscalers", 8},
	"verticalpodautoscalers.autoscaling.k8s.io": {"workloads", "VerticalPodAutoscalers", 9},
	"poddisruptionbudgets.policy":               {"workloads", "PodDisruptionBudgets", 10},

	"services":                                  {"network", "Services", 0},
	"endpoints":                                 {"network", "Endpoints", 1},
	"endpointslices.discovery.k8s.io":           {"network", "EndpointSlices", 2},
	"ingresses.networking.k8s.io":               {"network", "Ingresses", 3},
	"ingressclasses.networking.k8s.io":          {"network", "IngressClasses", 4},
	"networkpolicies.networking.k8s.io":         {"network", "NetworkPolicies", 5},
	"ipaddresses.networking.k8s.io":             {"network", "IPAddresses", 6},
	"servicecidrs.networking.k8s.io":            {"network", "ServiceCIDRs", 7},
	"gateways.gateway.networking.k8s.io":        {"network", "Gateways", 8},
	"gatewayclasses.gateway.networking.k8s.io":  {"network", "GatewayClasses", 9},
	"httproutes.gateway.networking.k8s.io":      {"network", "HTTPRoutes", 10},
	"grpcroutes.gateway.networking.k8s.io":      {"network", "GRPCRoutes", 11},
	"tcproutes.gateway.networking.k8s.io":       {"network", "TCPRoutes", 12},
	"tlsroutes.gateway.networking.k8s.io":       {"network", "TLSRoutes", 13},
	"udproutes.gateway.networking.k8s.io":       {"network", "UDPRoutes", 14},
	"referencegrants.gateway.networking.k8s.io": {"network", "ReferenceGrants", 15},

	"persistentvolumeclaims":                         {"storage", "PersistentVolumeClaims", 0},
	"persistentvolumes":                              {"storage", "PersistentVolumes", 1},
	"storageclasses.storage.k8s.io":                  {"storage", "StorageClasses", 2},
	"csidrivers.storage.k8s.io":                      {"storage", "CSIDrivers", 3},
	"csinodes.storage.k8s.io":                        {"storage", "CSINodes", 4},
	"csistoragecapacities.storage.k8s.io":            {"storage", "CSIStorageCapacities", 5},
	"volumeattachments.storage.k8s.io":               {"storage", "VolumeAttachments", 6},
	"volumeattributesclasses.storage.k8s.io":         {"storage", "VolumeAttributesClasses", 7},
	"volumesnapshots.snapshot.storage.k8s.io":        {"storage", "VolumeSnapshots", 8},
	"volumesnapshotcontents.snapshot.storage.k8s.io": {"storage", "VolumeSnapshotContents", 9},
	"volumesnapshotclasses.snapshot.storage.k8s.io":  {"storage", "VolumeSnapshotClasses", 10},

	"configmaps": {"config", "ConfigMaps", 0},
	"secrets":    {"config", "Secrets", 1},

	"serviceaccounts":                                                {"security", "ServiceAccounts", 0},
	"roles.rbac.authorization.k8s.io":                                {"security", "Roles", 1},
	"rolebindings.rbac.authorization.k8s.io":                         {"security", "RoleBindings", 2},
	"clusterroles.rbac.authorization.k8s.io":                         {"security", "ClusterRoles", 3},
	"clusterrolebindings.rbac.authorization.k8s.io":                  {"security", "ClusterRoleBindings", 4},
	"podsecuritypolicies.policy":                                     {"security", "PodSecurityPolicies", 5},
	"mutatingwebhookconfigurations.admissionregistration.k8s.io":     {"security", "MutatingWebhookConfigurations", 6},
	"validatingwebhookconfigurations.admissionregistration.k8s.io":   {"security", "ValidatingWebhookConfigurations", 7},
	"mutatingadmissionpolicies.admissionregistration.k8s.io":         {"security", "MutatingAdmissionPolicies", 8},
	"mutatingadmissionpolicybindings.admissionregistration.k8s.io":   {"security", "MutatingAdmissionPolicyBindings", 9},
	"validatingadmissionpolicies.admissionregistration.k8s.io":       {"security", "ValidatingAdmissionPolicies", 10},
	"validatingadmissionpolicybindings.admissionregistration.k8s.io": {"security", "ValidatingAdmissionPolicyBindings", 11},

	"limitranges":                            {"policies", "LimitRanges", 0},
	"resourcequotas":                         {"policies", "ResourceQuotas", 1},
	"priorityclasses.scheduling.k8s.io":      {"scheduling", "PriorityClasses", 0},
	"resourceclaims.resource.k8s.io":         {"allocation", "ResourceClaims", 0},
	"resourceclaimtemplates.resource.k8s.io": {"allocation", "ResourceClaimTemplates", 1},
	"deviceclasses.resource.k8s.io":          {"allocation", "DeviceClasses", 2},
	"resourceslices.resource.k8s.io":         {"allocation", "ResourceSlices", 3},

	"events":               {"admin", "Events (core)", 0},
	"events.events.k8s.io": {"admin", "Events", 1},
	"certificatesigningrequests.certificates.k8s.io":           {"admin", "CertificateSigningRequests", 2},
	"clustertrustbundles.certificates.k8s.io":                  {"admin", "ClusterTrustBundles", 3},
	"flowschemas.flowcontrol.apiserver.k8s.io":                 {"admin", "FlowSchemas", 4},
	"prioritylevelconfigurations.flowcontrol.apiserver.k8s.io": {"admin", "PriorityLevelConfigurations", 5},
	"leasecandidates.coordination.k8s.io":                      {"admin", "LeaseCandidates", 6},

	"customresourcedefinitions.apiextensions.k8s.io": {"extensions", "CustomResourceDefinitions", 0},
	"apiservices.apiregistration.k8s.io":             {"extensions", "APIServices", 1},
}

func Category(r Resource) string {
	if entry, ok := resourceCatalog[r.Key()]; ok {
		return categoryLabel(entry.category)
	}
	return "Other APIs"
}

func ResourceTitle(r Resource) string {
	if entry, ok := resourceCatalog[r.Key()]; ok {
		return entry.title
	}
	if r.Group == "" {
		return r.Name + " (core)"
	}
	return r.Name + " (" + r.Group + ")"
}

// CompareResources keeps the primary workload kinds ahead of supporting records.
// Unmapped API identities retain a deterministic alphabetical order.
func CompareResources(a, b Resource) int {
	categoryOrder := func(resource Resource) int {
		id := CategoryID(Category(resource))
		for i, category := range navigationCategories {
			if category.id == id {
				return i
			}
		}
		return len(navigationCategories)
	}
	if order := cmp.Compare(categoryOrder(a), categoryOrder(b)); order != 0 {
		return order
	}
	left, leftKnown := resourceCatalog[a.Key()]
	right, rightKnown := resourceCatalog[b.Key()]
	if leftKnown && rightKnown {
		if order := cmp.Compare(left.order, right.order); order != 0 {
			return order
		}
	}
	return cmp.Compare(a.Key(), b.Key())
}

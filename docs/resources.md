# Kubernetes resource support

[Documentation](README.md) · [Explore Kubernetes](kubernetes-explore.md)

Explore discovers the APIs served by the selected cluster. A resource appears only when that API is available; reading it still depends on your permissions. A category groups resources in the sidebar. It is not a Kubernetes API group.

## Resource categories

These are the current catalog placements. Optional extensions, newer APIs, and removed legacy APIs appear only on servers that expose them. An unrecognized API goes under **Other APIs**, using its exact API group to avoid confusing it with a built-in type.

| Sidebar category | Resource types |
| --- | --- |
| Overview | Namespaces |
| Cluster | Nodes, Leases |
| Containers | RuntimeClasses |
| Workloads | Pods, Deployments, StatefulSets, DaemonSets, ReplicaSets, ReplicationControllers, Jobs, CronJobs, HorizontalPodAutoscalers, VerticalPodAutoscalers, PodDisruptionBudgets, ControllerRevisions |
| Network | Services, Endpoints, EndpointSlices, Ingresses, IngressClasses, NetworkPolicies, IPAddresses, ServiceCIDRs, Gateways, GatewayClasses, HTTPRoutes, GRPCRoutes, TCPRoutes, TLSRoutes, UDPRoutes, ReferenceGrants |
| Storage | PersistentVolumeClaims, PersistentVolumes, StorageClasses, CSIDrivers, CSINodes, CSIStorageCapacities, VolumeAttachments, VolumeAttributesClasses, VolumeSnapshots, VolumeSnapshotContents, VolumeSnapshotClasses |
| Config | ConfigMaps, Secrets |
| Security | ServiceAccounts, Roles, RoleBindings, ClusterRoles, ClusterRoleBindings, PodSecurityPolicies, MutatingWebhookConfigurations, ValidatingWebhookConfigurations, MutatingAdmissionPolicies, MutatingAdmissionPolicyBindings, ValidatingAdmissionPolicies, ValidatingAdmissionPolicyBindings |
| Policies | LimitRanges, ResourceQuotas |
| Scheduling | PriorityClasses |
| Allocation | ResourceClaims, ResourceClaimTemplates, DeviceClasses, ResourceSlices |
| Admin | Events (core), Events, CertificateSigningRequests, ClusterTrustBundles, FlowSchemas, PriorityLevelConfigurations, LeaseCandidates |
| Extensions | CustomResourceDefinitions, APIServices |
| Other APIs | Other discovered APIs, including custom resources without a catalog mapping. |

Gateway API, volume snapshots, and VerticalPodAutoscalers require their APIs to be installed. Discovery does not install extensions or make an old API available on a newer cluster.

## What resource views include

All readable objects have Describe, YAML, Events, and Related sections. Content depends on the kind and available data. Unknown kinds use a generic view and server-provided table columns when available.

| Resource | Main information and workflows |
| --- | --- |
| Pods | Readiness, restarts, containers, images, probes, resources, volumes, scheduling, network, and conditions. Logs and Exec use a selected container. |
| Deployments, StatefulSets, DaemonSets | Desired/current/ready counts, rollout state, strategy, template, conditions, and owned resources. Logs and Exec choose owned Pods. Restart follows the configured strategy. |
| ReplicaSets, ReplicationControllers | Replica counts, selectors, Pod template, owner, and owned Pods. Logs and Exec use owned Pods. |
| Jobs, CronJobs | Completion or schedule state, template, conditions, and owned Jobs/Pods. Logs and Exec follow ownership. |
| Services | Type, addresses, ports, selectors, endpoints, and related workloads. |
| Ingresses | Class, hosts, paths, backend Services, TLS references, and reported addresses. |
| ConfigMaps, Secrets | Keys and values. Secret values remain hidden until explicitly revealed. |
| PersistentVolumeClaims, PersistentVolumes | Binding, capacity, access modes, storage class, reclaim policy, and references when available. |
| Nodes | Readiness, addresses, capacity, allocatable resources, conditions, and scheduling configuration. |
| Namespaces | Phase, metadata, and an action to use that namespace in Explore. |
| RBAC and admission resources | Rules, subjects, references, and applicable configuration. |
| Custom resources | Discovered columns, object fields, YAML, events, and available relationships. |

Metadata includes names, namespace where applicable, age, labels, annotations, and ownership. Missing data and failed related-resource requests are reported in the view; they are not proof that no related objects exist.

## Available actions

Actions belong to an opened object's details. Common controls stay visible; **a More** lists additional actions and reasons an operation is unavailable.

- **Logs and Exec:** Pods and supported Pod-owning workloads.
- **Scale:** resources whose API exposes a supported scale subresource.
- **Restart:** Deployments, StatefulSets, and DaemonSets.
- **Edit and Delete:** where the resource, API, and permissions allow them. Some kinds permit metadata-only edits.
- **Values:** ConfigMap and Secret keys, CertificateSigningRequest data, and ControllerRevision snapshots.
- **Use namespace:** Namespace objects.
- **Copy identity and Export YAML:** the captured object's identity and manifest. Secret YAML exports stay redacted.

See [Resource actions](kubernetes-actions.md) for confirmation, saving, and error behavior. Permission to list a resource does not grant permission to change it, read its logs, or open Exec.

## Limits

Tables retain at most 10,000 objects per scope. Logs have their own [retention limits](logs.md#retained-history-and-reconnecting). Narrow namespace or label selectors when needed.

Live usage metrics, historical charts, resource creation, bulk operations, node drain, attach, port forwarding, Job/CronJob execution controls, volume resize, and snapshot creation are not available.

Helm, Kustomize, Terraform, Argo, and Kargo connections currently save references and display settings. Kubernetes is the live resource integration in this build.

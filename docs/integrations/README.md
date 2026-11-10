# Tool integrations

[Documentation](../README.md) · [Workspaces](../guides/workspaces.md)

Plous brings Kubernetes, Helm, Argo CD, and Flux into the same cluster workspace, with Overview, Explore, and Activity tabs. Start with your terminal's context or save several cluster connections in a workspace.

## Current capabilities

| Technology | Capabilities | Guide |
| --- | --- | --- |
| Kubernetes | Resources, events, relationships, usage, logs, and explicit resource actions | [Kubernetes](kubernetes/README.md) |
| Helm | Release revisions, values, manifests, chart previews, and explicit rollback | [Helm](helm/README.md) |
| Argo CD | Applications, ApplicationSets, Projects, and reported controller state | [Argo CD](argo/README.md) |
| Flux | Kustomizations, HelmReleases, Sources, and reported reconciliation state | [Flux](flux/README.md) |

### What can change the cluster?

Kubernetes detail actions can edit YAML, scale, restart, or delete supported resources after explicit user intent. Container Exec runs the command you choose. Helm rollback requires an explicit preview and confirmation. Chart changes are previews; Helm install and upgrade execution are unavailable.

GitOps views report controller state. Plous does not fetch their repositories or reconcile source changes. Assistant tools use reads by default; explicitly enabled [active diagnostics](../reference/mcp.md#active-diagnostics) can run container commands or temporary tests. See [Kubernetes actions](kubernetes/README.md#resource-actions) and [Helm rollback](helm/README.md#rollback).

## Add a connection

1. Select a workspace in the left panel.
2. Open **Monitor → + Add Context**, or **+ Add Connection → Kubernetes Cluster**.
3. Choose a kubeconfig file, select contexts, and save.
4. Select the connection in the tree, then press **22** for Explore.

Use **Settings → Current Workspace** to edit or remove connections. Removing a connection removes its Plous entry without deleting cluster resources or changing kubeconfig.

## Saved tool connections

Use a connection name that identifies the target, such as `Production cluster`.

| Tool | Required fields besides name | Optional fields |
| --- | --- | --- |
| Kubernetes Cluster, including Helm | Kube Context | Kubeconfig Path, namespace, release storage, explicit repository/cache/registry configuration paths |

The shared [context picker](kubernetes/README.md#connect-a-cluster) adds each cluster once. Kubernetes is always available. Helm appears after detecting release metadata; Argo and Flux appear after discovering their APIs. Existing separate Kubernetes and Helm records retain their identities and histories.

Cluster setup saves access references without contacting the cluster or changing its active context. Saved configuration changes appear in [Activity](../guides/activity.md).

For controller-specific views, read [Argo CD](argo/README.md) or [Flux](flux/README.md).

## AI and integrations

The selected agent uses guarded Kubernetes, Helm, Argo, Flux, Workspace, and History tools. Availability follows the captured cluster; General chat has no connection tools. Current usage reads are saved as evidence without exposing unsaved chart history. See the [MCP tool reference](../reference/mcp.md#available-tools) and [AI chat](../guides/ai-chat.md).

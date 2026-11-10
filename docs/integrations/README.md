# Tool integrations

[Documentation](../README.md) · [Workspaces](../guides/workspaces.md)

Add a cluster once. Plous uses that connection for Kubernetes and any detected Helm, Argo CD, or Flux tools.

## Current capabilities

| Tool | Operations | Guide |
| --- | --- | --- |
| Kubernetes | Browse resources, events, logs, metrics, and YAML. Edit, scale, restart, delete, or open a container terminal. | [Kubernetes](kubernetes/README.md) |
| Helm | Read releases and revisions, compare configuration, preview charts, and roll back | [Helm](helm/README.md) |
| Argo CD | Read Applications, ApplicationSets, Projects, and their reported status | [Argo CD](argo/README.md) |
| Flux | Read Kustomizations, HelmReleases, Sources, and reconciliation status | [Flux](flux/README.md) |

### What can change the cluster?

Kubernetes actions and container terminals can make changes. Helm rollback requires a preview and confirmation. Chart previews do not install or upgrade anything.

Argo CD and Flux views are read-only. Agent tools also read by default. [Active diagnostics](../mcp/active-diagnostics.md) can explicitly enable container commands or temporary tests.

## Add a connection

1. Select a workspace.
2. Choose **Monitor → + Add Context** or **+ Add Connection → Kubernetes Cluster**.
3. Choose the kubeconfig file and contexts, then save.
4. Select the connection and press `22` for Explore.

Use **Settings → Current Workspace** to edit or remove connections. Removal does not delete cluster resources or edit kubeconfig.

## Saved tool connections

| Required | Optional |
| --- | --- |
| Name and Kubernetes context | Kubeconfig path, namespace, Helm release storage, repository/cache/registry configuration paths |

Kubernetes is always available. Helm appears when release metadata is found. Argo CD and Flux appear when their APIs are discovered.

Adding a context reads local metadata without contacting the cluster. Opening its views connects. Existing separate Kubernetes and Helm records keep their identities and histories.

See [connection setup](kubernetes/README.md#connect-a-cluster) and [Activity](../guides/activity.md).

## AI and integrations

Agents use the selected cluster and its Kubernetes permissions. General chat has no live cluster tools.

See the [tool reference](../reference/mcp.md#available-tools) and [AI setup](../guides/ai-chat.md).

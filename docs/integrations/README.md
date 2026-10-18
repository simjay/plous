# Tool integrations

[Documentation](../README.md) · [Workspaces](../guides/workspaces.md)

Plous brings Kubernetes and its configuration tools into two connected workflows: **Monitor** for live cluster observations and **Code** for source files and generated configuration. Both use the same workspace tree and the Overview, Explore, and Activity tabs.

Start with your terminal's context and launch directory, or save several clusters and sources in a workspace. A source and cluster can share a workspace without implying that the source produced the live resources.

## Current capabilities

| Technology | Monitor | Code | Guide |
| --- | --- | --- | --- |
| Kubernetes | Resources, events, relationships, usage, logs, and explicit resource actions | Browse and edit local YAML; inspect rendered objects | [Kubernetes](kubernetes/README.md) |
| Helm | Release revisions, values, manifests, chart previews, and explicit rollback | Browse and render supported charts with chosen inputs | [Helm](helm/README.md) |
| Kustomize | No standalone cluster service; Flux Kustomizations appear under Flux | Bases, overlays, components, dependency graphs, rendering, field history, and comparisons | [Code and Kustomize](kustomize/README.md) |
| Argo CD | Applications, ApplicationSets, Projects, and reported controller state | Browse declarations and related local configuration | [Argo and Flux](gitops/README.md) |
| Flux | Kustomizations, HelmReleases, Sources, and reported reconciliation state | Browse controller and source declarations | [Argo and Flux](gitops/README.md) |

For a first source workflow, follow [Code: from source to rendered resources](../guides/code.md).

### What can change the cluster?

Kubernetes detail actions can edit YAML, scale, restart, or delete supported resources after explicit user intent. Container Exec runs the command you choose. Helm rollback requires an explicit preview and confirmation. Chart changes are previews; Helm install and upgrade execution are unavailable.

GitOps Monitor views and assistant tools are read-only. Code edits local files and renders configuration without deploying it. Plous does not run deployment pipelines or automatically reconcile source changes into a cluster. See [Kubernetes actions](kubernetes/README.md#resource-actions), [Helm rollback](helm/README.md#rollback), and [Code boundaries](kustomize/README.md#engines-and-boundaries).

Argo and Flux use existing Kubernetes connections and appear when their APIs are discovered. They do not require separate Add Connection entries. Legacy standalone Argo, Terraform, and Kargo references remain readable and removable in Settings.

## Add a connection

1. Select a workspace in the left panel.
2. Open **+ Add Connection** and choose **Monitor → Kubernetes Cluster** or **Code → Source**.
3. Complete its setup and save.
4. Select the connection in the tree, then press **22** for Explore.

Use **Settings → Current Workspace** to edit or remove connections. Removing a connection removes its Plous entry; it does not delete the referenced files or resources.

## Saved tool connections

All forms ask for a connection name. Use a name that identifies the target, such as `Production cluster` or `Staging overlay`.

| Tool | Required fields besides name | Optional fields |
| --- | --- | --- |
| Kubernetes Cluster, including Helm | Kube Context | Kubeconfig Path, namespace, release storage, explicit repository/cache/registry configuration paths |
| Code Source | Location type and local directory or Git Repository URL | Branch for Git repositories; blank uses the default branch |

The shared [context picker](kubernetes/README.md#connect-a-cluster) adds each cluster once for Kubernetes and detected Helm, Argo, and Flux views. Existing separate records are preserved, including different namespaces and Helm settings.

Cluster setup saves access references without contacting the cluster or changing its active context. Adding a Code source starts a background source scan. Plous uses embedded Go libraries to read Git, render Helm and Kustomize, and access Kubernetes. Saved changes appear in [Activity](../guides/activity.md).

## AI and integrations

The selected agent uses the same guarded Kubernetes, Helm, Argo, Flux, Source, Workspace and History tools. Availability follows the captured connection or source; General chat has no connection tools. Current usage reads are saved as evidence but do not expose unsaved chart history. See the [MCP tool reference](../reference/mcp.md#available-tools) for operations and [AI chat](../guides/ai-chat.md) for setup and scope. Legacy Terraform or other references do not grant execution or arbitrary file access.

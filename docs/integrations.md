# Tool integrations

[Documentation](README.md) · [Workspaces](workspaces.md)

Add the tools used by your project to the same workspace. Each connection retains its own settings. What Explore can do depends on the selected tool and its current implementation.

## Current capabilities

| Tool | Available in this build | Guide |
| --- | --- | --- |
| Kubernetes | Live resources, details, logs, and supported operations. | [Connect a cluster](kubernetes.md) |
| Helm | Save a context and namespace reference; view connection settings. | [Connection fields](#saved-tool-connections) |
| Kustomize | Save an overlay or directory reference; view connection settings. | [Connection fields](#saved-tool-connections) |
| Terraform | Save a working directory and optional workspace; view connection settings. | [Connection fields](#saved-tool-connections) |
| Argo | Save repository and optional server references; view connection settings. | [Connection fields](#saved-tool-connections) |
| Kargo | Save a context and namespace/project reference; view connection settings. | [Connection fields](#saved-tool-connections) |

Saving a reference does not run a tool or verify its target. Helm release management, Kustomize builds, Terraform plans or applies, Argo synchronization, and Kargo promotions are not available.

## Add a connection

1. Select a workspace in the left panel.
2. Open **+ Add tool** and choose a tool.
3. Complete its setup and save.
4. Select the connection in the tree, then press **22** for Explore.

Use **Settings → Current workspace** to edit or remove connections. Removing a connection removes its Plous entry; it does not delete the referenced files or resources.

## Saved tool connections

All forms ask for a connection name. Use a name that identifies the target, such as `Production infrastructure` or `Staging overlay`.

| Tool | Required fields besides name | Optional fields |
| --- | --- | --- |
| Helm | Kube context | Kubeconfig path, namespace |
| Kustomize | Directory / overlay path | None |
| Terraform | Working directory | Terraform workspace |
| Argo | Git repository URL or path | Argo server, Git revision, repository path |
| Kargo | Kube context | Kubeconfig path, namespace / project |

Kubernetes uses a [context picker](kubernetes.md#add-a-kubernetes-context) that can add several contexts at once.

These paths and URLs remain references. Adding them does not read repository contents, run commands, or change another tool's active context. Saved changes appear in [Activity](activity.md).

## AI and integrations

The assistant can discuss any tool using the text you provide. Its live tool access currently covers read-only Kubernetes investigation. A saved Terraform, Helm, or other connection does not grant access to that tool's files or execution. See [AI assistant](ai-chat.md) for scope and limits.

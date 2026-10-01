# Connect Kubernetes

[Documentation](README.md) · [Integrations](integrations.md)

Use a Kubernetes connection to browse a cluster, inspect resources, read logs, and run supported actions. You need a workspace, a working kubeconfig, and permission to access the cluster.

## Before you connect

A **kubeconfig** is a local file containing cluster and credential references. A **context** selects a cluster, credentials, and an optional default namespace. You can add several contexts from different files to one workspace.

Plous uses the Kubernetes API directly; kubectl is not required. Install and authenticate any credential helper referenced by your kubeconfig. Plous does not create or start a cluster.

## Add a Kubernetes context

A Kubernetes connection stores a reference to a kubeconfig file and a context within it.

1. Select **+ Add tool** under the workspace, then choose **Kubernetes**. You can also use **Settings → Current workspace → Add tool**, or press Enter in an empty workspace's Overview.
2. In **Kubeconfig file**, check the displayed path. The initial value is `~/.kube/config`.
3. To use a different file, edit the path and choose **Refresh contexts**.
4. In **Contexts**, use arrows to select a row and **Space** or **Enter** to check it. You can check more than one. **Already added** entries cannot be checked again.
5. Select **Add selected contexts** and press **Enter**.

To add contexts later, use **+ Add context** under the workspace's expanded Kubernetes group. The action applies to the workspace containing that row.

Editing the path clears the old list and checked selections. **Refresh required** means you must reload before adding anything. Refreshing the same file also clears its checked selections. Esc or Cancel discards unsaved selections.

Setup reads the file shown in the field, even if your shell has a different `KUBECONFIG` environment variable. Contexts from different files can coexist in one workspace. Their file references stay separate.

Adding contexts reads local names and references. It does not contact a cluster or run authentication plugins. Opening [Kubernetes Explore](kubernetes-explore.md) uses the selected context to connect and may run its configured credential plugin. Neither step changes your shell's current Kubernetes context.

## Open your first resource list

Colon commands run while navigating, outside text fields.

1. Type `:pods` and press **Enter**.
2. If a connection picker opens, choose the context you just added and press **Enter**.
3. Wait for the list to load. The header shows the context and namespace currently being browsed.
4. If there are no Pods in that namespace, press **n**, select **All namespaces**, then press **Enter**. Your account may have access to only some namespaces.
5. Use **Up / Down** to select a Pod and **Enter** to open its Describe section.
6. Press **Esc** to return to the list.

An empty list can be a valid result. It does not by itself mean the connection failed. Follow [Explore Kubernetes](kubernetes-explore.md) for filtering, logs, and investigation, or [Troubleshooting](troubleshooting.md#the-resource-list-is-empty) if you expected resources.

## Continue

- [Explore resources](kubernetes-explore.md): namespaces, filtering, details, and relationships.
- [Resource support](resources.md): available types, actions, and limits.
- [Kubernetes playground](playground.md): examples for a disposable test cluster.

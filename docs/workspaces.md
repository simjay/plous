# Manage workspaces and connections

[Documentation](README.md)

A workspace groups the tool connections you want to use together. For example, `Commerce` might contain Kubernetes connections for two environments, a Terraform directory, and a Kustomize overlay. You choose the grouping; a workspace is not limited to one cluster.

All workspaces are local in this version. The **Remote (coming soon)** option is disabled.

## Create or select a workspace

On the first launch, press **Enter** in the empty main view. For another workspace, select **+ New workspace** in the tree or run `:n`.

Enter a name, move to **Save**, and press **Enter**. The tool picker opens next. Choose a tool or press **Esc** to keep the workspace empty.

To switch workspaces, focus the left panel with **Tab / Shift+Tab**, move to a workspace, and press **Enter**. Enter selects that scope and toggles its branch while keeping the current main mode. Left/Right only fold or expand branches. Finish or cancel an open form before switching.

## Add a tool connection

Select **+ Add tool** under the workspace, choose a tool, and complete its setup. You can also use **Settings → Current workspace → Add tool**, or press Enter in an empty workspace's Overview.

Each connection keeps its own name and settings. A workspace can contain several connections for the same tool. Select a connection in the tree to see its Overview or Explore view.

The [integration guide](integrations.md) lists each tool's fields and current capabilities. Saving a connection does not execute the tool or change its external active context.

## Add Kubernetes contexts

Use the [Kubernetes connection guide](kubernetes.md#add-a-kubernetes-context) to choose a kubeconfig file and add one or more contexts. Later, **+ Add context** adds connections to the workspace containing that row.

## Add other tools

Helm, Kustomize, Terraform, Argo, and Kargo have saved connection forms. See [connection fields](integrations.md#saved-tool-connections). Their Explore views currently show settings; live tool operations are not implemented.

## Edit or remove a connection

1. Select the workspace you want to change.
2. Open **Settings** with **ss** or `:s`.
3. Under **Current workspace**, select **Edit tool / connection**.
4. Update the fields and select **Save**, or select **Remove connection** and confirm.

Removing a connection removes its Plous entry. It does not delete resources, modify kubeconfig, or remove the referenced directory. Renaming a connection keeps its identity and history.

To rename the workspace itself, use **Settings → Current workspace → Rename workspace**.

## Understand Settings scopes

| Settings section | Applies to |
| --- | --- |
| Application | AI vendor connections shared across every workspace and General chat. |
| Current workspace | The selected workspace's name, tool connections, and local location. |

Settings can be opened before any workspace exists. It then offers **Create workspace** as well as AI setup.

Use arrows and Enter within Settings. Tab and Shift+Tab do nothing. Esc cancels a form back to Settings or its preceding setup step. Esc or **Close** from the Settings menu restores your previous view and focus. Unsaved form edits are not applied when you exit the app.

## Review changes in Activity

Press **33** to see creation, renaming, and connection changes for the workspace. Open a change to inspect its summary or compare the saved revisions. Follow [Activity and workspace history](activity.md) for the controls.

Next: [Choose an integration](integrations.md), [Set up AI](ai-chat.md), or [Find your saved data](data-and-privacy.md).

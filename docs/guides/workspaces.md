# Manage workspaces and connections

[Documentation](../README.md)

A workspace groups the tool connections you want to use together. For example, `Commerce` might contain Kubernetes contexts for staging and production. You choose the grouping; a workspace is not limited to one cluster.

All workspaces are local in this version. The **Remote (coming soon)** option is disabled.

## Create or select a workspace

Plous starts in **Current Workspace** without requiring a workspace. Monitor follows the terminal's kubeconfig default at launch. Use `:K` to choose another context inside Plous.

Use **Settings → Save as Workspace** to retain the selected cluster as an explicit saved connection. Its existing history remains under Current Workspace. To start empty, select **Workspaces → + New Workspace** in the tree or run `:n`.

Enter a name, move to **Save**, and press **Enter**. The connection picker opens next. Choose a connection or press **Esc** to keep the workspace empty.

To switch workspaces, focus the left panel with **Alt+Left / Alt+Right**, move to a workspace, and press **Enter**. Enter selects that scope and toggles its branch while keeping the current main mode. Left/Right only fold or expand branches. Saved connections start collapsed when Plous opens. Select one to restore its preferences and connection-specific chat draft. Finish or cancel an open form before switching.

You can navigate and fold the sidebar while context and tool discovery are loading. New results keep the highlighted branch and your fold choices. Kubernetes resource categories appear when discovery completes, including in Overview. If you start navigating before the launch context resolves, it becomes available under Monitor without switching your current panel or conversation.

## Add a tool connection

The sidebar groups cluster connections under **Monitor**. Use **Monitor → + Add Context** to add a cluster.

Select **+ Add Connection** under the workspace, choose a connection type, and complete its setup. You can also use **Settings → Current Workspace → Add Connection**, or press Enter in an empty workspace's Overview.

Each connection keeps its own name and settings. A workspace can contain several connections for the same tool. Select a connection in the tree to see its Overview or Explore view.

The [integration guide](../integrations/README.md) lists each tool's fields and current capabilities. Saving a cluster connection does not execute the tool or change its external active context.

## Add Kubernetes contexts

Use the [Kubernetes connection guide](../integrations/kubernetes/README.md#connect-a-cluster) to choose a kubeconfig file and add one or more contexts. Later, **+ Add Context** adds connections to the workspace containing that row.

## Add other tools

Current Workspace and saved Monitor connections use the same cluster tree. Kubernetes is always available. The [Helm view](../integrations/helm/README.md) appears after Plous finds release metadata in that context; read-only [Argo CD](../integrations/argo/README.md) and [Flux](../integrations/flux/README.md) views appear when their APIs are discovered. Select or expand the cluster to discover its tools. Press `g` on its row to retry discovery. See [connection fields](../integrations/README.md#saved-tool-connections).

## Edit or remove a connection

1. Select the workspace you want to change.
2. Open **Settings** with **ss** or `:s`.
3. Under **Current Workspace**, select **Edit tool / connection**.
4. Update the fields and select **Save**, or select **Remove connection** and confirm.

Removing a connection removes its Plous entry without deleting resources or modifying kubeconfig. Renaming a connection keeps its identity and history.

To rename the workspace itself, use **Settings → Current Workspace → Rename Workspace**.

## Understand Settings scopes

| Settings section | Applies to |
| --- | --- |
| Application | Main agent, mouse support, and AI vendor connections shared across the profile. |
| Current Workspace (terminal environment) | Choose a context or kubeconfig, save the selected cluster as a workspace. |
| Current Workspace (saved workspace selected) | The selected workspace's name, tool connections, and local location. |

In Current Workspace, Settings offers **Choose Context**, **Choose Kubeconfig**, and **Save as Workspace**, alongside Application controls. A kubeconfig override lasts for this session. Use `:n` to create an empty workspace.

Use arrows and Enter within Settings. Alt+Left/Right leave and resume Settings without applying edits. Esc cancels a form back to Settings or its preceding setup step. Esc or **Close** from the Settings menu restores your previous view and focus. Unsaved form edits are not applied when you exit the app.

## Activity and revisions

Press **33** or `:3` to review saved workspace configuration changes, recorded Helm operation outcomes, and observed release history. Workspace comparisons show exact Before/After revisions; they do not apply or roll back infrastructure.

The [Activity guide](activity.md) explains filtering, revision comparisons, original observations, and how this timeline differs from Sources, chart history, and Overview's recent changes. History stays in the local profile; see [Data and privacy](../reference/data-and-privacy.md) for backups.

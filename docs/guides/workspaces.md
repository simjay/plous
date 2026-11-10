# Manage workspaces and connections

[Documentation](../README.md)

Save clusters you use often in a workspace. For example, a `Commerce` workspace might contain staging and production contexts.

## Create or select a workspace

You can use **Current Workspace** without saving anything. It opens the current context from your terminal kubeconfig. Use `:K` to choose another context in Plous.

To save the current cluster, open **Settings → Save as Workspace**. Existing Current Workspace chat history stays there.

To start with an empty workspace:

1. Select **Workspaces → + New Workspace**, or type `:n`.
2. Enter a name and select **Save**.
3. Choose a connection, or press Esc to leave the workspace empty.

To switch, select a workspace or connection in the sidebar and press Enter. Finish or cancel an open form first.

## Add a tool connection

Select **Monitor → + Add Context** to add a cluster. You can also use **+ Add Connection** under the workspace or **Settings → Current Workspace → Add Connection**.

A workspace can contain several cluster contexts. Each connection keeps its own settings, selected namespace, and chat.

All workspaces are local. The Remote option is disabled.

## Add Kubernetes contexts

Choose the kubeconfig file and one or more contexts in the setup window. See [Connect a cluster](../integrations/kubernetes/README.md#connect-a-cluster) for the form fields.

Saving or selecting a connection does not change another tool's active context.

## Add other tools

Kubernetes is always available. Plous adds Helm when it finds release metadata, and Argo CD or Flux when it discovers their APIs.

Select or expand a cluster to discover its tools. Press `g` to retry discovery. See [integration support](../integrations/README.md).

## Edit or remove a connection

1. Select the workspace.
2. Open Settings with `ss` or `:s`.
3. Choose **Current Workspace → Edit tool / connection**.
4. Edit and save, or choose **Remove connection** and confirm.

Removing a connection deletes its Plous entry. It does not delete cluster resources or edit kubeconfig. Renaming keeps the connection's history.

Use **Current Workspace → Rename Workspace** to rename the workspace.

## Understand Settings scopes

| Section | Settings |
| --- | --- |
| Application | Agent, mouse support, and provider connections |
| Current Workspace | Current kubeconfig and context, or the selected saved workspace's connections and name |

A kubeconfig override in Current Workspace lasts for the session. Use Alt+Left / Alt+Right to leave and return to an unfinished Settings form.

## Activity and revisions

Press `33` to review workspace changes and Helm history. Workspace comparisons show saved Before / After values. They do not restore infrastructure.

See [Activity](activity.md) and [profile backups](../reference/data-and-privacy.md#back-up-or-recover-a-profile).

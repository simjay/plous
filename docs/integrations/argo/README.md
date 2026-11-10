# Argo CD

[Documentation](../../README.md) · [All integrations](../README.md) · [Connect Kubernetes](../kubernetes/README.md#connect-a-cluster)

Plous reads Argo CD controller objects through the selected Kubernetes connection. Expand its tool under Monitor after API discovery. No separate controller account, CLI, or Git connection is needed.

Argo CD views cover Applications, ApplicationSets, and Projects.

## Open the view

1. Expand a cluster connection under **Monitor**.
2. Expand **Argo**, select a category, and press **Enter**. Opening the tool itself restores its last category.
3. Select a row to read its compact preview, then press **Enter** for the full controller message and details.
4. Select a managed resource or related Application and press **Enter** to investigate it. **Esc** returns to the previous view.

Tables show one row per object with namespace and status columns. A short preview follows the selection. Small panels prioritize the list and keep details available through Enter. Category switching preserves the current filter and selection; each category saves its namespace and selected object across restarts.

The first list covers **All namespaces**. **n** opens the namespace picker; the scope applies to the controller objects, which can deploy workloads into other namespaces. Use **/** to filter the list, **Enter** to open a match, and **Backspace** to clear an applied filter. In details, **/** finds text and **Page Up / Page Down** scrolls. Select **Configuration & revisions** and press **Enter** to expand its metadata. Visible lists and details refresh every 15 seconds. **g** refreshes immediately. The [keymap](../../reference/keymap.md#argo-and-flux) lists the controls.

Opening or expanding a connection discovers these APIs. Saving a connection does not contact its cluster. Saved inactive connections stay disconnected at startup; Current Workspace connects to its selected context. A missing API, a successful empty list, a denied read, and a connection failure are different states. If a refresh fails, previously loaded rows remain labeled **Previous observations** with the error. When a cluster-wide list is denied, choose a namespace your account can read. After installing a controller, select the cluster connection in the sidebar and press **g** to discover its APIs again.

## Argo Applications

The **Applications** category monitors Argo CD Applications.

The list shows the Application name, namespace, **Sync**, **Health**, and target namespace. Destination cluster details remain in Configuration & revisions. These statuses answer different questions:

| Example | Meaning |
| --- | --- |
| Synced and Degraded | Argo reports that the desired configuration matches, but the application's reported health has a problem. |
| OutOfSync and Healthy | Argo reports a configuration difference while the current resources are healthy. |
| Not reported | The object does not supply that observation. |

Details show the controller's reason, reconciliation time, and reported managed resources. Configuration and revisions retain their own labels: a compared revision is not necessarily the last successfully deployed revision. Per-resource health can be missing even when application health is present.

Opening a managed resource uses the existing Kubernetes details view. Local destinations use the selected context. An external destination needs a saved context whose API endpoint matches; an Argo destination name alone is insufficient. Plous does not read Argo's stored destination credentials. Without matching access, the resource reference remains visible with an explanation.

## Argo ApplicationSets and Projects

**ApplicationSets** shows generator types, controller conditions, and related Application counts. **Projects** shows AppProject objects with repository and destination rule counts. Projects are configuration objects and do not receive an invented health status.

Details link to related Applications in the same namespace. Counts cover that namespace only, including when the table itself uses All namespaces. Applications in other namespaces are not included. ApplicationSets match owner identities; Projects match the declared project name. If Applications cannot be read, their count stays **Not reported** and details explain why.

## Limits

These views read Kubernetes only. API permissions and installed controller versions determine which information is available. The controller timestamp and the time Plous read the object are separate; a recent read does not prove that a controller is running.

Use the existing Kubernetes views for descendant Pods, events, and logs. These Monitor views do not fetch repository contents or provide sync/reconcile controls, suspension controls, rollback, desired/live comparisons, deployment history, or their own Activity history.

## Practice

Use the [GitOps playground](../gitops/playground.md) for disposable controller examples.

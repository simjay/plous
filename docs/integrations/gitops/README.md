# Argo and Flux

[Documentation](../../README.md) · [All integrations](../README.md) · [Connect Kubernetes](../kubernetes/README.md#connect-a-cluster)

Argo and Flux show what their controllers report in Kubernetes. They appear beside **Kubernetes** and **Helm** under the selected Monitor connection in Current Workspace or a saved workspace when their APIs are discovered. No separate controller account, CLI, Git connection, or Code source is needed.

Start with [Argo Applications](#argo-applications), [ApplicationSets and Projects](#argo-applicationsets-and-projects), [Flux Kustomizations and HelmReleases](#flux-kustomizations-and-helmreleases), or [Flux Sources](#flux-sources). These are read-only controller views; use [Code](../../guides/code.md) to inspect their source declarations.

For real controller examples on a disposable Minikube cluster, use the [Argo and Flux playground](playground.md).

## Open the view

1. Expand a cluster connection under **Monitor**.
2. Expand **Argo** or **Flux**, select a category, and press **Enter**. Opening the tool itself restores its last category.
3. Select a row to read its compact preview, then press **Enter** for the full controller message and details.
4. Select a managed resource, source object, or dependency and press **Enter** to investigate it. **Esc** returns to the previous view.

| Argo | Flux |
| --- | --- |
| Applications | Kustomizations |
| ApplicationSets | HelmReleases |
| Projects | Sources |

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

## Flux Kustomizations and HelmReleases

**Kustomizations** and **HelmReleases** have separate tables. Kustomizations show their last applied revision. HelmReleases show the declared chart name and latest reported release chart version; an attempted version does not stand in for an installed version. A Flux Kustomization is a live controller object; rendering a source `kustomization.yaml` remains in Code.

**Ready** reports successful reconciliation, not continuous workload health. **Not ready** shows the controller's failure. **Suspended** indicates that reconciliation is paused and may retain an earlier Ready condition. **Awaiting status** means the available condition does not describe the current object generation.

Details show the controller message, condition-change time, source reference, dependencies, and managed-resource inventory when reported. A failed dependency can be opened directly. Source links open the Kubernetes source object's status and declared metadata; they never fetch a repository or download its artifact.

Last-applied and last-attempted revisions remain separate. A HelmRelease chart version is labeled separately from Git revisions. A source fetch failure can coexist with an earlier successful deployment. Missing inventory means the controller did not report it, not that the deployment has no resources.

## Flux Sources

**Sources** lists discovered GitRepository, OCIRepository, HelmRepository, HelmChart, and Bucket objects. Its table shows the kind, reconciliation status, and fetched artifact revision where reported. A source kind that is not installed is omitted; a denied read or incomplete discovery remains visible as a coverage limitation.

Source details retain full revision values and declared metadata. These reads use the selected Kubernetes connection, with no repository checkout or artifact download.

## Limits

These views read Kubernetes only. API permissions and installed controller versions determine which information is available. The controller timestamp and the time Plous read the object are separate; a recent read does not prove that a controller is running.

Use the existing Kubernetes views for descendant Pods, events, and logs. These Monitor views do not fetch repository contents or provide sync/reconcile controls, suspension controls, rollback, desired/live comparisons, deployment history, or their own Activity history. Code source browsing remains separate.

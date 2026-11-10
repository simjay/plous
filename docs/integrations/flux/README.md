# Flux

[Documentation](../../README.md) · [All integrations](../README.md)

Read Flux Kustomizations, HelmReleases, and Sources through the selected Kubernetes connection. You do not need the Flux CLI or a Git connection.

## Open the view

1. Expand a cluster under **Monitor**.
2. Expand **Flux**, choose a category, and press Enter.
3. Select an object and press Enter for details.
4. Follow a dependency, source, or resource link with Enter. Esc returns.

Use `n` for namespaces, `/` to filter, Backspace to clear the filter, and `g` to refresh. Lists start with all namespaces and refresh every 15 seconds while visible.

The namespace picker filters controller objects. Their workloads can live in other namespaces.

If Flux is missing after installation, select the cluster and press `g` to rediscover APIs. Failed refreshes retain old rows labeled **Previous observations**.

## Flux Kustomizations and HelmReleases

| Status | Meaning |
| --- | --- |
| Ready | The controller reports successful reconciliation |
| Not ready | The controller reports a failure |
| Suspended | Reconciliation is paused. An earlier Ready condition can remain. |
| Awaiting status | Available status does not describe the current object generation |

Kustomizations show their last applied revision. HelmReleases show the declared chart and reported release version. Attempted and installed versions remain separate.

Details include messages, condition times, sources, dependencies, and managed resources when reported. Missing inventory does not mean there are no resources.

Ready does not guarantee continuing workload health. A source fetch can fail after an earlier successful deployment.

## Flux Sources

Sources lists discovered GitRepository, OCIRepository, HelmRepository, HelmChart, and Bucket objects. The table shows kind, status, and reported artifact revision.

Absent API kinds are omitted. Denied or incomplete reads are reported.

Source links open Kubernetes object status and metadata. They do not check out repositories or download artifacts.

## Limits

These views are read-only. They do not reconcile, suspend, roll back, fetch repositories, compare desired/live state, or provide their own Activity history.

A recent read does not prove that a controller is running. Use Kubernetes details, events, and logs to inspect the workloads themselves.

See the [keymap](../../reference/keymap.md#argo-and-flux) for controls.

## Practice

Try the [GitOps playground](../gitops/playground.md) in a disposable cluster.

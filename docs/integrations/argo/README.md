# Argo CD

[Documentation](../../README.md) · [All integrations](../README.md)

Read Argo CD Applications, ApplicationSets, and Projects through the selected Kubernetes connection. You do not need a separate Argo account, CLI, or Git connection.

## Open the view

1. Expand a cluster under **Monitor**.
2. Expand **Argo**, choose a category, and press Enter.
3. Select an object and press Enter for details.
4. Follow a resource link with Enter. Esc returns.

Use `n` for namespaces, `/` to filter, Backspace to clear the filter, and `g` to refresh. Lists start with all namespaces and refresh every 15 seconds while visible.

If Argo is missing after controller installation, select the cluster and press `g` to rediscover its APIs. If a read fails, old rows are labeled **Previous observations**.

## Argo Applications

The list shows Sync, Health, and target namespace. Sync and Health answer different questions:

| Status | Meaning |
| --- | --- |
| Synced and Degraded | Configuration matches, but Argo reports a health problem |
| OutOfSync and Healthy | Configuration differs, while Argo reports healthy resources |
| Not reported | The object did not supply that status |

Details show controller messages, reconciliation times, and reported managed resources. Expand **Configuration & revisions** for destination and revision metadata.

Managed-resource links use Kubernetes details. External destinations need a saved context with a matching API endpoint. An Argo destination name alone is insufficient, and Plous does not read Argo's stored destination credentials.

## Argo ApplicationSets and Projects

ApplicationSets shows generators, controller conditions, and related Application counts. Projects shows repository and destination rule counts, without inventing a health status.

Related-Application counts cover the object's namespace. If Applications cannot be read, the count stays **Not reported**.

## Limits

These are read-only controller views. They do not fetch repositories, sync applications, suspend controllers, roll back, or provide their own Activity history.

A recent API read does not prove the controller is running. Inspect descendant Pods, events, and logs through Kubernetes when checking workload health.

See the [keymap](../../reference/keymap.md#argo-and-flux) for controls.

## Practice

Try the [GitOps playground](../gitops/playground.md) in a disposable cluster.

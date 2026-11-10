# Cluster Overview

[Documentation](../README.md) · [Overview controls](../reference/keymap.md#cluster-overview)

Overview shows cluster health, resource usage, warnings, and inventory. Select a cluster connection and press `11` or `:1` to open it.

It covers the selected cluster across all namespaces. Selecting a workspace instead shows its saved connections.

## Take a first look

| Section | Look for | Open with Enter |
| --- | --- | --- |
| Cluster Pulse | CPU, memory, Node health, Pods, restarts, autoscaling | A chart or resource list |
| Recent Deployments | Reported rollout and release changes | The change record |
| Needs Attention | Failures and warnings | The resource or issue group |
| Infrastructure Snapshot | Kubernetes and detected integration inventory | Matching objects |

For an investigation, start with **Needs Attention**. Open a resource, then its Events or logs.

## Cluster Pulse

CPU and memory compare measured usage with Node allocatable capacity. Requests and limits are shown separately because they describe resource budgets, not consumption.

Press Enter on CPU or Memory for a larger chart. Enter again opens **Source details** with API sources, timestamps, and calculation notes. Esc returns through the chart to Overview.

Charts keep up to 15 minutes of samples. Gaps longer than 30 seconds remain visible. Press `e` to expand Pulse, `/` to find text, or `ff` for more room.

| Label | Meaning |
| --- | --- |
| Unavailable | Metrics are missing, incomplete, or do not match the observed Nodes |
| STALE | The source measurements have expired |
| Requests / limits | Declared budgets, including active unscheduled Pods |
| Node pressure | Nodes reporting pressure conditions |
| Pod phases | Reported phases, including completed Pods |
| Restarts | Cumulative restart counts |
| HPA direction | Desired replicas compared with current replicas |

Metrics require access to `metrics.k8s.io`. Missing measurements are not zero. Plous does not provide network throughput without a telemetry source.

### Understand change indicators

Restart and replica-change indicators compare reads of the same resource identities. Missing data, replacement resources, counter resets, or gaps over 45 seconds prevent a comparison.

An absent indicator does not confirm that nothing changed.

## Recent Deployments

This feed shows reported Deployment rollouts, Helm revisions, Argo CD sync operations, and Flux condition changes. It uses their reported timestamps.

The current cluster session keeps up to 200 distinct records. **History** marks older observations. Enter opens the saved observation rather than claiming it is current.

For saved workspace changes and recorded Helm operations, use [Activity](activity.md). Neither view is a complete cluster audit log.

## Needs Attention

Errors use **×** and warnings use **!**. Rows are ordered by severity and reported change time. Pods with the same controller and cause can form a group. Enter opens its members.

**NEW** means an error appeared between comparable complete reads within 45 seconds. It does not identify when the incident began.

An empty list means no warnings were found in the data Plous could read. Check availability before treating that as a healthy cluster.

## Infrastructure Snapshot

Select a tool card, choose a counter with Up / Down, and press Enter for matching objects. Esc restores the card.

The **Availability** summary explains missing APIs, denied access, unavailable metrics, and incomplete results. `?` means unknown. `+` and **Partial** mark incomplete counts.

Argo CD Applications stored on another management cluster are outside this cluster view.

## Navigate and expand

Arrows follow the displayed layout. Use `/` to filter lower lists, `e` to expand a section, and `ff` to expand main. Alt+Left / Alt+Right changes panels.

Selecting an Overview resource preserves the filters and namespace saved in Explore. Esc returns to the selected Overview item.

## Refresh, freshness, and retention

| Data | Automatic refresh | Retention |
| --- | --- | --- |
| CPU and memory | Every 5 seconds | Metrics expire after 90 seconds. Up to 15 minutes of samples in memory. |
| Resource and integration inventory | Every 15 seconds | Marked STALE after 45 seconds |
| Recent changes | With inventory reads | Up to 200 records in this cluster session |

Press `g` to refresh explicitly. Reads have a 10-second deadline. A repeated fetch may contain the same measurement.

Overview does not collect metrics after you leave or save historical charts. An assistant can save a separate read as [Sources](ai-chat.md#inspect-a-replys-sources).

## Next steps

Try [Investigate a restarting Pod](investigate-restarting-pod.md) or read the [Kubernetes guide](../integrations/kubernetes/README.md).

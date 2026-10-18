# Cluster Overview

[Documentation](../README.md) · [Kubernetes](../integrations/kubernetes/README.md) · [Overview keymap](../reference/keymap.md#cluster-overview)

Overview is your starting point for a cluster investigation. See resource health and usage, find what needs attention, and follow an observation into its supporting details.

Plous opens Overview for the terminal's current Kubernetes context at launch. To return, select a Monitor connection and press **11** or `:1`. It summarizes that one captured cluster across all namespaces. Selecting a workspace or Code source instead shows its connection summary.

## Take a first look

| Section | What to look for | Follow up with |
| --- | --- | --- |
| **Cluster Pulse** | CPU and memory usage, Node health, Pod phases, restarts, and autoscaling | Click or Enter for a focused metric chart or the corresponding resource list |
| **Recent Deployments** | Reported rollout, release, and controller changes | Enter to inspect the selected record |
| **Needs Attention** | Failures, unready resources, and controller warnings | Enter for the resource or the members of an issue group |
| **Infrastructure Snapshot** | Kubernetes inventory and detected Helm, Argo CD, and Flux objects | Select a counter and Enter to see matching objects |

For a first investigation, choose a **Needs Attention** row, inspect the resource, and open its Events or logs where available. Esc returns through the local views to Overview. A count or warning always needs its coverage: inaccessible objects cannot establish a healthy cluster.

## Cluster Pulse

CPU and memory show measured usage against Node allocatable capacity. Declared Pod requests and limits appear separately. Requests and limits include active unscheduled Pods, so they describe specifications rather than actual consumption. Containers without limits mean declared limits are not a finite cluster ceiling.

The history charts show actual observations with independent labeled scales. A chart's display maximum is not cluster capacity. The time window starts at one minute and grows with collected history, up to 15 minutes. Consecutive observations form a filled step chart; gaps longer than 30 seconds remain gaps. Repeated fetches of the same source measurement do not invent samples, and the chart does not extend before or after the observations.

Use **e** to expand Pulse. Compact tiles use `c` for CPU cores and `Gi` for GiB. Click or press Enter on CPU or Memory to open that metric's larger chart, aligned usage and budget values, coverage, and relevant warnings. Sampling continues while the metric view is open. Enter opens its **Source details**, with API sources, exact timestamps, and calculation notes. Esc returns to the chart, then to the selected Overview tile. Use the arrows or page keys to scroll, **/** to find text, and **ff** for more room.

Nodes, Pods, and Autoscaling open their corresponding resource lists across all namespaces in the selected cluster. Esc returns to the same Overview tile. These temporary lists preserve your saved Explore filters and namespace. Read the status labels as follows:

| Indicator | Meaning |
| --- | --- |
| **Unavailable** usage | Node metrics are missing, incomplete, or do not match the observed Node identities |
| **STALE** usage | The retained source measurements have expired |
| Requests or limits | Declared resource budgets, shown separately from measured consumption |
| Node pressure | Count of affected Nodes; absent conditions remain unknown |
| Pod phases | Observed inventory, including completed Pods |
| Restarts | Cumulative restart counts for the observed Pods |
| HPA direction | Desired replicas compared with current replicas; last-scale time comes from controller status |

Usage needs readable Node metrics from `metrics.k8s.io`. Network throughput is unavailable without a telemetry provider. See [resource-level charts](../integrations/kubernetes/README.md#usage-and-charts) for Pod, Node, workload, and quota inspection.

### Understand change indicators

A positive restart increase requires two complete reads of the same Pod UIDs and named containers, no counter decrease, and an interval of at most 45 seconds. HPA replica changes likewise require matching HPA identities. Both show their observation interval and retain both read times in evidence.

Replacement resources, missing statuses, failed or partial reads, and longer gaps suppress these comparisons. A missing indicator does not establish zero change.

## Recent Deployments

This feed includes timestamped Deployment rollout conditions, Helm storage revisions, Argo sync operations, and Flux condition transitions. They retain the meaning of their source timestamps. Objects without a reported change time do not receive an invented one.

The current cluster session retains up to 200 distinct records, including previous revisions and records no longer present in the latest read. Repeated polling does not duplicate them. Older records show **History**; Enter opens their captured evidence labeled as a previous observation.

This is a recent-observation feed, not a complete deployment audit log or Git push feed. For saved workspace revisions and recorded Helm outcomes, use [Activity](activity.md).

## Needs Attention

Rows lead with the cause: **×** for an error and **!** for a warning. Failures and warnings are ordered by severity, then newest reported change time. Multiple conditions on one resource share a row. Pods with the same controller identity and cause form a group; Enter opens the members, each with its own inspector.

The header distinguishes affected-resource counts from issue groups. **NEW** means an error first appeared between complete reads of the same integration sources within 45 seconds. It does not identify when an incident began; the displayed age remains the source's reported age.

An empty list means no warnings were found in the readable results. It says nothing about resources the connection could not read.

## Infrastructure Snapshot

Snapshot shows Kubernetes inventory alongside detected, readable Helm, Argo CD, and Flux inventory. Select a card, use Up/Down to select one of its counters, and Enter to open the corresponding resource table or list of observed objects. Status counters match reported status fields. Esc restores the card and its filter.

The **Availability** summary distinguishes missing APIs, unavailable telemetry, denied access, and incomplete results. A missing count shows `?`; incomplete controller counts show `+` and **Partial**. Unknown counters remain unavailable when opened. Enter on Availability shows the full explanation.

Argo Applications on a separate management cluster are outside the selected cluster's view. Remote-cluster references do not expand its scope.

## Navigate and expand

Arrows follow the visible layout. Left/Right moves between Pulse tiles or lower columns. Up/Down or j/k moves through rows and connects sections at their edges. Navigation stops at the outer edges; each section keeps its reading position when you leave it.

Use **/** to filter the lower lists, **e** or `:e` to expand the selected section, and **ff** or `:f` to expand main. Large main panels show three lower columns; narrower layouts stack sections or show one selected section. Expanding the view gives content more room without changing saved panel visibility. **Alt+Left/Right** changes panel focus without changing the selected observation.

In narrow sections, workload names and warning causes wrap, with source and age on separate lines. The outer main frame replaces the extra section frame to leave more room for the content. Fullscreen gives long entries more space.

## Refresh, freshness, and retention

| Data | Refresh while Overview is open | Freshness and retention |
| --- | --- | --- |
| CPU and memory | Every 5 seconds, independently of the other inventories | Metrics expire after 90 seconds; up to 15 minutes of real samples in memory |
| Workload and integration inventories | Every 15 seconds | Inventory older than 45 seconds is marked **STALE** |
| Recent change records | Updated from the observed inventories | Up to 200 distinct records for the current cluster session |

CPU and memory start after Kubernetes discovery, without waiting for Helm or controller inventory. Reads use a 10-second deadline; new plotted values still depend on the metrics provider publishing a new observation.

Use **g** or `:g` to refresh discovery and observations explicitly. Failed reads and acquisition limits stay visible. Refresh and resize preserve selection where the resource still exists. Inspecting a resource keeps saved Explore filters and namespaces intact; Esc can cancel a pending inspection.

Observations, chart samples, and section positions stay in memory. Overview does not collect background telemetry after you leave or save historical metrics. An assistant's explicit read can save a separate observation as [Sources](ai-chat.md#inspect-a-replys-sources).

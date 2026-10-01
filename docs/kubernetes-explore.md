# Explore Kubernetes

[Documentation](README.md) · [Kubernetes actions](kubernetes-actions.md) · [Troubleshooting](troubleshooting.md)

Use a Kubernetes connection in Explore to find resources, inspect their state, follow relationships, and read logs. This guide assumes you have [added a Kubernetes context](kubernetes.md#add-a-kubernetes-context).

## Know what you are looking at

| Term | Meaning in Explore |
| --- | --- |
| Context | The saved Kubernetes connection currently being used. Check it in the header. |
| Namespace | A named scope within a cluster. Namespaced resources are listed within this scope. |
| Resource type, or kind | A category of Kubernetes object, such as Pods or Deployments. |
| Object | An individual resource, such as a Pod named `checkout-abc123`. |
| Cluster-scoped | A resource type, such as Nodes, that is not limited to a namespace. |

For example, **Workloads / Pods** is a resource type. The rows below it are individual Pods. Searching those rows does not switch to Deployments or another type.

## Open a connection

Move to a Kubernetes connection in the sidebar and press **Enter** to select it, then **22** for Explore. Use **Right** to expand its branch if needed. Right only expands the tree; it does not select a connection or switch views.

To collapse a connection such as minikube, move to its row and press **Enter** or **Left**. Its resource categories disappear from the sidebar. The same fold controls apply to its workspace, the Kubernetes tool group, and resource categories. Use Left/Right to fold or expand without selecting another scope. Refreshing and restarting preserve deliberate folds.

You can instead type `:pods` and press **Enter** from any navigation panel. If no Kubernetes connection is selected, choose one when prompted. Use `:K` to choose another saved connection.

The header shows the current context, namespace, resource type, and connection state. Explore uses the connection's kubeconfig and may run its credential plugin. It does not change your shell's current context or rewrite kubeconfig.

## When Kubernetes requests run

Saved connections are configuration references. Plous does not keep a watch running for every saved context or every expanded branch.

**Explore** runs live browsing requests for its selected Kubernetes connection. Its current resource table uses a watch where available, or refreshes every five seconds when watching is unavailable. Detail views and logs make additional requests as needed.

Switching connections cancels the previous connection's discovery, watch, detail, and log work. Switching to **Overview** or **Activity**, selecting a workspace or tool scope, or quitting also stops that work. Returning to Explore starts it again. Settings, Help, AI focus, and sidebar folding leave the current Explore session running.

There is no separate disconnect command. Use **11** for Overview or **33** for Activity to stop live browsing while keeping the connection selected.

## Find a resource type

Choose whichever route fits the task:

- **Browse:** expand the connection, expand a category such as **Workloads**, then select **Pods** and press Enter.
- **Search the sidebar:** focus it, press **/**, type `deployments`, select a result, and press Enter. Search includes collapsed branches in loaded catalogs.
- **Jump directly:** run `:pods`, `:deployments`, or `:services`.
- **Use the optional picker:** run `:k`, including when the sidebar is hidden. Type to search, use Up/Down to select, and Enter to open. Left/Right changes categories; Esc cancels.

Common types appear first in each category. Categories group related resources, such as Workloads, Network, Storage, and Config. Only categories containing discovered resources appear. Installed custom resources also appear, with server-provided columns when available.

See [resource commands](keymap.md#resource-commands) for more names and aliases. The `o` key in Pod details is **Choose container**; it does not change the table's resource type.

## Choose a namespace

Press **n** to open the namespace picker. Type to search, use arrows to select, and press Enter to apply. **All namespaces** lists the selected type across the namespaces your request can access.

For a direct selection, run `:N shop`. Replace `shop` with your namespace. `:N *` selects all namespaces; `:N all` selects a namespace literally named `all`. You can also choose the type and namespace together, such as `:pods shop`.

If namespace listing is denied, use **Enter namespace manually**. Being unable to list namespaces does not necessarily mean you cannot read resources inside a known namespace. An all-namespace request can also be denied even when one namespace is accessible.

The scope is remembered for the saved connection. It does not change the connection's configured default namespace. Nodes and other cluster-scoped types are labeled **cluster-scoped** and ignore namespace filtering.

## Filter and sort a table

1. Press **/** and type a name or other text shown in the rows.
2. Press **Enter** to apply the filter.
3. Move through matching rows with arrows.
4. Press **Backspace** while navigating to clear the filter. The bar shows **Backspace clear** when it is available.

Filtering is case-insensitive and searches the loaded table data. **Esc** cancels an unfinished edit. It preserves a filter you have already applied. Backspace inside the input field deletes one character normally.

For Kubernetes labels, press **f** and enter a selector such as `app=checkout`. A label selector limits the Kubernetes request; the text filter searches the returned rows. Clearing the text filter does not clear the selector. Reopen **f**, erase the selector, and press Enter to remove it.

Press uppercase **S** to choose a sort column. Selecting the same column reverses its order. Clearing a text filter preserves sorting, namespace, labels, and the selected resource.

Tables adapt to the available width. Use **bb** or **ii** to hide a side panel if you need more room. Left/Right does not scroll table columns.

## Inspect an object

Select a row and press **Enter**. Details have four sections:

| Section | Key | What to look for |
| --- | --- | --- |
| Describe | `d` | Resource-specific state, conditions, configuration, and metadata. |
| YAML | `y` | The object's manifest. Managed fields are omitted. |
| Events | `e` | Kubernetes events for this object, such as scheduling or startup failures. |
| Related | `r` | Owners, dependent resources, and referenced objects. |

All four sections keep the context, namespace, resource type, object name, and section controls visible. **d** stands for **Describe**. Previously loaded sections stay visible during a background refresh; the refresh indicator does not shift the content.

Use **[ / ]** to move between sections. Related groups selectable rows under Owner, Managed resources, Referenced resources, and Selector matches. Selector matches are explicitly distinct from ownership. Permission and API gaps appear with the results. Select a row and press Enter to follow it. Esc returns to the previous resource, then to the originating list. Filters and selection are retained.

Pod summaries show container state, readiness, restart counts, failures, and previous exits. Workloads put reported condition reasons and messages before replica counts and template configuration, so rollout problems are visible early. The full Conditions section retains transition times. Other types have their own details, such as Service ports, storage bindings, and access rules. Endpoint readiness reflects reported state, not a network traffic test.

The **YAML** tab uses Plous Editor in read-only mode with syntax colors and line numbers. Use arrows or page keys to move, **/** to search, **n/N** for matches, **z** to select text, and **y** to copy the selection or current line. Copies contain the source text without line numbers or visual wrapping. Ordinary typing and paste do not edit the document; paste is accepted only in its search field. **E Edit** explicitly starts the editing workflow.

Esc clears a local search or selection first, then returns to the detail section that opened YAML with its scroll position preserved. YAML opened directly from a list returns to that list. Inside YAML, the tab's hint changes to `:y` because `y` copies text. Refreshing changed content clears search and selection while keeping the reading position where possible. Resizing or moving focus preserves both.

Secret YAML starts redacted. **v** reveals or hides values in that view; **z** selects text. Changing visibility clears search and selection so copying uses the currently visible version. This display choice is not saved. [Data and privacy](data-and-privacy.md#kubernetes-data) explains what stays local and what can be exported.

Run `:g` to reload the object and section. Missing permissions or APIs can limit details and relationships; the view reports those gaps.

## Read container logs

Read container logs in the [Logs guide](logs.md), including previous runs, merged workload logs, search, and reading without losing your place.

## Example: investigate a restarting Pod

Use this sequence with a Pod you are authorized to inspect:

1. Run `:pods` and confirm the context and namespace in the header.
2. Press **/**, enter part of its name, and press Enter.
3. Open the Pod with Enter. In **Describe**, inspect readiness, restarts, and container failure details.
4. Press **e** to read Events. If events are absent, Kubernetes may no longer retain them.
5. Press **l** for logs. Use **o** if the failing container is not selected. Press **p** to try the previous run's logs.
6. Press Esc to return to the Pod detail, then **r** to inspect its owner or referenced configuration.
7. Return to the table with Esc and press **Backspace** to remove the text filter.

This workflow inspects the resource without changing it. To make a change, follow [Kubernetes actions](kubernetes-actions.md).

## Understand loading and availability

A dot sweeps back and forth beside the loading message while a request is pending. The message identifies the operation, and the animation stops when it finishes or fails. The same indicator is used for AI responses.

**Loading** means a list request has not finished. **No Pods in namespace …** is a completed empty result. **No resources match this filter** means the text filter or label selector excluded the rows. A connection or authorization error is shown separately.

Lists update through Kubernetes watches. When watching is denied, Plous refreshes the list periodically. A stale result remains visible with its error. Press **g** or run `:g` from the table to reconnect and rediscover available APIs.

A failed API group does not hide healthy groups. Resources served only by a non-preferred version remain discoverable. Access still depends on the cluster's APIs and your credentials.

## Current limits

Lists hold at most 10,000 objects per scope. Narrow the namespace or label selector for larger collections. Sorting and text filtering apply to the loaded collection.

Live usage metrics, historical charts, attach, and port forwarding are not available. Resource browsing state is saved per connection; object data, events, and logs are not saved as a browsing history or automatically sent to AI.

Next: [Kubernetes actions](kubernetes-actions.md) or [Troubleshooting](troubleshooting.md).

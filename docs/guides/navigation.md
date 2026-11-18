# Explore clusters in Plous

The sidebar selects the cluster, the main panel shows its resources, and the right panel holds the optional [assistant](ai-chat.md). The blue border shows which panel receives your keys.

## Workspaces and connections

**Current Workspace** opens the current context in your terminal kubeconfig. Enter `:K` to choose another context in Plous. Selecting one leaves the kubeconfig's active context as it was.

- **Save the current cluster:** Open **Settings → Save as Workspace**. Existing Current Workspace chat stays there.
- **Group several clusters:** Open **Workspaces → + New Workspace** (or `:n`), name the workspace, then select **Monitor → + Add Context** to add contexts from your kubeconfig.
- **Switch or maintain connections:** Select the workspace or connection in the sidebar. Use **Settings → Current Workspace** to rename, edit, or remove entries. Each connection keeps its settings, namespace, and chat. Removing a connection leaves its cluster resources alone.

Use Right and Left to expand and collapse the tree. Select a cluster to discover its tools and press `g` to retry discovery. See [integrations](../integrations/README.md) for what each tool supports.

## Choose a main view

| View | Shortcut | Use it to |
| --- | --- | --- |
| [Overview](#read-overview) | `11` or `:1` | Check health and find warnings across the cluster |
| [Explore](../integrations/kubernetes/README.md#explore-resources) | `22` or `:2` | Browse resource lists, details, events, logs, and YAML |
| [Activity](#review-activity) | `33` or `:3` | Review saved workspace changes, Helm history, and alerts |

Press both digits in sequence. In Explore, `:pods` opens the Pod list and `n` chooses its namespace.

## Read Overview

Start with **Needs Attention**. Enter opens a warning or a group of affected resources. **NEW** marks an error that appeared between comparable reads. Check **Availability** before interpreting an empty list, unknown count, or partial result.

| Section | What it shows |
| --- | --- |
| Cluster Pulse | CPU and memory usage against allocatable capacity, Node health, Pods, and autoscaling. Enter opens a chart, then its source details. |
| Recent Deployments | Observed Kubernetes rollouts, Helm revisions, Argo CD syncs, and Flux status changes |
| Infrastructure Snapshot | Kubernetes and detected integration inventory. Enter opens objects behind a count. |

CPU and memory require access to `metrics.k8s.io`. Pulse refreshes every 5 seconds and keeps up to 15 minutes of samples in memory. Inventory refreshes every 15 seconds. Recent Deployments keeps up to 200 observations in the cluster session. Use `g` to refresh and `e` to expand a section. See [usage and charts](../integrations/kubernetes/README.md#usage-and-charts) for resource-level metrics.

## Review Activity

Use Left and Right to choose **Workspace changes**, **Plous actions**, or **Observed Helm history**. Press `/` to filter and Enter for details.

- **Workspace changes:** Use `d` for a read-only Before / After comparison. To recover saved settings, see [profile backups](../reference/data-and-privacy.md#back-up-or-recover-a-profile).
- **Plous actions:** Check the Helm operation's target and outcome. Check the current release before retrying a failed or unconfirmed operation.
- **Observed Helm history:** Use `O` for original observations. A saved revision describes what Plous saw at the time. Activity persists in your profile.

## Monitor cluster health

Start a monitor in a separate terminal. This example checks a Deployment and Service in `prod/payments` every two minutes.

```sh
plous monitor start payments --context prod --namespace payments \
  --deployment checkout --service checkout-api --every 2m
```

Open Monitors with `:4` or the header's alert count. Select a monitor and press Enter to read its alerts. The list shows running monitors across contexts, their last complete check, and the next scheduled check. **Overdue** marks a missed check deadline. A monitor keeps running when you exit the TUI.

From another terminal, use `plous monitor list` to see saved monitors and `plous monitor status payments` for details. In Plous, `:j` opens **Activity → Alerts** for all saved monitors. Press `/` to filter and Enter to read an alert.

Stop a monitor before removing its saved data. Removal asks for confirmation.

```sh
plous monitor stop payments
plous monitor remove payments
```

To resume a stopped monitor, run `plous monitor start payments`. Add `--every 10m` to change its interval. To reuse its name for a different scope, start it with the new context, namespace, and targets, then confirm the replacement. See [options and alert rules](../reference/configuration.md#monitors), [keyboard controls](../reference/keymap.md#commands), and [saved monitor data](../reference/data-and-privacy.md#monitor-data).

## Move, search, and go back

- **Move:** Alt+Left and Alt+Right cycle visible panels. Tab switches the sidebar and main during navigation. Ctrl+] enters or leaves the assistant.
- **Find:** Press `/` in the focused panel. In a resource table, Enter applies the text filter and Backspace clears it while navigating. In details or logs, Enter starts reviewing matches. Use `n` and `N` to move between them.
- **Go back:** Esc closes the current input, search, or detail. Use `ff` to expand or restore the main panel.
- **Get help:** Press `hh` for the current view's controls, or open the [keymap](../reference/keymap.md).

Practice with [a restarting Pod](investigate-restarting-pod.md).

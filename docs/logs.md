# Read container logs

[Documentation](README.md) · [Kubernetes actions](kubernetes-actions.md)

Open a Pod or workload, choose a container, and follow its output. You can read older lines while new logs continue arriving.

## Open the view

Open a Pod or supported workload's details, then press **l** or run `:l`.

For a Deployment, StatefulSet, DaemonSet, ReplicaSet, ReplicationController, Job, or CronJob, the picker starts on **All Pods**. Press Enter to merge application-container logs from its owned Pods, or select an individual Pod. All Pods follows controller ownership, including intermediate ReplicaSets and Jobs. Label matches alone do not qualify.

Each merged line starts with `[pod/container]`. The prefix has a stable color per Pod, shared by its containers. Wrapped messages align below the message; narrow views use a small continuation indent to leave room for content. Full source names remain searchable and copy as plain text. Lines appear in arrival order, so older history from another source can arrive later.

In merged Logs, **o** filters by container name across Pods. The default includes application containers; choose an init or ephemeral container by name when needed. Current logs check Pod membership every three seconds and add or remove sources as the workload changes. A restarted container starts a new source. A failed source leaves the other streams running; use **g** to retry failures. Previous-instance logs use the Pods found when the request starts.

The header reports Pod and source counts, live and ended streams, failures, and waiting containers. At most 32 sources are selected in Pod-name order. If there are more, a **Partial coverage** notice shows the omitted count. Choose a Pod or container to narrow the scope.

In Pod Describe or Logs, **o Choose container** selects a regular, init, or ephemeral container. Pod Logs also offers **All application containers** to merge that Pod's regular containers. The Logs action's container picker offers the same option for Pods with multiple containers. Init and ephemeral containers remain explicit choices. On first opening a Pod, Plous prioritizes a failing or unready container when one exists.

## Controls

| Key in Logs | Action |
| --- | --- |
| `o` | Choose a container or All application containers. |
| `p` | Previous instance: show logs from the selected container before its last restart. |
| `u` | Time range: request the latest 200 lines or a recent time window. |
| `t` | Show or hide timestamps. |
| `w` | Wrap lines: split long lines across screen rows. |
| `z` | Expand or restore Logs immediately, without a colon or Enter. |
| Space | Hold your reading position, or jump to the latest output. |
| `g` | Reload logs, clearing and fetching the selected history again. |
| `v` / `y` | Select text / copy selection or current line using terminal clipboard support. |
| `n` / `N` | Next / previous search match. |
| `/` | Find text in the loaded buffer and pause following. |
| Esc | Close search first, or return from Logs to the detail that opened it. |

The header identifies the Pod or workload, namespace, container selection, current or previous run, and time range. **Live** describes the connection; **Following** or **Reading** describes your position. Controls sit above the output. Compact panels prioritize **z Expand** and leave more room for log lines; **:h** or **hh** opens the complete controls in Help.

Scrolling, Space, or search holds your position while new logs continue arriving. A counter shows new lines. After closing search, **Space** jumps to the latest retained output without restarting the request.

## Retained history and reconnecting

Each source starts with up to 200 lines. The combined logs retain up to 128 KiB or 5,000 complete lines in memory. The view reports when older lines are removed; retained lines stay in place while you read. Search only examines this buffer. Changing container, source, or time range starts a new buffer; **g** reconnects explicitly. Previous-run logs finish when Kubernetes has returned them. Kubernetes may no longer have logs from an earlier container run.

If a request fails, the retained output includes recovery steps. Press **End** to read the error, **p** to return from previous-instance logs to current logs, or **g** to retry after correcting access or connection problems. Reloading clears the current buffer.

Esc from workload Logs returns directly to the originating workload detail. Changing containers or time ranges does not change that return point.

See [Keyboard shortcuts](keymap.md) for the complete key reference.

# Kubernetes

[Documentation](../../README.md) · [All integrations](../README.md)

Browse a selected cluster, inspect resources and run explicit actions from object details. Plous uses the Kubernetes API directly and does not change your external active context. You need a kubeconfig and permission for each operation. Configured authentication helpers must be installed separately.

Start with [resource browsing](#explore-resources), then open [logs](#read-logs), [usage charts](#usage-and-charts), or [YAML editing](#edit-and-review-yaml). [Resource actions](#resource-actions) and [container terminals](#container-terminals) describe the explicit changes you can make.

## Connect a cluster

Current Workspace opens the merged kubeconfig current-context at launch, using its namespace or default. It reads inherited KUBECONFIG paths or ~/.kube/config and contacts only the selected target. Use `:K` to choose another context. External changes show a notice; they do not switch an active view. Missing configuration offers Settings → Choose Kubeconfig and Refresh.

To keep explicit connections, use Save as Workspace, or add them to an existing workspace:

1. Choose **Monitor → + Add Context**, or **+ Add Connection → Kubernetes Cluster**.
2. Check the displayed kubeconfig path, initially `~/.kube/config`. Editing it requires **Refresh Contexts**.
3. Check contexts with Space or Enter. **Already added** rows identify existing connections.
4. Optionally use **Name** or Right to set a display name; blank uses the context name.
5. Choose **Add Selected Contexts**. Esc cancels without saving.

Setup reads the displayed file, independently of inherited KUBECONFIG. Changing/refreshing the path clears pending selections and names. Equivalent paths and symlinks count as the same file for duplicates. Adding a context reads local metadata without connecting or running authentication helpers. Opening its views connects through that context. Neither action edits kubeconfig.

## Explore resources

Type `:pods` in navigation, choose a connection if prompted, then Enter on a row. The header identifies the context and namespace. Use n to select a namespace or All namespaces; `*` means all, while `all` is a literal namespace name. Empty results can be valid; missing permission is reported separately.

Resource navigation lives under the connection in the sidebar. Search it with `/`, or use a supported resource command such as `:deployments`. API discovery determines available types; it does not prove that you can read them. Group-qualified resource identities disambiguate matching names.

In a table, `/` edits the text filter, L edits the label selector and S selects sorting. Applying a filter retains it; Esc cancels only an unfinished edit. Backspace clears the applied text filter without changing namespace, labels or sorting. Refresh keeps selection where possible. Columns adapt to available width; Help explains the current view.

Enter opens details. Describe summarizes observed status and configuration; YAML shows the captured object; Events and Related expose available supporting observations and links. Selecting a related object creates a return path. Esc restores the originating detail or table with its filters, selection and scroll.

## Resource support

Kubernetes covers core objects, workloads, networking, storage, configuration, security, policy, scheduling and discovered extension APIs. Detail sections/actions depend on resource kind and served subresources. A cluster can omit APIs or deny access. Unknown and incomplete data stay explicit.

Relationships distinguish ownership from selectors and references. A Service selector alone does not prove a Pod is its owner. Readiness is distinct from Pod phase or a controller's desired replica count. Usage graphs show observed samples and gaps; unavailable metrics are not zeros.

The assistant has bounded read-only resource, describe, usage and log tools with projected facts. User actions below are separate and require explicit intent. Bulk creation, node drain, Job/CronJob execution, volume resize/snapshot creation, attach and port forwarding are unavailable.

## Usage and charts

| View | Chart | What the values mean |
| --- | --- | --- |
| Pod, Node or supported workload Describe | CPU and memory over time | Observed measurements; workload totals require complete owned-Pod coverage |
| Pod chart inspection | Container shares and request/limit bars | Latest container measurements against the Describe snapshot's declared budgets |
| Node inspection and Describe | Usage and declared-request allocation | Measured consumption and Pod requests shown separately against allocatable capacity |
| Pod or Node table, then **I** | Top ten CPU and memory consumers | Valid latest measurements within the current list filters, ranked independently per metric |
| Workload Describe | Ready/current replicas | Controller-reported counts, with desired replicas labeled separately |
| ResourceQuota Describe | Used/hard quantities | Reported quota accounting, including object counts; configured limits stay separate |

### Inspect samples

Pod, Node, and supported workload details show CPU and memory history. **I** opens chart inspection, **Left / Right** selects a real sample, **W** switches between 5 and 15 minutes, and **P** follows latest. Click a plotted sample to inspect it. Selection stays on that observation as updates arrive; if it leaves the visible or retained window, Plous reports the change and follows latest. **ff** gives the charts the main screen. Scroll to reach further charts and source details. Esc closes inspection before leaving Describe or restoring fullscreen. Help and Settings preserve selection; `/` returns to Describe search. See the [chart keymap](../../reference/keymap.md#usage-charts).

### Sampling and terminal limits

Charts use thin terminal lines, rounded scales with headroom, and gaps for missing observations. Scale maxima are display bounds, not capacity or limits. CPU and memory have separate zero-based scales. The Metrics API is polled every 15 seconds while the relevant view is active. Up to 15 minutes stays in memory; there is no earlier or saved metric history. Display controls do not make extra requests.

These are character-cell charts: your terminal controls fonts, cell dimensions and glyph rendering. Plous has no pixel graphics, hover tooltips, drag-to-zoom, background collector or durable metric store.

Pod and Node points use their source timestamps. Workload points are immutable collection snapshots of their active Pods. The readout labels the collection time and the range of contributing source timestamps. Unchanged measurements do not add another point; freshness still follows the oldest source measurement.

The assistant's `kubernetes_usage_read` tool reads current Pod, Node or owned-workload measurements through the same identity, freshness and aggregation rules. It reports CPU in nanocores, memory in bytes, measurement times and coverage. It works without an open chart and saves that read as evidence. It cannot read unsaved chart history. Acquisition limits and missing permissions remain explicit; incomplete workload measurements do not become a partial numeric total.

### Budgets and coverage

Pod chart inspection includes the latest per-container CPU and memory shares and usage against each container's requests and limits. Unspecified or zero budgets have no percentage. Node charts compare measured usage with allocatable capacity; Describe also shows declared Pod requests against allocatable capacity, with its existing coverage notice. Reservations and measured consumption stay separate. These are current reported specifications, not runtime allocation accounting.

In Pod or Node lists, **I** shows the top ten CPU and memory consumers within the current filters. Bars are relative to the largest measured object. Each value has a source timestamp, and missing or stale measurements are excluded with a coverage count. Rankings do not collect per-object history. Enter does not open a resource from rankings; return to the list with **I** or **Esc** to choose one.

Workload Describe shows ready versus current replicas, with desired replicas separately labeled. A Deployment whose controller has reported zero replicas retains an explicit zero summary. ResourceQuota Describe shows reported used versus hard quantities, including object counts. Configured quota limits are shown separately when reported accounting is absent. Quota usage represents accounting, not live consumption. Missing values and zero denominators remain explicit.

## Resource actions

Open details, check context/namespace/name, then use a displayed action or **a More**. Availability depends on kind, API and permissions. Lists do not submit operations. The action bar shows the selected control; Enter activates that control. Confirmations initially select Back.

| Action | Detail key | Behavior |
| --- | --- | --- |
| Logs | l | Read a Pod or merge owned workload Pod logs |
| Exec | x | Run an explicitly selected command in a container |
| Scale | S | Change replicas through a served scale API |
| Restart | R | Request a Deployment/StatefulSet/DaemonSet rollout |
| Edit | E | Edit and validate/save captured YAML |
| Delete | D | Request graceful deletion of the identified object |
| Copy identity | Y | Copy context and object identity |
| Values | V | Inspect/copy supported values or snapshots |
| Export YAML | w | Create a new private local YAML file, without overwrite |
| Use namespace | u | Set browsing scope from a Namespace object |

Capitalization matters: d/e/r are detail sections; D/E/R are actions. See the [keymap](../../reference/keymap.md#kubernetes-explore) for full commands. Esc cancels unfinished input through its entry path. Leaving a submitted operation does not undo it.

Scale/restart progress checks the captured object every two seconds for up to five minutes. OnDelete and partitioned strategies have their own completion semantics. An HPA may adjust replicas afterward. Enter/Esc stops observation and returns to details; g checks again without resubmitting. Failure, replacement, changed desired state and timeout remain distinct.

Delete uses normal graceful Kubernetes behavior and does not bypass finalizers. It binds the original UID, so replacement under the same name is distinguishable. Completion means the original object disappeared; dependents may still terminate. Storage confirmations include available binding/reclaim/reference information or a gap notice.

## Edit and review YAML

1. Open details and press **E**. The Editor starts in Normal.
2. Press i to type/paste, then Esc to return to Normal.
3. Use `:w` to validate and save, or `:wq` to save and close.
4. Wait for **Saved to Kubernetes**. Server acceptance is separate from rollout/readiness.

`:v` optionally opens a diff, including server defaults. Saving there still requires `:w` or the explicit Save control. Enter while reading never saves. Esc returns with draft and undo intact. The save must match the exact draft, target and comparison identity.

Invalid YAML, rejected updates and conflicts keep the draft. **!** opens the full error. Resolve a conflict against the current object before saving again. **Save result unknown** means acceptance could not be confirmed; inspect the live object before retrying. Nothing retries automatically.

u/U undo/redo. `:q` closes unchanged text or offers Keep editing/Discard changes; `:q!` discards. An unchanged `:wq` closes without a request. `:quit` and Ctrl+C exit Plous without applying resource drafts. Tab indents in Insert; Alt+Left/Right change panels in every editor mode. Help and chat preserve in-memory edits. See [editor controls](../../reference/keymap.md#plous-editor-and-diff-reviewer).

The Editor accepts up to 2 MiB; undo keeps up to 100 snapshots within 16 MiB. Some resources permit metadata-only editing. Editing a Secret explicitly reveals values. Drafts, undo, comparisons and diagnostics stay in memory and are not saved in Plous sessions.

## Read logs

Press l from a Pod or supported workload. Workload **All Pods** follows controller ownership through intermediate ReplicaSets/Jobs, not labels alone. Pod **All application containers** merges its regular containers. Choose init/ephemeral containers explicitly with o. A failing/unready container is prioritized initially.

Merged lines identify pod/container with stable Pod colors and preserve arrival order. Current logs recheck membership every three seconds and start new sources for restarted containers. Failed sources leave others running. At most 32 sources are selected in Pod-name order; omitted sources produce a partial-coverage notice. Previous-instance logs use the initial membership snapshot.

Each source initially requests up to 200 lines. Use u for time range, p for previous instance, t for timestamps, w for wrapping and o for container selection. Scrolling, Space or `/` holds the reading position while collection continues; Space jumps live afterward. g reloads explicitly and clears the buffer. Esc returns to the original detail, preserving its section.

Logs retain at most 128 KiB or 5,000 complete lines and disclose eviction. Search examines that retained buffer only. Changing source/range starts a new buffer. Previous logs may be unavailable after Kubernetes removes them. End reaches retained errors; correct access or selection before retrying.

## Container terminals

Press x from details, choose a running container, select sh, bash or a custom command, then Start. Only that command runs; shells are not probed or retried automatically. Commands have the container's permissions and may change files or processes.

While **Terminal input active**, keys, Esc, Tab, colon and paste go to the remote process. **Alt+Left/Right** move panels directly and resume input on return. **Ctrl+]** releases input to Plous controls. **Ctrl+C always saves and exits Plous**. Mouse reporting is disabled during capture.

Output review uses a snapshot while the process continues. Scroll/find/select/copy with the shared viewer; End refreshes the snapshot. r resumes input, t sends a remote interrupt and x closes the connection. Closing does not guarantee process termination. Finished sessions retain output/errors; e edits the command and R explicitly retries after rechecking the original Pod UID.

Normal-screen history is limited to 5,000 rows and 2 MiB of text/styles; alternate-screen programs expose the current screen rather than scrollback. Retained output is memory-only. Full controls are in the [Exec keymap](../../reference/keymap.md).

## Values, exports and recovery

V inspects supported ConfigMap/Secret keys, CSR request/certificate or ControllerRevision snapshots. Secret values stay hidden until explicitly opened. Values use a read-only Editor; Back/Copy controls operate on the selected value. Closing/hiding clears its buffer. Copy and Export are explicit transfers outside the view. Secret YAML exports remain masked.

If an operation fails, read its full error and inspect live state before submitting again: the server may have accepted a request before a connection failed. See [troubleshooting](../../reference/troubleshooting.md), [privacy](../../reference/data-and-privacy.md#kubernetes-data) and the disposable [playground](playground.md).

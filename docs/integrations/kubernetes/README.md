# Kubernetes

[Documentation](../../README.md) · [All integrations](../README.md)

Browse resources, read logs and metrics, edit YAML, and run actions from object details. Plous uses your kubeconfig without changing another tool's active context.

## Connect a cluster

Current Workspace opens your terminal's current Kubernetes context. Use `:K` to choose another. To use another kubeconfig, open **Settings → Choose Kubeconfig**.

To save connections:

1. Choose **Monitor → + Add Context** or **+ Add Connection → Kubernetes Cluster**.
2. Check the kubeconfig path, initially `~/.kube/config`.
3. After changing the path, select **Refresh Contexts**.
4. Check contexts with Space or Enter. Use **Name** or Right for an optional display name.
5. Choose **Add Selected Contexts**.

Changing or refreshing the path clears pending selections. **Already added** marks an existing connection.

Setup reads the displayed file without connecting or running authentication helpers. Opening a saved connection connects through that context. Neither step edits kubeconfig.

## Explore resources

Type `:pods` and press Enter. Choose a connection if prompted. Check the context and namespace in the header.

| Key | Action |
| --- | --- |
| `n` | Choose a namespace |
| `/` | Edit the row filter |
| Backspace | Clear the applied text filter |
| `L` | Edit the label selector |
| `S` | Choose sorting |
| Enter | Open resource details |
| `g` | Refresh |

Use `*` for all namespaces. `all` means a namespace literally named `all`.

Find other resource types in the sidebar with `/`, or use a command such as `:deployments`. Qualify ambiguous types with their API group.

Details provide Describe, YAML, Events, and Related views. Following a related object keeps a return path. Esc returns to the previous resource or list.

## Resource support

Plous discovers core, workload, networking, storage, configuration, security, policy, and extension APIs. Available actions depend on the resource and your permissions.

Missing access and incomplete data are reported. Readiness, Pod phase, and desired replica counts are different observations.

Bulk creation, node drain, starting existing Jobs/CronJobs, volume resizing or snapshots, attach, and port forwarding are unavailable.

## Usage and charts

| View | Measurements |
| --- | --- |
| Pod, Node, workload Describe | CPU and memory history |
| Pod inspection | Container shares and request/limit bars |
| Node inspection | Usage and declared requests against allocatable capacity |
| Pod or Node list, then `I` | Top ten CPU and memory consumers within the current filters |
| Workload Describe | Ready, current, and desired replicas |
| ResourceQuota Describe | Reported used and hard quantities |

### Inspect samples

Press `I` in Describe. Left / Right selects a sample, `W` chooses 5 or 15 minutes, and `P` follows the latest. Click a sample to inspect it.

Use `ff` for more room. Esc returns to Describe. Help and Settings keep the selected sample. See [chart controls](../../reference/keymap.md#usage-charts).

### Sampling and terminal limits

The Metrics API is polled every 15 seconds while the view is active. Up to 15 minutes stays in memory. There is no background collector or saved chart history.

Pod and Node points use source timestamps. Workload points show collection time and the range of contributing source times. Repeated measurements do not create extra samples.

Graphs leave gaps for missing data. Their scale maxima are display bounds, not capacity. These are terminal character-cell charts, without hover or drag-to-zoom.

Agent usage reads save their own results. They cannot recover unsaved TUI chart history.

### Budgets and coverage

Requests and limits describe declared budgets, not actual consumption. Missing or zero budgets have no percentage.

Workload usage totals require complete measurements for owned Pods. Top-consumer rankings exclude missing or stale measurements and report how many were usable.

Rankings are read-only. Press `I` or Esc to return to the resource list before opening an object.

## Resource actions

Open details and check the context, namespace, and object name before acting.

| Action | Key |
| --- | --- |
| Logs | `l` |
| Container terminal | `x` |
| Scale replicas | `S` |
| Rollout restart | `R` |
| Edit YAML | `E` |
| Graceful deletion | `D` |
| Copy identity | `Y` |
| Inspect values | `V` |
| Export YAML | `w` |
| Use the open Namespace | `u` |

Lowercase `d/e/r` open detail sections. Uppercase `D/E/R` perform actions. Use **a More** for additional actions.

Scale and restart check progress every two seconds for up to five minutes. Enter or Esc returns to details. Press `g` to check again without resubmitting. An HPA or workload update can change the result afterward.

Delete identifies the original object by UID and respects finalizers. Its disappearance does not confirm that all dependents have finished terminating.

## Edit and review YAML

1. Press `E` in details.
2. Press `i` to edit, then Esc for Normal mode.
3. Use `:w` to validate and save, or `:wq` to save and close.
4. Wait for **Saved to Kubernetes**.

`:v` opens an optional comparison. It does not save. Save there still requires `:w` or the explicit Save control.

Rejected updates and conflicts keep your draft. Press `!` for the full error. **Save result unknown** means acceptance was not confirmed. Inspect the live object before retrying.

Use `u/U` for undo/redo, `:q` to close, or `:q!` to discard. Ctrl+C and `:quit` exit Plous without applying the draft. See [editor controls](../../reference/keymap.md#plous-editor-and-diff-reviewer).

The editor accepts up to 2 MiB. Undo keeps up to 100 snapshots within 16 MiB. Some resources allow metadata-only editing. Editing a Secret explicitly reveals its values.

## Read logs

Press `l` in Pod or workload details. Use `o` to choose a container. Init and ephemeral containers must be selected explicitly.

Workload **All Pods** follows controller ownership. Pod **All application containers** merges regular containers.

| Key | Action |
| --- | --- |
| `p` | Previous container instance |
| `u` | Time range |
| `t` | Timestamps |
| `w` | Line wrapping |
| Space | Hold position or jump live |
| `/` | Search the loaded buffer |
| `g` | Clear and reload logs |

Collection continues while you read or search. Current-log membership is checked every three seconds. Up to 32 sources are selected in Pod-name order, with omissions reported.

Each source initially requests up to 200 lines. The buffer keeps at most 128 KiB or 5,000 complete lines and reports eviction. Previous logs depend on Kubernetes retention.

## Container terminals

Press `x`, choose a running container, select sh, bash, or a custom command, then Start. Plous does not probe shells or retry commands automatically.

Commands run with the container's permissions and can change files or processes.

While input is captured, Esc, Tab, colon, and paste go to the container. Alt+Left / Alt+Right changes panels. Ctrl+] opens Plous output review. **Ctrl+C saves and exits Plous**, including during Exec.

In review, `r` resumes input, `t` sends a remote interrupt, and `x` closes. End refreshes the output snapshot. Closing does not guarantee remote process termination.

Output stays in memory, limited to 5,000 history rows and 2 MiB. See [Exec controls](../../reference/keymap.md#containers-and-logs).

## Values, exports and recovery

Press `V` to inspect supported ConfigMap or Secret keys, CSR data, or ControllerRevision snapshots. Copy and Export explicitly transfer data outside the view. Secret YAML exports remain masked.

If an operation fails, inspect the full error and live state before retrying. The server may have accepted it before the connection failed.

See [Troubleshooting](../../reference/troubleshooting.md), [Data and privacy](../../reference/data-and-privacy.md#kubernetes-data), or the disposable [playground](playground.md).

# Work with Kubernetes resource actions

[Documentation](README.md) · [Explore Kubernetes](kubernetes-explore.md)

Actions operate on the object you have opened in Explore. Some, such as Scale or Delete, change the cluster. Start by checking the context, namespace, and object name in the view.

The Log Viewer, Editor, Diff Reviewer, and Exec use grouped action bars. Each control separates its key from its label; toggles show **On** or **Off**. A **›** marker identifies the focused action that Enter activates. **Unavailable** means the action cannot run. Narrow panels wrap controls and keep the focused action visible. Open Help for the full key list when space is limited.

## Open an action

1. Find the resource in Explore and press **Enter** to open its details.
2. Use a displayed action key, or press **a** for **More** and select an action with arrows and Enter.
3. Read any input, permission message, or review shown for that action.
4. For edited YAML, use **Save**. Scale, Restart, and Delete use explicit confirmation controls.

Common actions appear below the detail sections. Narrow panels place additional actions in More. Lists do not expose operations directly. Actions depend on the resource type, its served APIs, and your permissions; More explains unavailable operations.

Confirmations initially select **Back**. Use **Left / Right** to choose a control and **Enter** to activate it. Esc cancels an unfinished action. Direct actions return to their originating detail; actions opened from More return through that menu.

## Choose an action

| Action | Key | Command | Use it for |
| --- | --- | --- | --- |
| Logs | `l` | `:logs` | Read a Pod's logs or merge a workload's owned Pods with All Pods. |
| Exec | `x` | `:exec` | Open a terminal in a running container. |
| Scale | `S` | `:scale` | Change replicas on a resource with a served scale API. |
| Restart | `R` | `:restart` | Request a Deployment, StatefulSet, or DaemonSet rollout restart. |
| Edit | `E` | `:edit` | Edit YAML and save with automatic validation; diff review is optional. |
| Delete | `D` | `:delete` | Review and request graceful deletion. |
| Copy identity | `Y` | `:copyidentity` | Copy context and object identity to the terminal clipboard. |
| Values | `V` | `:copyvalue` | Inspect and copy supported values or snapshots. |
| Export YAML | `w` | `:exportyaml` | Write the object's YAML to a new local file. |
| Use namespace | `u` | `:usenamespace` | Use the open Namespace as the browsing scope. |

Capitalization matters. Lowercase **d/e/r** opens Describe/Events/Related; uppercase **D/E/R** opens Delete/Edit/Restart. **S** sorts in a table and scales in details. In Logs, the log controls take precedence.

Use **:z / :zoom** to expand or restore Logs, Exec output review, a document, or a comparison. Reading position and drafts remain intact. **:L / :linenumbers** toggles the Editor gutter. Line ranges below logs and comparisons show your position in retained content.

## Edit YAML

Follow [Edit and review YAML](editor.md) for saving, optional diff review, conflict handling, and editor controls.

## Scale or restart a workload

For Scale, open a supported resource, press **S**, enter the desired replica count, and review the confirmation. Plous shows a targeting HorizontalPodAutoscaler when it can read one. An autoscaler may subsequently adjust the count.

For Restart, open a Deployment, StatefulSet, or DaemonSet and press **R**. The request updates the Pod template and follows that workload's configured update strategy. Read the explanation for OnDelete or partitioned strategies; they do not behave like an ordinary full rolling update.

After acceptance, the progress view checks the captured object every two seconds for up to five minutes. Scale shows current and ready replicas. Restart shows updated and available counts, completion, or the controller's reported failure. OnDelete explains that Pods need manual replacement; partitioned StatefulSets report completion only for the eligible Pods. Custom scale APIs without a supported readiness check direct you to their reported status.

**Enter** or **Esc** returns to the resource details and stops these checks. It does not undo or cancel an accepted operation. A connection error, changed desired state, replacement object, or observation timeout is shown separately from completion. After checks stop, **g** checks status again without submitting the change again. Use arrows or Page Up/Down to read long messages.

## Delete an object

Press **D** from details and review the target and consequences. The confirmation includes the object's unique identity so that an object recreated under the same name is not mistaken for the original.

The progress view shows the deletion timestamp, grace period, and remaining finalizers until the original object disappears. A new object with the same name is identified as a replacement. Completion refers to the original object; dependent resources can still be terminating.

Deletion uses Kubernetes's normal graceful behavior. It does not bypass finalizers or force deletion. Storage-related confirmations show available binding, reclaim-policy, and Pod-reference information, or explain why it could not be read. Deleting a saved connection in Settings is a separate action and does not delete cluster resources.

## Open a container terminal

Follow the [container terminal guide](exec.md) to choose a shell, control input, and review earlier output.

## Copy or export data

**Copy identity** includes the context and the object's identifying fields. It uses terminal clipboard support; a terminal that blocks clipboard access may not copy it.

**Values** lets you choose and inspect a ConfigMap or Secret key before copying it. A Secret value stays hidden until you open that key. CertificateSigningRequests expose the decoded request and issued certificate, including Export PEM. ControllerRevisions expose their immutable snapshot.

Opening a value displays it in the main panel using Plous Editor in read-only mode. Use arrows or page keys to read, `/` to find, `v` to select, and `y` to copy a selection or line. `:z` expands/restores the view; `:L` toggles line numbers. Paste cannot edit the value.

The Back action is initially selected. Press `a` to switch between reading and actions; use Left/Right and Enter to activate Back, Copy value, or Export PEM where available. Copy value copies the entire value. Esc clears search or selection, restores an expanded view, then hides the value and returns to the picker. Hiding or closing a value clears its Editor buffer. Help and panel changes retain the open preview.

**Export YAML** asks for a local path and creates a new file with private permissions. It never overwrites an existing file. Secret YAML exports remain redacted.

These are explicit copies or exports outside the browsing view. See [Data and privacy](data-and-privacy.md#kubernetes-data).

## If an action is missing or fails

Wait for the current object to finish loading, then check **More** for the reason. A cluster may not serve the required API, or your account may lack that operation's permission. Read permission does not imply edit, delete, log, or Exec permission.

After a connection error during a change, refresh the resource and inspect its state before deciding whether to submit again. A server may have accepted the request before the connection failed.

Creation, bulk operations, node drain, Job/CronJob execution controls, volume resize/snapshot creation, attach, and port forwarding are not available. See [Troubleshooting](troubleshooting.md#an-action-is-unavailable-or-a-change-failed) for recovery steps.

# Troubleshooting

[Documentation](README.md)

Start with the symptom below. The footer shows which panel has focus; the Explore header shows the context and namespace. Check these before changing configuration.

## Plous will not launch

**It says an interactive terminal is required.** Run Plous directly in a terminal window. Its interface cannot run through redirected input/output or a pipe. `./bin/plous --version` and `./bin/plous --help` can be used without opening the interface.

**It says to resize the terminal.** Make the window at least 80 columns by 16 rows. Hidden actions are disabled while the window is too small. Ctrl+C can still save and exit.

**Building fails.** Check that your Go version satisfies the requirement in [Get started](getting-started.md#before-you-begin). The first build needs network access to download dependencies. See [Development](development.md) for build commands.

## A shortcut types text or does nothing

- If a text field or chat composer is active, letters and colons are text. Press Esc to leave chat typing or cancel an unfinished field/form before navigating.
- Mode shortcuts are **11**, **22**, and **33**, with no Enter. A single number does nothing.
- Colon commands need Enter: type `:pods`, then press Enter. Paste into the colon box is ignored; type the command.
- Arrows, search, and resource keys belong to the focused panel. Use Tab/Shift+Tab to move focus.
- Tab does nothing in Settings, Help, or a resource picker. Use arrows and Enter, then Esc to return.

If only the first key of a double-key shortcut is pending, Esc clears it before any other back action. The footer shows the pending prefix.

## A panel disappeared

**bb** toggles the workspace sidebar; **ii** toggles the AI assistant. These choices persist after restart. `:c` reveals the assistant and enters typing.

Esc from a root main view focuses a visible sidebar but does not reveal a hidden one. Use bb to show it.

## My Kubernetes context is missing

1. Open **+ Add context** under the Kubernetes group, or **Add tool → Kubernetes**.
2. Check the exact **Kubeconfig file** path. Setup initially reads `~/.kube/config`; it does not substitute your shell's `KUBECONFIG` value.
3. If you change the path, select **Refresh contexts**. **Refresh required** means the displayed list is not ready for selection.
4. Check whether the context is marked **Already added**. Look for that connection in the workspace tree.
5. Read any file error below the path. Plous needs a readable, valid kubeconfig containing named contexts.

For minikube, verify that your minikube setup has created a context in the file you are loading. Plous does not create contexts or start minikube. Adding a context successfully does not establish that its cluster is reachable.

## Explore cannot connect

Check the context in the Explore header and the connection's kubeconfig path in **Settings → Current workspace**.

| Message or symptom | What to check |
| --- | --- |
| Connection refused, timeout, or unreachable server | Whether that cluster is running and reachable from this computer; check any required network or VPN connection. |
| Authentication failed or credentials expired | Renew access using your usual cluster login process, then run `:g`. |
| Credential plugin failed or was not found | Whether the kubeconfig's authentication program is installed and available to the terminal running Plous. Complete any interactive login outside Plous, then refresh Explore; plugins cannot prompt for terminal input inside the app. |
| Certificate/TLS error | Whether the kubeconfig and cluster certificate information are current and correct. |
| Access denied / RBAC | Whether the account can perform the requested read or operation in this namespace. |

**RBAC** is Kubernetes's permission system. Being able to connect does not imply access to every namespace or resource.

Compare with your usual Kubernetes client using the same kubeconfig file and context. Once the underlying access works, **g** or `:g` from a table reconnects and reloads the API catalog. A discovery warning may affect only one API group; other available resources can still be browsed.

## The resource list is empty

1. Confirm the context and resource type in the header.
2. Check the namespace. Press **n** to select another one, or try **All namespaces** if your account permits it.
3. If the bar shows **Backspace clear**, press Backspace while navigating the table.
4. Check for a **Labels:** selector. Press **f**, erase the selector, then Enter to remove it.
5. Press **g** or run `:g` if the cluster or connection has changed.

**Loading** is a pending request. **No resources match this filter** indicates filtering. **No Pods in namespace …** is a completed empty list. An access error is not an empty list.

If listing namespaces is denied, use **Enter namespace manually** or `:N NAME`, replacing `NAME` with a namespace you can access. An all-namespace request can be denied while a request inside that namespace succeeds.

## My search or filter will not clear

In a resource table, **Enter** commits the filter. **Esc** preserves that applied filter. Press **Backspace while navigating rows** to clear it. If the caret is still in the search field, Backspace edits one character; Enter returns to results, while Esc cancels the edit.

Clearing the text filter does not clear namespace, labels, or sorting. In tree, Activity, history, or text-find search, Esc closes search and restores the underlying view.

## Logs are empty, old, or paused

- Use **o Choose container** to confirm which container you are reading.
- Check whether **p** selected previous-container logs. Kubernetes may no longer retain that run.
- Use **u** to choose a time range.
- Scrolling and `/` search pause following. Close search with Esc, then press **Space** to resume.
- Search examines the loaded buffer only. It does not retrieve older logs.
- Check log permissions separately from Pod read permissions. Log access needs permission to read `pods/log` as well as the selected Pod. After access is corrected, **g Reload logs** retries.
- If previous-instance logs are unavailable, **p** switches back to current logs. The error does not leave the log view or change its return point.

Esc returns from Logs to the resource and section that opened it. A workload log view returns to its workload details.

## I edited YAML, but it has not saved

Press Esc to finish typing, then use **`:w` Save** to validate and save to Kubernetes. Wait for **Saved to Kubernetes** before closing with **`:q`**, or use **`:wq`** to save and close together. **`:v` Review changes** only previews a diff; use the same **`:w`** command to save from there. See [Edit YAML](kubernetes-actions.md#edit-yaml).

**Save failed** keeps your draft in the editor. Press **!** in Normal for the full error. **Save result unknown** means a write was attempted but its result could not be confirmed; check the live resource before retrying.

## An action is unavailable or a change failed

Open the object's details and wait for loading to finish. **a More** lists available actions and reasons for unavailable ones. An action requires both a suitable API and permission for that operation.

A validation error leaves the edit unapplied. A conflict means the object changed; refresh and review the current version before preparing another change. After a network error during submission, inspect the resource before retrying because the server may already have accepted the request.

After Scale or Restart is accepted, the progress view checks readiness and reports completion, failure, or unavailable observation. Esc stops checking without undoing the change; `g` rechecks without resubmitting it. Graceful deletion can wait on finalizers. See [operation progress](kubernetes-actions.md#scale-or-restart-a-workload).

## Exec captures Esc and Tab

Inside the container terminal, ordinary keys belong to the remote process. Press **Ctrl+]** to review output. Use **r** to resume input, **t** to interrupt the remote process, or **x** / **Esc** to close back to details. Ctrl+C exits Plous itself.

A failed Exec session keeps its output and shows **Session failed**. Use **e Edit command** to choose a different executable or shell, **R** to retry the same command, or **x** to return to the resource.

## AI chat will not send

1. Open Settings and confirm that an OpenAI or Anthropic connection exists.
2. Edit it to supply a valid API key. A blank key field preserves the old key rather than clearing it.
3. Enter chat with **cc**. With an empty draft, freshly type `/model` and press Enter to choose a supported model.
4. If another reply is running, leave typing with Esc and run `:X` before sending another request.
5. Read the error for account/model access, rate limits, or a proxy problem. Plous does not switch models or retry automatically.

If `/model` is sent as a message, check whether it was pasted or restored. Only a freshly typed command, alone in the draft, opens the picker.

If Shift+Enter sends instead of inserting a newline, use **Ctrl+J**. Long conversations are summarized automatically; you do not need to reset them solely because they have grown. An individual message can still be too large. Shorten that message and send it again. See [AI chat](ai-chat.md#understand-the-context-meter) for limits.

## An AI investigation stops or cannot read a resource

| Symptom | What to do |
| --- | --- |
| No Kubernetes connection, wrong namespace, or a request outside the allowed scope | Select the intended connection and resource or concrete namespace in Explore, then send a new message. Changing the view does not retarget a running investigation. |
| Write or unsupported command denied | The assistant supports resource reads and bounded Pod logs. Use the explicit [Kubernetes actions](kubernetes-actions.md) for changes. |
| Time, call, context, or output limit reached | Narrow the request to one resource, a smaller question, or a shorter log window. Limits stop the operation; it does not continue automatically. |
| Summarization failed | The original messages and previous summary remain saved. Address the reported provider error or shorten the request before retrying. |
| Earlier cluster state is missing | The assistant can compare saved investigation results. Browsing the dashboard does not archive every resource or log result. |

After a failure or stop, press Esc to leave typing and use **`:p`** to restore the last prompt into an empty draft. Edit and send it to start a new request. After an unexpected exit, saved partial text remains visible with an interrupted status; the operation does not resume itself.

## Another process is using the data directory

Only one Plous instance can open the same profile. Exit the other instance normally, or start the new one with a different `--data-dir`.

The presence of `store.lock` does not mean a process still holds the lock. Do not delete it while another instance is running; the process releases its lock when it exits.

## Plous reports corrupt data

Preserve a copy of the reported data directory. Plous leaves corrupt files in place for recovery instead of resetting the profile.

To keep working independently, launch with a new `--data-dir` path. That does not recover the old conversations or settings. If you have a backup, inspect a restored copy in a separate directory as described in [Data and privacy](data-and-privacy.md#back-up-or-recover-a-profile).

## Report a bug

Include the Plous version, OS, terminal name, window size, expected behavior, and the steps that produced the problem. For input problems, include which panel or editor had focus. For Kubernetes problems, describe the resource type and whether the issue also occurs with the same context in your usual client.

Run `./bin/plous --version` to get the version. You can include a screenshot, but remove private names, tokens, API keys, and Secret values. Do not attach your kubeconfig or whole Plous data directory.

Report the issue in the [project issue tracker](https://github.com/simjay/plous/issues).

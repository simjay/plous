# Troubleshooting

[Documentation](../README.md)

Start with the symptom below. The footer shows which panel has focus; the Explore header shows the context and namespace. Check these before changing configuration.

Jump to [launch](#plous-will-not-launch), [keyboard and panels](#panel-or-editing-keys-do-not-work), [cluster access](#explore-cannot-connect), [missing integrations](#helm-argo-or-flux-is-missing), [Code and rendering](#code-is-empty-or-a-render-fails), [AI setup](#ai-chat-will-not-send), [MCP](#an-external-agent-cannot-use-plous-tools), or [profile recovery](#plous-reports-corrupt-data).

## Plous will not launch

**It says an interactive terminal is required.** Run Plous directly in a terminal window. Its interface cannot run through redirected input/output or a pipe. `./bin/plous --version` and `./bin/plous --help` can be used without opening the interface.

**It says to resize the terminal.** Make the window at least 80 columns by 16 rows. Hidden actions are disabled while the window is too small. Ctrl+C can still save and exit.

**Building fails.** Check that your Go version satisfies the requirement in [Get started](../getting-started/README.md#before-you-begin). The first build needs network access to download dependencies. See [Development](../contributing/README.md) for build commands.

## Panel or editing keys do not work

Check the [panel controls](keymap.md#switch-panels) first. Panel travel skips hidden panels. Plous accepts several encodings of the same physical keys, but cannot recover a modifier that the terminal removes or a shortcut that it handles itself.

| Terminal | What to check |
| --- | --- |
| [iTerm2](https://iterm2.com/documentation-preferences-profiles-keys.html) | Profile key mappings and **Treat option as Alt for special keys like arrows**. Plous accepts both Alt and Meta arrows, plus Escape+b/f. |
| [Terminal.app](https://support.apple.com/guide/terminal/change-profiles-keyboard-settings-trmlkbrd/mac) | The profile's Keyboard mappings and **Use Option as Meta key** setting. |
| [Ghostty](https://ghostty.org/docs/config/reference#macos-option-as-alt) | Key bindings and `macos-option-as-alt`. Option behavior for printable characters can depend on the keyboard layout. |
| [Kitty](https://sw.kovidgoyal.net/kitty/keyboard-protocol/) | Plous accepts CSI-u/Kitty input and distinct press/release events. Check custom key mappings if a shortcut never reaches it. |
| [WezTerm](https://wezterm.org/config/key-encoding.html) | Key assignments run before terminal encoding. Check assignments before changing CSI-u or Kitty settings. |
| [Alacritty](https://alacritty.org/config-alacritty.html) | Keyboard bindings and, on macOS, `window.option_as_alt`. Its default is `None`. |
| [VS Code terminal](https://code.visualstudio.com/docs/terminal/advanced#keyboard-shortcuts-and-the-shell) | `terminal.integrated.commandsToSkipShell` determines which shortcuts VS Code handles. Check a conflicting command before broadly forwarding every shortcut. |
| [Windows Terminal](https://learn.microsoft.com/en-us/windows/terminal/panes) | Alt+arrows can belong to the terminal's own pane navigation. Remove or change that conflicting terminal binding to pass the keys through. Plous itself supports macOS and Linux; native Windows is unsupported. |
| [tmux](https://github.com/tmux/tmux/wiki/Modifier-Keys) | Compare the key outside and inside tmux. The terminal and tmux must agree on input, and tmux and Plous must agree on forwarding. Keep the terminal's correct `TERM` value. |

In a separate shell, run `cat -v`, press the failing key, then Ctrl+C. Compare modified and plain arrows. If they produce the same bytes, Plous cannot distinguish them. This checks legacy shell input; an application that negotiates an extended protocol may receive different bytes. Include the output, terminal version, and whether tmux or SSH is involved in a bug report.

If Shift+Enter arrives as ordinary Enter, use the existing [newline fallback](keymap.md#ai-assistant). Backspace sent as either DEL or BS works in text input; BS does not add a navigation shortcut. A lost modifier or intercepted shortcut requires a terminal setting change, not another Plous control scheme.

The automated suite replays keyboard encodings and exercises an isolated tmux session when installed. This is not certification of every terminal version, OS, keyboard layout, or user profile. See [keyboard coverage](../../tests/README.md#keyboard-compatibility).

## Copy or paste does nothing

Native terminal Copy needs a native terminal selection. With Plous mouse support On, dragging in fields and viewers creates a Plous selection: click **Copy selection** in the footer, or use `y` in a viewer. Turn Mouse support Off in Settings to use native terminal selection everywhere.

Paste into an active field, search, composer, or editable document. Navigation and colon-command input ignore paste. Mac terminals normally use ⌘V; GNOME Terminal uses Ctrl+Shift+V. Plous cannot intercept shortcuts consumed by the terminal. See [Copy and paste](keymap.md#copy-and-paste) for forwarded shortcuts, local clipboard tools, and remote sessions.

## Current Workspace has no target

Use `:K` to choose a configured context, or **Settings > Choose Kubeconfig** to change the session's paths. After fixing a missing or malformed file, use `:g` to retry. Plous never picks the first context as a fallback. If the default changes externally, the notice offers context selection; existing views stay bound to their captured target. Relaunch to use a changed shell environment.

## A shortcut types text or does nothing

- If a text field or chat composer is active, letters and colons are text. Press Esc to leave chat typing or cancel an unfinished field/form before navigating.
- Mode shortcuts are **11**, **22**, and **33**, with no Enter. A single number does nothing.
- Colon commands need Enter: type `:pods`, then press Enter. Paste into the colon box is ignored; type the command.
- Arrows, search, and resource keys belong to the focused panel. Use Alt+Left/Right to move focus, including while typing or using a captured terminal.
- Alt+Left/Right leave and resume Settings, Help and resource pickers without submitting edits. On Mac, Option+Left/Right supports both modified arrows and Escape+b/f. See the [keymap](keymap.md#switch-panels) if your terminal intercepts those shortcuts.

If only the first key of a double-key shortcut is pending, Esc clears it before any other back action. The footer shows the pending prefix.

## A panel disappeared

**bb** toggles the workspace sidebar; **ii** cycles the AI assistant through normal width, double width, hidden, then normal. Size and visibility persist after restart. `:c` reveals the assistant and enters typing.

Esc from a root main view focuses a visible sidebar but does not reveal a hidden one. Use bb to show it.

## My Kubernetes context is missing

1. Open **+ Add Context** under Monitor, or **Add Connection → Monitor → Kubernetes Cluster**.
2. Check the exact **Kubeconfig File** path. Setup initially reads `~/.kube/config`; it does not substitute your shell's `KUBECONFIG` value.
3. If you change the path, select **Refresh Contexts**. **Refresh required** means the displayed list is not ready for selection.
4. Check whether the context is marked **Already added**. Look for that connection in the workspace tree.
5. Read any file error below the path. Plous needs a readable, valid kubeconfig containing named contexts.

For minikube, verify that your minikube setup has created a context in the file you are loading. Plous does not create contexts or start minikube. Adding a context successfully does not establish that its cluster is reachable.

## Explore cannot connect

Check the context in the Explore header and the connection's kubeconfig path in **Settings → Current Workspace**.

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
4. Check for a **Labels:** selector. Press **L**, erase the selector, then Enter to remove it.
5. Press **g** or run `:g` if the cluster or connection has changed.

**Loading** is a pending request. **No resources match this filter** indicates filtering. **No Pods in namespace …** is a completed empty list. An access error is not an empty list.

If listing namespaces is denied, use **Enter namespace manually** or `:N NAME`, replacing `NAME` with a namespace you can access. An all-namespace request can be denied while a request inside that namespace succeeds.

## Helm, Argo, or Flux is missing

Select the cluster in the sidebar and press **g** to repeat discovery. Helm appears after release metadata is detected; Argo and Flux appear when their APIs are discovered. A tool appearing in a source repository does not establish that its controller or release storage exists in this cluster.

**Tool discovery incomplete** means a check failed or was denied. Confirm permissions for that context before interpreting a hidden branch as absence. Helm checks its configured storage, or Secrets and then ConfigMaps by default; it can fall back to the context namespace when cluster-wide discovery is denied. See [Helm discovery](../integrations/helm/README.md#connection-and-discovery) and [GitOps discovery](../integrations/gitops/README.md#open-the-view).

## Code is empty or a render fails

| Symptom | What to check |
| --- | --- |
| Current Workspace shows the wrong directory | Code uses the directory where Plous was launched. Relaunch there or add an explicit saved source. |
| A technology browser is empty | Check the selected source and scan status. Open Files to confirm the path, then **g** to refresh. A scan does not render. |
| A file is missing from browsing or search | Check the profile's `.plousignore`, repository `.gitignore`, and displayed coverage limits. See [source exclusions](data-and-privacy.md#global-source-scanning-exclusions). |
| My edit is missing from rendered output | Save the file first. Rendering uses saved bytes, not the editor's unsaved buffer. |
| A remote source shows old content | Refresh the source. Reads and renders use its acquired revision until refreshed. |
| Rendering fails | Open **Technical details** for the full diagnostic. Check declared dependencies, input limits, and [supported engine behavior](../integrations/kustomize/README.md#engines-and-boundaries). |
| Previous output or a stale result remains visible | The latest render did not replace that result. Correct the input and use **r** to retry. |
| The assistant cannot explain generated output | Render the target in Code first. Its analysis tool reads captured results and cannot initiate rendering. |

Follow the [Code walkthrough](../guides/code.md) for the edit, render, and compare sequence. A successful local render does not establish Kubernetes acceptance or readiness.

## My search or filter will not clear

In a resource table, **Enter** commits the filter. **Esc** preserves that applied filter. Press **Backspace while navigating rows** to clear it. If the caret is still in the search field, Backspace edits one character; Enter returns to results, while Esc cancels the edit.

Clearing the text filter does not clear namespace, labels, or sorting. In tree, Activity, history, or text-find search, Esc closes search and restores the underlying view.

## Charts are empty, stale, or unexpected

Open a supported Pod, Node or workload in Describe and wait for its details to load. **I** opens inspection. The Metrics API must be available and your account must be allowed to read it; access to ordinary resources alone is insufficient.

| Symptom | Meaning or next step |
| --- | --- |
| No earlier samples | Collection runs every 15 seconds while the relevant view is active. At most 15 minutes stays in memory; changing the range cannot retrieve older history. |
| Metrics unavailable or denied | Read the displayed error and check Metrics API availability and permission for the same context. Unavailable measurements are not zero. |
| Stale or partial workload data | Check the source times and coverage. Missing, replaced or stale measurements can prevent a complete total. A workload with no active Pods has no measured total. A recent collection time does not make an old source measurement fresh. |
| Top consumers omits objects | Rankings use the current filtered list and exclude invalid or stale measurements. Check the coverage count and list filters. |
| A budget has no percentage | The request, limit or reported quota denominator may be absent or zero. Declared reservations and quota accounting are separate from measured consumption. |
| A chart's maximum changes | The axis is a rounded display bound with headroom, not a resource limit. **P** returns to following latest after inspection. |
| Text or plots are cramped | Try **ff** or a larger terminal. Fonts, cell shape and glyph rendering belong to the terminal; Plous has no pixel-chart or font setting. |

See [usage and charts](../integrations/kubernetes/README.md#usage-and-charts) for each chart's data and [chart controls](keymap.md#usage-charts) for selection and return behavior.

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

Press Esc to finish typing, then use **`:w` Save** to validate and save to Kubernetes. Wait for **Saved to Kubernetes** before closing with **`:q`**, or use **`:wq`** to save and close together. **`:v` Review changes** only previews a diff; use the same **`:w`** command to save from there. See [Edit YAML](../integrations/kubernetes/README.md#edit-and-review-yaml).

**Save failed** keeps your draft in the editor. Press **!** in Normal for the full error. **Save result unknown** means a write was attempted but its result could not be confirmed; check the live resource before retrying.

## An action is unavailable or a change failed

Open the object's details and wait for loading to finish. **a More** lists available actions and reasons for unavailable ones. An action requires both a suitable API and permission for that operation.

A validation error leaves the edit unapplied. A conflict means the object changed; refresh and review the current version before preparing another change. After a network error during submission, inspect the resource before retrying because the server may already have accepted the request.

After Scale or Restart is accepted, the progress view checks readiness and reports completion, failure, or unavailable observation. Esc stops checking without undoing the change; `g` rechecks without resubmitting it. Graceful deletion can wait on finalizers. See [operation progress](../integrations/kubernetes/README.md#resource-actions).

## Exec captures Esc and Tab

Inside the container terminal, ordinary keys belong to the remote process. Press **Ctrl+]** to review output. Use **r** to resume input, **t** to interrupt the remote process, or **x** / **Esc** to close back to details. Ctrl+C exits Plous itself.

A failed Exec session keeps its output and shows **Session failed**. Use **e Edit command** to choose a different executable or shell, **R** to retry the same command, or **x** to return to the resource.

## AI chat will not send

Check **Agent** in Settings first. Claude Code and Codex require their installed CLI on PATH and its own sign-in, without a Plous provider connection. Use **cc** to enter the native terminal, **Alt+Left/Right** to move panels directly, and **Ctrl+]** to toggle focus between the agent and the previous panel. If launch options or MCP setup fail, the native screen or panel shows the error. Fix the installation, close with `:X`, then retry with **cc**. There is no exact Codex version allowlist. Plous does not grant extra permissions or switch agents to recover. See [agent setup](../guides/ai-chat.md#choose-an-agent). The following provider checks apply to Plous.

1. Check the error for invalid [runtime configuration](configuration.md#agent-runtime-configuration), including retired settings from an earlier development build.
2. Open Settings and confirm that an OpenAI or Anthropic connection exists.
3. Edit it to supply a valid API Key. A blank key field preserves the old key rather than clearing it.
4. Enter chat with **cc**. With an empty draft, freshly type `/model` and press Enter to choose a supported model.
5. If another reply is running, leave typing with Esc and run `:X` before sending another request.
6. Read the error for account/model access, rate limits, or a proxy problem. Plous does not switch models or retry automatically.

If `/model` is sent as a message, check whether it was pasted or restored. Only a freshly typed command, alone in the draft with the Plous agent selected, opens the picker.

If Shift+Enter sends instead of inserting a newline, use **Ctrl+J**. Older complete exchanges are compacted when the selected model's capacity requires it. Original messages and projected observations remain saved. A single oversized current exchange can still fail; narrow the requested result. See [context limits](../guides/ai-chat.md#limits-and-recovery).

## An external agent cannot use Plous tools

Plous connects the selected native agent through a private session MCP bridge. No manual server registration is needed. Check the agent panel's scope and full startup or tool error.

If it shows an old connection, press **Ctrl+]**, close with `:X`, select the intended connection, and reopen with **cc**. If a fatal tool failure revoked access, correct the reported problem and reopen the session the same way. Changing the selected page or hiding the assistant does not retarget or restart its tools.

The [MCP troubleshooting table](../guides/plous-mcp.md#troubleshoot-the-connection) covers executable setup, authentication, missing tools, blocked view requests, and evidence-save failures. Native CLI conversations stay with the CLI; saved observations are in Plous's labeled tool-evidence history.

## An AI investigation stops or cannot read a resource

| Symptom | What to do |
| --- | --- |
| Missing connection or wrong namespace | Select the intended context or Code source in Plous, then choose or name the namespace. Chat is restricted to that one connection. Changing the selected page does not retarget a running response. |
| Write or unsupported command denied | The assistant supports the scoped reads in the [tool reference](mcp.md#available-tools). Use the explicit [Kubernetes actions](../integrations/kubernetes/README.md#resource-actions) for changes. |
| Plous turn budget or an individual read limit reached | Narrow the request to one resource or a shorter log window. Built-in Plous turns have shared time, call, and token limits; native sessions keep CLI-managed context and spending. Individual Plous tool reads remain bounded for every agent. |
| Search is pending or unavailable | Known saved references remain readable. Lexical retrieval works without embeddings. Check the explicit embedding connection if semantic search remains unavailable. |
| Old runtime configuration is rejected | Remove retired `history_mode`, `rlm`, CLM and raw-result options. Use the smaller [current configuration](configuration.md#agent-runtime-configuration). |
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

Run `./bin/plous --version` to get the version. You can include a screenshot, but remove private names, tokens, API Keys, and Secret values. Do not attach your kubeconfig or whole Plous data directory.

Report the issue in the [project issue tracker](https://github.com/simjay/plous/issues).

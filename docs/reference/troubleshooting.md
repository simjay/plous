# Troubleshooting

[Documentation](../README.md)

Check the focused panel, cluster context, and namespace first. The border, footer, and view header identify them.

## Plous will not launch

Run Plous directly in an interactive terminal, without a pipe or redirected input/output. `--version` and `--help` work without the interface.

Resize to at least 80 columns by 16 rows. For build failures, check the [Go requirement](../getting-started/installation.md#build-from-source) and network access for dependencies.

## Panel or editing keys do not work

Use Alt+Left / Alt+Right, or Option+Left / Option+Right on macOS. If the terminal consumes a shortcut, Plous cannot receive it.

Check your terminal's key mappings:

- [iTerm2](https://iterm2.com/documentation-preferences-profiles-keys.html): Option handling for special keys.
- [Terminal.app](https://support.apple.com/guide/terminal/change-profiles-keyboard-settings-trmlkbrd/mac): Use Option as Meta key.
- [Ghostty](https://ghostty.org/docs/config/reference#macos-option-as-alt): `macos-option-as-alt`.
- [Kitty](https://sw.kovidgoyal.net/kitty/keyboard-protocol/) and [WezTerm](https://wezterm.org/config/key-encoding.html): custom key assignments and protocol settings.
- [Alacritty](https://alacritty.org/config-alacritty.html): `window.option_as_alt`.
- [VS Code](https://code.visualstudio.com/docs/terminal/advanced#keyboard-shortcuts-and-the-shell): commands handled before the shell.
- [Windows Terminal](https://learn.microsoft.com/en-us/windows/terminal/panes): pane shortcuts can consume Alt+arrows. Native Windows Plous is unsupported.
- [tmux](https://github.com/tmux/tmux/wiki/Modifier-Keys): compare the key inside and outside tmux. Keep the terminal's correct `TERM`.

In a separate shell, run `cat -v`, press the key, then Ctrl+C. If modified and plain arrows produce the same bytes, Plous cannot distinguish them. Extended keyboard protocols can send different bytes inside an application.

See the [keymap](keymap.md#switch-panels) and [keyboard test coverage](../../tests/README.md#keyboard-compatibility).

## Copy or paste does nothing

With mouse support on, drag to create a Plous selection, then click **Copy selection** in the footer. Viewers also support `y`.

Turn **Settings → Application → Mouse support Off** to use native terminal selection everywhere.

Paste into a field, search, composer, or editable document. Navigation and colon-command input ignore paste. See [Copy and paste](keymap.md#copy-and-paste) for forwarded shortcuts and remote sessions.

## Current Workspace has no target

Use `:K` to choose a context or **Settings → Choose Kubeconfig** to choose a file. After fixing it, use `:g` to retry.

Plous does not fall back to the first context. Relaunch to use a changed shell environment.

## A shortcut types text or does nothing

- Leave a text field or Plous composer with Esc before using navigation shortcuts.
- Press both keys for `11`, `22`, or `33`. Do not press Enter.
- Type colon commands directly, then Enter. Paste does not run them.
- Move focus with Alt+Left / Alt+Right before using panel-specific keys.
- Esc clears a pending first shortcut key before going back.

## A panel disappeared

Use `bb` for the sidebar. `ii` cycles the assistant through normal width, double width, and hidden. `:c` reveals it and enters typing.

Esc at a root main view focuses a visible sidebar but does not reveal a hidden one.

## My Kubernetes context is missing

1. Open **Monitor → + Add Context**.
2. Check the displayed kubeconfig path. Setup starts with `~/.kube/config`, independently of the shell's `KUBECONFIG`.
3. After changing the path, select **Refresh Contexts**.
4. Check for **Already added** and look in the workspace tree.
5. Read any file error below the path.

Plous does not create kubeconfig contexts or start local clusters.

## Explore cannot connect

Check the view's context and **Settings → Current Workspace** connection fields.

| Error | Check |
| --- | --- |
| Refused, timeout, unreachable | Cluster availability, network, VPN |
| Authentication failed | Renew the cluster login, then refresh |
| Credential plugin failed | Install the helper on PATH and complete any interactive login outside Plous |
| TLS error | Current kubeconfig and cluster certificates |
| Access denied | Kubernetes permissions for this resource and namespace |

Compare with your usual Kubernetes client using the same kubeconfig and context. Press `g` after fixing access. A discovery failure can affect one API group without blocking others.

## The resource list is empty

1. Check context and resource type.
2. Press `n` to check the namespace.
3. Press Backspace to clear an applied text filter.
4. Use `L` to clear a label selector if needed.
5. Press `g` to refresh.

An access error is not an empty list. If all-namespace access is denied, use `:N NAME` for a namespace you can read.

## Helm, Argo, or Flux is missing

Select the cluster and press `g` to repeat discovery. Helm needs release metadata. Argo CD and Flux need their controller APIs.

**Tool discovery incomplete** means a check failed or was denied. Check permissions before treating a hidden branch as absence.

See [Helm discovery](../integrations/helm/README.md#connection-and-discovery), [Argo CD](../integrations/argo/README.md#open-the-view), or [Flux](../integrations/flux/README.md#open-the-view).

## My search or filter will not clear

Esc closes search or cancels an unfinished filter edit. It keeps a table filter you already applied.

Press Backspace in table navigation to clear the text filter. Namespace, labels, and sorting stay unchanged.

## Charts are empty, stale, or unexpected

Metrics need access to `metrics.k8s.io`, separate from ordinary resource reads.

| Symptom | Explanation |
| --- | --- |
| No old samples | Charts collect while the view is active and keep up to 15 minutes in memory |
| Unavailable metrics | Check the displayed error and Metrics API permissions |
| Incomplete workload total | Missing, stale, or replaced Pod measurements prevent a complete total |
| Missing top consumers | Rankings use current filters and exclude invalid measurements |
| No budget percentage | The denominator is absent or zero |
| Changing chart maximum | The scale is a display bound, not a resource limit |
| Cramped plot | Use `ff` or a larger terminal |

See [usage charts](../integrations/kubernetes/README.md#usage-and-charts). Missing measurements are not zero.

## Logs are empty, old, or paused

- Use `o` to check the container.
- Use `p` to switch between current and previous-instance logs.
- Use `u` to check the time range.
- Close search with Esc, then press Space to follow new output.
- Check `pods/log` permission separately from Pod read permission.
- Press `g` to reload after fixing access.

Search reads only the loaded buffer. Kubernetes may no longer retain previous-instance logs.

## I edited YAML, but it has not saved

Press Esc for Normal mode, then `:w` to validate and save. `:wq` saves and closes. `:v` only opens a comparison.

Wait for **Saved to Kubernetes**. **Save failed** keeps the draft. Press `!` for the full error.

**Save result unknown** means the write may have reached Kubernetes. Inspect the live object before retrying. See [YAML editing](../integrations/kubernetes/README.md#edit-and-review-yaml).

## An action is unavailable or a change failed

Open resource details and use **a More** for available actions and disabled-action reasons. The API and your permissions must support the operation.

Validation failures leave the edit unapplied. Conflicts require checking the current object. Network errors during submission can leave an unknown result.

Esc from Scale or Restart stops progress checking without undoing the change. `g` checks again without resubmitting. Deletion can wait on finalizers.

## Exec captures Esc and Tab

Those keys go to the container. Press Ctrl+] for output review, `r` to resume input, `t` for a remote interrupt, or `x` to close.

Ctrl+C exits Plous. For a failed session, `e` edits the command and `R` explicitly retries it.

## AI chat will not send

Check the selected agent first.

For Claude Code or Codex, check the installed command and native sign-in. Plous provider keys do not authenticate these CLIs. See [native panel troubleshooting](../guides/plous-mcp.md#troubleshoot-the-connection).

For Plous:

1. Check for an invalid [runtime configuration](configuration.md#agent-runtime-configuration).
2. Add or edit a provider connection in Settings.
3. Supply a valid API key. A blank key keeps the saved value.
4. Press `cc`, type `/model` afresh as the whole draft, and choose a supported model.
5. Stop another running response with `:X` if needed.
6. Read the full error for provider access, rate limits, or proxy failures.

Pasted or restored `/model` is literal text. If Shift+Enter sends a message, use Ctrl+J for a newline.

## An external agent cannot use Plous tools

Inside Plous, MCP setup is automatic. Check the assistant header and full startup or tool error.

For a wrong cluster or revoked tool access, press Ctrl+], close with `:X`, select the target, and reopen with `cc`.

For a standalone client, use [MCP troubleshooting](../mcp/troubleshooting.md). Native conversations stay with the CLI. Plous's tool-evidence history stores the returned cluster results.

## An AI investigation stops or cannot read a resource

| Problem | Action |
| --- | --- |
| Wrong target | Select the cluster before starting a response or native session |
| Unsupported action | Check the [tool list](mcp.md#available-tools) and [diagnostic setup](../mcp/active-diagnostics.md) |
| Budget or read limit reached | Ask about one resource or a shorter log window |
| Search unavailable | Check indexing status and optional embedding configuration |
| Retired configuration rejected | Remove `history_mode`, `rlm`, and other retired options |
| Earlier state missing | Only explicitly saved evidence is available. Dashboard browsing does not archive every resource. |

Use `:p` to restore the last prompt into an empty draft, edit it, and send again. Interrupted responses retain accepted text but do not resume automatically.

## Another process is using the data directory

Exit the other instance or use a different `--data-dir`. Do not delete `store.lock` while a process is using the profile. The lock is released when that process exits.

## Plous reports corrupt data

Preserve a copy of the reported directory. Plous keeps corrupt files for recovery instead of resetting the profile.

Use a new `--data-dir` to work separately. This does not recover old settings or conversations. Inspect backups in a separate directory as described in [Data and privacy](data-and-privacy.md#back-up-or-recover-a-profile).

## Report a bug

Include `plous --version`, OS, terminal, window size, expected behavior, reproduction steps, and the full sanitized error. For input bugs, name the focused panel or editor.

Remove private names, tokens, keys, and Secret values from screenshots. Do not attach kubeconfig or a whole profile.

Use the [issue tracker](https://github.com/simjay/plous/issues).

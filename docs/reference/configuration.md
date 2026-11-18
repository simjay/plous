# Configuration

[Documentation](../README.md)

Use Settings for saved preferences and command-line flags for launch options. Do not edit Plous revision files by hand.

## Command-line flags

| Flag | Purpose |
| --- | --- |
| `--version` | Print the application version and exit |
| `--data-dir PATH` | Choose a profile directory instead of `~/.plous/` |
| `--allow-exec` | Allow agent commands in existing containers for this launch |
| `--diagnostic-image IMAGE` | Allow temporary diagnostic Jobs for this launch |
| `--help` | Print usage |

For a separate profile:

```sh
plous --data-dir "$HOME/plous-profiles/testing"
```

Profiles have separate settings, credentials, workspaces, and conversations. They can still connect to the same cluster. Only one TUI can open the same workspace data at a time.

For agents outside Plous, find your client in the [agent directory](../guides/ai-chat.md#choose-an-agent).

## Settings

Press `ss` or type `:s` while navigating.

| Setting | Where |
| --- | --- |
| Agent and native CLI entry commands | Application → Agent |
| OpenAI and Anthropic connections | Application → Agent → Plous |
| Mouse support | Application |
| Model for built-in chat | Type `/model` in the Plous composer |
| Workspace name and connections | Current Workspace |
| Workspace location | Local only. Remote is disabled. |

Save applies a change. Esc cancels. Exiting does not apply an unfinished form.

## Tool connections

A workspace can contain several cluster contexts. Kubernetes, Helm, Argo CD, and Flux share each cluster connection.

See [connection fields](../integrations/README.md#saved-tool-connections). Saved changes appear in [Activity](../guides/navigation.md#review-activity).

## Kubernetes connections

Saved connections keep a kubeconfig path, context, and optional default namespace. Setup starts with `~/.kube/config`. After editing the path, select **Refresh Contexts** before adding a context.

Current Workspace uses the terminal's `KUBECONFIG` path list, or `~/.kube/config` if unset. It follows Kubernetes merge rules. Choosing another kubeconfig in Current Workspace Settings lasts for that session.

Plous notices external context changes but does not switch an open view. Selecting a namespace in Explore does not edit kubeconfig or change another client's context.

Authentication uses the referenced kubeconfig and its plugins. Plous does not copy those credentials. See [connection setup](../integrations/kubernetes/README.md#connect-a-cluster).

<a name="monitor-watches"></a>

## Monitors

Each monitor has a unique name, one Kubernetes context, and a namespace. Names allow 1 to 63 lowercase letters, digits, or hyphens and must start and end with a letter or digit. See the [monitor workflow](../guides/navigation.md#monitor-cluster-health).

| Start option | Purpose |
| --- | --- |
| `--context NAME`, `--namespace NAME` | Required when creating a monitor or changing its scope. |
| `--deployment NAME`, `--daemonset NAME`, `--service NAME` | Repeat to select up to 32 named targets. With no targets, check every Deployment in the namespace. |
| `--every DURATION` | Set the interval from 1 to 60 minutes. Default `5m`. |
| `--kubeconfig PATH` | Use a different kubeconfig. |
| `--replace` | Approve replacing a stopped monitor's scope and clearing its saved alerts. |

Services are included when selected with `--service`. All monitor commands accept `--data-dir PATH` to use a different [data directory](data-and-privacy.md#data-directory-and-profiles).

Plous applies these rules to Kubernetes status with zero model calls.

| Alert | Opens when |
| --- | --- |
| Stalled rollout | A Deployment reports `ProgressDeadlineExceeded` for its current rollout |
| Low availability | Two consecutive checks find `Available=False` on a Deployment or a DaemonSet availability deficit beyond its rolling-update allowance |
| Container crash loop | Two consecutive checks find `CrashLoopBackOff` for an owned container |
| No ready endpoints | Two consecutive EndpointSlice checks find zero ready endpoints for a selected Service |

Incomplete reads show coverage gaps and preserve alerts that depend on the missing data.

Pod logs are read on demand. If an application emits errors while Kubernetes still reports it as ready, the scheduled status checks can miss the problem. Use the Pod's Logs view or [plous-debug](../agent-skills/plous-debug.md) to inspect the affected container, including its previous instance after a restart.

<a name="manage-watches"></a>

### Manage monitors

| Command | Result |
| --- | --- |
| `plous monitor list` | Show saved monitors and whether they are running |
| `plous monitor status payments` | Show scope, the last complete check, the next scheduled check, and open alerts |
| `plous monitor stop payments` | Stop this monitor and wait for shutdown |
| `plous monitor start payments` | Resume this monitor with its saved settings |
| `plous monitor start payments --every 10m` | Resume with a different interval and keep the saved scope and alerts |
| `plous monitor remove payments` | Delete a stopped monitor's saved settings and alerts after confirmation |

Changing a stopped monitor's scope shows both scopes and asks for replacement confirmation. Enter cancels. Confirmation clears its saved alerts. `--replace` supplies approval for scripts.

`stop` waits up to five seconds for shutdown and reports an error if the wait times out. After it succeeds, `remove` or a replacement can follow immediately. See [retention and cleanup](data-and-privacy.md#monitor-data).

### Scheduling and coverage

Each monitor runs in its own foreground process and prints alert and coverage changes. It checks immediately, then repeats on its interval. When a check finds no objects and reports a coverage gap, Plous backs off to a maximum of one hour between retries. The next check time includes that backoff. A check is overdue if it is still outstanding 30 seconds after its scheduled time. Earlier monitor processes can show **Next check unknown** until restarted with the current build.

For unattended use, configure your operating system's process supervisor to run `plous monitor start NAME` with access to its kubeconfig. The supervisor starts and can restart the process. Plous runs the checks while the process is up. Time spent offline is a gap in coverage. See [saved monitor data](data-and-privacy.md#monitor-data).

## AI connections

Configure OpenAI or Anthropic under **Application → Agent → Plous → Add provider connection**. Saving a provider does not switch the active agent.

Type `/model` afresh in the Plous composer to choose a model. The selection applies across workspaces and is fixed for each response when it starts.

Claude Code and Codex use their own authentication and model settings. Edit **Entry command** to use a wrapper such as `celai codex`. Saving a different agent or command closes the native session after saving its evidence.

Provider options include a display name, API proxy, environment variable for the key, and custom instructions. A saved key takes precedence over an environment reference. Proxies require HTTPS except on loopback.

See [Plous agent setup](../guides/plous-agent.md), [credentials](data-and-privacy.md#credentials), and [active diagnostics](../mcp/active-diagnostics.md).

## Agent runtime configuration

Optional configuration lives at `<data-dir>/agent/config.yaml`. Set `PLOUS_AGENT_CONFIG` to use another file.

Defaults:

```yaml
max_calls: 128
max_tokens: 1000000
timeout_seconds: 900
```

These positive limits apply to one built-in Plous response, including context compaction:

| Setting | Limit |
| --- | --- |
| `max_calls` | Model calls |
| `max_tokens` | Total tokens used by the turn |
| `timeout_seconds` | Elapsed time |

They do not change the model's context capacity. Plous does not automatically retry with another provider or model. Remove retired `history_mode` and `rlm` settings, which are rejected.

Native Claude Code and Codex sessions manage their own calls, context, retries, and spending. These turn limits do not apply to them.

For optional semantic history search, set both `embedding.connection` and `embedding.model`. The connection is independent of chat selection. Without embeddings, Monitor history uses lexical search. General history always uses lexical search.

Keep provider keys in Settings or an environment-variable reference, not this file.

## Remembered preferences

Plous saves panel visibility, assistant size, tree folds, conversations, and drafts. Each connection also remembers its Explore namespace, resource type, filters, and sorting.

Each launch starts in Overview for the terminal's current context. Text searches, open resource content, unsaved edits, and chart samples are temporary.

The layout adapts to terminal size. There is no user-configurable layout or theme file. See [Data and privacy](data-and-privacy.md) for stored files.

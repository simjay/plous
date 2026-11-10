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

Profiles have separate settings, credentials, workspaces, and conversations. They can still connect to the same cluster. Only one process can open a profile at a time.

For agents outside Plous, see [plous-debug setup](../agent-skills/plous-debug.md) or [MCP setup](../mcp/setup.md).

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

See [connection fields](../integrations/README.md#saved-tool-connections). Saved changes appear in [Activity](../guides/activity.md).

## Kubernetes connections

Saved connections keep a kubeconfig path, context, and optional default namespace. Setup starts with `~/.kube/config`. After editing the path, select **Refresh Contexts** before adding a context.

Current Workspace uses the terminal's `KUBECONFIG` path list, or `~/.kube/config` if unset. It follows Kubernetes merge rules. Choosing another kubeconfig in Current Workspace Settings lasts for that session.

Plous notices external context changes but does not switch an open view. Selecting a namespace in Explore does not edit kubeconfig or change another client's context.

Authentication uses the referenced kubeconfig and its plugins. Plous does not copy those credentials. See [connection setup](../integrations/kubernetes/README.md#connect-a-cluster).

## AI connections

Configure OpenAI or Anthropic under **Application → Agent → Plous → Add provider connection**. Saving a provider does not switch the active agent.

Type `/model` afresh in the Plous composer to choose a model. The selection applies across workspaces and is fixed for each response when it starts.

Claude Code and Codex use their own authentication and model settings. Edit **Entry command** to use a wrapper such as `celai codex`. Saving a different agent or command closes the native session after saving its evidence.

See [AI setup](../guides/ai-chat.md), [credentials](../guides/ai-chat.md#credentials-and-advanced-settings), and [active diagnostics](../mcp/active-diagnostics.md).

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

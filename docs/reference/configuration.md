# Configuration

[Documentation](../README.md)

Use Settings for saved configuration and command-line flags to choose a profile at launch. Plous manages its data files; editing revision records by hand is not a configuration interface.

## Command-line flags

Run these in your shell, outside Plous:

| Flag | Behavior |
| --- | --- |
| `--version` | Print the embedded application version and exit. |
| `--data-dir PATH` | Use an independent data directory instead of `~/.plous/`. |
| `--help` | Print command-line usage and exit. |

```sh
./bin/plous --data-dir "$HOME/plous-profiles/testing"
```

Profiles have separate workspaces, credentials, conversations, and preferences. They can still point to the same cluster. Only one process can open a profile at a time.

## Settings

Press **ss** while navigating, or run `:s`.

| Setting | Scope | Guide |
| --- | --- | --- |
| Main agent: Plous, Claude Code or Codex | Application profile; changed in Settings | [Choose an agent](../guides/ai-chat.md#choose-an-agent) |
| Mouse support | Application profile; turn off for native terminal selection | [Mouse controls](keymap.md#mouse-controls) |
| Plous provider connections and credentials | Application profile; Settings → Agent → Plous | [AI setup](../guides/ai-chat.md) |
| Selected Plous model | Application profile; changed in the composer | [Choose a model](../guides/ai-chat.md#choose-a-model) |
| Workspace name and tool connections | Selected saved workspace | [Workspaces](../guides/workspaces.md) |
| Workspace location | Local; Remote is disabled | [Workspaces](../guides/workspaces.md) |

Select **Save** to apply a form. Esc cancels to its preceding menu. Exiting the application does not apply unfinished edits.

## Tool connections

Connection fields belong to the selected tool. A workspace can combine context references, working directories, and repositories. See [Tool integrations](../integrations/README.md#saved-tool-connections) for the required and optional fields. Saved connection changes appear in [Activity](../guides/activity.md).

## Kubernetes connections

Each connection retains its kubeconfig path, context, and optional namespace. Setup starts with `~/.kube/config`; choose **Refresh Contexts** after changing that path. The shown file takes precedence over the shell's `KUBECONFIG` setting during setup.

Current Workspace resolves the inherited `KUBECONFIG` path list using Kubernetes merge rules, otherwise `~/.kube/config`. A path chosen in Current Workspace Settings lasts for the session. Saved workspace setup still reads the explicit file shown in its form. External default changes are noticed without switching the selected target. Code captures the process working directory at launch and keeps it fixed for the session. It uses the exact directory, without searching upward for a Git root.

The connection's default namespace and Explore's selected namespace are separate. Changing the picker updates the browsing scope for that connection. It does not rewrite kubeconfig or change another client's context.

Live access uses the credentials and authentication plugins in the referenced kubeconfig. Plous does not copy those credentials into its profile. See [connection setup](../integrations/kubernetes/README.md#connect-a-cluster) and [connection troubleshooting](troubleshooting.md).

## AI connections

Open **Settings → Agent**, select **Plous**, and choose **Add provider connection** to configure OpenAI or Anthropic. Provider setup is shown only for Plous; external agents show their installed CLI status. Previewing an agent or saving a provider does not change the active agent. Advanced fields include a display name, API Proxy URL, environment-variable credential reference, and custom instructions. A proxy does not add models to the supported catalog.

With Plous selected, use `/model` or `/m` as a freshly typed, complete message in the composer to select a model. The selection applies across workspaces. Chat captures only the active connection and the selected page when you send a message; see [AI chat](../guides/ai-chat.md) for its contents and read-only tool scope. Optional embeddings and shared execution limits are documented in [agent runtime configuration](#agent-runtime-configuration).

Claude Code and Codex use their installed CLI's authentication and default model. The saved Agent choice applies across workspaces and survives restart. Saving another agent closes the current native terminal and saves its tool evidence before applying the choice. An unavailable executable never selects another agent automatically. Plous applies tool restrictions for the captured session without rewriting global CLI settings. See [agent setup](../guides/ai-chat.md#choose-an-agent).

## Agent runtime configuration

Optional YAML lives at `<data-dir>/agent/config.yaml`; `PLOUS_AGENT_CONFIG` selects another file. Settings are captured once per turn. Defaults are:

```yaml
max_calls: 128
max_tokens: 1000000
timeout_seconds: 900
```

For the built-in Plous agent, main and tool-free context compaction share these positive limits. `max_calls` bounds model calls, `max_tokens` bounds aggregate turn token use, and `timeout_seconds` bounds elapsed time. These are turn allowances, not the selected model's context capacity. No automatic provider retry or fallback model occurs. Remove old `history_mode` and `rlm` settings; they are rejected as unknown rather than silently ignored.

All three turn budgets apply only to the built-in Plous agent. Native external agents have no Plous-imposed process deadline or session-wide Plous tool-call cap. CLI-native authentication, model calls, compaction, provider retries and spending remain with the executable.

Existing optional Monitor embeddings use `embedding.connection` and `embedding.model` together, referencing a saved provider connection independently of chat selection. Leaving them unset uses lexical history search. Code and General are lexical only. Embeddings are not required for the baseline.

Provider keys belong in Settings or an environment-variable credential reference, not this YAML file. See [AI chat](../guides/ai-chat.md#credentials-and-advanced-settings) for credentials and [Data and privacy](data-and-privacy.md) for storage.

## Remembered preferences

Plous starts on Overview for the terminal's current kubeconfig context on every launch. It preserves panel visibility, tree folds, conversations, and connection-specific drafts. Explore also remembers each connection's namespace, resource type, filters, and sort order.

Applied resource-table filters are saved. Panel text searches, unfinished filter edits, open resource content, and unsaved form or resource edits are temporary. Chart samples, ranges, scales and inspection selections are also temporary. The panel layout adapts to terminal size; there is no user-configurable layout or theme file. See [navigation](../guides/navigation.md) for panel controls and [Data and privacy](data-and-privacy.md) for storage details.

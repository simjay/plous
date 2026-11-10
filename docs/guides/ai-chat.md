# Set up AI chat

[Documentation](../README.md) · [Claude Code and Codex](plous-mcp.md)

The right panel can run Plous, Claude Code, or Codex. Choose an agent in Settings. You can use the rest of Plous without AI.

| Agent | What you need |
| --- | --- |
| Plous | An OpenAI or Anthropic API key |
| Claude Code | An installed and authenticated `claude` CLI |
| Codex | An installed and authenticated `codex` CLI |

## Choose an agent

1. Select the cluster you want to investigate.
2. Press `ss` or type `:s` to open Settings.
3. Open **Application → Agent**.
4. Use Left / Right to choose an agent, then **Save**.

Browsing choices does not switch agents. Save applies your choice across workspaces and restarts. Switching away from a native agent closes its session after saving its tool evidence.

### Executables and authentication

Claude Code and Codex use your installed CLIs and their own sign-in. Plous does not install them or copy their credentials.

To use a wrapper, edit the agent's **Entry command** in Settings. For example, `celai codex` or `"/opt/Company Tools/celai" claude`.

The first word must name an installed executable. Plous runs it directly without a shell. Aliases, environment expansion, pipes, and redirects do not work. Wrappers must forward the CLI's launch arguments.

If the command is unavailable, Plous reports an error and keeps your agent selection. See [native panel setup](plous-mcp.md#use-the-assistant-panel).

### Use the native terminal

Press `cc` to enter the CLI. Use Alt+Left / Alt+Right to move between panels, or Ctrl+] to leave and return to the agent.

Ctrl+C goes to the focused native CLI. To exit Plous, press Ctrl+] first, then Ctrl+C. See the [native terminal controls](plous-mcp.md#use-the-native-terminal).

### Session scope and saved evidence

A native session keeps the cluster it opened with. To change clusters, [close and reopen the session](plous-mcp.md#switch-connections-or-close-a-session).

The CLI stores its conversation. Plous separately saves the tool results used during that session. See [saved evidence](plous-mcp.md#inspect-saved-evidence).

## Connect a vendor

For the built-in Plous agent:

1. Open **Settings → Application → Agent** and choose **Plous**.
2. Select **Add provider connection**.
3. Choose OpenAI or Anthropic, enter the API key, and save.
4. Save the Plous agent choice, close Settings, and press `cc`.

You can save several provider connections. Adding a provider does not switch the active agent or replace an existing model selection.

## Choose a model

With Plous selected, type `/model` or `/m` as the whole message and press Enter. Select a model and press Enter again. Esc keeps the current choice.

| Provider | Models in Plous | Initial choice |
| --- | --- | --- |
| OpenAI | GPT-5.4 mini, GPT-5.4 | GPT-5.4 mini |
| Anthropic | Claude Haiku 4.5, Claude Sonnet 5.5, Claude Opus 5.5 | Claude Sonnet 5.5 |

The catalog comes from Plous, not your account. Your provider must permit the model. An unavailable saved model requires a new selection. Plous does not silently substitute another.

The selection applies across workspaces. Changing providers sends subsequent request context to that provider.

Paste does not run model commands. Type the command afresh to open the picker, or send `//model` to include `/model` literally. Native agents handle their own model commands.

## Workspace context

Check the assistant header before sending. Each response uses the selected cluster as it was when the response started. Changing views does not redirect a running response.

Reads can use other namespaces permitted by the same Kubernetes credentials. They cannot use another cluster. General chat and archived histories have no live cluster tools.

Try:

> Why is this Pod restarting? Check its status, events, and the last 100 log lines. Tell me what you found and what is still uncertain.

Agents read by default. Container commands and temporary tests require [active diagnostics](../mcp/active-diagnostics.md). Resource edits, scaling, deletion, and deployments remain user actions.

## Ask the assistant to show a view

Inside Plous, all three agents can open views. For example:

- “Show this Deployment's YAML.”
- “Open the app container's logs.”
- “Compare Helm release revisions 2 and 3.”

Use Alt+Left / Alt+Right to enter the opened view. If a form or editor blocks the request, finish it and ask again.

Live views may read the cluster again. Sources shows the saved result that supported the original answer.

## Compose and navigate

With Plous selected, press `cc` or `:c` to type. Enter sends. Shift+Enter or Ctrl+J adds a newline. Esc leaves typing and keeps the draft and running response.

In conversation navigation, use arrows to scroll, Page Up / Down for a page, and Home / End for the oldest / latest output. Press `/` to find text.

Use `:A` or `:readreply` for the wider reply reader. It supports search, text selection, copying, and source mode. Esc returns to the conversation.

## Inspect a reply's sources

1. Press Esc to leave typing.
2. Select a reply with `[` / `]`.
3. Press Enter for **Sources** or **Reply details**.
4. Select a source and press Enter to read its saved result.

Sources retains the original observation time and any missing or incomplete data. It does not make a new cluster or provider request. Sources survives restart.

Check the observation time before treating a saved result as current. Esc returns through the source details to your reply and draft.

## Manage conversations

| Command | Action |
| --- | --- |
| `:c` / `:chat` | Open the composer |
| `:C` / `:newchat` | Start a new conversation |
| `:H` / `:openchat` | Browse history |
| `:X` / `:stopchat` | Stop a response |

One response runs at a time. Moving panels or hiding the assistant does not stop it. New conversations keep earlier history.

Each cluster connection has its own conversation and draft. **Choose history scope** opens other saved histories for reading without granting cluster tools. Use `/` to filter titles and dates.

## Limits and recovery

Plous shows a local context estimate. It can shorten older exchanges in the active context while keeping the originals in history. The default response timeout is 15 minutes. See [runtime configuration](../reference/configuration.md#agent-runtime-configuration).

Claude Code and Codex manage their own context, model calls, and spending. Plous does not impose a session-wide deadline on them.

For a failed Plous response, read the error, fix its cause, and send again. `:p` restores the last prompt into an empty draft. Accepted text stays visible.

If saving fails, copy the answer with the reply reader and resolve the storage problem. A save failure during exit keeps Plous open. Native evidence saves can be retried with `:X`.

## Credentials and advanced settings

Edit or remove providers in Settings. A blank key keeps the stored value. A replacement key changes it. Removing a provider does not delete conversations.

Advanced fields include a display name, API proxy, environment-variable key reference, and custom instructions. A saved key takes precedence over its environment reference. Proxies require HTTPS except on loopback and do not expand the model catalog.

Keys are stored as plaintext in private `credentials.json`. Prompts, tool results, and retrieved history go to the selected provider. Read [Data and privacy](../reference/data-and-privacy.md) before sending sensitive cluster data.

## Next steps

Try [Investigate a restarting Pod](investigate-restarting-pod.md), then inspect the answer's Sources. For an agent outside Plous, install [plous-debug](../agent-skills/plous-debug.md).

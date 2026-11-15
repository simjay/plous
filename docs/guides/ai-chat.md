# Use the assistant in Plous

The right panel can run Plous, Claude Code, or Codex. Select a cluster, open **Settings → Application → Agent** with `ss`, then choose and save your agent.

| Agent | What you need |
| --- | --- |
| Plous | An API key from OpenAI or Anthropic |
| Claude Code | An installed and authenticated `claude` CLI |
| Codex | An installed and authenticated `codex` CLI |

Use [plous-debug](../agent-skills/plous-debug.md) if you want to stay in your coding agent outside Plous.

## Connect the Plous agent

1. Choose **Plous → Add provider connection** under **Agent**.
2. Select a vendor, enter its API key, and save the connection.
3. Save the agent choice and press `cc` to start typing.

To choose a model, type `/model` as the whole message and press Enter. Choose from the configured connections. This selection applies across workspaces. See [provider settings](../reference/configuration.md#ai-connections) and [data and privacy](../reference/data-and-privacy.md#ai-requests) for how keys and requests are handled.

## Run Claude Code or Codex

Save your CLI as the selected agent, check the cluster in the header, then press `cc` to enter its terminal. Plous supplies debugging instructions and cluster tools automatically. The CLI keeps its own sign-in, model settings, and conversations. It starts in the directory where you launched Plous.

Use **Entry command** for an executable or wrapper such as `celai codex`. Plous starts it directly. The wrapper must forward launch arguments.

- **Leave or return to the terminal:** Press Ctrl+] or move to another panel with Alt+Left or Alt+Right. Hiding the panel leaves the session running.
- **Interrupt or quit:** Ctrl+C in the terminal goes to the CLI. Press Ctrl+] first, then Ctrl+C to exit Plous.
- **Change clusters:** Leave the terminal with Ctrl+], close it with `:X`, select the new cluster, and press `cc` to start a session for that cluster.

If the CLI fails to open, check its entry command, PATH, and native sign-in. For tool compatibility or startup errors, see [assistant troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send) and [MCP connection details](../reference/mcp.md#mcp-connection-details).

## Investigate a workload

Check the assistant header, then select a Pod or name its namespace and workload in your request.

> Why is this Pod restarting? Check its status, recent events, and the failing container’s logs. Tell me what you observed and what still needs checking.

Ask “Open the failing container’s logs in Plous” to inspect the result yourself. Use Alt+Left to move from the assistant to main. Finish any open form or editor if it blocks the requested view.

A Plous reply uses the cluster selected when it starts. A native session keeps the cluster it opened with. Reads can span namespaces allowed by your Kubernetes credentials. [Tool permissions](../reference/mcp.md#scope-and-delivery) and [active diagnostics](../mcp/active-diagnostics.md) explain what the agent can access.

## Inspect a reply’s sources

For Plous chat, press Esc to leave typing, use `[` and `]` to choose a reply, then press Enter for **Sources**. It opens the saved observation, including its timestamp and any missing data. Opening a live view can fetch newer data.

For a native session, close it with `:X`, select **Plous** in Settings, and open history with `:H`. Choose **Claude Code tool evidence** or **Codex tool evidence** for that cluster. Plous saves tool results. Native prompts and replies stay with the CLI.

## Manage conversations

| Key | Action in Plous chat |
| --- | --- |
| `cc` | Start typing. Enter sends, Shift+Enter adds a newline, and Esc keeps the draft. |
| `:C` / `:H` | Start a new conversation or open history |
| `:X` | Stop a response |
| `:A` | Open the wider reply reader |
| `:p` | Restore the last prompt into an empty draft |

Moving panels leaves the response running. Drafts and conversations are saved per cluster connection. See [assistant controls](../reference/keymap.md#ai-assistant) or try the [restarting Pod exercise](investigate-restarting-pod.md).

# Use an agent with Plous

Run an agent beside your cluster views in Plous, or [connect your coding agent over MCP](../agent-skills/plous-debug.md) to investigate from your repository.

## Choose an agent

| Agent and setup | Where it runs |
| --- | --- |
| [Plous](plous-agent.md) | Assistant panel. Requires an OpenAI or Anthropic API key. |
| [Claude Code](../agent-skills/clients/claude.md) | Assistant panel or standalone MCP |
| [Codex](../agent-skills/clients/codex.md) | Assistant panel or standalone MCP |
| [Antigravity CLI](../agent-skills/clients/agy.md) | Standalone MCP |
| [Cursor Agent](../agent-skills/clients/cursor.md) | Standalone MCP |
| [Gemini CLI](../agent-skills/clients/gemini.md) | Standalone MCP |
| [Grok Build](../agent-skills/clients/grok.md) | Standalone MCP |
| [OpenCode](../agent-skills/clients/opencode.md) | Standalone MCP |

## Run Claude Code or Codex

The panel starts the CLI in the directory where you launched Plous. The CLI uses its own sign-in, model settings, and conversation storage. Plous connects its tools automatically.

- **Move between panels.** Press `Ctrl+]` to return to the previous panel or press it again to focus the terminal. `Alt+Left` and `Alt+Right` cycle through panels. Hiding the assistant leaves the session running.
- **Interrupt or exit.** `Ctrl+C` in the terminal goes to the CLI. To exit Plous, press `Ctrl+]` first, then `Ctrl+C`.
- **Change clusters.** Press `Ctrl+]` and close the session with `:X`. Select the new cluster and press `cc` to open a session there.

If the terminal fails to open, check the CLI’s sign-in, PATH, and **Entry command**. The agent’s setup page shows its default command. See [assistant troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send).

## Investigate a workload

Check the cluster in the assistant header. Select a Pod or name the namespace and workload in your request.

> Why is the checkout Pod restarting in staging/shop? Check its status, recent events, and the failing container’s logs. Tell me what you found and what still needs checking.

To inspect the result yourself, ask “Open the failing container’s logs in Plous.” Press Alt+Left to move from the assistant to the main panel. An unfinished form or editor can block a view change.

The built-in agent captures the selected cluster at the start of each reply. A native session keeps its starting cluster until you close it. Reads can span namespaces allowed by your Kubernetes credentials. See [tool permissions](../reference/mcp.md#scope-and-delivery) and [active diagnostics](../mcp/active-diagnostics.md).

## Inspect a reply’s sources

In built-in Plous chat, press Esc to leave typing, select a reply with `[` or `]`, and press Enter to open **Sources**. It shows the saved observation, its timestamp, and any gaps. Use [Plous chat controls](plous-agent.md#manage-conversations) to navigate conversations.

To read a native session’s tool evidence, close it with `:X`, select **Plous** in Settings, and open history with `:H`. Choose **Claude Code tool evidence** or **Codex tool evidence** for the cluster. The CLI keeps its prompts and replies.

Standalone MCP uses its own [evidence profile](../mcp/setup.md#saved-evidence).

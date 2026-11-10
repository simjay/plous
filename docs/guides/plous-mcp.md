# Use Claude Code or Codex inside Plous

[Documentation](../README.md) · [AI chat](ai-chat.md)

Run your installed Claude Code or Codex CLI in the right panel. Plous connects it to cluster tools automatically.

For agents running outside Plous, use [MCP setup](../mcp/setup.md) or [plous-debug](../agent-skills/plous-debug.md).

<span id="use-plous-mcp"></span>

## Use the assistant panel

1. Install and sign in to `claude` or `codex`.
2. Select the cluster you want to investigate in Plous.
3. Open **Settings → Application → Agent** with `ss`.
4. Choose Claude Code or Codex and save.
5. Check the cluster name in the assistant header, then press `cc` to type.

The CLI starts when its panel is visible. It uses its own authentication and model settings. You do not need a Plous API key.

For a wrapper such as `celai codex`, edit **Entry command** in the same Settings view. See [agent setup](ai-chat.md#choose-an-agent).

## Use the native terminal

The CLI owns its prompt, Tab, Esc, slash commands, and conversation menus.

| Key | Action |
| --- | --- |
| `cc` from navigation | Enter the terminal or reopen a closed session |
| Alt+Left / Alt+Right | Move between Plous panels |
| Ctrl+] | Leave the agent or return to it |
| Ctrl+C in the terminal | Send Ctrl+C to the CLI |
| Ctrl+] then Ctrl+C | Save and exit Plous |
| `:X` from navigation | Close the native session |
| `:C` from navigation | Start a new native conversation |

Clicking another panel moves focus. Clicking the agent resumes the same session. Hiding or resizing the panel leaves it running.

### Read earlier output

Use the wheel over the panel or Page Up / Down. Normal-screen history keeps up to 5,000 rows in memory.

Press End or Esc, scroll to the bottom, or start typing to return to the live prompt. Fullscreen CLI screens handle their own history.

### Conversation storage and working directory

The CLI runs in the directory where you launched Plous. It keeps its own conversation files and credentials.

Plous associates sessions with the agent, launch directory, and cluster. Reopening can resume that conversation. Use the CLI's `/resume` for its history.

Plous saves tool results separately. It does not copy native prompts and replies into Plous chat.

## Try an investigation

Select a Pod and ask:

> Why is this Pod restarting? Check its status, recent events, and the failing container's logs.

Then ask:

> Show the relevant logs in Plous.

You can also ask for current resource usage, a Helm revision comparison, or an Argo CD or Flux failure. Include the resource name and namespace when the current page does not identify them.

## Skills, tools and MCP

MCP connects the CLI to Plous's tools. Plous supplies debugging instructions automatically, so the panel needs no separate skill installation.

Hosted sessions disable repository tools, delegation, personal skills, plugins, hooks, and unrelated MCP servers. The CLI's native sign-in and slash commands remain available. Global CLI settings stay unchanged.

Use [plous-debug](../agent-skills/plous-debug.md) outside Plous when you want your agent's normal repository tools as well as cluster tools.

## Available tools

| Task | Examples |
| --- | --- |
| Read cluster data | Resources, events, logs, metrics, Helm releases, Argo CD and Flux status |
| Read saved evidence | Earlier tool results and workspace Activity |
| Open a Plous view | Resource details, lists, logs, Helm comparisons, saved evidence |

The [MCP reference](../reference/mcp.md#available-tools) lists tool names and limits.

Opening a view keeps your keyboard focus and draft. An unfinished form or editor can block the request.

## Inspect saved evidence

1. Press Ctrl+] to leave the agent, then `:X` to close its session.
2. Select **Plous** in Settings.
3. Open history with `:H`.
4. Choose **Claude Code tool evidence** or **Codex tool evidence** for that connection.
5. Select a reply with `[` / `]`, then press Enter for Sources.

Sources reads saved results without contacting the cluster again. See [Sources controls](ai-chat.md#inspect-a-replys-sources).

## Switch connections or close a session

A running session keeps its original cluster even when you browse another connection.

To change the agent's target:

1. Press Ctrl+], then `:X`.
2. Select the intended cluster.
3. Press `cc` and check the new session's header.

Closing the session or exiting Plous ends its MCP access. Moving panels, opening Help, or hiding the assistant does not close it.

## Scope, access and limits

Cluster reads can use any namespace allowed by your Kubernetes credentials. They cannot access a different cluster. General chat and archived histories have no live tools.

Agents read by default. Start Plous with `--allow-exec` for container commands or `--diagnostic-image IMAGE` for temporary tests. These operations stay in the selected default namespace. See [Active diagnostics](../mcp/active-diagnostics.md).

Resource editing, deployment, rollback, scaling, and deletion remain user actions.

Plous limits individual tool operations. The CLI manages its model context, retries, and spending. There is no Plous-imposed session deadline.

The local connection stays on loopback, but cluster data can go to the CLI's provider. See [Data and privacy](../reference/data-and-privacy.md).

## Troubleshoot the connection

| Problem | Action |
| --- | --- |
| Executable unavailable | Check Entry command and the PATH inherited by Plous |
| Sign-in required | Use the CLI's native authentication |
| Tools use the previous cluster | Close the session, choose the cluster, and reopen |
| No live tools | Select a cluster connection |
| Requested view blocked | Finish the form or editor and ask again |
| Tool access stopped | Fix the reported error, close with `:X`, and reopen with `cc` |
| Evidence cannot be saved | Fix the storage error and retry `:X` |
| Launch options rejected | Check the installed CLI's compatibility and the full startup error |

Native launch checks used Claude Code 2.1.284 and codex-cli 0.160.0 with mock providers. Other versions must support the launch options and tool controls. There is no exact Codex version allowlist.

See [MCP connection details](../reference/mcp.md#mcp-connection-details) or [AI troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send).

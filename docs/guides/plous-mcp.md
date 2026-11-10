# Use Claude Code or Codex inside Plous

[Documentation](../README.md) · [AI chat](ai-chat.md) · [MCP reference](../reference/mcp.md) · [Data and privacy](../reference/data-and-privacy.md)

This guide covers the native Claude Code or Codex terminal inside Plous's right assistant panel. Plous configures its MCP connection automatically, supplying scoped cluster reads, saved evidence, and actions to open Plous views.

The TUI bridge is private to the active native session. For agents running independently, [configure MCP](../mcp/setup.md) directly or [install plous-debug](../agent-skills/plous-debug.md) for guided debugging. The built-in Plous agent calls the same guarded tools directly.

<span id="use-plous-mcp"></span>

## Use the assistant panel

You need Plous and an installed CLI. Entry commands default to `claude` and `codex`; configure enterprise wrappers such as `celai codex` in **Settings → Application → Agent → Entry command**. The command's executable must be on PATH or specified by path. The external CLI uses its own authentication and model selection; it needs no API key in Plous Settings. See [agent requirements and tested versions](ai-chat.md#choose-an-agent).

1. **Select what to investigate.** Open the intended Monitor connection in the sidebar.
2. **Choose your agent.** Open **ss** or `:s`, select **Application → Agent**, choose Claude Code or Codex with Left/Right, and Save. Saving closes any current native session and saves its tool evidence before starting the selected agent. From a captured terminal, use Alt+Left to reach the main panel or Ctrl+] to return to the previous panel before opening Settings.
3. **Check the scope.** The native terminal starts automatically when its panel is visible. Its heading identifies the captured connection. Plous creates and configures the private MCP connection at startup.
4. **Ask a question.** Press **cc** to enter the native prompt. Name the object and the evidence you want, then submit using the CLI's controls.
5. **Follow the evidence.** Ask the agent to show YAML, logs, or a comparison. **Alt+Left/Right** moves into the opened view and back to the conversation.

If an external agent was already running when you selected a different cluster, [close and reopen its session](#switch-connections-or-close-a-session) to capture that target. Changing the visible page alone does not change its tools.

## Use the native terminal

The selected native terminal starts automatically when its panel is visible, including after restarting Plous. Startup waits for the initial connection selection and preserves panel focus. Press **cc** or `:c` to enter it or reopen a closed session. The CLI owns Tab, Esc, slash commands, and its composer. **Alt+Left/Right** moves panels directly and resumes native input when you return. With mouse support on, clicking another panel moves focus; clicking the native panel resumes the same session.

**Ctrl+]** toggles focus between the agent and the previous panel. **Ctrl+C** goes to the focused native terminal for its own interrupt behavior; while idle, the CLI may clear input or exit instead. To save and exit Plous, press **Ctrl+]**, then **Ctrl+C**. From Plous navigation, `:X` closes the native session. **cc** then resumes its latest conversation in the captured scope; `:C` opens a new one. Use the CLI's `/resume` for its history.

If there is no saved conversation, **cc** opens a fresh prompt, including after closing before your first message. If Claude Code reports a saved session ID with no conversation, Plous retries once with a fresh session. Other startup errors remain visible.

### Read earlier output

Use the mouse wheel over the native panel or **Page Up/Down**. Plous retains up to 5,000 normal-screen history rows in memory and holds your reading position while the CLI runs or the panel resizes. Scroll to the bottom, press **End** or **Esc**, or start typing/pasting to return to the live prompt. Up/Down belongs to the CLI while typing. Fullscreen CLI views manage their own history; wheel events reach them when they enable mouse input.

### Conversation storage and working directory

Both CLIs run in the directory where you launched Plous. Session metadata is separate for each agent, launch directory, and captured cluster, so resuming does not select another scope's conversation. The CLI retains authentication, model choice, and conversation files. Plous reconnects its private tool bridge on resume and stores projected tool evidence separately, without copying native prompts or replies into Plous chat.

## Try an investigation

Adapt these prompts to your selected resources:

| Start from | Ask |
| --- | --- |
| A Pod in Monitor | “Why is this Pod restarting? Check status, recent events, and the last 100 log lines from the failing container. Separate observations from likely causes.” |
| A Deployment | “What is its current CPU and memory usage? Tell me when it was measured and whether every active Pod is covered.” |
| A Helm release with retained revisions 2 and 3 | “Compare revision 3 with revision 2. Show the values comparison and summarize the recorded resource changes.” |
| An Argo Application or Flux Kustomization | “What does the controller report as failing? Check the referenced objects available in this cluster and explain any gaps.” |
| A connection with earlier saved observations | “Compare these two saved observations. Which resource identities changed, and is the comparison complete?” |

For a first investigation, select a Pod, ask the first question, then ask “Show the relevant logs in Plous.” The opened viewer gives you a place to inspect the evidence yourself. Opening live logs can make a fresh read; opening saved Sources reads the original recorded observation.

## Skills, tools and MCP

[MCP](https://modelcontextprotocol.io/docs/learn/architecture) carries tool discovery and calls between the native CLI and Plous. Tools are executable operations with structured inputs; skills are workflow instructions that can guide an agent's choices. Inside the TUI, Plous supplies embedded debugging instructions and tool descriptions, so no skill installation is needed. Independent coding agents can install the same workflow as [plous-debug](../agent-skills/plous-debug.md).

Native launch settings disable built-in access and delegation tools, personal skills, plugins, hooks, and unrelated MCP servers for these sessions. The CLI keeps its own prompt, authentication, model controls, and conversations. Global CLI settings stay unchanged.

Native onboarding, sign-in, and built-in slash commands such as Claude Code's `/login` remain available. Model-invoked skills and skill shell execution are disabled separately from those commands. Independent coding agents using [plous-debug](../agent-skills/plous-debug.md) retain their own repository tools.

## Available tools

You can ask for information or ask the assistant to show it in the main panel:

| Capability | Examples |
| --- | --- |
| Read live cluster evidence | Kubernetes resources, events, bounded logs, current usage, Helm releases, Argo and Flux status |
| Revisit saved work | Scoped conversation history, saved observations, workspace configuration and Activity metadata |
| Open a view | Resource details, filtered lists, logs, Helm revision comparisons and saved evidence |

The available tools depend on the captured scope. The [MCP reference](../reference/mcp.md#available-tools) lists the exact tool names, argument conventions, and evidence limits. Opening a view preserves keyboard focus and unfinished work; a form or editor can block the request. UI tools return an `opened`, `blocked`, or `failed` receipt.

Read tools are bound to the captured cluster. History tools inspect saved observations. Detailed [Monitor](../reference/mcp.md#monitor-evidence) and [Historical](../reference/mcp.md#historical-evidence) coverage is in the reference.

## Inspect saved evidence

Plous saves tool observations before delivering them to the agent, in a **Claude Code tool evidence** or **Codex tool evidence** conversation. Native prompts and replies remain in the CLI's own storage.

1. Press **Ctrl+]** to leave the agent panel, then `:X` to close the session.
2. Select **Plous** in Settings.
3. Open `:H` / `:openchat` and select the external tool-evidence entry in that scope.
4. Select a reply with **[ / ]**, then Enter to open **Sources** or **Reply details**.

Sources uses saved turn-bound observations and makes no new cluster or provider request. It survives restart and retains the original observation time and coverage. See [Sources controls](ai-chat.md#inspect-a-replys-sources).

## Switch connections or close a session

A native session keeps its captured target even while you browse another connection. To investigate a different one:

1. Press **Ctrl+]**, then `:X` to close the native session.
2. Select the intended cluster.
3. Press **cc** to open a session for that scope and check its heading.

Hiding or resizing the assistant, opening Help, or moving panels leaves the process running. Closing the session or exiting Plous closes its MCP access. Reopening reconnects tools and can resume the CLI conversation associated with that agent, launch directory, and scope. Use `:C` for a new native conversation.

## Scope, access and limits

Plous agent tools default to reads. Launching with `--allow-exec` enables bounded commands in existing containers; `--diagnostic-image IMAGE` enables temporary in-cluster tests. These capabilities can affect cluster or application state and are restricted to the captured default namespace. See [active debugging](../mcp/active-diagnostics.md). Deployment, rollback, and general resource editing remain human-operated [resource actions](../integrations/kubernetes/README.md#resource-actions).

The selected namespace is a page hint, not an access boundary: cluster reads may use any namespace allowed by the captured credentials. Naming another cluster or finding a repository link cannot expand access. General chat and retained history-only views grant no live connection tools.

Each read has operation and output limits. Native sessions have no Plous-imposed process deadline or session-wide tool-call cap; model context and spending remain CLI-managed. See [scope and delivery](../reference/mcp.md#scope-and-delivery) for the contract.

The TUI MCP traffic stays on loopback, but requested evidence can be sent to the CLI's model provider. Cluster credentials remain with Plous. File and log excerpts can contain sensitive text; see [Data and privacy](../reference/data-and-privacy.md).

## Troubleshoot the connection

| Symptom | What to check |
| --- | --- |
| Agent executable unavailable | Check the agent's Entry command in Settings. Its executable must be on the PATH inherited by Plous or specified by path. Plous keeps your selection and reports the error. |
| CLI requests sign-in | Use its native authentication flow. Adding a Plous API key does not sign in an external CLI. |
| Tools refer to a previous connection | Close the native session, select the intended target, and reopen it with **cc**. |
| No live cluster tools | Open a concrete connection; General and history-only views have no live tools. |
| A requested view is blocked | Finish or close the form, editor, or overlay, then request the view again. |
| Plous tool access stopped | Read the reported failure, close with `:X`, and reopen with **cc**. A fatal tool failure revokes access while leaving the CLI open. |
| Session closure cannot save evidence | Resolve the storage error and retry `:X`. See [save recovery](ai-chat.md#limits-and-recovery). |
| CLI launch options or MCP setup fail | Check [agent compatibility](ai-chat.md#choose-an-agent) and the full error, then close and retry after correcting the installation. |

For exact transport behavior, see [MCP connection details](../reference/mcp.md#mcp-connection-details). For broader recovery, see [AI troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send).

# Set up AI chat

[Documentation](../README.md) · [Plous MCP](plous-mcp.md) · [Data and privacy](../reference/data-and-privacy.md)

The right assistant panel runs the agent selected in Settings. Plous is the default and uses OpenAI or Anthropic. Claude Code and Codex use your installed CLI and its authentication. All three use the same scoped Plous tools for connection reads and saved evidence. Tool views work without AI setup. The assistant is read-only: it cannot edit, deploy, scale or delete resources.

| Agent | Setup | Conversation and model controls |
| --- | --- | --- |
| **Plous** | Save an OpenAI or Anthropic provider connection in Settings | Plous composer, `/model`, history, and Sources |
| **Claude Code** | Install and authenticate `claude` | The CLI's native prompt, menus, model selection, and conversation history |
| **Codex** | Install and authenticate `codex` | The CLI's native prompt, menus, model selection, and conversation history |

For the built-in agent, start with [Connect a vendor](#connect-a-vendor). For an installed agent, use [Choose an agent](#choose-an-agent) below or the [MCP walkthrough](plous-mcp.md#use-plous-mcp).

## Choose an agent

Select the cluster or Code source you want to investigate first. Open Settings with **ss** or `:s`, choose **Agent** under Application, use Left/Right to choose Plous, Claude Code or Codex, then select Save. Selecting an option previews its setup; Save applies the agent choice, which persists across workspaces and restarts. Plous shows its provider connections. Claude Code and Codex show executable availability and use their own sign-in and model controls. Saving another agent closes the current native session and saves its tool evidence before switching. Finish or stop a Plous response before changing it.

### Executables and authentication

Claude Code requires `claude` on PATH; Codex requires `codex` on PATH. Install the CLI first. Sign in through its native interface, either inside the panel or outside Plous. Plous does not install either binary or copy its credentials into Plous settings. An external agent needs no Plous API-key connection. Installing a CLI does not change the selected agent. If a selected binary disappears, Plous reports it and keeps your selection. Your Plous drafts remain separate.

Claude Code and Codex run their own interactive terminal UI inside the right panel. Their prompt, menus, model selection and conversation controls come directly from the installed application. The current native integration was checked with **Claude Code 2.1.284** and **codex-cli 0.160.0**, using local mock providers. There is no exact Codex version allowlist; installed releases must support the launch options and tool controls.

### Use the native terminal

The selected native terminal starts automatically when its panel is visible, including after restarting Plous. Startup waits for the initial connection selection and keeps the current panel focus. Use **cc** or `:c` to enter it or reopen a closed session. Keys, including Tab, Esc and slash commands, go to the CLI. **Alt+Left/Right** move panels directly and resume native input when you return. Mac Option-arrow sequences are supported, including Escape+b/f. With mouse support on, click another panel to move focus and click the native panel to resume typing in the same session. **Ctrl+]** toggles focus between the agent and the previous panel. **Ctrl+C** goes to the focused native terminal for its own interrupt behavior; while idle, the CLI may clear input or exit instead. To save and exit Plous, press **Ctrl+]**, then **Ctrl+C**. From Plous navigation, `:X` closes the native session. After closing it, **cc** resumes its latest conversation in the captured scope; `:C` opens a new one. Use the CLI's `/resume` for its history. Plous does not recreate these controls or copy native replies into the Plous transcript.

When there is no saved conversation, **cc** opens a fresh prompt, including after closing before your first message. Existing conversations still resume. If Claude Code reports that its saved session ID has no conversation, Plous retries once with a fresh session; other startup errors remain visible.

Use the mouse wheel over the native panel or **Page Up/Down** to read terminal output. The wheel moves one row at a time and stops at the top and bottom. Plous retains up to 5,000 normal-screen history rows in memory. Reading holds the displayed output steady while the CLI continues running, including across panel resizing. Scroll to the bottom, press **End** or **Esc**, or start typing/pasting to return to the live prompt. Up/Down still belongs to the CLI while typing. Fullscreen CLI views manage their own history; wheel events reach them when they enable mouse input. Clicks continue to select Plous panels.

### Session scope and saved evidence

Each native session captures the selected connection and retains it while you browse elsewhere. Its panel heading identifies that scope. To change the agent's scope, leave its panel with Ctrl+], close the session with `:X`, select the connection, then use **cc**. Hiding the panel or opening Help does not end the session.

The CLI retains its own authentication, model choice and conversation files. Both CLIs run in the directory where you launched Plous. Session metadata is stored separately for each agent, launch directory and captured scope, so resuming does not select another scope's conversation. Plous reconnects its tool bridge on resume and leaves global CLI settings unchanged. Built-in access tools, hooks and unrelated MCP servers are disabled for these sessions. Native onboarding, sign-in and built-in slash commands such as Claude Code's `/login` remain available. Model-invoked skills and skill shell execution are disabled separately from those commands.

Plous saves projected tool observations in a conversation named **Claude Code tool evidence** or **Codex tool evidence**. To inspect them, select Plous in Settings, then open that entry in history and its Sources. These entries describe tool-session lifecycle and observations; they do not contain a copy of the native conversation.

[Plous MCP](plous-mcp.md) connects these external agents to the same read-only tools used by the built-in agent. Plous configures the connection automatically. The guide explains skills versus tools, the available reads, scope and evidence, and why no manual MCP registration is needed.

## Connect a vendor

For the built-in Plous agent:

1. Open Settings with **ss** or `:s` while navigating.
2. Under **Application**, open **Agent**, select **Plous**, then choose **Add provider connection**.
3. Choose OpenAI or Anthropic, enter its API Key and select **Save**.
4. Provider Save returns to the Agent view. Save the Plous agent choice if needed, close Settings, then press **cc** to write a message.

Provider forms return to the pending Agent choice on Save or Cancel. Canceling removal restores the provider draft. Configuring a provider does not switch agents, and saved Plous providers remain available while another agent is selected.

You can save several connections. The first chooses an initial model; adding another preserves the selection. Esc cancels a form without saving. AI needs no separate interpreter or sandbox installation. Optional limits are in [agent configuration](../reference/configuration.md#agent-runtime-configuration).

## Choose a model

With Plous selected, freshly type `/model` or `/m` as the entire draft and submit it. Select a model under a saved vendor connection, then Enter; Esc keeps the current choice. The picker is local and makes no provider request. A checkmark marks the saved selection.

| Vendor | Available choices in Plous | Initial default |
| --- | --- | --- |
| OpenAI | GPT-5.4 mini, GPT-5.4 | GPT-5.4 mini |
| Anthropic | Claude Haiku 4.5, Claude Sonnet 5.5, Claude Opus 5.5 | Claude Sonnet 5.5 |

The catalog is maintained by Plous, not discovered from your account. Your provider must allow the selected model. The selection applies across workspaces and is captured when each response starts. An unsupported saved choice requires explicit replacement; requests never silently fall back.

With an external agent selected, slash commands are handled by that CLI. With Plous selected, pasted or restored `/model` is literal message text. Clear and type it again to open the picker. `//model` and `//m` send the literal command. Switching vendors sends subsequent request context and retrieved evidence to the newly selected vendor.

## Workspace context

The assistant header shows the active cluster context or Code source. It may read only that connection. Kubernetes, Helm, Argo and Flux share the captured cluster; Files and source-analysis views share the selected source. Namespace is a page hint: the assistant may read other namespaces allowed by the same cluster credentials. Naming another connection does not grant access; select it in Plous first.

Sending captures the connection and safe current-page hint. Switching views afterward does not retarget the running response. Switching connections restores their separate conversations and drafts. General chat and read-only retained histories have no live tools.

Cluster tools return dated projected facts, including current Pod, Node and workload usage with source times and coverage. These reads work without opening a chart, but cannot retrieve unsaved chart history. Code file tools use current saved local bytes or a pinned Git revision, while analysis tools read captured results without starting rendering. History tools search/read saved messages and observations directly. Recent exchanges are already in context, so a follow-up need not trigger retrieval.

For an investigation, name the subject and ask for evidence, for example: `Why is this Pod restarting? Check status, recent events and the last 100 log lines. Separate observations from likely causes.` Use explicit [resource actions](../integrations/kubernetes/README.md#resource-actions) when you choose to change something.

Search, pagination and missing permissions can limit an answer. A search match is not a complete inventory. A saved observation is not proof of current cluster state. Review the answer's stated scope and Sources before relying on it.

## Ask the assistant to show a view

All three agents can open Plous views in the main panel. For example:

- “Show this Deployment's YAML.”
- “Show the Pods table in production, filtered by api.”
- “Open the app container's logs and highlight connection refused.”
- “Show deployment.yaml at line 12.”
- “Compare the web release's values at revisions 2 and 3.”
- “Show the saved evidence behind that answer.”

Keyboard focus and your chat draft stay where they are. Alt+Left/Right moves into the opened view. An open form, editor or overlay can block navigation; finish it and ask again. If you navigate while a view is loading, the late result does not replace your screen.

Logs initially show the latest 200 lines; **g** reconnects using the normal log controls. Source files open read-only at the requested line. Source and evidence documents support find, selection, copy, Help and fullscreen; Esc or `:q` closes them. Comparisons use exact retained Helm revisions and mask sensitive values. Saved evidence retains its original timestamps, coverage and continuation fields. Opening a view does not change the agent's captured connection or authorize resource writes.

## Compose and navigate

With **Plous** selected, **cc** or `:c` enters typing. Enter sends; Shift+Enter or Ctrl+J adds a newline. Paste adds to the draft without sending. Esc leaves typing and preserves draft/response. The composer grows to five rows. Plous shows the selected model and a local context estimate.

While navigating, arrows or j/k scroll, Page Up/Down moves by page, Home reaches the oldest message, and End follows the latest output. Incoming text preserves your position while reading older output. `/` finds text; Enter finishes the query and n/N moves between matches. Page keys also scroll while typing; Home/End then move the draft caret.

`:A` / `:readreply` opens the selected finished or stopped answer in the wider Markdown Viewer. It supports rendered/source mode, find, selection and copying. Links show destinations without opening a browser. Esc restores the conversation and draft. See the [keymap](../reference/keymap.md) for exact controls.

## Inspect a reply's sources

These controls apply to Plous conversations, including saved external tool-evidence entries.

Leave typing with Esc. Use [ or ] to select a reply, then Enter to open **Sources** or **Reply details**. One source opens directly; several show a list. Details include operation, original observation identity/time and available usage. Enter opens captured content in the read-only viewer.

A history receipt records the retrieval time; its original record has the earlier observation time. Sources uses only saved turn-bound reads and makes no new API or provider request. It survives restart. A citation by itself is not proof that the answer is supported. Denied or unavailable results are labeled.

Esc closes each local layer and restores the reply, scroll and draft. Help remains available; Alt+Left/Right leave and resume Sources with its reading position intact. Routine model/usage details are here; meaningful scope or coverage limits belong in the answer.

## Manage conversations

The following controls manage built-in Plous conversations. Native CLI sessions use the controls described under [Choose an agent](#choose-an-agent).

| Command | Result |
| --- | --- |
| `:c` / `:chat` | Open the composer, keeping its draft |
| `:C` / `:newchat` | Start a conversation in the active scope |
| `:H` / `:openchat` | Browse conversation history |
| `:X` / `:stopchat` | Stop the running response, retaining accepted text |

Only one response runs at a time. Hiding chat, moving panels or opening Help does not stop it. New conversations do not delete earlier history.

History's **Choose history scope** opens retained cluster, directory, saved-connection or General histories. Other scopes are read-only and do not grant tools or inject their messages into the selected connection. Use New to return to the active scope. Titles/dates can be filtered with `/`.

Drafts and selected conversations survive restart. Interrupted, cancelled and failed attempts keep their status; partial text is not treated as a completed answer. Only the current development storage format is supported.

## Limits and recovery

For the Plous agent, the context meter estimates the full provider request locally, without a counting API call. When needed, Plous compacts older complete exchanges while preserving originals. Main and compaction share call, token and time allowances. The default turn timeout is 15 minutes. See [configuration](../reference/configuration.md#agent-runtime-configuration).

Native Claude Code and Codex sessions have no Plous-imposed deadline or session-wide tool-call cap. They keep running while you navigate, resize or hide the panel. The three turn budgets apply only to the Plous agent. Native model selection, context, retries, compaction and spending remain CLI-managed. A fatal Plous tool failure stops tool access without closing the terminal; close and reopen the session to reconnect.

Press **Ctrl+]** to leave the native agent panel, then use **ii** or `:i` to cycle normal width, double width, hidden, and back to normal. Press **Ctrl+]** again to resume input in the same running CLI.

If saving native tool evidence fails, `:X` retries the save; changing agents and normal exit wait for a successful save.

Errors and stopped Plous replies retain accepted text. Plous does not automatically restart a failed turn. `:p` / `:restoreprompt` restores the last prompt into an empty draft. Fix the reported issue or narrow the question before sending again. A new conversation is optional and does not erase history.

A failed response save leaves text visible and does not mark it complete. Use the reply reader to copy it. A save failure during exit keeps Plous open so you can address storage and retry exit. See [AI troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send).

## Credentials and advanced settings

Settings can edit/remove provider connections without deleting conversations. A blank key retains its saved value; Save replaces it. Removing the selected connection chooses a default on a remaining supported connection, or requires setup if none remains.

Advanced fields are display name, API proxy, environment-variable key reference and custom instructions. A saved key takes precedence over its environment reference. Proxies require HTTPS except on loopback; redirects are disabled. A proxy does not expand the model catalog. Changing vendor clears pending key/endpoint fields.

Keys are plaintext in private `credentials.json`, not encrypted or stored in a keychain. Prompts, selected tool evidence and retrieved history go to the selected provider. Source scanning exclusions do not prohibit exact named reads. See [privacy and source exclusions](../reference/data-and-privacy.md).

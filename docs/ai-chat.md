# Set up AI chat

[Documentation](README.md) · [Data and privacy](data-and-privacy.md)

The AI assistant can discuss infrastructure work, retrieve earlier workspace conversations, and help interpret configuration through OpenAI or Anthropic. You can use it before adding any tool connections. Workspace and tool views also work without AI setup.

The assistant can investigate your selected Kubernetes connection using read-only tools. It can retrieve saved conversations, investigation results, workspace details, and Activity. It cannot apply, delete, scale, execute a shell, or deploy anything.

## Connect a vendor

You need an API key for the vendor you choose and access to the model you plan to use. You can set this up before creating a workspace.

1. Press **ss** while navigating, or run `:s`.
2. Under **Application**, select **Add AI connection**.
3. Choose **Vendor** with **Left / Right**, then press Enter.
4. Paste your API key into the masked field.
5. Move to **Save** and press Enter.

Plous creates a name for the connection. The first connection selects a default model. Adding another connection keeps your current selection. Esc cancels the form without saving the key.

Use arrows and Enter within Settings. Tab and Shift+Tab do nothing there. You can save multiple vendor connections, including separate accounts with the same vendor.

## Send your first message

1. Close Settings with Esc.
2. Press **cc**, or run `:c`, to enter the composer at the bottom of the right panel.
3. Type a message, such as `What should I check before applying an infrastructure change?`
4. Press **Enter** to send it. The reply streams into the conversation above.

The selected vendor and model appear above the input. Focusing the right panel alone does not start typing; use cc or `:c`.

**Shift+Enter** inserts a newline. If your terminal sends the message instead, use **Ctrl+J** as the newline fallback. Pasting multiline text only adds it to the draft; it does not send it.

Press **Esc** to leave typing while preserving the draft and any running reply. After that, colon commands navigate again. A colon typed inside the composer is message text.

## Workspace context

Workspace chat captures the workspace, connection, and current page when you send a message. In Explore, this includes the selected resource identity and filters, without copying resource bodies or logs. Namespaced reads use the selected resource's namespace or concrete visible namespace when available, then the saved namespace or kubeconfig default. Switching views or connections does not retarget a running investigation.

The assistant can use `get`, `describe`, and bounded Pod `logs`. Events are available through resource reads. Every read stays on the captured connection. Namespaced resources stay within the captured namespace; cluster-scoped resources, such as Nodes, use that connection's cluster. All-namespace lists, Secret resources, raw API paths, plugins, and shell commands are unavailable. Every call is checked in Go, including calls made inside an investigation.

For a larger question, the assistant can run a bounded investigation that searches records, filters results, and asks smaller model questions. The same selected model handles every step. Progress appears in the reply status, and Stop cancels the operation.

Earlier conversations in the same workspace can be searched when relevant. General chat can search its current conversation only. Historical Kubernetes answers depend on observations saved during earlier investigations; dashboard browsing does not create a cluster history.

## Investigate a Kubernetes problem

Select a Kubernetes connection and open the relevant resource in Explore. For a namespace-wide question, choose a concrete namespace. Press **cc** and ask a focused question, for example:

- `Why is this Pod restarting? Check its status, recent events, and last 100 log lines. Explain which evidence supports the likely cause.`
- `Which Deployments in this namespace are not ready, and what is blocking them?`
- `Compare this error with the investigation saved earlier in this workspace. What has changed?`

The assistant decides which reads it needs. Resource and log excerpts used in an investigation are sent to your selected vendor and saved with the conversation. See [AI requests](data-and-privacy.md#ai-requests) for the data boundary.

There is one read-only assistant mode. Asking it to restart, edit, or delete a resource does not authorize a write. Use the explicit [Kubernetes actions](kubernetes-actions.md) when you decide to make a change.

## Choose a model

1. Enter the composer with **cc**.
2. With an empty draft, type `/model` and press Enter. `/m` also works.
3. Use arrows to select a model under a saved vendor connection.
4. Press Enter to select it, or Esc to keep the current choice.

The picker works locally and makes no AI request. A checkmark marks the saved selection, independently of the highlighted row. **Manage in Settings** opens AI connection setup.

This version's built-in catalog contains:

| Vendor | Available choices in Plous | Initial default |
| --- | --- | --- |
| OpenAI | GPT-5.4 mini, GPT-5.4 | GPT-5.4 mini |
| Anthropic | Claude Haiku 4.5, Claude Sonnet 5.5, Claude Opus 5.5 | Claude Sonnet 5.5 |

The catalog is maintained by Plous; it is not a list discovered from your account. Your vendor account must allow the chosen model. Arbitrary model IDs and additional vendors are not supported.

The selection is shared across workspaces and remembered after restart. A running reply keeps the model it started with; your new selection applies to subsequent messages. Switching vendors sends the active Q&A context and any retrieved evidence to the newly chosen vendor. A new workspace conversation can still retrieve earlier conversations in that workspace. Use a [separate data profile](data-and-privacy.md#data-directory-and-profiles) when you need separate history.

`/model` must be the entire, freshly typed draft. Pasted and restored text is treated as a message, even if it reads `/model`. Clear it and type the command again to open the picker. `//model` and `//m` send the literal command text.

## Manage conversations

Before workspace creation, chat belongs to **General chat**. Each workspace has separate conversations and a draft. Selecting another workspace switches to its chat scope; it does not move the old conversation. There is no chat entry in the sidebar.

Run these commands while navigating. Press Esc first if you are typing.

| Command | What happens |
| --- | --- |
| `:c` / `:chat` | Open the current composer and keep its draft. |
| `:C` / `:newchat` | Start a new conversation in the current scope and keep the unsent draft. |
| `:H` / `:openchat` | Open conversation history. Arrows select; Enter opens; Esc closes. |
| `:X` / `:stopchat` | Stop the running response and keep the received text. |

Only one response runs at a time. You can change panels or workspaces while it runs; the reply stays with its original conversation. Hiding the assistant or opening Help does not stop it.

To read older messages, focus the assistant and press **Esc** if you are typing. **Up / Down** or **k / j** scrolls one line, **Page Up / Page Down** scrolls a page, and **Home** goes to the oldest message. **End** returns to the latest reply and follows new output. While reading older messages, incoming reply text keeps your reading position steady. The position indicator shows when you are reading earlier text.

Page Up / Page Down also works while typing without changing your draft. Home / End still moves the draft's caret in that mode.

To find a past message, focus the assistant while navigating and press **/**. Enter finishes typing the query; n/N moves to the next or previous match; Esc clears search and keeps the found text visible. In history, `/` filters titles and dates.

Your messages have a gold marker and shaded text. Assistant replies use purple headings, with code and quoted text styled separately. The model and connection used for each reply appear beneath **Assistant**. Failures and stopped replies have their own status colors.

New conversations and drafts reopen after restart. This agent uses a new conversation format; chats from the previous implementation are not loaded or migrated. Stopping a reply or exiting normally cancels it and keeps saved text. An unfinished reply after an unexpected exit is marked interrupted when reopened. Failed, cancelled, and interrupted attempts remain visible but are excluded from continuing Q&A context.

## Edit or remove a vendor connection

Open Settings and select the connection under Application. A blank API key field keeps its saved key; enter a new key and Save to replace it.

**Remove AI connection** asks for confirmation and retains saved conversations. If you remove the currently selected connection, Plous chooses a default model on a remaining supported connection. With none remaining, you need to add one before sending again.

If the selected model is no longer supported, use `/model` to choose another. Plous does not silently substitute a different model when a request fails.

## Advanced connection settings

Most users only need the vendor and API key. **Advanced…** contains:

| Setting | Use it when |
| --- | --- |
| Display name | You want to distinguish accounts, such as personal and team credentials. |
| API proxy URL | Your organization uses a proxy for the same supported vendor and models. |
| Environment variable | You already keep the API key in an environment variable. A saved key takes precedence. |
| Custom instructions | You want additional instructions included with every request on this connection. |

A remote proxy URL must use HTTPS. Loopback addresses also allow HTTP. Redirects are disabled, so use the final endpoint. Changing the vendor in the form clears its credential and endpoint fields; Save applies the change.

Replies use text. There is no structured-output mode.

## Understand the context meter

Below the composer, **Context ~… / … tokens** estimates how much input context the current conversation and draft use against the selected model's capacity. A token is a unit of text processed by the model; it is not always a word.

The estimate includes instructions, saved workspace details, completed conversation exchanges, and the draft. While a reply streams, it also previews that exchange's growing context; a failed or stopped exchange is excluded from later requests. The `~` means approximate. Counting happens locally and does not send your draft to a provider. Per-reply token usage is a separate figure, not the sum used by this meter.

Before a request would exceed the usable context budget, Plous summarizes older Q&A and keeps newer exchanges when they fit. Original messages and investigation records remain available for retrieval. The summary is saved and restored when the conversation reopens. Temporary tool evidence does not accumulate in later Q&A prompts.

A new message is limited to 256 KiB. Each model response is limited to 4,096 output tokens and 1 MiB locally. Investigations also have shared call, time, script-step, and output limits. A failed compaction preserves the previous context and stops the request.

## If a reply fails

Requests time out after three minutes. Partial output stays visible, and Plous does not retry automatically. While navigating, **`:p`** / **`:restoreprompt`** restores the last sent message into an empty draft. Edit it and press Enter to make a new request.

If an investigation reaches a limit, narrow the question to one resource or a shorter log window. If summarization fails, the original messages and previous summary remain saved. Restore the prompt and retry after addressing the reported error. Starting a new conversation is optional; it does not delete the earlier records.

For credential, model, or proxy problems, see [AI troubleshooting](troubleshooting.md#ai-chat-will-not-send). For scope and investigation limits, see [investigation recovery](troubleshooting.md#an-ai-investigation-stops-or-cannot-read-a-resource).

## API key storage

Keys are saved separately in `credentials.json` under your Plous data directory, with owner-only permissions. They are plaintext, not encrypted or stored in the operating system's keychain. See [Credentials](data-and-privacy.md#credentials) for retention and replacement behavior.

For exactly what a message sends to a vendor, see [AI requests](data-and-privacy.md#ai-requests).

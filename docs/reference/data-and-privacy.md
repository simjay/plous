# Data and privacy

[Documentation](../README.md)

Plous saves settings and conversations locally. Browsing contacts your clusters. AI investigations can send cluster data to your chosen model provider.

## Data directory and profiles

The default directory is `~/.plous/`. To use a separate profile:

```sh
plous --data-dir "$HOME/plous-profiles/learning"
```

Use that path again to reopen the same profile. Only one Plous process can open it at a time.

Profiles separate saved data. They do not isolate the clusters they connect to. Native agents also keep their own storage outside the Plous profile.

## What is saved

| Data | Contents |
| --- | --- |
| Workspaces | Connection settings and their revision history |
| AI settings | Agent choice, provider connections, selected Plous model |
| Credentials | API keys in a separate `credentials.json` file |
| Conversations | Original Plous messages, saved tool results, request status, available usage |
| Native sessions | Resume metadata and tool evidence. Native prompts and replies stay with the CLI. |
| Session preferences | Panel visibility, tree folds, selected conversations, drafts |
| Browsing preferences | Resource type, namespace, applied filters, sorting, selected Helm release |
| Helm Activity | Operation outcomes and observed revision metadata, without values or manifests |

Open searches, unfinished forms, and unsaved resource edits are temporary.

General chat lives at the profile root. Connection and current-cluster histories use `connections/` and `contexts/`. Indexes are derived from the saved records. Optional runtime configuration is in `agent/config.yaml`.

Cluster history uses context, cluster and authentication entry names, endpoint, and trust settings to identify the connection. Token rotation preserves it. Endpoint or trust changes separate it. Recreating a cluster with identical identifiers cannot be detected reliably.

## Retained data from earlier builds

Archived Code connections and directory histories remain readable where their stored format is supported. They grant no filesystem tools and cannot be added as new connections.

Older development chat formats and migration readers are retired. Back up a profile before trying a new build.

## Native assistant sessions

Claude Code and Codex store their own conversations and credentials. Plous separately saves the tool results returned during their sessions.

`native-agents/` holds resume metadata for each agent, launch directory, and cluster. The CLI runs in the directory where you launched Plous.

The panel's MCP configuration is temporary and removed when the session ends. Cluster credentials remain with Plous. The CLI's own privacy and retention settings govern its conversation files.

## Independent coding-agent sessions

`plous setup codex` or `plous setup claude` installs the skill and updates the client's MCP configuration in its own home directory.

Each `plous mcp serve` process saves evidence under `~/.plous/mcp/session-*`, or the chosen `--data-dir` root. These profiles remain after exit and are not imported into TUI history automatically.

Closing the client stops local work and attempts temporary Job cleanup. It cannot guarantee that an exec process stopped inside an application container. See [MCP scope and saved evidence](../mcp/scope.md).

## Credentials

API keys are plaintext in `credentials.json`. The file has owner-only permissions (`0600`), and the data directory uses `0700`. Plous does not encrypt them or use a keychain.

- A blank key field keeps the existing key.
- Save applies a replacement key.
- Removing a provider removes a key when no current connection uses it.
- An interrupted save can leave an unused key until the next successful connection save or removal.
- Environment-variable credential references remain supported.

Backups of `credentials.json` contain usable keys. Keep profiles and backups private and out of source control.

Keys are not inserted into conversation messages or errors. A secret you paste into chat is still message content and can be stored and sent.

## Kubernetes data

Plous reads kubeconfig and uses its authentication plugins. It saves references to that file, not copies of its credentials.

Browsing alone does not save resource bodies, live metrics, events, logs, revealed Secret values, or Exec output to conversations or workspace revisions. An assistant's tool read does save the result delivered to the agent.

Explicit actions can copy or export data:

| Action | Destination |
| --- | --- |
| Copy | System clipboard through a local tool or terminal request |
| Export YAML | New local file, with Secret values redacted |
| Export PEM | New private file for the selected CSR request or certificate |
| Reveal or edit a Secret | Visible values for the current operation |
| Paste into chat | Stored message sent to the model provider |

Clipboard reads require an explicit Paste action. Plous does not separately archive clipboard contents.

Agent resource reads block Secret objects and redact known credential fields and patterns. Logs, command output, and other application text can still contain secrets.

## Helm data

Helm uses the selected kubeconfig. Temporary configuration, cache, plugins, and chart downloads are private to the process and removed afterward.

Chart contents, draft values, release bodies, and comparisons stay in memory. Browsing preferences and operation outcomes are saved separately.

Agent reads receive filtered release data. User-confirmed rollback can change the cluster and run chart hooks. Chart previews do not install or upgrade releases.

## AI requests

A request can include your message, recent conversation context, the selected cluster and page, and tool results or history retrieved during the investigation.

Plous filters API data before recording or sending it. Raw API bodies are not saved as reasoning evidence. Filtering does not guarantee that arbitrary application text is free of secrets.

The built-in agent may summarize older exchanges in its active context. Original messages stay unchanged on disk. Native agents manage their own context and conversations.

Optional embeddings send fragments of saved conversation text and tool results to the configured embedding provider. This can be a different connection from the chat provider. Derived indexes also contain retained text and need the same protection as the profile.

Saving evidence does not wait for background embedding work. Search reports missing or delayed indexing. See [search and indexing](../contributing/ai-assistant.md#search-and-indexing).

There is no automatic retention deletion for new conversations. The context meter is a local estimate and does not send drafts for token counting.

## Back up or recover a profile

1. Exit Plous.
2. Copy the whole data directory to a private location, preserving permissions.
3. Keep kubeconfig and native CLI conversation backups separately.

To inspect a backup, copy it to another directory and launch Plous with that `--data-dir`. Keep your current profile intact while checking it.

If data is corrupt, preserve a copy and follow [storage recovery](troubleshooting.md#plous-reports-corrupt-data). Deleting a lock file does not release an active process's lock.

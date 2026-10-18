# Data and privacy

[Documentation](../README.md)

Plous stores workspaces, settings, conversations, and session state locally. Kubernetes, Helm, Argo, and Flux browsing and AI chat make network requests when you use them. This page explains the boundary between saved configuration, cluster data, and messages sent to an AI vendor.

## Data directory and profiles

The default data directory is `~/.plous/`. Launch with `--data-dir` to use another independent profile:

```sh
./bin/plous --data-dir "$HOME/plous-profiles/learning"
```

Use the same path on later launches to reopen that profile. Each profile has its own workspaces, AI connections and keys, chat history, drafts, and session. Only one Plous process can open a given directory at a time.

A separate profile isolates Plous's saved state. It does not isolate the clusters referenced by its connections. Paths to kubeconfig files, repositories, and working directories still refer to files on your computer.

External agents use their installed CLI's authentication, conversation storage, and diagnostic state. Plous saves their projected tool evidence and session metadata in the selected profile, without copying native prompts or replies into Plous conversations. Scoped connector reads stay in the Plous host. See [native assistant sessions](#native-assistant-sessions) for the storage boundary.

## What is saved

| Data | Saved behavior |
| --- | --- |
| Workspaces and tool connections | Exact configuration revisions, including names and references. |
| AI settings | Selected main agent, vendor connections and selected Plous model, with settings revisions. |
| API Keys entered in Settings | Separate `credentials.json` file. |
| Conversations | Original Q&A, projected observations, native conversation checkpoints, status, and available usage. |
| Global source scanning exclusions | `.plousignore`, populated with defaults and preserved on restart. |
| Session | Current selection, panel visibility, tree expansion, selected conversations, and chat drafts. |
| Kubernetes browsing preferences | Resource type, namespace, applied text filter, label selector, and sorting per connection. |
| Helm browsing preferences | Section, namespace, applied filter, and selected release per connection. |
| Helm Activity | Operation identities and outcomes, and metadata of observed retained revisions. No values or manifests. |

The editable panel-search queries are temporary. Applying a Kubernetes table filter turns it into a saved browsing preference. Unsubmitted form edits are not saved when you exit.

General conversations live at the profile root. Saved-connection, current-cluster and launch-directory conversations use `connections/`, `contexts/` and `directories/` scope folders. Each conversation retains original messages, projected evidence and separate execution records. Indexes are derived data. Runtime settings use `agent/config.yaml`; configuration revisions are application-managed and must not be edited by hand.

Current Workspace cluster history identity uses a versioned fingerprint of context, cluster and authentication entry names, API endpoint, and transport/trust settings. Paths, namespaces, and rotating credentials do not define it. Replacing an endpoint or trust identity separates history; renewing a token preserves it. Identical names, endpoint, and trust after cluster recreation cannot prove a new cluster incarnation. Kubeconfig and credentials remain in memory and their original files, outside the scope registry.

Launch-directory history uses a separate versioned fingerprint of the absolute directory path with symbolic links resolved. The same directory keeps its conversations, drafts, and source preferences when you switch clusters. Different paths have separate histories; moving a directory starts a new scope. Selecting a Code source grants the assistant access only to that source through the existing guarded source reads.

## Global source scanning exclusions

Plous creates `~/.plous/.plousignore`, or `<data-dir>/.plousignore` for a custom profile. It applies to file browsing, text search and configuration discovery in every Code source. There are no workspace-local Plous ignore files. Existing contents, including an empty file, are preserved.

The initial patterns are:

```gitignore
.git/
node_modules/
.venv/
venv/
__pycache__/
.pytest_cache/
.mypy_cache/
```

Patterns use Git ignore syntax and are relative to each selected source root. Repository `.gitignore` rules also apply. A path is skipped if either rule set excludes it; a `!` exception only reverses rules within its own set. Ignored parent directories must also be included before their children can be scanned. New scans and searches use the current policy. An existing paginated search keeps its starting policy; refresh Code to update an existing inventory.

Ignore rules control scanning, not permission to read a named file. A permitted exact file read or declared render dependency can still read an ignored file. Source boundaries, symlink restrictions, Git internals and the assistant's separate sensitive-file/content restrictions remain enforced. Do not use `.plousignore` as a secret-access policy.

## Native assistant sessions

Claude Code and Codex render their own terminal UI and save conversations using their own storage and authentication. Plous does not copy terminal replies into its conversation format. It saves projected tool observations in labeled tool-evidence entries, with lifecycle metadata and no captured native prompt. The CLI's privacy and retention settings govern its conversation files.

`native-agents/` under the Plous data directory holds session metadata for each agent, launch directory and captured scope: resume markers, Claude session IDs and Codex runtime state. The CLI working directory remains the directory where you launched Plous. The MCP configuration is temporary, uses a fresh bearer token and is removed when the process ends. Connector credentials remain with Plous. Native CLI settings and authentication are read through the installed executable; Plous does not copy them into its profile.

## Credentials

API Keys entered in Settings are plaintext in `credentials.json`, with owner-only read/write permissions (`0600`). The data directory uses owner-only access (`0700`). Plous does not encrypt the files or use the operating system's keychain.

Settings, session, and chat request records hold a credential reference rather than the API Key value. The key is used for authentication and is not inserted into chat messages or error details.

- A blank key field while editing keeps the existing key.
- A replacement key is saved only when you select Save.
- Removing a connection removes its key when no current connection uses it.
- An interrupted save can leave an unused key until the next successful connection save or removal.
- Existing environment-variable credentials remain usable. Saving a key replaces that connection's credential source without rewriting old history.

Backups containing `credentials.json` contain usable credentials. Keep the data directory and its backups private and out of source control. Text you deliberately paste into a chat is ordinary message content, even if it contains a secret.

## Kubernetes data

Context setup reads local metadata and saves references to kubeconfig. It does not copy kubeconfig credentials into Plous. Live browsing uses those external credentials and can run configured authentication plugins.

Resource objects, live usage samples and graphs, events, logs, revealed Secret values, and Exec terminal output are not written into workspace revisions, session files, or chat records by browsing alone. The session saves browsing preferences, not a copy of the objects. An assistant's explicit current-usage read is different: its projected measurements, times and coverage are saved as turn evidence. It can read those saved observations later, but cannot recover the TUI's unsaved chart history.

Explicit actions can copy or export data:

- Copy sends the chosen text, identity, or value to the system clipboard using local clipboard tools or a terminal request. Clipboard reads require an explicit Paste action; Plous does not save clipboard contents separately.
- Export YAML creates a new local file and redacts Secret values.
- Export PEM writes the selected CertificateSigningRequest request or certificate to a new private file.
- Secret value inspection and manifest editing reveal values for the current operation.
- Pasting resource data into AI chat includes that text in the message you send.

Assistant requests include a small current-page description. When an investigation reads Kubernetes resources or logs, its bounded results are sent to the selected AI vendor and saved in the conversation. Secret resource reads are blocked and known credential fields/patterns are redacted; arbitrary sensitive text in logs or user-provided records may still be included.

## Repository Code data

Code saves source settings and navigation preferences. File metadata, remote Git objects, captured charts, selected rendering inputs, and rendered resources stay in memory or temporary directories. Loading a remote source and explicitly rendering a missing pinned chart can make network requests. Source contents are not added to automatic assistant context. When you send a message, the selected source root or acquired Git commit and completed results become available to guarded read tools. Local file tools read current saved contents when called; rendered analysis retains its captured inputs. Requested file excerpts, masked fields, findings and comparison evidence are sent to the selected model and retained in Sources. Git reads use the already acquired revision; these reads do not fetch or render. Unsaved buffers are excluded.

Local source editing exposes original bytes. Saving changes only the explicitly selected local file and checks for external edits. Unsaved source buffers and Helm Rendering Inputs are not restored after exit. Remote editable copies require a new directory and contain captured files, without Git metadata. See [Kubernetes Code](../integrations/kustomize/README.md) for coverage and source limits.

## Helm data

Helm uses the selected kubeconfig and explicit optional repository/registry inputs. Each process uses private temporary configuration, cache, and plugin directories. Temporary chart archives are removed after commands. Downloaded chart contents, draft values, release bodies, and comparisons remain in memory and are not restored after exit. Browsing metadata and operation outcomes are saved separately from workspace configuration revisions.

The assistant receives bounded, masked release reads when requested. Release notes and arbitrary chart-source access are not exposed to it. User-operated rollback can change the cluster and execute chart hooks only after explicit confirmation. Chart changes are previews; installation and upgrade execution are not offered.

## AI requests

When you send a message, the selected agent receives host instructions, the captured connection and page hint, and the current question. The built-in Plous agent also receives recent completed exchanges from its Plous conversation and may replace older active context with a labeled summary. External agents receive the captured scope instructions and manage their own native conversation context. Plous does not inject its chat transcript. Original conversation messages remain unchanged on disk.

Live tools compute typed projections in Go. Only those projected facts, coverage, selected excerpts and source identities enter new reasoning records or model context. Raw API responses remain transient. Models do not receive Kubernetes managed fields, complete Helm manifests or arbitrary raw response blobs through history. A discarded historical field cannot be reconstructed by fetching current state.

The main agent retrieves original history excerpts and calculations through bounded Go tools. Selected historical evidence goes to the provider used by that agent. Installed Claude Code or Codex runs as the main agent process; there are no delegated worker agents.

Monitor history supports lexical and explicitly configured semantic retrieval. Semantic setup currently requires a turn with captured Kubernetes or Helm capabilities; history-only views use lexical search. Once started, background indexing continues until the history store closes. Embedding requests send fragments of original conversation text and projected observations to the selected embedding connection, independently of the chat connection. Code and General history use lexical indexing only. Repository contents are not semantically indexed. Derived indexes repeat retained text and optional vectors locally and need the same protection as the profile. See the [indexing lifecycle](../contributing/ai-assistant.md#search-and-indexing).

Saving evidence and delivering live tool observations do not wait for background embeddings. A semantic history search does wait for its query embedding, so an answer using that search can be delayed. Search reports unavailable or delayed indexing. Native provider continuation and usage are saved separately from searchable evidence. New reasoning records contain no model thinking, context-edit programs or raw API bodies.

Known credentials are removed from recorded provider and tool output. Arbitrary secrets in user-pasted messages, source excerpts or logs may remain. User and assistant conversation text stays original. Protect the profile and review what you send.

Chat storage supports only the current format. Earlier development histories and their migration readers have been retired. New saved conversations have no automatic retention deletion. Drafts are not sent for token counting. The Plous context meter is a local estimate; external agents display CLI-managed context.

## Back up or recover a profile

1. Exit Plous so the profile is no longer being written.
2. Copy the entire data directory to a private backup location, preserving permissions.
3. Keep referenced kubeconfig files and project directories available separately; they are not bundled into the profile.

To inspect a restored backup, copy it to a separate directory and launch Plous with `--data-dir` pointing there. Avoid overwriting your current profile while checking a backup.

Plous reports corrupt data and preserves it rather than replacing it with empty state. If a profile will not open, preserve a copy and follow [storage troubleshooting](troubleshooting.md#plous-reports-corrupt-data). Removing a lock file is not the way to release an active process's lock.

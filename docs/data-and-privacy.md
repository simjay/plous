# Data and privacy

[Documentation](README.md)

Plous stores workspaces, settings, conversations, and session state locally. Kubernetes browsing and AI chat make network requests when you use them. This page explains the boundary between saved configuration, cluster data, and messages sent to an AI vendor.

## Data directory and profiles

The default data directory is `~/.plous/`. Launch with `--data-dir` to use another independent profile:

```sh
./bin/plous --data-dir "$HOME/plous-profiles/learning"
```

Use the same path on later launches to reopen that profile. Each profile has its own workspaces, AI connections and keys, chat history, drafts, and session. Only one Plous process can open a given directory at a time.

A separate profile isolates Plous's saved state. It does not isolate the clusters referenced by its connections. Paths to kubeconfig files, repositories, and working directories still refer to files on your computer.

## What is saved

| Data | Saved behavior |
| --- | --- |
| Workspaces and tool connections | Exact configuration revisions, including names and references. |
| AI settings | Vendor connections and selected model, with settings revisions. |
| API keys entered in Settings | Separate `credentials.json` file. |
| Conversations | Complete Q&A, execution records, retrieved evidence, compaction summaries, status, and available usage. |
| Session | Current selection, panel visibility, tree expansion, selected conversations, and chat drafts. |
| Kubernetes browsing preferences | Resource type, namespace, applied text filter, label selector, and sorting per connection. |

The editable panel-search queries are temporary. Applying a Kubernetes table filter turns it into a saved browsing preference. Unsubmitted form edits are not saved when you exit.

A typical directory contains:

```text
credentials.json
session.json
store.lock
settings/
  <revision>/change.json
conversations/
  <conversation>/metadata.json
  <conversation>/conversation.jsonl
  <conversation>/reasoning.jsonl
workspaces/
  <workspace-id>/revisions/<revision>/change.json
  <workspace-id>/conversations/<conversation>/...
```

`conversations/` at the root holds General chat. Workspace conversations are stored with their workspace. These are application-managed files; use the app to edit configuration rather than changing its revision records by hand.

## Credentials

API keys entered in Settings are plaintext in `credentials.json`, with owner-only read/write permissions (`0600`). The data directory uses owner-only access (`0700`). Plous does not encrypt the files or use the operating system's keychain.

Settings, session, and chat request records hold a credential reference rather than the API key value. The key is used for authentication and is not inserted into chat messages or error details.

- A blank key field while editing keeps the existing key.
- A replacement key is saved only when you select Save.
- Removing a connection removes its key when no current connection uses it.
- An interrupted save can leave an unused key until the next successful connection save or removal.
- Existing environment-variable credentials remain usable. Saving a key replaces that connection's credential source without rewriting old history.

Backups containing `credentials.json` contain usable credentials. Keep the data directory and its backups private and out of source control. Text you deliberately paste into a chat is ordinary message content, even if it contains a secret.

## Kubernetes data

Context setup reads local metadata and saves references to kubeconfig. It does not copy kubeconfig credentials into Plous. Live browsing uses those external credentials and can run configured authentication plugins.

Resource objects, events, logs, revealed Secret values, and Exec terminal output are not written into workspace revisions, session files, or chat records by browsing alone. The session saves browsing preferences, not a copy of the objects.

Explicit actions can copy or export data:

- Copy sends the chosen identity or value to your terminal clipboard.
- Export YAML creates a new local file and redacts Secret values.
- Export PEM writes the selected CertificateSigningRequest request or certificate to a new private file.
- Secret value inspection and manifest editing reveal values for the current operation.
- Pasting resource data into AI chat includes that text in the message you send.

Assistant requests include a small current-page description. When an investigation reads Kubernetes resources or logs, its bounded results are sent to the selected AI vendor and saved in the conversation. Secret resource reads are blocked and known credential fields/patterns are redacted; arbitrary sensitive text in logs or user-provided records may still be included.

## AI requests

When you send a message, the selected vendor receives instructions, captured workspace/page scope, active Q&A context, and your message. It may then receive relevant excerpts from read-only Kubernetes tools, earlier workspace chats, saved investigation records, and Activity. The assistant can make additional model calls for recursive investigation and compaction using the same captured model selection.

General chat has no workspace or Kubernetes access and searches only its current conversation. Workspace searches exclude other workspaces and general chats. Failed, cancelled, and interrupted answers remain in the archive but are excluded from continuing Q&A context. Switching vendors sends the active context and any newly retrieved evidence to the newly selected vendor.

Starting a new conversation in the same workspace does not prevent retrieval from earlier workspace conversations. Use a separate profile when those histories must stay separate.

Building the initial page description does not read repository files or live resources. Kubernetes tools use the selected connection's kubeconfig credentials and may run its configured authentication plugin. The agent has no arbitrary file, network, shell, or repository-reading tool.

Drafts are not sent for token counting. The context meter uses a local estimate. Compaction sends older Q&A to the selected model for summarization and preserves the originals locally.

## Conversation records

Each conversation has `conversation.jsonl` for Q&A and `reasoning.jsonl` for request metadata, intermediate agent calls, tool calls/results, denials, and compaction summaries. `metadata.json` stores its identity and title. The reasoning file contains observable execution records, not private model thoughts.

The request and captured model/scope are recorded before provider activity. Streaming checkpoints append deltas; a final record stores the answer and status. A crash can lose uncheckpointed text, and an unfinished response is marked interrupted when reopened. Incomplete trailing JSONL writes are ignored and removed before the next append. Corruption in a completed record is reported.

Old `chats/` records are not loaded or migrated. They are left on disk; the new implementation does not delete them automatically. The app has no automatic retention cleanup or conversation-deletion interface in this version.


## Back up or recover a profile

1. Exit Plous so the profile is no longer being written.
2. Copy the entire data directory to a private backup location, preserving permissions.
3. Keep referenced kubeconfig files and project directories available separately; they are not bundled into the profile.

To inspect a restored backup, copy it to a separate directory and launch Plous with `--data-dir` pointing there. Avoid overwriting your current profile while checking a backup.

Plous reports corrupt data and preserves it rather than replacing it with empty state. If a profile will not open, preserve a copy and follow [storage troubleshooting](troubleshooting.md#plous-reports-corrupt-data). Removing a lock file is not the way to release an active process's lock.

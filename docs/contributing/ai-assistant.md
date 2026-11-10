# AI implementation

[Contributing](README.md) · [Tool reference](../reference/mcp.md)

Plous uses one Eino agent, integration-owned Go readers, and direct history tools. Native Claude Code and Codex sessions use the same guarded tools through MCP.

## Runtime prerequisites

The built-in agent needs a configured provider. Native agents need an installed executable and its own authentication. Neither CLI is bundled or installed by Plous.

Client checks used Codex 0.160.0 and Claude Code 2.1.284 with mock providers. There is no exact Codex version allowlist.

## Configuration

[Runtime configuration](../reference/configuration.md#agent-runtime-configuration) defines budgets and optional embeddings. The host captures settings when an operation starts. Native agents manage their own model context and spending.

## Runtime structure

| Owner | Responsibility |
| --- | --- |
| `run.go` | Eino conversation loop |
| `compaction.go` | Fit older completed exchanges into active context |
| `call.go`, `budget.go` | Provider dispatch and turn allowance |
| `model/` | Provider wire format and usage |
| `history/` | Messages, evidence, Sources, indexes |
| `agent.Tools` | Standard-library-only schemas and calls |
| `tool_session.go` | Target checks, serialized calls, persistence, lifetime |
| `agent.Backend` | Built-in turn and events |
| `cli.StartTerminal` | Installed CLI in a PTY |
| `app/chat.Native` | Terminal screen and input lifecycle |

`OpenExternalTools` captures the target and committed history boundary, creates a tool-evidence conversation, and exposes `agent.Tools` through authenticated loopback MCP.

Native sessions can contain multiple CLI turns. Their target stays fixed until closure and does not inherit Eino turn budgets. Closure revokes tools and saves status. Fatal tool failures revoke access without closing the CLI.

Claude Code launches with strict MCP configuration and other tools, skills, and hooks disabled. Codex first reads inherited MCP configuration through a temporary App Server, then launches its native terminal with those servers disabled. Overrides use one inline TOML table. Global settings and credentials stay unchanged.

Both CLIs run in the launch directory. Resume metadata is separate for each agent, directory, and cluster. Plous does not parse native transcripts into built-in conversations. Session closure or exit stops the process group and removes temporary MCP configuration.

## Main loop and connection tools

Each operation captures its agent, cluster, and page hint. Built-in turns also capture model and credentials. External sessions capture the executable command.

Integrations own schemas, argument rules, read policies, and projections. Schema generation advertises those rules, and execution validates the same Go definitions. Single-operation tools infer their operation from the tool name.

The host validates scope, serializes calls, and saves results before delivery. Invalid arguments receive feedback. Cancellation, exhausted budgets, and history scope or integrity failures stop execution.

Only projected facts, identities, times, and completeness enter saved observations. Raw API bodies are temporary. Neither browsing another page nor naming another connection expands access.

General chat and history-only views have no live connection tools. A page's namespace is context, not a Kubernetes access boundary.

## Standalone MCP and active diagnostics

`app/debugging` owns `plous setup` and `plous mcp serve`. Setup installs the embedded skill and managed MCP entry. Serve opens a separate profile, captures the explicit cluster and default namespace, and adapts `agent.Tools` through `cli.ServeStdio`. It has no `agent.UI` port.

Kubernetes owns `kubernetes_exec_run` and `kubernetes_diagnostic_run`. Launch options explicitly enable them. Both record intent before dispatch and results before delivery.

Temporary tests use a fixed Job template, deadlines, identity-checked cleanup, and TTL fallback. Exec cancellation does not guarantee remote termination. See [diagnostic contracts](../reference/mcp.md#active-diagnostics).

Usage projections share the TUI's identity, ownership, freshness, and aggregation rules. The agent uses a guarded reader, not the unrestricted TUI client.

## UI tools

`ui_read`, `kubernetes_show`, `helm_show`, and `history_show` use `agent.UI`. The tool session validates the target. The app loop prepares the view through `app/navigation`, rechecks current input and scope, then applies it without changing focus.

Viewers reuse Monitor, documents, and Diff Reviewer. Navigation results are execution metadata, outside searchable evidence. Standalone MCP has no UI tools.

Tests cover rendering, draft preservation, cancellation, and late navigation. PlousBench uses the real loader with a deterministic display endpoint.

## History tools

| Operation | Results |
| --- | --- |
| `search` | Matching committed messages and observations, with index status |
| `read` | Original text or projected rows at a saved reference |
| `window` | User and assistant exchanges around a reference |
| `count` | Deduplicated typed counts with completeness and unknowns |
| `compare` | Shared, added, and removed identities for compatible observations |

History owns argument validation. Only `count` and `compare` accept `field` / `equals`. Only `read` accepts `fields`.

Observation offsets are row indices. Text offsets are UTF-8 bytes. Follow the returned continuation field for the operation. Counts use the whole saved observation, not only the displayed page.

Access stops at the captured committed watermark, plus observations saved by the current turn. There is no model-facing JSONL path, raw-body lookup, filesystem fallback, or arbitrary evaluator.

## Search and indexing

Opening a conversation starts lexical catch-up. Commits notify active indexes. User messages and observations become searchable after commit, and assistant replies after settlement.

General chat searches its conversation. Connection history can search that connection's conversations. Index lag is reported, and known references remain readable before catch-up. An empty incomplete search does not prove absence.

Optional semantic retrieval uses an explicit embedding connection. Background failures do not block saves or live reads, though a semantic query can wait for its embedding.

Indexes cover saved messages and observations. They do not scan repositories, native checkpoints, navigation results, model reasoning, or the cluster continuously. Derived indexes can be rebuilt without live access.

## Saved records and Sources

Conversation JSONL keeps original messages, IDs, timestamps, and status. Reasoning shards contain projected observations. Execution shards contain requests, native checkpoints, and usage outside searchable evidence.

One history store owns persistence and recovery. Close it before releasing the profile lock.

Cancellation keeps completed observations and accepted partial text. Interrupted turns do not resume automatically. The current format has no legacy readers or migrations.

Sources reads saved results and history receipts without API or provider calls, including after restart. Known credentials are redacted, but arbitrary logs and user text can remain sensitive. See [privacy](../reference/data-and-privacy.md).

## Verify follow-ups and history

Use temporary profiles, fixture APIs, and scripted models. Test recent context and older retrieval separately. Save a fact, correct it, ask a follow-up, restart, and repeat. Assert saved references and timestamps for actual retrieval.

Cover pagination, partial results, missing fields, replaced identities, scope boundaries, persistence-before-delivery, Sources, cancellation, invalid arguments, provider errors, and budgets.

Run `make check` for application changes. For concurrency changes:

```sh
make test-race \
  TEST_PACKAGES='./internal/ai/... ./tests/integration/ai'
```

[PlousBench](../../benchmark/README.md) checks production readers, durable evidence, failure behavior, latency, response size, and API reads through direct calls and MCP. It does not measure model answer quality.

Run `make test-agent-clients` after changing schemas or native launch controls. Both CLIs must already be installed. The harness uses temporary homes, synthetic keys, loopback providers, and stubbed tool execution.

It checks tool schemas, unchanged call arguments, matching result IDs, inherited-server exclusion, streaming, and resume. It does not verify live accounts, subscriptions, or answer quality.

To check one installed client:

```sh
PLOUS_TEST_INSTALLED_CLI=codex make test-unit \
  TEST_PACKAGES='./tests/integration/nativeagents' \
  TEST_FLAGS='-count=1 -run TestInstalledNativeCLIWithMockProvider'
```

Use `claude-code` instead of `codex` for Claude Code.

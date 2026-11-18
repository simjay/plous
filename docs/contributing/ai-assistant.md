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

The built-in turn captures the model, credentials, cluster connection, and page hint. A hosted CLI session captures its executable and holds its cluster connection until closure. General chat and history-only views have no live cluster tools. The connection sets the cluster and credentials. A read can select a namespace those credentials allow.

## Agent tool design

**Use one public tool per domain and authority boundary.** A tool name identifies the owner and kind of access. An operation selects the work within that boundary. Its schema states which arguments the operation accepts.

| Change | Where it belongs |
| --- | --- |
| Read another aspect of Kubernetes within the captured cluster | Add an operation or option to `kubernetes_read` |
| Read another aspect of a Helm release | Extend `helm_read` |
| Read saved monitor status or alerts | Use `monitor_read` within the captured Kubernetes connection |
| Return an additional fact from an existing read | Extend the owning projection or use its field selector |
| Execute a command or start a temporary diagnostic | Use `kubernetes_run` with the operation's host grant |
| Read or open a Plous view | Use `plous_view` through the session UI port |
| Inspect or change the standalone session target | Use `kubernetes_context_read` or the separately confirmed `kubernetes_context_select` |

The tool name alone does not grant access. Exec and temporary diagnostics have independent grants even though they share `kubernetes_run`. The MCP client can filter or annotate the whole tool by name. The Go handler checks the requested operation and target on every call. A skill describes a workflow and uses the same tools as other callers.

### Follow a call through the code

1. **Register the tool in the host.** [The registry](../../internal/ai/tools.go) supplies the name, schema, handler binding, and scope-based availability for built-in Plous and MCP. It adds common arguments such as `purpose` and declares which connection type a cluster-bound tool accepts. Standalone target tools are registered by [the standalone host](../../internal/app/debugging/session.go). They work before a cluster tool session exists.
2. **Keep domain rules with the implementation.** Each integration owns its operation arguments and validation in its `monitor/read_tool.go`. Kubernetes diagnostics own their argument and grant definitions in [run_tool.go](../../internal/integrations/kubernetes/monitor/run_tool.go). Use the same definitions for schemas and runtime field admission. [Schema assembly](../../internal/integrations/observation/operation_schema.go) shares the wire format. [Argument parsing](../../internal/integrations/observation/arguments.go) supplies one validated object for host routing and domain decoding. Check resolved scope, resource identity, and contextual rules in Go. For example, the diagnostic handler requires a port for TCP and rejects it for DNS or HTTP. The client may show the schema as a hint. Runtime validation still applies.
3. **Keep transport and authority separate.** [The tool session](../../internal/ai/tool_session.go) resolves `connection_id` against the connection type declared in the registry and supplies the captured connection to [the cluster binding](../../internal/ai/cluster_tools.go). The session serializes calls, records active diagnostic intent, and persists observations before delivery. Saved-history queries and view receipts have dedicated handling in the same session. MCP transports the shared contract. The TUI can call integration readers directly through typed Go inputs and keeps its own action and confirmation flow.
4. **Project facts at the read boundary.** An observation carries identities, typed facts, observation time, and coverage. Raw API bodies are temporary. Kubernetes `get` can acquire selected scalar `paths` under masking. The `fields` option selects already projected row facts for delivery. A saved reference lets a caller page through the same observation.

The TUI builds the same tool options for the context meter, built-in turns, and native agents. The TUI and standalone MCP hosts supply the [monitor reader](../../internal/app/monitor/read_tool.go). The reader owns the monitor schema and saved alert access. The shared tool session checks the captured Kubernetes connection before invoking it.

### Extend an existing reader

For a new Kubernetes CRD, first try the generic path. `discover` lists served API types, `list` enumerates objects of one type, and `get` reads a named object. Explore and the agent share [catalog acquisition](../../internal/integrations/kubernetes/monitor/discovery.go) and [version resolution](../../internal/integrations/kubernetes/common/discovery.go) through their own transports. A new type can use those operations and request masked fields without a kind-specific handler. Add an integration projection when Go must compute a domain fact, such as following Pod ownership or establishing readiness from controller status. Compute mechanical facts as typed Go values so the UI, monitor, and agent projection can reuse them.

When changing the contract, update its schema and validator together. Test one valid call and the relevant denials, including unrelated arguments supplied as zero, empty, or false. Check that the same operation and scope work through direct calls and MCP. If you replace a tool name, update the skill, docs, fixtures, and benchmark in the same change. Preserve saved references and the meaning of old observations so Sources and history comparisons still work.

## Standalone MCP and active diagnostics

`app/debugging` owns `plous setup` and `plous mcp serve`. Setup installs the embedded skill and managed MCP entry. Serve opens a separate profile and adapts `agent.Tools` through `cli.ServeStdio`. Each call captures the selected target, which follows kubeconfig’s current context by default. An operator-confirmed target change applies between calls. Standalone sessions have no `agent.UI` port.

Kubernetes owns `kubernetes_run exec` and `kubernetes_run diagnostic`. Launch options explicitly enable them. Both record intent before dispatch and results before delivery.

Temporary tests use a fixed Job template, deadlines, identity-checked cleanup, and TTL fallback. Exec cancellation does not guarantee remote termination. See [diagnostic contracts](../reference/mcp.md#active-diagnostics).

Usage projections share the TUI's identity, ownership, freshness, and aggregation rules. The agent uses a guarded reader, not the unrestricted TUI client.

## UI tools

`plous_view` uses `agent.UI` for `get` and `open`. The tool session validates the target. The app loop prepares the view through `app/navigation`, rechecks current input and scope, then applies it without changing focus.

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

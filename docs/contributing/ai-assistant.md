# AI implementation

[Documentation](../README.md) · [AI chat](../guides/ai-chat.md) · [Plous MCP](../guides/plous-mcp.md) · [Contributing](README.md)

Plous runs one selected main agent: built-in Eino with OpenAI or Anthropic, or the user-installed Claude Code or Codex executable. Each calls the same connection tools and history. Go owns scope, projection, persistence and limits; the model interprets the returned evidence. There are no delegated agents, recursive workers or interpreter processes.

## Runtime prerequisites

The executable includes the Eino provider adapters and Plous tools. The built-in agent needs a supported saved provider connection and network access to it. It needs no Python, external interpreter, OS sandbox utility, embedding service or Go compiler at runtime. Kubernetes credential helpers remain a separate connection requirement. External agents need the installed executable on PATH and its own authentication. Native terminal checks use Codex 0.160.0 and Claude Code 2.1.284. Compatible installed releases are not limited by an exact version allowlist. Neither binary is bundled or installed by Plous.

## Configuration

The [configuration reference](../reference/configuration.md#agent-runtime-configuration) owns the optional runtime YAML, defaults, turn budgets, and embedding settings. The host captures them at operation start; native CLI sessions keep their own model context and spending controls.

## Runtime structure

`internal/ai/run.go` owns the Eino conversation loop. `compaction.go` handles context fitting. `call.go` and `budget.go` handle provider dispatch and turn spending. `model/` owns the provider wire format, while `history/` owns durable messages and evidence.

The loop accesses tools through `internal/ai/agent.Tools`, a standard-library-only contract containing JSON schemas and calls. `tool_session.go` implements that contract using the captured target and history access; `tools.go` registers the integration schemas. The session serializes reads, bounds delivery, saves evidence before returning it, and rejects calls after cancellation, fatal failure or closure. Its lifetime ends with the turn. Tool schemas and projectors have one implementation regardless of the caller.

`agent.Backend` defines the built-in Eino turn and events. External assistants use `cli.StartTerminal` instead: an installed interactive executable runs in a PTY, while `app/chat.Native` owns its terminal screen and input lifecycle. The shared bounded terminal input queue is in `internal/ui/terminal`. No stdout text is interpreted as model protocol or copied into Plous replies.

`OpenExternalTools` captures one scope and history boundary, creates a labeled tool-evidence conversation and exposes the same `agent.Tools` session through the authenticated loopback MCP bridge. A native process can contain multiple CLI turns. Its captured tool scope lasts until the process closes; it does not inherit Eino turn budgets. Navigation cannot retarget it. Session completion closes source readers and saves lifecycle status; partial observations remain available after restart.

Claude Code launches interactively with built-in tools, skills and hooks disabled and strict MCP configuration. Codex uses a short-lived App Server configuration read to identify inherited MCP server names without starting a thread or clients. Its native launch disables those entries, chooses an unused Plous bridge name and uses read-only sandboxing. MCP overrides use one inline TOML table because CLI dotted keys do not interpret quoted server names. Disabled entries carry a harmless transport and no copied credentials. The Code Mode host stays disabled; the Plous namespace is exposed directly. Installed-binary tests verify that a requested Code Mode execution is rejected.

The native CLI owns its conversation and resume UI. Per-agent, per-launch-directory, per-scope session metadata keeps resume selection separate; both CLIs run in the directory where Plous was launched. Plous retains only a resume marker and tool evidence; it neither parses native transcripts nor translates them into Eino checkpoints. Explicit session closure or application exit stops the process group on supported macOS/Linux builds and revokes its tool bridge. Fatal tool failures revoke the bridge without closing the CLI or losing its draft. Temporary MCP configuration is removed on closure. Global CLI settings and authentication remain unchanged.

## Main loop and connection tools

`internal/ai/run.go` uses native ordered messages, recent complete exchanges and a compiled tool registry. The host compacts older complete exchanges when needed, using the captured main model without tools. It preserves recent context and original saved records. An oversized current exchange or exhausted allowance stops explicitly.

Capture the main agent, one connection and a safe page hint at operation start. Eino additionally captures its provider/model and credentials; external agents capture the executable path. Both live dispatch and history enforce the captured scope. Page namespace does not restrict the cluster's authorized namespaces. General and unavailable history-only views have no connection tools. UI navigation cannot retarget an operation.

The [MCP tool reference](../reference/mcp.md#available-tools) describes the shared readers and their evidence. Eino calls them directly; external agents discover and call them through the captured native session's MCP bridge. Both paths use the same host session.

Schemas, operation policies and domain projectors belong to their integrations. Existing operation definitions own accepted and required arguments; schema generation and runtime validation reuse them and shared bounds. Source also checks which JSON fields were supplied, so an unsupported explicit zero or false value cannot disappear during decoding. Execution consumes the typed Go operation definitions directly; schema generation is only for advertising the inputs. Single-operation tools infer their operation from the tool name and reject an explicit operation argument. Integration-specific semantic validation stays in Go beside the reader. Presentation schemas and domain validation also belong to their integration, Source or History; AI registers them and checks the captured connection.

Unknown operations and writes are rejected before acquisition. Projection records carry identity, time, selected fields and coverage. Persist a validated observation before model delivery; raw API responses are transient. Invalid arguments receive corrective feedback. History scope/integrity, cancellation and budget failures stop. Denied live operations return an explicit tool result.

Kubernetes current usage reuses integration-owned identity, ownership, freshness and coverage rules with the TUI. The agent path supplies a guarded, bounded reader; it never borrows the TUI's unrestricted client. Projected CPU nanocores and memory bytes retain source intervals and collection time. Direct Metrics API `get` reads remain raw samples without that object join.

Source file tools read current saved local bytes or an already acquired Git commit. Search uses bounded pages, stable continuation options and content identities. Captured analysis does not render or fetch a repository. Scanning follows the profile and repository ignore rules; exact reads and declared render inputs bypass scan exclusions while enforcing access restrictions. See [source exclusions](../reference/data-and-privacy.md#global-source-scanning-exclusions).

## UI tools

`ui_read`, `kubernetes_show`, `helm_show`, `source_show` and `history_show` use a standard-library `agent.UI` port supplied by the app. The guarded tool session validates the requested view and captured connection. Evidence refs resolve through the same history access boundary. The app receives requests on its event loop, prepares targets through `internal/app/navigation`, then rechecks the current scope and input before applying them. Only the UI loop changes focus or widgets. Keyboard focus stays unchanged.

Views reuse Monitor, the shared read-only document and Diff Reviewer. Source display follows the guarded source file policy; Helm comparisons mask sensitive values. UI responses enter the execution journal, outside searchable facts. The runtime never supplies UI capabilities without a host port. PlousBench uses the real loader with a deterministic display endpoint; app tests cover actual rendering, draft preservation, cancellation and late navigation.

## History tools

| Operation | Contract |
| --- | --- |
| `search` | Scoped committed conversation/observation excerpts with index coverage |
| `read` | Original text or a page of projected rows at an exact saved reference |
| `window` | Original user/assistant exchanges around a reference, retaining response status |
| `count` | Deduplicated typed observation count with completeness and unknowns |
| `compare` | Shared, added and removed identities for matching schema, selection and scope |

History owns the operation-specific argument definitions and rejects inappropriate arguments before storage access, including explicit zero values. Only `count` and `compare` accept the `field`/`equals` filter; only `read` accepts `fields` for selecting returned row facts.

Read offsets are row indices for observations and UTF-8 byte offsets for text. Follow `page.next_offset`, `next_text_offset` or `next_window` as appropriate. Counts use the whole saved observation, regardless of displayed rows. Missing fields or partial collections make exact conclusions unavailable. Search-hit counts do not establish resource counts or absence.

Access is bounded by the captured committed history watermark. The turn can admit its own newly saved observations. No model-facing JSONL paths, raw-body lookup, filesystem fallback or arbitrary evaluator exists. Referencing a record without reading it does not establish evidence delivery.

## Search and indexing

Opening a conversation starts background lexical catch-up; turn setup reuses it. Durable commits notify active indexes. User messages and projected observations become eligible after commit; assistant replies after settlement. General searches its conversation, while connection scope can search that connection's conversations. Index lag is explicit; known saved references remain readable before catch-up. Empty incomplete search does not prove absence.

Optional Monitor semantic retrieval uses an explicitly configured embedding connection. Background failure does not block saving or live reads; a semantic query may wait for its embedding and report fallback/lag. Usage is separate. Repository contents, native checkpoints, derived history receipts and model reasoning are not a searchable corpus. Derived indexes can be rebuilt without live access.

History indexing does not scan Code or the cluster. Code discovery starts on opening/refresh, retains bounded configuration metadata and releases raw candidate buffers. Search traverses files per request; rendering collects only declared inputs for the selected target. Neither creates a persistent repository text/vector index. A local multi-file capture is not an atomic filesystem snapshot.

## Saved records and Sources

Original conversation JSONL preserves messages, IDs, times and lifecycle. Reasoning shards contain only projected observations. Execution shards hold requests, native checkpoints and usage outside searchable evidence. One injected history store owns durability/recovery; close it before releasing the profile lock.

Cancellation retains completed observations and accepted partial response text. Interrupted turns do not resume automatically. Recent completed Plous conversation exchanges survive restart, while failed/cancelled status remains explicit. The current format has no legacy chat readers or migrations.

Sources opens saved turn-bound projections and history receipts without API or provider calls, including after restart. Known credentials are redacted, but arbitrary user text or logs may remain sensitive. See [privacy](../reference/data-and-privacy.md).

## Verify follow-ups and history

Use temporary profiles, scripted models and fixture APIs. Test recent context and older explicit retrieval separately. Save a decision, correct it, ask a follow-up, restart and repeat. For actual retrieval assert the original saved references and timestamps, not merely a correct final answer.

Verify independent fixture facts, paginated/partial lists, unknown readiness, replaced identities, selectors, history scope/watermarks, persistence-before-delivery, Sources, cancellation, invalid arguments, provider errors and shared budgets. Run `make check`; use focused `make test-race TEST_PACKAGES='./internal/ai/... ./tests/integration/ai'` for concurrency changes.

[PlousBench](../../benchmark/README.md) calls this runtime's guarded tools directly and through MCP. It checks returned facts, scope, persistence and failure behavior, then measures tool latency, response size and API reads. It does not run the Eino loop or measure model answer quality.

Run `make test-agent-clients` after changing tool schemas or native launch controls. It requires both CLIs already installed and uses temporary CLI homes, synthetic keys and loopback provider APIs. It checks that all 17 production tool schemas reach each client's model request, every tool call reaches MCP with unchanged arguments, and every result returns to the model under the matching call ID, including after resume. Tool execution is stubbed here; PlousBench checks the production readers and persistence separately. The executable harness lives in `tests/integration/nativeagents`. The client checks also cover inherited-server exclusion, streaming, completion and session resume. They do not verify live accounts, subscription access or model answer quality.

To check one installed client:

```sh
PLOUS_TEST_INSTALLED_CLI=claude-code make test-unit TEST_PACKAGES='./tests/integration/nativeagents' TEST_FLAGS='-count=1 -run TestInstalledNativeCLIWithMockProvider'
PLOUS_TEST_INSTALLED_CLI=codex make test-unit TEST_PACKAGES='./tests/integration/nativeagents' TEST_FLAGS='-count=1 -run TestInstalledNativeCLIWithMockProvider'
```

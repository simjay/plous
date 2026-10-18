# MCP reference

[Documentation](../README.md) · [MCP setup and examples](../guides/plous-mcp.md) · [AI implementation](../contributing/ai-assistant.md)

This page describes the MCP bridge implemented by this Plous build. For connecting an installed agent and asking a first question, start with the [MCP guide](../guides/plous-mcp.md).

## Available tools

The temporary MCP server normally uses the name `plous`; Codex adds a suffix if that name is already present in your settings. These are the tool names it can advertise; the actual list depends on the captured connection. The built-in agent uses these same schemas and implementations.

| Type | Tools | Coverage |
| --- | --- | --- |
| Object inspection | `kubernetes_resources_read`, `helm_releases_read`, `argo_resources_read`, `flux_resources_read` | Resource state, release inventory and controller status within each technology |
| Configuration and files | `helm_configuration_read`, `source_files_read` | Masked release values/manifests/hooks; source file lists, contents and search |
| Logs | `kubernetes_logs_read` | Bounded Pod or owned-workload log excerpts |
| Usage | `kubernetes_usage_read` | Current Pod, Node or workload measurements and budgets |
| Analysis and comparison | `helm_diff_read`, `source_analysis_read` | Retained release differences; captured Code resources, fields, provenance, findings and comparisons |
| App records | `workspace_read`, `history_read` | Saved workspace/Activity metadata; scoped saved messages and observations |
| Presentation | `ui_read`, `kubernetes_show`, `helm_show`, `source_show`, `history_show` | Inspect the current view; show Kubernetes resources/lists/logs, Helm revision comparisons, source locations or saved evidence |

Tool names end in `_read` for information returned to the agent or `_show` for a view presented to the user. Reads retain their projected facts, calculations, coverage and bounded pages. Shows return presentation receipts. `kubernetes_logs_read`, `kubernetes_usage_read` and `helm_diff_read` take no `operation` argument. `kubernetes_show` selects `view=resource`, `list` or `logs`; the other show tools have one destination and take no `view` argument. `helm_show` currently opens revision comparisons, not release lists. For both Helm comparison tools, `revision` is the after state and `compare_revision` is the before state.

The captured connection and page hint arrive with the session instructions; there is no separate `plous_context` tool. History is called directly, with no RLM or recursive recall service.

Integration-owned Go definitions supply accepted arguments, required fields and bounds to the advertised schemas and validators. Execution checks those definitions directly without reading generated schemas. Unknown or inappropriate arguments are rejected before acquisition. Calls include a purpose. Connection reads name the captured connection identity; supplying another identity cannot grant access. Exact schemas belong to the [tool registry](../../internal/ai/tools.go) and its integration-owned definitions. History operations and continuation rules are documented in [History tools](../contributing/ai-assistant.md#history-tools).

Schemas use an object root with operation-dependent constraints nested under `dependentSchemas`. This keeps connector and history tools visible to Claude Code without requiring its optional root-union schema rewriting. The same constraints apply to built-in Plous requests and native-agent requests.

### Monitor evidence

Kubernetes, Helm, Argo and Flux use the same captured cluster. Their evidence has different meanings: a Helm revision records a release, while a controller's reported sync or health does not establish current downstream resource health. The agent may need additional Kubernetes reads to investigate that state.

`kubernetes_usage_read` takes a resource and name for a current Pod, Node or supported workload. Measurements use the same rules as TUI charts, including ownership checks and complete coverage before totaling workload usage. This reads current APIs without depending on an open chart. Results retain CPU nanocores, memory bytes, source intervals, collection time and coverage. Unknown measurements remain distinct from zero. The saved result can be read through History or Sources, but the operation cannot retrieve unsaved TUI chart history.

`kubernetes_logs_read` requires a Pod or supported workload name and an explicit container. Workload reads follow controller ownership, check Pod identities before and after collection, and return one row per attempted Pod. They read at most 16 Pods, with a combined byte limit of 64 KiB and at most 500 lines per Pod. Failed and skipped sources are reported; excerpts never establish complete log history.

`helm_diff_read` uses `revision` for the after revision and optional `compare_revision` for the before revision. An explicit pair requires two distinct positive retained revisions. Without a pair, it compares the selected or latest revision to its predecessor; revision 1 uses an empty baseline.

Missing permissions, partial lists, stale metrics and unknown fields remain explicit. Bounded log excerpts may omit relevant lines. Use [Kubernetes](../integrations/kubernetes/README.md), [Helm](../integrations/helm/README.md) and [Argo and Flux](../integrations/gitops/README.md) for each integration's coverage.

### Code and historical evidence

Source file reads use current saved local bytes or an already acquired Git revision. Unsaved editor buffers are excluded. Catalogs, rendered resources and field provenance use captured Code results. These tools do not start rendering, fetch another revision or acquire another repository; open or render the relevant configuration in Code before asking about its analysis.

History contains evidence that was saved, not a continuous archive of everything you browsed. A fresh read observes the present and cannot recover an unrecorded past value. Search finds candidate records; search-hit totals are not resource counts. Typed counts and comparisons report completeness and unknowns. The tool can read its own newly saved observations during the same session.

## Scope and delivery

The selected cluster or source is captured when the native session opens. The current namespace and page are hints, not extra permission boundaries. All cluster reads use that connection's Kubernetes credentials; a request may name another permitted namespace but cannot select another connection. General chat and history-only views have no live connection tools.

The host serializes calls, validates arguments and scope, and saves projected observations before returning them. Saved observations include identity, timestamps and coverage; raw API bodies are transient. Correctable argument errors return feedback. Fatal tool, scope or history-integrity failures revoke tool access. The native terminal remains open so its draft and conversation remain available.

Read results and individual operations are bounded. Native sessions have no Plous-imposed process deadline or session-wide tool-call cap. Model calls, context, retries and spending belong to the CLI. Eino turn budgets apply only to the built-in Plous agent.

## MCP connection details

Plous creates a temporary `http://127.0.0.1:<port>/mcp` endpoint protected by a per-session bearer token and configures the installed agent to use it. You do not copy an endpoint or token into global CLI settings. Access ends with the session; the bridge is not a remote workspace service.

The supported requests, protocol version and response rules are listed under [Protocol behavior](#protocol-behavior). Installed-client checks use the versions listed in [agent setup](../guides/ai-chat.md#choose-an-agent).

The local MCP connection carries requests to Plous. Provider inference still uses the CLI's configured account and network connection. Both installed agents render their own UI through a PTY. Codex App Server is used only for a configuration read before launching its native terminal.

See [AI implementation](../contributing/ai-assistant.md#runtime-structure) for the shared tool contract, adapter lifecycle and verification commands, or [AI troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send) for setup failures.

UI tools are available while Plous hosts the agent. They reuse the application views and return `opened`, `blocked` or `failed`; they do not send keyboard input or write resources. UI receipts are saved as execution metadata, separate from searchable observations. See [showing views](../guides/ai-chat.md#ask-the-assistant-to-show-a-view).

## Protocol behavior

| Property | Implemented behavior |
| --- | --- |
| Transport | JSON-RPC 2.0 over HTTP POST on loopback; one request per body |
| Authentication | A fresh per-session bearer token; requests with an Origin header are rejected |
| Initialization | `initialize` returns protocol version `2025-03-26` and the tools capability |
| Requests | `initialize`, `ping`, `tools/list`, `tools/call` |
| Notifications | `notifications/initialized` and `notifications/cancelled` are accepted without a response body |
| Discovery | Tool names, descriptions, input schemas and read-only annotations |
| Results | Tool output returned as text content; presentation results are receipts |
| Other MCP features | No resources, prompt templates, public registration or standalone server |

Each request ID must be a string or integer. A tool-call ID cannot be reused within the bridge session. Invalid arguments and denied reads can appear in a tool result, so callers must inspect the content rather than treating transport success as successful acquisition. Unsupported methods return a JSON-RPC error. After a fatal tool failure, later authenticated requests receive a closed-session response.

The bridge's protocol version describes its implemented subset; it does not claim support for every feature of newer MCP specifications. The [bridge implementation](../../internal/ai/cli/bridge.go) owns transport details, and the [native launch configuration](../../internal/ai/cli/terminal_config.go) owns client setup. See [verification](../contributing/ai-assistant.md#verify-follow-ups-and-history) for the installed-client mock-provider checks and their limits.

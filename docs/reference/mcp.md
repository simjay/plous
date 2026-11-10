# MCP tools and permissions

[Documentation](../README.md) · [MCP server](../mcp/README.md) · [Client setup](../mcp/setup.md) · [AI implementation](../contributing/ai-assistant.md)

This page describes the tools and transports implemented by this Plous build. Start with [MCP client setup](../mcp/setup.md) for direct tool access, [plous-debug](../agent-skills/plous-debug.md) for a guided skill, or the [assistant panel](../guides/plous-mcp.md) inside the TUI.

| Mode | Transport | Setup | Plous view tools |
| --- | --- | --- | --- |
| Claude Code or Codex inside Plous | Private loopback HTTP | Automatic when the native session opens | Available |
| Independent coding agent | Stdio subprocess | `plous setup codex` or `plous setup claude` | Omitted |

## Available tools

Standalone setup registers `plous_debug`. The TUI MCP server normally uses the name `plous`; Codex adds a suffix if that name is already present in your settings. These are the tool names it can advertise; the actual list depends on the captured connection. The built-in agent uses these same schemas and implementations.

| Type | Tools | Coverage |
| --- | --- | --- |
| Object inspection | `kubernetes_resources_read`, `helm_releases_read`, `argo_resources_read`, `flux_resources_read` | Resource state, release inventory and controller status within each technology |
| Release configuration | `helm_configuration_read` | Masked release values/manifests/hooks |
| Logs | `kubernetes_logs_read` | Bounded Pod or owned-workload log excerpts |
| Active diagnostics | `kubernetes_exec_run`, `kubernetes_diagnostic_run` | Explicitly enabled commands in existing containers, or temporary HTTP/DNS/TCP tests |
| Usage | `kubernetes_usage_read` | Current Pod, Node or workload measurements and budgets |
| Analysis and comparison | `helm_diff_read` | Retained release differences |
| App records | `workspace_read`, `history_read` | Saved workspace/Activity metadata; scoped saved messages and observations |
| Presentation | `ui_read`, `kubernetes_show`, `helm_show`, `history_show` | Inspect the current view; show Kubernetes resources/lists/logs, Helm revision comparisons or saved evidence |

Diagnostic tool names end in `_run` and may change cluster or application state. Other tool names end in `_read` for information returned to the agent or `_show` for a view presented to the user. Reads retain their projected facts, calculations, coverage and bounded pages. Shows return presentation receipts. `kubernetes_logs_read`, `kubernetes_usage_read` and `helm_diff_read` take no `operation` argument. `kubernetes_show` selects `view=resource`, `list` or `logs`; the other show tools have one destination and take no `view` argument. `helm_show` currently opens revision comparisons, not release lists. For both Helm comparison tools, `revision` is the after state and `compare_revision` is the before state.

The captured connection and page hint arrive with the session instructions, and connection schemas enumerate their captured identities. History is called directly through `history_read`.

Integration-owned Go definitions supply accepted arguments, required fields and bounds to the advertised schemas and validators. Execution checks those definitions directly without reading generated schemas. Unknown or inappropriate arguments are rejected before acquisition. Calls include a purpose. Connection reads name the captured connection identity; supplying another identity cannot grant access. Exact schemas belong to the [tool registry](../../internal/ai/tools.go) and its integration-owned definitions. History operations and continuation rules are documented in [History tools](../contributing/ai-assistant.md#history-tools).

Schemas use an object root with operation-dependent constraints nested under `dependentSchemas`. This keeps connector and history tools visible to Claude Code without requiring its optional root-union schema rewriting. The same constraints apply to built-in Plous requests and native-agent requests.

### Monitor evidence

Kubernetes, Helm, Argo and Flux use the same captured cluster. Their evidence has different meanings: a Helm revision records a release, while a controller's reported sync or health does not establish current downstream resource health. The agent may need additional Kubernetes reads to investigate that state.

`kubernetes_usage_read` takes a resource and name for a current Pod, Node or supported workload. Measurements use the same rules as TUI charts, including ownership checks and complete coverage before totaling workload usage. This reads current APIs without depending on an open chart. Results retain CPU nanocores, memory bytes, source intervals, collection time and coverage. Unknown measurements remain distinct from zero. The saved result can be read through History or Sources, but the operation cannot retrieve unsaved TUI chart history.

`kubernetes_logs_read` requires a Pod or supported workload name and an explicit container. Workload reads follow controller ownership, check Pod identities before and after collection, and return one row per attempted Pod. They read at most 16 Pods, with a combined byte limit of 64 KiB and at most 500 lines per Pod. Failed and skipped sources are reported; excerpts never establish complete log history.

`helm_diff_read` uses `revision` for the after revision and optional `compare_revision` for the before revision. An explicit pair requires two distinct positive retained revisions. Without a pair, it compares the selected or latest revision to its predecessor; revision 1 uses an empty baseline.

Missing permissions, partial lists, stale metrics and unknown fields remain explicit. Bounded log excerpts may omit relevant lines. Use [Kubernetes](../integrations/kubernetes/README.md), [Helm](../integrations/helm/README.md) and [Argo CD](../integrations/argo/README.md) and [Flux](../integrations/flux/README.md) for each integration's coverage.

### Historical evidence

History contains evidence that was saved, not a continuous archive of everything you browsed. A fresh read observes the present and cannot recover an unrecorded past value. Search finds candidate records; search-hit totals are not resource counts. Typed counts and comparisons report completeness and unknowns. The tool can read its own newly saved observations during the same session.

## Active diagnostics

Both built-in and external agents use the same diagnostic tools when explicitly enabled. They require `purpose`, the captured Kubernetes `connection_id`, and its concrete default `namespace`. `timeout_seconds` defaults to 30 and accepts 1-120. Active tests cannot select another namespace or connection.

- `kubernetes_exec_run`: enabled with `--allow-exec`. Requires Pod `name`, exact `uid`, `container`, and `command` as an argument array. There is no inserted shell, stdin, or TTY. Combined stdout/stderr is bounded to 64 KiB. Exit code, timeout, output limits, and unknown remote outcomes are recorded.
- `kubernetes_diagnostic_run`: enabled with `--diagnostic-image IMAGE`. Requires `test=http|dns|tcp` and `target`; TCP also requires `port`. HTTP uses a complete http(s) URL without embedded credentials and performs one GET without redirects or retries. The host chooses the image and fixed Job configuration. The result includes runner identity, exit status, output, and cleanup. A temporary runner's network access does not establish application access.

Active intents are saved in execution records before dispatch. Results use projected observations and the same persistence-before-delivery path as reads. Transport or storage failures must not cause automatic retries of state-changing tests. See [setup, permissions, cleanup, and examples](../mcp/active-diagnostics.md).

## Scope and delivery

The selected cluster is captured when the native session or standalone process opens. For reads, the current namespace and page are hints, not extra permission boundaries: requests may name another namespace allowed by the captured Kubernetes credentials. Active diagnostics stay restricted to the captured default namespace. Neither kind of tool can select another connection. General chat and history-only views have no live connection tools.

The host serializes calls, validates arguments and scope, and saves projected observations before returning them. Saved observations include identity, timestamps and coverage; raw API bodies are transient. Correctable argument errors return feedback. Fatal tool, scope or history-integrity failures revoke tool access. The native terminal remains open so its draft and conversation remain available.

Read results and individual operations are bounded. Native sessions have no Plous-imposed process deadline or session-wide tool-call cap. Model calls, context, retries and spending belong to the CLI. Eino turn budgets apply only to the built-in Plous agent.

## MCP connection details

`plous mcp serve --context NAME` serves standalone clients over newline-delimited stdio without opening a port. It captures the explicit context and uses a separate durable profile per process. See [client setup](../mcp/setup.md) and [scope and lifecycle](../mcp/scope.md). UI tools are omitted.

For the assistant panel, Plous creates a temporary `http://127.0.0.1:<port>/mcp` endpoint protected by a per-session bearer token and configures the installed agent to use it. You do not copy an endpoint or token into global CLI settings. Access ends with the session.

The supported requests, protocol version and response rules are listed under [Protocol behavior](#protocol-behavior). Installed-client checks use the versions listed in [agent setup](../guides/ai-chat.md#choose-an-agent).

The local MCP connection carries requests to Plous. Provider inference still uses the CLI's configured account and network connection. Both installed agents render their own UI through a PTY. Codex App Server is used only for a configuration read before launching its native terminal.

See [AI implementation](../contributing/ai-assistant.md#runtime-structure) for the shared tool contract, adapter lifecycle and verification commands, or [AI troubleshooting](../reference/troubleshooting.md#ai-chat-will-not-send) for setup failures.

UI tools are available while Plous hosts the agent. They reuse the application views and return `opened`, `blocked` or `failed`; they do not send keyboard input or write resources. UI receipts are saved as execution metadata, separate from searchable observations. See [showing views](../guides/ai-chat.md#ask-the-assistant-to-show-a-view).

## Protocol behavior

| Property | Implemented behavior |
| --- | --- |
| Transport | JSON-RPC 2.0 over TUI loopback HTTP POST or standalone newline-delimited stdio; one request per message |
| Authentication | TUI HTTP uses a fresh per-session bearer token and rejects Origin headers. Stdio uses its subprocess pipes, with no listener or bearer credential. |
| Initialization | `initialize` returns protocol version `2025-03-26` and the tools capability |
| Requests | `initialize`, `ping`, `tools/list`, `tools/call` |
| Notifications | `notifications/initialized` is accepted; `notifications/cancelled` cancels a matching active tool request without a response body |
| Discovery | Tool names, descriptions, input schemas, and annotations that distinguish reads from active diagnostics |
| Results | Tool output returned as text content; presentation results are receipts |
| Other MCP features | No resources or prompt templates |

Each request ID must be a string or integer. A tool-call ID cannot be reused within the bridge session. Invalid arguments and denied reads can appear in a tool result, so callers must inspect the content rather than treating transport success as successful acquisition. Unsupported methods return a JSON-RPC error. After a fatal tool failure, later authenticated requests receive a closed-session response.

The bridge's protocol version describes its implemented subset; it does not claim support for every feature of newer MCP specifications. The [bridge implementation](../../internal/ai/cli/bridge.go) owns transport details, and the [native launch configuration](../../internal/ai/cli/terminal_config.go) owns client setup. See [verification](../contributing/ai-assistant.md#verify-follow-ups-and-history) for the installed-client mock-provider checks and their limits.

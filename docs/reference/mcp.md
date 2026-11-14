# MCP tools and permissions

[Documentation](../README.md) · [Client setup](../mcp/setup.md)

This reference lists Plous's agent tools. For a guided setup, use [plous-debug](../agent-skills/plous-debug.md) or the [assistant panel](../guides/plous-mcp.md).

## Available tools

Standalone setup names its server `plous_debug`. The panel normally uses `plous`, with a suffix in Codex if needed. The built-in agent uses the same tool implementations. See [Standalone target discovery](#standalone-target-discovery) for the standalone server's context lookup.

| Task | Tools | Results |
| --- | --- | --- |
| Inspect objects | `kubernetes_resources_read`, `helm_releases_read`, `argo_resources_read`, `flux_resources_read` | Resources, releases, controller status |
| Read Helm configuration | `helm_configuration_read` | Filtered values, manifests, hooks |
| Read logs | `kubernetes_logs_read` | Pod or workload log excerpts |
| Run diagnostics | `kubernetes_exec_run`, `kubernetes_diagnostic_run` | Explicitly enabled commands or temporary tests |
| Read usage | `kubernetes_usage_read` | Current Pod, Node, or workload usage and budgets |
| Compare Helm revisions | `helm_diff_read` | Differences between stored revisions |
| Read app records | `workspace_read`, `history_read` | Workspace and Activity data, saved messages and evidence |
| Open Plous views | `ui_read`, `kubernetes_show`, `helm_show`, `history_show` | Current view information and navigation results. Panel sessions only. |

`_read` tools return information. `_show` tools open a view. `_run` tools can change cluster or application state.

Argument rules:

- `kubernetes_logs_read`, `kubernetes_usage_read`, and `helm_diff_read` take no `operation` argument.
- `kubernetes_show` accepts `view=resource`, `list`, or `logs`.
- Other show tools take no `view` argument. `helm_show` opens revision comparisons, not release lists.
- For Helm comparisons, `revision` is the after revision and `compare_revision` is the before revision.

Resource and history calls include a purpose. Cluster calls use the captured connection identity. Supplying another identity does not grant access. Integration-owned schemas validate arguments before execution. See the [tool registry](../../internal/ai/tools.go) and [history implementation](../contributing/ai-assistant.md#history-tools).

### Monitor evidence

All integrations use the same selected cluster. Their statuses describe different things. A Helm release record or controller's Ready condition does not prove current Pod health.

| Tool | Important limits |
| --- | --- |
| `kubernetes_usage_read` | Reads current metrics without opening a chart. Reports times, missing measurements, and completeness. Cannot recover unsaved chart history. |
| `kubernetes_logs_read` | Requires a Pod or supported workload and an explicit container. Reads at most 16 Pods, 500 lines per Pod, and 64 KiB combined. Reports failed or skipped Pods. |
| `helm_diff_read` | An explicit comparison needs two distinct positive stored revisions. Otherwise it compares with the predecessor. Revision 1 uses an empty baseline. |

Workload logs follow controller ownership and check Pod identities before and after collection. Excerpts are not complete log history.

See the [Kubernetes](../integrations/kubernetes/README.md), [Helm](../integrations/helm/README.md), [Argo CD](../integrations/argo/README.md), and [Flux](../integrations/flux/README.md) guides for integration details.

### Historical evidence

History contains results that were saved. It is not a continuous archive of resource contents. A current read cannot recover a value that was never recorded.

Search matches identify candidate records, not resource counts. Counts and comparisons report incomplete or unknown inputs.

## Active diagnostics

Enable these through [diagnostic setup](../mcp/active-diagnostics.md). Both require a purpose, the selected Kubernetes `connection_id`, and its default `namespace`.

| Tool | Required inputs | Behavior |
| --- | --- | --- |
| `kubernetes_exec_run` | Pod `name`, exact `uid`, `container`, and `command` argument array | One command in an existing container. No added shell, stdin, or TTY. |
| `kubernetes_diagnostic_run` | `test` (`http`, `dns`, or `tcp`), `target`, and `port` for TCP | One built-in probe in a temporary Job using the configured image. Plous builds the command. |

`timeout_seconds` defaults to 30 and accepts 1–120. Output is limited to 64 KiB. Temporary cleanup can take another 10 seconds.

HTTP targets need a full HTTP(S) URL without embedded credentials. Tests do not follow redirects or retry.

Plous records requests before running them and saves results before returning them. A transport or storage failure must not cause an automatic retry of an operation that may already have changed state.

## Standalone target discovery

A standalone server follows kubeconfig’s current context unless configured with `--context`. Call `kubernetes_context_read` with an empty argument object to get the current context, default namespace, and connection IDs.

## Scope and delivery

Each standalone call captures a single target. Reads may use any namespace allowed by those Kubernetes credentials. Diagnostic operations stay in the captured default namespace.

General chat and history-only views have no live cluster tools. To change a pinned standalone target, update its arguments and reconnect. To change the panel's target, [close and reopen its session](../guides/plous-mcp.md#switch-connections-or-close-a-session).

Plous validates and serializes calls, then saves results before delivery. Fatal storage, history, or target errors disable further tool access. A native CLI remains open so you can inspect the error.

Native sessions have no Plous-imposed process deadline or session-wide tool-call cap. The CLI manages model calls and spending. Built-in agent budgets are described in [Configuration](configuration.md#agent-runtime-configuration).

## MCP connection details

| Mode | Transport | Configuration |
| --- | --- | --- |
| Standalone | Newline-delimited stdin/stdout, with no listening port | [Client setup](../mcp/setup.md) |
| Inside Plous | Temporary loopback HTTP endpoint with a per-session bearer token | Automatic. Ends when the native session closes. |

Do not copy the panel's endpoint or token into global client settings.

The local MCP connection does not make model inference local. Cluster data can go to the CLI's configured provider.

Plous view tools preserve focus and edits. They return `opened`, `blocked`, or `failed` and do not send keystrokes or write resources. Their navigation results are stored separately from searchable cluster evidence.

See [AI implementation](../contributing/ai-assistant.md#runtime-structure) for the adapter and tool contracts.

## Protocol behavior

| Property | Support |
| --- | --- |
| Protocol | JSON-RPC 2.0, one request per message |
| Authentication | Panel HTTP uses a fresh bearer token and rejects Origin headers. Stdio uses subprocess pipes. |
| Initialization | Protocol version `2025-03-26`, tools capability |
| Requests | `initialize`, `ping`, `tools/list`, `tools/call` |
| Notifications | `notifications/initialized`, plus cancellation of a matching active request |
| Discovery | Tool descriptions, schemas, and read/diagnostic annotations |
| Results | Text content. View tools return navigation results. |
| Other features | No resources or prompt templates |

Request IDs must be strings or integers. A tool-call ID cannot be reused in the same session. Inspect tool content for errors even when transport succeeds.

Unsupported methods return JSON-RPC errors. After a fatal tool failure, later calls receive a closed-session response.

Schemas use an object root with operation-specific constraints under `dependentSchemas`. See the [bridge](../../internal/ai/cli/bridge.go), [launch configuration](../../internal/ai/cli/terminal_config.go), and [client checks](../contributing/ai-assistant.md#verify-follow-ups-and-history) for implementation details.

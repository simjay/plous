# MCP scope and saved evidence

[MCP server](README.md)

A standalone MCP client starts `plous mcp serve` as a subprocess. There is no listening port or permanent background server.

## Captured access

The process keeps the cluster, kubeconfig, default namespace, and diagnostic permissions it started with. Reconnect to change them. Browsing another cluster in the Plous UI does not change a standalone server.

Kubernetes, Helm, Argo CD, and Flux tools use the same cluster. Reads can use any namespace allowed by your credentials. Diagnostic operations stay in the configured default namespace.

The client manages its model, authentication, context, and spending. Plous limits individual operations but does not impose a session-wide deadline or tool-call cap.

## Saved evidence

Each process saves its tool results in a separate profile under `~/.plous/mcp/session-*`, or the supplied `--data-dir`. Its startup message on stderr shows the path.

Results survive process exit. History tools see only that process's saved evidence. Earlier standalone sessions are not imported or searched automatically.

Logs and command output may contain application secrets and can be sent to the client's model provider. Kubeconfig credentials stay with Plous. See [Data and privacy](../reference/data-and-privacy.md).

## Close or reconnect

Closing the client connection stops local work and attempts cleanup of temporary diagnostic Jobs. It cannot guarantee that an exec command stopped inside an application container.

If a command times out or its result cannot be saved, check its outcome before retrying. It may already have changed application state.

A fatal tool failure disables further calls for that process. Fix the reported problem before reconnecting. Saved evidence remains after exit.

Inside Plous, [native sessions](../guides/plous-mcp.md#switch-connections-or-close-a-session) save evidence in the Plous profile. Hiding the panel leaves them running.

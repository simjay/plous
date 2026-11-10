# MCP scope, evidence, and lifecycle

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md#scope-and-delivery)

The agent starts `plous mcp serve` as a stdio subprocess. No listening port, permanent server, or additional AI model is needed. Setup pins the context, kubeconfig path list, default namespace, and enabled capabilities. Reconnect after rerunning setup to change those choices. Browsing another connection in the TUI does not redirect a standalone server.

Every standalone process uses its own private profile under `~/.plous/mcp/session-*`, or under the `--data-dir` root supplied to setup or serve. This avoids locking the TUI profile or another agent's evidence. The startup diagnostic on stderr identifies the profile. Evidence survives process exit; `history_read` sees only that standalone profile's session evidence. Earlier standalone sessions are not automatically searched or imported into the TUI.

Active requests are committed before execution, and observations are committed before delivery. A delivery or storage failure must not trigger an automatic retry of a command that could have changed state. A lost client connection cancels local work and attempts temporary Job cleanup. It cannot guarantee termination of an exec process inside an application container.

Logs, command output, and API response excerpts can contain sensitive application data and may be sent to your agent's model provider. Kubeconfig credentials stay with Plous. See [Data and privacy](../reference/data-and-privacy.md).

## Captured access

The server captures one cluster context for its lifetime. Kubernetes, Helm, Argo CD, and Flux use that cluster. Reads may use any namespace allowed by its Kubernetes credentials; active diagnostics stay within the captured default namespace. Naming a different connection cannot grant access.

The standalone tool list excludes TUI presentation tools. The process has no Plous-imposed session-wide deadline or tool-call cap. Individual reads and diagnostic operations remain bounded. Your client manages its model context, authentication, and spending.

## Close or reconnect

Closing the client connection cancels local work and closes the MCP subprocess. Reconnect after changing the configured target or diagnostic grants. A fatal tool failure revokes tool access for that process; resolve the reported cause before reconnecting. Saved evidence is retained after exit.

Inside the TUI, [native sessions](../guides/plous-mcp.md#switch-connections-or-close-a-session) capture their target when opened and save evidence in the Plous profile. Hiding their panel does not close the process.

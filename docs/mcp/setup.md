# Connect an MCP client

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md)

Use Plous's cluster tools from Codex or Claude Code.

Install [Plous](../getting-started/installation.md) first. The client must be able to find `plous` on PATH.

## Configure your client

Choose the example for your client. Replace `/path/to/kubeconfig`, `staging`, and `shop` with your kubeconfig path, context, and namespace.

### Codex

Add this entry to `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` if you use a custom home:

```toml
[mcp_servers.plous_debug]
command = "plous"
args = [
  "mcp", "serve",
  "--agent", "codex",
  "--kubeconfig", "/path/to/kubeconfig",
  "--context", "staging",
  "--namespace", "shop",
]
tool_timeout_sec = 150
```

### Claude Code

Add `plous_debug` to `mcpServers` in `~/.claude.json`. Keep your other entries.

```json
{
  "mcpServers": {
    "plous_debug": {
      "type": "stdio",
      "command": "plous",
      "args": [
        "mcp", "serve",
        "--agent", "claude",
        "--kubeconfig", "/path/to/kubeconfig",
        "--context", "staging",
        "--namespace", "shop"
      ]
    }
  }
}
```

Use an absolute executable path only if the client cannot find `plous`. The command field does not expand shell aliases or `~`.

## Check the connection

1. Restart or reconnect the client.
2. Check that it lists `kubernetes_resources_read` and `kubernetes_logs_read`.
3. Ask: “Check the Pods and warning events in staging/shop.”
4. Confirm that the answer names the correct cluster and namespace.

A connected MCP client can still encounter Kubernetes permission errors. See [MCP troubleshooting](troubleshooting.md) if reads fail.

## Target and evidence options

| Option | Purpose |
| --- | --- |
| `--kubeconfig PATHS` | Choose a file or platform-separated path list. Defaults to the terminal kubeconfig. |
| `--context NAME` | Choose the Kubernetes context. Required. |
| `--namespace NAME` | Set the default namespace. Diagnostic operations stay in this namespace. |
| `--data-dir PATH` | Choose where Plous stores evidence profiles. |
| `--allow-exec` | Allow agent commands in existing containers. |
| `--diagnostic-image IMAGE` | Allow temporary HTTP, DNS, and TCP tests. |

Reconnect after changing these options. Plous does not change your kubeconfig or another tool's active context.

Reads work by default. To enable commands or temporary tests, follow [Active diagnostics](active-diagnostics.md). The client timeout must allow time for the test and cleanup.

## Use MCP inside the TUI

Choose Claude Code or Codex in **Settings → Application → Agent** and save. Plous configures MCP for that session automatically. See [Claude Code and Codex inside Plous](../guides/plous-mcp.md).

For automatic MCP configuration with the debugging skill, use [plous-debug setup](../agent-skills/plous-debug.md#install).

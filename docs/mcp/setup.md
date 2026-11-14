# Connect an MCP client

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md)

Connect Codex or Claude Code to Plous and verify which Kubernetes context it can read.

Install [Plous](../getting-started/installation.md) and make sure your client can find `plous` on PATH. You need a working kubeconfig and access to the cluster you want to inspect.

Jump to [Codex](#codex) or [Claude Code](#claude-code).

## Configure your client

These examples follow the current context in the client's `KUBECONFIG` or standard kubeconfig.

### Codex

Add this entry to `~/.codex/config.toml` or `$CODEX_HOME/config.toml` if you use a custom home.

```toml
[mcp_servers.plous_debug]
command = "plous"
args = [
  "mcp", "serve",
  "--agent", "codex",
]
env_vars = ["KUBECONFIG"]
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
        "--agent", "claude"
      ]
    }
  }
}
```

Use an absolute executable path only if the client cannot find `plous`. The command field does not expand shell aliases or `~`.

## Check the connection

1. Restart or reconnect the client.
2. In the client's MCP tools, look for `kubernetes_context_read`.
3. Ask “Which Kubernetes context and default namespace is Plous using?”
4. Verify the context and namespace before asking the agent to inspect Pods or events.

A connected MCP client can still encounter Kubernetes permission errors. See [MCP troubleshooting](troubleshooting.md) if reads fail.

## Target and evidence options

Add these arguments to your client's `args` list to choose a target or change where Plous saves evidence.

| Option | Purpose |
| --- | --- |
| `--kubeconfig PATHS` | Choose a file or platform-separated path list. Defaults to the terminal kubeconfig. |
| `--context NAME` | Pin a context. Omit this to follow kubeconfig’s current context. |
| `--namespace NAME` | Override the selected context’s default namespace. Diagnostic operations use this namespace. |
| `--data-dir PATH` | Choose where Plous stores evidence profiles. |

Reconnect after changing launch arguments. See [Scope and saved evidence](scope.md) for how Plous resolves the target and stores results.

To enable container commands or temporary tests, follow [Active diagnostics](active-diagnostics.md).

## Use MCP inside the TUI

Choose Claude Code or Codex in **Settings → Application → Agent** and save. Plous configures MCP for that session automatically. See [Claude Code and Codex inside Plous](../guides/plous-mcp.md).

For automatic MCP configuration with the debugging skill, use [plous-debug setup](../agent-skills/plous-debug.md#install).

# Configure MCP manually

[Coding agent setup](../agent-skills/plous-debug.md) · [Tools and permissions](../reference/mcp.md)

Use manual configuration when you need to register the Plous cluster tools yourself. For automatic setup, use [plous-debug](../agent-skills/plous-debug.md#install).

[Install Plous](../getting-started/installation.md) and make sure your client can find `plous` on PATH. Check that your kubeconfig can read the cluster you want to inspect.

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

### Other supported clients

Register a local stdio server named `plous_debug` with command `plous` and arguments `mcp`, `serve`, `--agent`, followed by your client name. The client names are `opencode`, `grok`, `agy`, `cursor`, and `gemini`. Use the client’s MCP settings or the [configuration file listed for that client](../agent-skills/plous-debug.md#installed-files). OpenCode takes the executable and its arguments in one `command` array.

In Cursor, set the server's `env.KUBECONFIG` to `${KUBECONFIG:-}` to forward the shell's value and allow Plous to use the standard kubeconfig when it is unset.

## Check the connection

1. **Reconnect the client.** Look for `kubernetes_context_read` in its MCP tools.
2. **Ask for the active target.** “Which Kubernetes context and default namespace is Plous using?”
3. **Compare the result with your intended cluster.** Verify both values before requesting Pod or event reads.

<!-- plous-example: mcp-target -->

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

Plous configures the connection automatically when you [use Claude Code or Codex in the assistant panel](../guides/plous-mcp.md).

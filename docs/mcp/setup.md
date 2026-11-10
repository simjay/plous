# MCP setup and client configuration

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md)

Install [Plous on your PATH](../getting-started/installation.md#install-on-your-path), then use `plous` as the MCP command. The standalone server requires an explicit Kubernetes context. It speaks MCP over stdin/stdout and opens no listening port.

## Register a standalone server

Configure your MCP client to start the installed executable with an argument array:

```text
command: plous
args: ["mcp", "serve", "--agent", "codex", "--kubeconfig", "/path/to/kubeconfig", "--context", "staging", "--namespace", "shop"]
```

Use `--agent claude` for Claude Code. The accepted client identities are `codex` and `claude`. Use real paths, a context present in that kubeconfig, and a concrete default namespace. The MCP client must inherit a PATH that includes Plous. Shell aliases and `~` expansion are not applied to the executable field. Use an absolute path only if the client cannot find `plous` on PATH; managed [plous-debug setup](../agent-skills/plous-debug.md#install) discovers and records the executable automatically.

For Codex, the corresponding entry in its MCP configuration is:

```toml
[mcp_servers.plous_debug]
command = "plous"
args = ["mcp", "serve", "--agent", "codex", "--kubeconfig", "/path/to/kubeconfig", "--context", "staging", "--namespace", "shop"]
tool_timeout_sec = 150
```

For Claude Code, merge this entry into the existing `mcpServers` object:

```json
{
  "mcpServers": {
    "plous_debug": {
      "type": "stdio",
      "command": "plous",
      "args": ["mcp", "serve", "--agent", "claude", "--kubeconfig", "/path/to/kubeconfig", "--context", "staging", "--namespace", "shop"]
    }
  }
}
```

Retain other client settings and MCP entries. These examples configure tools only. For managed installation of both the MCP entry and skill, follow [plous-debug installation](../agent-skills/plous-debug.md#install).

## Check the connection

1. Restart or reconnect your client after changing its configuration.
2. Confirm that its tool list includes `kubernetes_resources_read` and `kubernetes_logs_read`. Standalone mode omits Plous view tools.
3. Ask for the status and events of a known workload, naming the intended context and namespace. Check the returned target, timestamps, and coverage.

A successful MCP connection does not establish that Kubernetes access is permitted. Missing permissions and failed API requests remain tool errors. Configure the client's tool timeout to allow the selected diagnostic deadline and temporary cleanup if using [active diagnostics](active-diagnostics.md).

## Target and evidence options

| Option | Behavior |
| --- | --- |
| `--kubeconfig PATHS` | File or platform-separated path list; omitted uses the terminal kubeconfig |
| `--context NAME` | Required explicit context captured for the process |
| `--namespace NAME` | Concrete default namespace; active tests stay within it |
| `--data-dir PATH` | Root for separate per-process evidence profiles |
| `--allow-exec` | Enable bounded commands in existing containers |
| `--diagnostic-image IMAGE` | Enable temporary HTTP, DNS, and TCP tests with the chosen image |

Reconnect to capture changed options. Registration does not change kubeconfig or an external tool's active context. See [scope and lifecycle](scope.md).

## Use MCP inside the TUI

Select the intended cluster, choose Claude Code or Codex in **Settings → Application → Agent**, and Save. Plous creates and configures the private session bridge automatically. Follow the [native assistant panel guide](../guides/plous-mcp.md) for terminal controls, authentication, and saved evidence.
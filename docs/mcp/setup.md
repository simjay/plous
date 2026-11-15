# MCP setup and troubleshooting

For automatic setup, use [plous-debug](../agent-skills/plous-debug.md). Use this page to register MCP manually or fix a connection. [Install Plous](../getting-started/installation.md) first and make sure your client can find `plous` on PATH.

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

Register a local stdio server named `plous_debug` with command `plous` and arguments `mcp`, `serve`, `--agent`, followed by your client name. The client names are `opencode`, `grok`, `agy`, `cursor`, and `gemini`. Use the client’s MCP settings or the [configuration file listed for that client](#installed-files-and-removal). OpenCode takes the executable and its arguments in one `command` array.

In Cursor, set the server's `env.KUBECONFIG` to `${KUBECONFIG:-}` to forward the shell's value and allow Plous to use the standard kubeconfig when it is unset.

## Check the connection

1. **Reconnect the client.** Look for `kubernetes_context_read` in its MCP tools.
2. **Ask for the active target.** “Which Kubernetes context and default namespace is Plous using?”
3. **Compare the result with your intended cluster.** Verify both values before requesting Pod or event reads.

<!-- plous-example: mcp-target -->

A connected MCP client can still encounter Kubernetes permission errors. See [MCP troubleshooting](#troubleshoot-the-connection) if reads fail.

## Target and evidence options

Add these arguments to your client's `args` list to choose a target or change where Plous saves evidence.

| Option | Purpose |
| --- | --- |
| `--kubeconfig PATHS` | Choose a file or platform-separated path list. Defaults to the terminal kubeconfig. |
| `--context NAME` | Pin a context. Omit this to follow kubeconfig’s current context. |
| `--namespace NAME` | Override the selected context’s default namespace. Diagnostic operations use this namespace. |
| `--data-dir PATH` | Choose where Plous stores evidence profiles. |

Reconnect after changing launch arguments. See [Saved evidence](#saved-evidence) for profile and retention details.

To enable container commands or temporary tests, follow [Active diagnostics](active-diagnostics.md).

## Troubleshoot the connection

| Symptom | Check |
| --- | --- |
| Client cannot find Plous | Run `command -v plous` in its environment. Supply that path if the client cannot resolve it. |
| Server stops or reads fail | Read stderr for kubeconfig, network, permission, or storage errors. Fix the reported issue, then reconnect. |
| Wrong cluster | Call `kubernetes_context_read`. Switch kubeconfig's current context, or change a pinned `--context` and reconnect. |
| Diagnostics missing | [Enable the diagnostic option](active-diagnostics.md), check RBAC, and reconnect. |
| Earlier evidence missing | Check the profile printed on startup. The client searches evidence from the current process and context. |
| Command timed out or cleanup is pending | Check the outcome or reported Job and Pod identities before retrying. |
| Existing setup entry blocks installation | Resolve the unrelated skill or unmanaged MCP entry. Setup preserves it. |

Use the [assistant panel guide](../guides/ai-chat.md#run-claude-code-or-codex) for native CLI startup or sign-in. See [tool permissions](../reference/mcp.md) for which tools each connection grants.

## Saved evidence

A standalone client starts `plous mcp serve` locally. Each call captures one cluster and namespace, and reads may span namespaces allowed by the same credentials. Diagnostics use the default namespace. The assistant panel uses a separate connection.

Each process stores results in a separate profile, usually `~/.plous/mcp/session-*`. Find the actual path in the server's startup message. Results survive exit. History searches are limited to that process's evidence for the current context. See [Data and privacy](../reference/data-and-privacy.md#independent-coding-agent-sessions).

Closing the connection stops local work and attempts to clean up temporary Jobs. Check the outcome before retrying an interrupted command. The container command may still be running.

## Installed files and removal

`plous setup` prints both installed paths, including custom homes. Rerun setup to update launch flags without replacing the client's tool filters or unrelated servers.

| Agent | Skill directory | MCP config |
| --- | --- | --- |
| Codex | `~/.agents/skills/plous-debug/` | `~/.codex/config.toml` or `$CODEX_HOME/config.toml` |
| Claude Code | `~/.claude/skills/plous-debug/` | `~/.claude.json` |
| OpenCode | `~/.agents/skills/plous-debug/` | `~/.config/opencode/opencode.json` or existing config |
| Grok Build | `~/.grok/skills/plous-debug/` | `~/.grok/config.toml` |
| Antigravity CLI | `~/.gemini/skills/plous-debug/` | `~/.gemini/config/mcp_config.json` |
| Cursor Agent | `~/.agents/skills/plous-debug/` | `~/.cursor/mcp.json` |
| Gemini CLI | `~/.gemini/skills/plous-debug/` | `~/.gemini/settings.json` |

To remove the connection, delete the `plous_debug` entry from `mcp_servers` in Codex or Grok Build, `mcp` or `mcp.servers` in OpenCode, or `mcpServers` in the other clients. Delete the skill directory once none of your clients use it, then reconnect. Evidence remains in the standalone profiles.

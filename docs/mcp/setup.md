# MCP setup and troubleshooting

Use the [agent directory](../guides/ai-chat.md#choose-an-agent) to find your client’s setup command. Once connected, [check the Kubernetes context and namespace](#check-the-connection).

## Configure your client

`plous setup` registers the server and installs both skills. For manual registration, [install Plous](../getting-started/installation.md) on the host running your client, then add a local stdio server.

| Field | Value |
| --- | --- |
| Name | `plous_debug` |
| Command | `plous`, or its absolute path if the client cannot find it |
| Arguments | `mcp`, `serve`, `--agent`, then the client name you would pass to `plous setup` |

OpenCode takes the executable and all arguments in one `command` array. See configuration examples for [Codex](../agent-skills/clients/codex.md#manual-mcp-configuration) and [Claude Code](../agent-skills/clients/claude.md#manual-mcp-configuration), or check [Cursor’s environment setting](../agent-skills/clients/cursor.md#common-issues).

## Check the connection

1. **Reconnect the client.** Look for `kubernetes_context_read` in its MCP tools.
2. **Ask for the active target.** “Call `kubernetes_context_read` and show the context, default namespace, kubeconfig source, or exact error.”
3. **Compare the result with your intended cluster.** Verify both values before requesting Pod or event reads.

<!-- plous-example: mcp-target -->

A connected MCP client can still encounter Kubernetes permission errors. See [MCP troubleshooting](#troubleshoot-the-connection) if reads fail.

## How Plous chooses a context

By default, the MCP subprocess uses its own `KUBECONFIG` or `~/.kube/config` and follows that configuration's `current-context` on each call. A GUI or remote agent may have a different environment from your terminal. The namespace comes from the context, with `default` as the fallback.

The context check reads local configuration. A resource read checks API reachability, authentication, and Kubernetes permissions.

## Change targets while the agent is running

Tell the agent which target to use.

- **Follow a file’s current context.** “Use `~/clusters/team.yaml` for Plous.”
- **Choose a context in that file.** “Use `develop` from `~/clusters/team.yaml`.”
- **Choose a context in the default kubeconfig.** Give the agent the context name.

Confirm the selection if your client shows a prompt. Plous reports the selected context and namespace before the agent continues.

If the client cannot show the prompt, the agent gives you a control path. In a terminal on the MCP host, use that path in the following command.

```sh
plous mcp target CONTROL "$HOME/clusters/team.yaml" develop
```

Omit the context to follow the selected file’s current context. Use an empty kubeconfig argument to choose a named context in the default configuration. To clear the selection, say “Use the default kubeconfig again” or run `plous mcp target CONTROL` by itself. The change takes effect on the same MCP connection and leaves kubeconfig files untouched. Other agents and the Plous terminal UI have their own selections.

## Target and evidence options

Use `--data-dir PATH` to change where Plous stores evidence. Reconnect after changing launch arguments. See [Saved evidence](#saved-evidence) for profile and retention details.

To enable container commands or temporary tests, follow [Active diagnostics](active-diagnostics.md).

## Troubleshoot the connection

| Symptom | Check |
| --- | --- |
| Skill loads but `plous_debug` or `kubernetes_context_read` is absent | Check the client’s MCP settings. Run setup on the host running the agent and reconnect. A skill can load independently of its server. |
| Client cannot find Plous | Run `command -v plous` in its environment. Supply that path if the client cannot resolve it. |
| Codex reports `Transport closed` | See [Codex connection diagnostics](../agent-skills/clients/codex.md#common-issues). |
| Plous reports `tool access stopped` | Plous has revoked access for this connection. Reconnect the client, then confirm the target with `kubernetes_context_read`. For interrupted active diagnostics, verify the remote outcome before retrying. |
| Plous reports too many in-flight requests | Wait for outstanding calls to finish, then submit the rejected call again. Plous did not dispatch it. |
| Context check reports missing files or a missing context | Check `kubeconfig_source` and the paths in the error. Ask the agent to use the intended file or a named context. |
| Context check succeeds but resource reads fail | Check that the MCP host can reach the API, has any required authentication helper on its PATH, and has credentials with access to the requested resources. |
| Kubernetes credential helper times out | Complete the configured provider's sign-in on the MCP host, then retry the read. Plous reports the timeout on the MCP subprocess's stderr. |
| Wrong cluster | Call `kubernetes_context_read`, then ask the agent to use the intended target. |
| Diagnostics missing | [Enable the diagnostic option](active-diagnostics.md), check RBAC, and reconnect. |
| Earlier evidence missing | Check the profile printed on startup. The client searches evidence from the current process and context. |
| Command timed out or cleanup is pending | Check the outcome or reported Job and Pod identities before retrying. |
| Existing setup entry blocks installation | Resolve the unrelated skill or unmanaged MCP entry. Setup preserves it. |

Use the [assistant panel guide](../guides/ai-chat.md#run-claude-code-or-codex) for native CLI startup or sign-in. See [tool permissions](../reference/mcp.md) for which tools each connection grants.

## Saved evidence

A standalone client starts `plous mcp serve` locally. Each call captures one cluster and namespace, and reads may span namespaces allowed by the same credentials. Diagnostics use the default namespace. The assistant panel uses a separate connection.

Each process stores results in a separate profile, usually `~/.plous/mcp/session-*`. Find the actual path in the server's startup message. Results survive exit. History searches are limited to that process's evidence for the current context. See [Data and privacy](../reference/data-and-privacy.md#independent-coding-agent-sessions).

Closing the connection stops local work and attempts to clean up temporary Jobs. Check the outcome before retrying an interrupted command. The container command may still be running.

Cancelling a standalone read leaves the connection usable if Plous finishes saving its evidence. Kubernetes API requests for resource pages and Pod log bodies have individual read deadlines. A timeout can leave partial evidence from pages that already completed.

## Installed files and removal

Setup prints the Plous executable and the paths to the MCP config and both skills. It can register the integration before you have a kubeconfig. After [updating Plous](../getting-started/installation.md#update), rerun your agent’s setup command and reconnect. Repeat any custom `--data-dir` or [diagnostic options](active-diagnostics.md). Setup preserves tool filters, environment overrides, and unrelated servers.

Setup replaces older target flags with the current-context default. [Choose another target](#change-targets-while-the-agent-is-running) once the client is connected. If you upgrade from the old tool names, [check configured filters against the current inventory](../reference/mcp.md#available-tools).

Find the default config and skill paths on your [agent’s page](../guides/ai-chat.md#choose-an-agent).

To remove the connection, delete the `plous_debug` entry from `mcp_servers` in Codex or Grok Build, `mcp` or `mcp.servers` in OpenCode, or `mcpServers` in the other clients. Delete the `plous-debug` and `plous-monitor` skill directories once none of your clients use them, then reconnect. Evidence remains in the standalone profiles.

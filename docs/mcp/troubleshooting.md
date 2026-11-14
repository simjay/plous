# MCP troubleshooting

[MCP server](README.md) · [Setup](setup.md)

First check whether the agent runs inside Plous or as a separate MCP client.

| Problem | Action |
| --- | --- |
| Client cannot find Plous | Check its PATH. Use `command -v plous` in the same environment. Supply that path only if the client cannot resolve `plous`. |
| Server exits at startup | Read stderr. Check the kubeconfig, required context, namespace, and writable data directory. |
| Connected, but reads fail | Read the Kubernetes authentication, network, or RBAC error. Client sign-in does not grant cluster access. |
| Wrong cluster | Call `kubernetes_context_read` to check the target. Switch the current context in kubeconfig for a live connection. For a pinned connection, change `--context` and reconnect. |
| Diagnostic tools missing | Enable the relevant [diagnostic option](active-diagnostics.md) and reconnect. Check Kubernetes permissions. |
| No tools for opening Plous views | These are available only [inside Plous](../guides/plous-mcp.md). |
| Earlier evidence missing | Each standalone process has its own [evidence profile](scope.md). Earlier sessions are not searched automatically. |
| Tool access disabled | Fix the reported storage, history, or target error before reconnecting. |
| Command timed out | Check whether it ran before retrying. A container process may still be running. |
| Cleanup pending | Check the reported Job and Pod identities. Deletion is not confirmed yet. |
| Setup refuses an existing entry | Resolve the unrelated skill or unmanaged `plous_debug` configuration. Setup does not overwrite it. |

For native CLI startup and sign-in, see [assistant panel troubleshooting](../guides/plous-mcp.md#troubleshoot-the-connection). For profile recovery, see [Troubleshooting](../reference/troubleshooting.md).

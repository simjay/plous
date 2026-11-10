# MCP troubleshooting

[MCP server](README.md) · [Setup](setup.md)

Check which mode is running: a standalone subprocess or the assistant panel inside Plous. Their registration, profiles, and view tools differ.

| Symptom | What to check |
| --- | --- |
| Server executable is missing | Check that the MCP client inherits a PATH containing `plous`. Use `command -v plous` in the same environment; supply that path only when the client cannot resolve the command. Rebuilding a checkout does not update a separate installed copy. |
| Server exits before exposing tools | Read its stderr diagnostic. Check the kubeconfig path, required context, concrete namespace, and writable evidence directory. |
| MCP connects but cluster reads fail | Check the returned authentication, network, or RBAC error for the captured target. Client authentication and Kubernetes authentication are separate. |
| Tools refer to the wrong cluster | Correct the configured `--context` and reconnect. Changing the TUI's selection does not retarget a standalone process. |
| Exec or temporary diagnostics are missing | Enable the corresponding launch grant, reconnect, and verify Kubernetes permissions. See [active diagnostics](active-diagnostics.md). |
| Plous view tools are absent | Standalone MCP supplies cluster and evidence tools. Use the [TUI assistant panel](../guides/plous-mcp.md) to open Plous views. |
| Previous investigation evidence is missing | Each standalone process has its own evidence profile. Earlier sessions are not automatically searched. See [scope and evidence](scope.md). |
| Tool access stops after a failure | Read the full tool error and fix the reported scope, storage, or history-integrity problem before reconnecting. |
| A command times out or cleanup is pending | Inspect its recorded outcome and runner identity before retrying. An exec process may still have run; temporary Job removal needs confirmation. |
| Managed setup refuses an existing entry | Preserve the existing configuration and resolve the unmanaged `plous_debug` server or skill collision. Setup refuses to overwrite unrelated content. |

For native panel launch and sign-in problems, use [assistant panel troubleshooting](../guides/plous-mcp.md#troubleshoot-the-connection). For application recovery, see [Troubleshooting](../reference/troubleshooting.md).

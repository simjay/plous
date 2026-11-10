# MCP server

[Documentation](../README.md)

Give Codex or Claude Code access to Plous's cluster tools through Model Context Protocol (MCP). Agents can read Kubernetes, Helm, Argo CD, and Flux data and revisit saved tool results.

Reads work by default. Container commands and temporary tests require [explicit setup](active-diagnostics.md).

## Connect your client

- [Setup](setup.md): connect an MCP client.
- [Tools and permissions](../reference/mcp.md): tool names and supported operations.
- [Active diagnostics](active-diagnostics.md): enable commands and temporary tests.
- [Scope and saved evidence](scope.md): cluster access, storage, and reconnection.
- [Troubleshooting](troubleshooting.md): fix connection and tool errors.

## Choose the connection mode

| Mode | Connection | Tools |
| --- | --- | --- |
| Standalone | Your client starts `plous mcp serve` over stdin/stdout | Cluster reads, saved evidence, and enabled diagnostics |
| Inside Plous | Plous configures a private connection for its assistant panel | The same tools, plus actions that open Plous views |

The built-in Plous agent uses these tools directly and needs no MCP setup.

For debugging instructions as well as tools, install [plous-debug](../agent-skills/plous-debug.md). Other MCP clients must support the [implemented protocol](../reference/mcp.md#protocol-behavior) and need separate compatibility checks.

# MCP server

[Documentation](../README.md) · [Choose your workflow](../getting-started/workflows.md)

Plous exposes cluster tools through Model Context Protocol (MCP). An agent can inspect Kubernetes, Helm, Argo CD, and Flux, revisit saved evidence, and run explicitly enabled diagnostics. The standalone server runs as a local subprocess; the TUI and an installed skill are optional.

## Connect your client

- [Setup and client configuration](setup.md): register a standalone stdio server, or use the automatic connection inside Plous.
- [Tools and permissions](../reference/mcp.md): exact tool names, evidence limits, transports, and supported protocol behavior.
- [Active diagnostics](active-diagnostics.md): enable container commands or temporary in-cluster tests.
- [Scope, evidence, and lifecycle](scope.md): understand the captured target, saved observations, and process lifetime.
- [Troubleshooting](troubleshooting.md): resolve startup, tool access, target, and storage problems.

## Choose the connection mode

| Mode | Starts the server | Tool surface |
| --- | --- | --- |
| Standalone MCP | Your client launches `plous mcp serve` over stdio | Cluster and session-evidence tools |
| Claude Code or Codex inside Plous | Plous creates a private loopback HTTP bridge for the native session | Cluster, saved evidence, and Plous view tools |

The built-in Plous agent calls the same guarded tool implementations directly. It does not need a separate MCP connection.

For a guided workflow in an independent coding-agent session, install [plous-debug](../agent-skills/plous-debug.md). MCP remains usable without the skill. Plous's setup and installed-client verification cover Codex and Claude Code; other clients must support the [implemented protocol subset](../reference/mcp.md#protocol-behavior) and need their own compatibility checks.

# Agent skills

[Documentation](../README.md)

A skill gives your coding agent instructions for a task. Plous's **plous-debug** skill guides Kubernetes investigations using cluster tools provided through MCP.

## Available skills

| Skill | Purpose | Agents |
| --- | --- | --- |
| [plous-debug](plous-debug.md) | Investigate failures, test causes, and check deployed fixes | Codex and Claude Code |

Run `plous setup` to install the skill and configure its MCP connection. The [plous-debug guide](plous-debug.md) covers installation, examples, and removal.

## Skills and MCP

You can [connect MCP](../mcp/setup.md) without installing a skill. MCP provides the tools. The skill explains how to use them during debugging.

The [assistant panel inside Plous](../guides/plous-mcp.md) already receives debugging instructions. Install plous-debug for agents running outside Plous.

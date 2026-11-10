# Agent skills

[Documentation](../README.md) · [Choose your workflow](../getting-started/workflows.md)

Agent skills provide instructions for completing a task with your coding agent. Plous MCP tools provide the cluster operations those instructions use. Skills use the agent's existing model and authentication.

## Available skills

| Skill | Use it to | Supported agents |
| --- | --- | --- |
| [plous-debug](plous-debug.md) | Investigate Kubernetes failures, reproduce problems, and verify deployed fixes | Codex and Claude Code |

The plous-debug page contains installation, invocation, examples, limits, and removal in one place. `plous setup` installs that skill and registers its MCP server together.

## Skills and MCP

You can [configure MCP directly](../mcp/setup.md) and call the tools without installing a skill. A skill adds a recommended investigation workflow, including how to distinguish evidence, likely causes, and verified outcomes.

Inside the [Plous assistant panel](../guides/plous-mcp.md), Plous supplies embedded debugging guidance. Personal skill discovery is disabled in hosted native sessions. Install plous-debug for independent coding-agent sessions.

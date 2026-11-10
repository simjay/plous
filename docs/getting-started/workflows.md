# Choose your workflow

[Documentation](../README.md) · [Installation](installation.md)

Build Plous once, then choose where you want to work. All workflows use your Kubernetes permissions. The terminal workspace works without AI setup.

| I want to… | Start here | What runs |
| --- | --- | --- |
| Inspect clusters in an interactive terminal | [Terminal UI quickstart](README.md) | Plous with Overview, Explore, Activity, and an optional assistant panel |
| Give an agent direct access to Plous tools | [MCP server setup](../mcp/setup.md) | A local `plous mcp serve` subprocess launched by the client |
| Guide my coding agent through debugging | [plous-debug](../agent-skills/plous-debug.md) | A skill plus Plous MCP tools inside an existing Codex or Claude Code session |

## Terminal UI

Use the [quickstart](README.md) to inspect your first workload. The right assistant panel can run the built-in Plous agent or your installed Claude Code or Codex. Plous configures MCP automatically for those native panels.

## MCP server

Use [MCP setup](../mcp/setup.md) to connect tools directly without installing a skill or opening the TUI. The server exposes cluster reads and saved evidence, with optional active diagnostics. Your client supplies the model and authentication.

## Agent skills

Use [plous-debug](../agent-skills/plous-debug.md) for a guided investigation, reproduction, and verification workflow in an independent coding-agent session. Managed setup installs the skill and registers MCP together. Your coding agent keeps its normal repository tools.

A skill supplies workflow instructions; MCP supplies executable tools. Choose [Agent skills](../agent-skills/README.md) for guided debugging or [MCP server](../mcp/README.md) for direct integration.

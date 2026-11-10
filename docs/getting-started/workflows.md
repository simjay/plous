# Choose your workflow

[Documentation](../README.md) · [Installation](installation.md)

Install Plous once, then choose where to work.

## Terminal UI

Inspect clusters in a terminal with Overview, Explore, and Activity. AI is optional. The assistant panel can run Plous, Claude Code, or Codex.

[Open the quickstart](README.md).

## MCP server

Give an agent access to Plous's cluster tools without opening the terminal UI. Your MCP client starts Plous as a subprocess and supplies the model and authentication.

[Connect an MCP client](../mcp/setup.md).

## Agent skills

Add **plous-debug** to an existing Codex or Claude Code session. The skill guides debugging, and MCP supplies the cluster tools. Your agent keeps its repository tools.

[Install plous-debug](../agent-skills/plous-debug.md).

All three workflows use your Kubernetes permissions. Agent reads are enabled by default. Commands and temporary tests require [explicit setup](../mcp/active-diagnostics.md).

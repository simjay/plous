# Plous for Antigravity CLI

Connect Antigravity CLI to Plous’s Kubernetes tools over standalone MCP.

## Install and connect

Install Antigravity CLI and [Plous](../../getting-started/installation.md) on the host that runs the agent. Register the skills and server.

```sh
plous setup agy
```

Open Antigravity CLI in your repository. [Check the target](../../mcp/setup.md#check-the-connection), then invoke `/plous-debug`.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup agy`.

## Common issues

Setup registers `plous_debug` in `~/.gemini/config/mcp_config.json` and installs both skills under `~/.gemini/skills/`. Gemini CLI uses a separate settings file. If the server is missing, check the paths printed by setup against the client you opened.

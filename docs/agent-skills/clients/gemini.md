# Plous for Gemini CLI

Connect Gemini CLI to Plous’s Kubernetes tools over standalone MCP.

## Install and connect

Install Gemini CLI and [Plous](../../getting-started/installation.md) on the host that runs the agent. Register the skills and server.

```sh
plous setup gemini
```

Reopen Gemini CLI in your repository. [Confirm the context and namespace](../../mcp/setup.md#check-the-connection), then ask it to use `plous-debug`.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup gemini`.

## Common issues

Look for `plous_debug` in `~/.gemini/settings.json`. Both skills go under `~/.gemini/skills/`. If you set `GEMINI_CLI_HOME`, use the paths printed by setup. Antigravity CLI uses a separate MCP config file.

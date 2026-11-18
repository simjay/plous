# Plous for Grok Build

Connect Grok Build to Plous’s Kubernetes tools over standalone MCP.

## Install and connect

Install Grok Build and [Plous](../../getting-started/installation.md) on the host that runs the agent. Register the skills and server.

```sh
plous setup grok
```

Reopen Grok Build in your repository. [Check the context and namespace](../../mcp/setup.md#check-the-connection), then invoke `/plous-debug`.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup grok`.

## Common issues

Look for `plous_debug` in `~/.grok/config.toml`. Setup installs both skills under `~/.grok/skills/`. If `GROK_HOME` is set, setup uses `config.toml` and `skills/` under that directory.

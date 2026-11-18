# Plous for OpenCode

Connect OpenCode to Plous’s Kubernetes tools over standalone MCP.

## Install and connect

Install OpenCode and [Plous](../../getting-started/installation.md) on the host that runs the agent. Register the skills and server.

```sh
plous setup opencode
```

Reopen OpenCode in your repository. [Check the target](../../mcp/setup.md#check-the-connection), then ask it to use `plous-debug`.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup opencode`.

## Common issues

Setup prints the config file it updated. The default is `~/.config/opencode/opencode.json`, or `opencode/opencode.json` under an absolute `XDG_CONFIG_HOME`. It can update an existing config file and installs both skills under `~/.agents/skills/`. If the server is missing, check whether a project config overrides the user entry.

# Plous for Cursor Agent

Connect Cursor Agent to Plous’s Kubernetes tools over standalone MCP.

## Install and connect

Install Cursor Agent and [Plous](../../getting-started/installation.md) on the host that runs the agent. Register the skills and server.

```sh
plous setup cursor
```

Reopen Cursor Agent in your repository. If Cursor asks you to approve `plous_debug`, review the server. [Check the target](../../mcp/setup.md#check-the-connection), then ask the agent to use `plous-debug`.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup cursor`.

## Common issues

Setup adds `plous_debug` to `~/.cursor/mcp.json` and installs both skills under `~/.agents/skills/`. The generated server entry forwards `KUBECONFIG` using `${KUBECONFIG:-}`. If the client sees a different environment, [check which kubeconfig Plous is using](../../mcp/setup.md#how-plous-chooses-a-context).

# Plous for Claude Code

Run Claude Code in the Plous assistant panel, or connect a standalone Claude Code session to Plous over MCP. Both use Claude Code’s own sign-in.

## In the Plous panel

1. Install and sign in to the `claude` CLI. [Launch Plous](../../getting-started/README.md#install-and-launch) from the repository you want Claude Code to open.
2. Select the cluster. Press `ss`, open **Application → Agent**, select **Claude Code**, and save.
3. Press `cc` to open Claude Code. Check the cluster in the assistant header before asking for reads.

The default **Entry command** is `claude`. A wrapper must forward Plous’s launch arguments. See [switching clusters and leaving the terminal](../../guides/ai-chat.md#run-claude-code-or-codex).

## Install and connect

[Install Plous](../../getting-started/installation.md) on the host running standalone Claude Code, then register the skills and server.

```sh
plous setup claude
```

Reopen `claude` in your repository. Look for `plous_debug` in `/mcp` and [check the target](../../mcp/setup.md#check-the-connection). Then invoke `/plous-debug` and describe the symptom.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup claude`.

## Common issues

Setup adds a user-level server in `~/.claude.json` and installs both skills under `~/.claude/skills/`. If the skill opens but cluster tools are missing, inspect `plous_debug` in `/mcp` for the connection error.

## Manual MCP configuration

Add `plous_debug` to `mcpServers` in `~/.claude.json`. Keep the other entries.

```json
{
  "mcpServers": {
    "plous_debug": {
      "type": "stdio",
      "command": "plous",
      "args": ["mcp", "serve", "--agent", "claude"]
    }
  }
}
```

See [manual registration and connection checks](../../mcp/setup.md#configure-your-client).

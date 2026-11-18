# Plous for Codex

Run the Codex CLI in the Plous assistant panel, or connect a standalone Codex session to Plous over MCP. Both use Codex’s own sign-in.

## In the Plous panel

1. Install and sign in to the `codex` CLI. [Launch Plous](../../getting-started/README.md#install-and-launch) from the repository you want Codex to open.
2. Select the cluster. Press `ss`, open **Application → Agent**, select **Codex**, and save.
3. Press `cc` to open Codex. Check the cluster in the assistant header before asking for reads.

The default **Entry command** is `codex`. If you use a wrapper such as `celai codex`, set it there and make sure it forwards Plous’s launch arguments. See [switching clusters and leaving the terminal](../../guides/ai-chat.md#run-claude-code-or-codex).

## Install and connect

[Install Plous](../../getting-started/installation.md) on the host running standalone Codex, then register the skills and server.

```sh
plous setup codex
```

Reopen Codex in your repository. Look for `plous_debug` in `/mcp` and [check the target](../../mcp/setup.md#check-the-connection). Then type `$plous-debug` and describe the symptom.

## Upgrade

[Update a standalone setup](../../mcp/setup.md#installed-files-and-removal) with `plous setup codex`.

## Common issues

Setup writes to `$CODEX_HOME/config.toml` or, by default, `~/.codex/config.toml`. It installs both skills under `~/.agents/skills/`. Run setup with the Codex home your client uses. If Codex reports `Transport closed`, open `/mcp verbose` and check the launch error.

## Manual MCP configuration

Add the server to Codex’s config file. [Install the skills with setup](#install-and-connect) if you want to invoke them by name.

```toml
[mcp_servers.plous_debug]
command = "plous"
args = ["mcp", "serve", "--agent", "codex"]
env_vars = ["KUBECONFIG"]
tool_timeout_sec = 150
```

See [manual registration and connection checks](../../mcp/setup.md#configure-your-client).

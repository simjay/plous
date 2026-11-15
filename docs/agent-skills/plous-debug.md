# Debug with your coding agent

Use **plous-debug** to investigate Kubernetes failures and verify fixes from your repository.

## Install

[Install Plous v0.0.5 or later](../getting-started/installation.md). Run the command for your agent to add the skill and connect the cluster tools.

- **OpenCode** · `plous setup opencode`
- **Grok Build** · `plous setup grok`
- **Antigravity CLI** · `plous setup agy`
- **Cursor Agent** · `plous setup cursor`
- **Gemini CLI** · `plous setup gemini`
- **Codex** · `plous setup codex`
- **Claude Code** · `plous setup claude`

## Start in your repository

Restart or reconnect your agent and open your repository. Follow its workspace trust prompts. If it asks you to approve an MCP server, review `plous_debug`.

- In Codex, type `$plous-debug`.
- In Claude Code, Grok Build, and Antigravity CLI, enter `/plous-debug`.
- In OpenCode, Cursor Agent, and Gemini CLI, ask the agent to use `plous-debug`.

Ask the agent to confirm the Kubernetes context and namespace before investigating. Plous follows the current context in the agent's kubeconfig. A context switch takes effect on the next tool call.

## Investigate a checkout error

Suppose checkout starts returning 502 after a deployment. Invoke the skill from your repository and give the agent the symptom and the intended target.

> Checkout started returning 502 in staging/shop after the last deploy. Confirm the Kubernetes context, then check the checkout Pods, events, and logs around a failed request. Tell me what you observed and what you think caused it.

An illustrative result would separate the evidence from the suspected cause.

| | Example |
| --- | --- |
| **Observed** | Checkout logged a timeout calling `payments.shop.svc:8081`. The payments Service exposes port `8080`. |
| **Suspected cause** | Checkout may be using an old port in its runtime configuration. |
| **Next check** | Trace where checkout gets its payments URL. Compare the deployed setting and the code's default with the payments Service port. |

Ask the agent to follow that lead into the repository.

> Trace how checkout builds the payments URL. Check whether the deployment sets the port or the code falls back to a default. If you find a mismatch, fix it in the repository and add a test that would have caught it.

After you deploy the fix, ask the agent to check the original failure.

> I deployed the fix to staging/shop. Confirm the intended image is running. Check fresh checkout logs and, if active diagnostics are enabled, repeat the failing request. Tell me which checks passed and which still need evidence.

## When logs aren’t enough

Choose where to run an active test based on what you need to learn.

| If you need to know… | Run the test from… |
| --- | --- |
| Whether the checkout application can reach payments | The existing checkout container, if it has the required executable. |
| Whether an internal endpoint responds from a separate workload | A temporary diagnostic Job with the required tools. Its Pod can have different network access from checkout. |

For the checkout example, a useful follow-up is “From the checkout container, check whether the payments health endpoint responds on port 8080. Report the Pod and the result.”

Both options require explicit enablement and Kubernetes permissions. Add `--allow-exec` for commands in a container or `--diagnostic-image IMAGE` for temporary Jobs that run HTTP GET, DNS, or TCP checks. Reconnect the agent after changing setup options. [Active diagnostics, permissions, and cleanup](../mcp/active-diagnostics.md)

## What setup installs

The [plous-debug skill instructions](../../skills/plous-debug/SKILL.md) guide the investigation. A local MCP connection gives your coding agent Plous's cluster tools and saved evidence. Your agent uses its usual repository tools, model, and sign-in. You can also [configure the cluster tools manually](../mcp/setup.md).

If you run Claude Code or Codex [inside the Plous assistant panel](../guides/plous-mcp.md), Plous connects the tools and supplies the debugging instructions when the panel starts.

## Configuration reference

### Pin a context

Use explicit options when you want the server to stay on one context. This example also sets the default namespace for diagnostics.

```sh
plous setup codex --kubeconfig /path/to/kubeconfig --context staging --namespace shop
```

The same options work for every client. Rerun setup without `--context` and reconnect to resume following your current context. [Context and saved evidence](../mcp/scope.md)

### Installed files

| Client | Skill | MCP configuration |
| --- | --- | --- |
| Codex | `~/.agents/skills/plous-debug/SKILL.md` | `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` |
| Claude Code | `~/.claude/skills/plous-debug/SKILL.md` | `~/.claude.json` |
| OpenCode | `~/.agents/skills/plous-debug/SKILL.md` | `~/.config/opencode/opencode.json` or existing user config |
| Grok Build | `~/.grok/skills/plous-debug/SKILL.md` | `~/.grok/config.toml` |
| Google Antigravity CLI | `~/.gemini/skills/plous-debug/SKILL.md` | `~/.gemini/config/mcp_config.json` |
| Cursor Agent | `~/.agents/skills/plous-debug/SKILL.md` | `~/.cursor/mcp.json` |
| Gemini CLI | `~/.gemini/skills/plous-debug/SKILL.md` | `~/.gemini/settings.json` |

Setup runs without prompts and prints both file paths. It honors `XDG_CONFIG_HOME` for OpenCode, `GROK_HOME` for Grok Build, and `GEMINI_CLI_HOME` for Gemini CLI. Use `--home PATH` for a separate installation home or `--data-dir PATH` for a separate evidence location.

Setup forwards `KUBECONFIG` to Cursor. If it is unset, Plous uses `~/.kube/config`. An existing Cursor environment override or explicit `--kubeconfig` still applies.

Rerun setup to update the executable and launch flags in an existing installation. Setup keeps client settings such as disabled state, tool filters, and environment overrides. Setup also keeps other MCP servers and stops if an unrelated skill or a different `plous_debug` launch occupies the destination.

### Remove the integration

Remove the MCP registration from the configuration file printed during setup.

- **Codex and Grok Build:** Remove the `mcp_servers.plous_debug` table or entry. You can also delete the surrounding Plous marker comments if they are present.
- **OpenCode:** Remove `plous_debug` from `mcp` or `mcp.servers`.
- **Other clients:** Remove `mcpServers.plous_debug`.

Delete the installed skill directory when you no longer use it with any client that shares that path. Reconnect your agent. Saved evidence stays on disk until you remove the standalone profiles. [Evidence storage locations](../mcp/scope.md#saved-evidence)

## Next steps

Try the [restarting Pod exercise](../guides/investigate-restarting-pod.md) in a disposable cluster. Give the agent the failing Pod's name and compare its findings with the guide. See [connection troubleshooting](../mcp/troubleshooting.md) if the cluster tools do not appear.

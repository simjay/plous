# plous-debug

[Agent skills](README.md) · [MCP tools](../reference/mcp.md)

Use **plous-debug** in Codex or Claude Code to investigate a Kubernetes failure, connect runtime evidence to the code in your repository, and verify a deployed fix. The skill uses Plous's cluster tools through a local MCP server while the agent uses its usual tools for code and tests.

## Install

[Install Plous](../getting-started/installation.md), then run the setup command for your agent.

| Agent | Setup | Invoke in your agent |
| --- | --- | --- |
| Codex | `plous setup codex` | `$plous-debug` |
| Claude Code | `plous setup claude` | `/plous-debug` |

Setup runs without prompts. Restart or reconnect your agent afterward. The server follows the current context in the agent's kubeconfig. If you switch contexts, the next tool call uses the new one.

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

## Configuration reference

### Pin a context

Use explicit options when you want the server to stay on one context. This example also sets the default namespace for diagnostics.

```sh
plous setup codex --kubeconfig /path/to/kubeconfig --context staging --namespace shop
```

Use `claude` for Claude Code. If an older setup pinned your context, rerun setup without `--context` and reconnect once to start following your current context. [Context and saved evidence](../mcp/scope.md)

### Installed files

| Client | Skill | MCP configuration |
| --- | --- | --- |
| Codex | `~/.agents/skills/plous-debug/SKILL.md` | `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` |
| Claude Code | `~/.claude/skills/plous-debug/SKILL.md` | `~/.claude.json` |

Rerun setup to update a managed installation. Setup preserves other MCP entries and stops if an unrelated skill or unmanaged `plous_debug` entry occupies the destination. Use `--home PATH` to choose a separate installation home.

### Remove the integration

Delete the installed `plous-debug` skill directory. For Codex, remove the block between `BEGIN PLOUS DEBUG MCP` and `END PLOUS DEBUG MCP`. For Claude Code, remove the `mcpServers.plous_debug` entry. Reconnect your agent. Saved evidence stays on disk until you remove its standalone profiles. [Evidence storage locations](../mcp/scope.md#saved-evidence)

## Next steps

Practice with [Investigate a restarting Pod](../guides/investigate-restarting-pod.md), configure [MCP without the skill](../mcp/setup.md), or read the [skill instructions](../../skills/plous-debug/SKILL.md).

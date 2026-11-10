# plous-debug

[Agent skills](README.md) · [MCP reference](../reference/mcp.md) · [Plous assistant panel](../guides/plous-mcp.md)

Use Plous from your existing Codex or Claude Code session to investigate Kubernetes failures, reproduce in-cluster errors, and verify deployed fixes. Your coding agent keeps its normal repository tools. Plous supplies cluster tools through a local MCP subprocess; the Plous TUI does not need to be running.

One skill, **plous-debug**, guides all three workflows. It contains investigation guidance rather than another model or agent. The standalone server and Plous assistant use the same guarded tool implementations.

## Install

Install Plous so `plous` is available on PATH. Setup discovers and records the executable automatically; you do not need to supply its path. Use your installed coding agent and its own authentication.

```sh
plous setup codex
# Or:
plous setup claude
```

Interactive setup asks for a kubeconfig path and context. For explicit or noninteractive setup:

```sh
plous setup codex --kubeconfig /path/to/kubeconfig --context staging --namespace shop
plous setup claude --kubeconfig /path/to/kubeconfig --context staging --namespace shop
```

Setup checks the Kubernetes API connection, installs the skill, and registers the `plous_debug` MCP server. It does not change kubeconfig, the active Kubernetes context, or your selected Plous assistant. `--skip-check` skips only the API check, not local target validation. It does not check that every resource or diagnostic permission is available.

| Client | Installed skill | MCP configuration |
| --- | --- | --- |
| Codex | `~/.agents/skills/plous-debug/SKILL.md` | `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` |
| Claude Code | `~/.claude/skills/plous-debug/SKILL.md` | `~/.claude.json` |

Existing client settings and other MCP servers are retained. Setup refuses an unmanaged `plous_debug` server or an unrelated skill at its destination. Rerun setup to update Plous's own installation. `--home PATH` chooses a separate installation home, including both the skill and client configuration.

Restart or reconnect your coding agent after setup. Codex can invoke `$plous-debug`; Claude Code can invoke `/plous-debug`. Both can also select the skill when a request matches its description. Organization settings may restrict skill or MCP availability.

## Investigate, reproduce, and verify

Start with the symptom and intended target:

> Use plous-debug. Checkout returns 502 in staging/shop. Inspect the failing workload's status, events, and current or previous container logs. Connect the observed error to repository code and test the fix locally.

Reads include Kubernetes resources, events, bounded container logs, available usage, and relevant Helm, Argo, and Flux evidence. Exact tool names are in the [MCP reference](../reference/mcp.md#available-tools). The agent must identify the container before requesting its logs. Missing permissions and partial results remain explicit.

After deploying the fix through your existing process:

> Verify the checkout fix. Confirm the intended image is running, repeat the failing request if active diagnostics are enabled, and check fresh errors. Separate local test results from cluster verification.

The agent cannot infer a successful deployment from a repository edit or a passing health endpoint.

## Enable active debugging

Setup exposes reads by default. To enable bounded commands in existing containers, add `--allow-exec` when running setup. To enable temporary HTTP, DNS, or TCP tests, supply `--diagnostic-image IMAGE`. Both apply to the captured default namespace and require Kubernetes permission for the operation.

Follow [Active diagnostics](../mcp/active-diagnostics.md) for commands, image requirements, limits, remote-outcome handling, and cleanup. State-changing tests need explicit intent; a failed connection does not establish that an action never ran.

## Scope, evidence, and lifecycle

Setup fixes the cluster context, default namespace, and diagnostic grants until the agent reconnects. Reads may use other namespaces allowed by the captured credentials. Each standalone process saves its own evidence profile; previous standalone sessions are not automatically imported or searched.

Read [Scope, evidence, and lifecycle](../mcp/scope.md) for profile locations, persistence, and cancellation. Logs and command output may be sent to your agent's model provider; kubeconfig credentials stay with Plous.

## Remove the integration

Remove the installed `plous-debug` skill directory and its `plous_debug` MCP configuration. For Codex, remove only the block between `BEGIN PLOUS DEBUG MCP` and `END PLOUS DEBUG MCP`. For Claude Code, remove only `mcpServers.plous_debug`. Reconnect the client. Saved evidence remains until you remove its standalone profiles.

## Next steps

Use the [restarting-Pod tutorial](../guides/investigate-restarting-pod.md) as a concrete target. [MCP setup](../mcp/setup.md) covers manual tool connections without a skill. The installed instructions are maintained in [skills/plous-debug](../../skills/plous-debug/SKILL.md).
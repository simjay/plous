# plous-debug

[Agent skills](README.md) · [MCP tools](../reference/mcp.md)

Use **plous-debug** in Codex or Claude Code to investigate Kubernetes failures, test a suspected cause, and check a deployed fix.

Your agent keeps its usual repository tools. Plous adds cluster tools through MCP. The Plous terminal UI does not need to be running.

## Install

Install [Plous](../getting-started/installation.md), then run the command for your agent:

```sh
plous setup codex
```

For Claude Code:

```sh
plous setup claude
```

Setup asks for your kubeconfig and context. It checks the API connection, installs the skill, and configures MCP. It finds the Plous executable automatically.

Restart or reconnect your agent. Invoke `$plous-debug` in Codex or `/plous-debug` in Claude Code.

To supply the target without prompts:

```sh
plous setup codex \
  --kubeconfig /path/to/kubeconfig \
  --context staging \
  --namespace shop
```

Use `claude` in place of `codex` for Claude Code. Both use the client's existing authentication.

### Files installed

| Client | Skill | MCP configuration |
| --- | --- | --- |
| Codex | `~/.agents/skills/plous-debug/SKILL.md` | `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` |
| Claude Code | `~/.claude/skills/plous-debug/SKILL.md` | `~/.claude.json` |

Setup preserves other MCP entries. It stops if an unrelated skill or unmanaged `plous_debug` entry already uses the destination. Rerun setup to update an installation managed by Plous.

`--home PATH` chooses a separate installation home. `--skip-check` skips the API connection test but still validates the local target settings.

## Investigate, reproduce, and verify

Name the symptom, context, and namespace:

> Use plous-debug. Checkout returns 502 in staging/shop. Check its Pods, events, and logs. Find the cause and test a fix in this repository.

The agent can read Kubernetes objects, events, logs, metrics, and available Helm, Argo CD, and Flux data. See the [tool list](../reference/mcp.md#available-tools).

Deploy through your usual process. Then ask:

> Verify the checkout fix in staging/shop. Check the running image and new errors. Repeat the failing request if diagnostics are enabled.

A passing local test does not confirm that a fix is deployed.

## Enable active debugging

Reads are enabled by default. Add `--allow-exec` during setup to allow commands in existing containers.

```sh
plous setup codex --allow-exec
```

Add `--diagnostic-image IMAGE` to allow temporary HTTP, DNS, and TCP tests. Both options apply to the selected default namespace and require Kubernetes permissions.

See [Active diagnostics](../mcp/active-diagnostics.md) for examples, image requirements, and cleanup.

## Scope, evidence, and lifecycle

The cluster, default namespace, and diagnostic permissions stay fixed until the agent reconnects. Reads can use other namespaces permitted by your Kubernetes credentials.

Each MCP process saves its own tool evidence. It does not search earlier standalone sessions automatically. Logs and command output may be sent to the agent's model provider. Kubeconfig credentials stay with Plous.

See [Scope and saved evidence](../mcp/scope.md) for storage locations and cancellation behavior.

## Remove the integration

Delete the installed `plous-debug` skill directory and remove its MCP entry:

- **Codex:** remove the block between `BEGIN PLOUS DEBUG MCP` and `END PLOUS DEBUG MCP`.
- **Claude Code:** remove `mcpServers.plous_debug`.

Reconnect the client. Saved evidence remains until you delete the standalone profiles.

## Next steps

Try [Investigate a restarting Pod](../guides/investigate-restarting-pod.md). Use [MCP setup](../mcp/setup.md) for a connection without the skill. The skill's source is [skills/plous-debug](../../skills/plous-debug/SKILL.md).

# Debug on Kubernetes with your coding agent

Use `plous-debug` to inspect workloads, events, and logs from the coding agent you already use.

## Install

[Install Plous v0.0.5 or later](../getting-started/installation.md), then run your agent's setup command.

| Coding agent | Setup command |
| --- | --- |
| Antigravity CLI | `plous setup agy` |
| Claude Code | `plous setup claude` |
| Codex | `plous setup codex` |
| Cursor Agent | `plous setup cursor` |
| Gemini CLI | `plous setup gemini` |
| Grok Build | `plous setup grok` |
| OpenCode | `plous setup opencode` |

## Start in your repository

1. **Reconnect your agent.** Open your repository and follow its trust prompts. If it asks for MCP approval, review `plous_debug`.
2. **Load the skill.** In Codex, type `$plous-debug`. In Claude Code, Grok Build, or Antigravity CLI, use `/plous-debug`. In the other clients, ask your agent to use `plous-debug`.
3. **Confirm the target.** Ask “Which Kubernetes context and default namespace is Plous using?” By default, each call follows the current context in the agent’s kubeconfig.

If the target is wrong or tools are missing, see [connection troubleshooting](../mcp/setup.md#troubleshoot-the-connection).

## Follow an incident from cluster to code

> Checkout started returning 502s in staging/shop after the last rollout. Confirm the cluster, then check checkout Pods, events, and logs. Separate what you observed from the likely cause.

If the logs show a timeout at port `8081` and the payments Service exposes `8080`, ask the agent to trace where the checkout configuration gets its port. After you deploy a fix, ask it to confirm the intended image is running and check fresh logs.

When logs leave a question open, [enable active diagnostics](../mcp/active-diagnostics.md) to run a test from a specific container or a temporary Job. This requires explicit flags and Kubernetes permissions.

## What setup installs

Setup installs the [skill instructions](../../skills/plous-debug/SKILL.md) and registers local cluster tools over MCP. Your agent keeps its usual model, sign-in, and repository tools. For Claude Code or Codex [inside Plous](../guides/ai-chat.md#run-claude-code-or-codex), Plous connects the tools itself.

To pin a context instead of following kubeconfig, supply its name during setup and reconnect:

```sh
plous setup codex --context staging --namespace shop
```

See [MCP setup and troubleshooting](../mcp/setup.md) for manual config, launch options, [installed files and removal](../mcp/setup.md#installed-files-and-removal), and saved evidence. Practice with [a restarting Pod](../guides/investigate-restarting-pod.md) in a disposable cluster.

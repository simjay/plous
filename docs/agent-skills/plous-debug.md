# Debug on Kubernetes with your coding agent

Use `plous-debug` to investigate a Kubernetes symptom from your coding agent. The agent reads cluster evidence, checks the relevant code or configuration, and proposes a fix for you to review and apply.

## Install

[Choose your coding agent](../guides/ai-chat.md#choose-an-agent) for its setup command and instructions for invoking `plous-debug`.

## Start in your repository

1. Reopen your agent so it loads the skills and MCP server. If the client asks for MCP approval, review `plous_debug`.
2. Invoke `plous-debug` and describe the symptom. The skill checks the Kubernetes context and namespace before reading resources.

See [connection troubleshooting](../mcp/setup.md#troubleshoot-the-connection) if the target is wrong or tools are missing.

## Follow an incident from cluster to code

> Checkout started returning 502s in staging/shop after the last rollout. Diagnose the failure, show the evidence, and tell me what to change.

**Diagnosis.** Checkout in `staging/shop` is likely calling the wrong payments port after the rollout.

**Evidence**

- **Checkout container log** shows `dial tcp payments:8081: i/o timeout` (14:32 UTC, `record:…`).
- **Payments Service in `shop`** exposes only port `8080` (14:33 UTC, `record:…`).
- **`charts/checkout/values.yaml:42`** sets `payments.port` to `8081`. That matches the failed connection in the log.

**Proposed fix.** Update `payments.port` to `8080` if that is the intended Service port. After deployment, repeat the checkout request that returned 502.

The example abbreviates the record IDs. In the same MCP session, ask “Show me the Service evidence” to reopen that observation.

When logs leave a question open, [enable active diagnostics](../mcp/active-diagnostics.md) to run a test from a specific container or a temporary Job. This requires explicit flags and Kubernetes permissions.

## What setup installs

Setup installs [plous-debug](../../skills/plous-debug/SKILL.md) and [plous-monitor](plous-monitor.md) and registers one local MCP server for their cluster tools. For Claude Code or Codex in the Plous panel, Plous connects the tools when it opens the CLI.

See [MCP setup](../mcp/setup.md) for changing targets, upgrades, removal, and saved evidence. To try an investigation in a disposable cluster, use the [restarting Pod exercise](../guides/investigate-restarting-pod.md).

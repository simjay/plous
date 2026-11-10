# Documentation

[Project README](../README.md) · [Choose your workflow](getting-started/workflows.md)

Inspect Kubernetes interactively, connect tools through MCP, or guide your coding agent with a skill. These guides describe the current development build.

## Choose your workflow

<div class="docs-paths">
<a href="getting-started/README.md"><span class="docs-path-icon" aria-hidden="true">&gt;_</span><strong>Terminal UI</strong><span>Launch Plous, select a cluster, and inspect your first workload.</span><span class="docs-path-action">Open the quickstart <span aria-hidden="true">→</span></span></a>
<a href="mcp/setup.md"><span class="docs-path-icon" aria-hidden="true">↗</span><strong>MCP server</strong><span>Connect an agent directly to cluster tools and saved evidence.</span><span class="docs-path-action">Configure MCP <span aria-hidden="true">→</span></span></a>
<a href="agent-skills/plous-debug.md"><span class="docs-path-icon" aria-hidden="true">✳</span><strong>Agent skills</strong><span>Use plous-debug to investigate, reproduce, and verify failures.</span><span class="docs-path-action">Set up plous-debug <span aria-hidden="true">→</span></span></a>
</div>

All workflows use your Kubernetes permissions. AI agents use their configured provider or CLI authentication. The terminal workspace works without AI setup.

## Get started

[Choose your workflow](getting-started/workflows.md) · [Installation and updates](getting-started/installation.md) · [Quickstart](getting-started/README.md)

## Terminal UI

1. [Learn the controls](guides/navigation.md), then [understand Overview](guides/overview.md).
2. [Save workspaces and connections](guides/workspaces.md) and [review Activity](guides/activity.md).
3. [Set up AI chat](guides/ai-chat.md) or [Claude Code and Codex inside Plous](guides/plous-mcp.md).

## MCP server

[Overview](mcp/README.md) · [Setup and client configuration](mcp/setup.md) · [Tools and permissions](reference/mcp.md) · [Active diagnostics](mcp/active-diagnostics.md) · [Scope and lifecycle](mcp/scope.md) · [Troubleshooting](mcp/troubleshooting.md)

## Agent skills

Read the [Agent skills overview](agent-skills/README.md), then follow [plous-debug](agent-skills/plous-debug.md). Use it to investigate, reproduce, and verify Kubernetes failures from your existing coding-agent session.

## Tutorials

Start with [Investigate a restarting Pod](guides/investigate-restarting-pod.md). The [Kubernetes playground](integrations/kubernetes/playground.md) and [GitOps playground](integrations/gitops/playground.md) provide optional examples that create resources in a disposable cluster.

## Integrations

[Capabilities and connection setup](integrations/README.md) · [Kubernetes](integrations/kubernetes/README.md) · [Helm](integrations/helm/README.md) · [Argo CD](integrations/argo/README.md) · [Flux](integrations/flux/README.md)

## Keep these nearby

| Reference | Use it for |
| --- | --- |
| [Keymap](reference/keymap.md) | Exact keys and commands for each view |
| [Configuration](reference/configuration.md) | Contexts, profiles, providers, and remembered settings |
| [MCP tools](reference/mcp.md) | Tool contracts, transports, and diagnostic permissions |
| [Data and privacy](reference/data-and-privacy.md) | Stored evidence, credentials, and backups |
| [Troubleshooting](reference/troubleshooting.md) | Setup, access, agent, and storage problems |

## Develop Plous

The [developer guide](contributing/README.md) covers contributing, architecture, tests, benchmarks, releases, and website maintenance.

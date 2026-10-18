<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/lockup-dark.svg">
    <img src="assets/logo/lockup.svg" alt="Plous" width="320">
  </picture>
</h1>

**Your Kubernetes journey, in one terminal.**

Plous is an open-source **Kubernetes lifecycle tool** for inspecting clusters, working with configuration, and investigating problems with AI. It brings Kubernetes, Helm, Kustomize, Argo CD, and Flux into a shared terminal workspace.

The name comes from Greek **πλοῦς (plous)**, meaning [voyage](https://apps.perseids.org/woodhouse/l/voyage). Kubernetes is named for the [helmsman](https://kubernetes.io/docs/concepts/overview/); Plous is named for the whole journey, from source configuration to running workloads.

[Getting started](#getting-started) · [Documentation](docs/README.md) · [MCP](docs/guides/plous-mcp.md) · [Contributing](docs/contributing/README.md)

> **Early development: `0.0.1`.** Plous is not ready to ship.

## Features

- **Cluster overview.** Health, CPU and memory usage, recent observed changes, and resources that need attention.
- **Kubernetes operations.** Resource browsing, YAML, events, related objects, workload logs, container terminals, and explicit edit, scale, restart, and delete actions.
- **Helm and GitOps.** Inspect release revisions, compare values and manifests, preview chart changes, and roll back explicitly. Read Argo CD and Flux controller status through the same cluster connection.
- **Configuration workspace.** Browse and edit local files, explore Kustomize dependencies, render charts and overlays, inspect captured field history, and compare generated output.
- **AI investigation.** Use Plous with OpenAI or Anthropic, or your installed Claude Code or Codex. Ask about the selected connection and inspect the saved evidence behind tool results. Assistant tools are read-only.
- **Saved workspaces.** Group contexts and repositories, retain configuration revisions, and return to connection-specific conversations and preferences.

Cluster browsing works without an AI account, and Code works without a cluster. See [integration support](docs/integrations/README.md) for current capabilities and limits.

## Getting started

Requires **macOS or Linux**, **Go 1.26.4+**, Git, Make, and an interactive terminal at least 80 columns by 16 rows.

```sh
git clone https://github.com/simjay/plous.git
cd plous
make run
```

Plous opens with your terminal's Kubernetes context and launch directory. Use **Alt+Left/Right** to change panels and **hh** for Help while navigating. Run `:quit` or press **Ctrl+C** to save the session and exit. In Claude Code or Codex input, Ctrl+C goes to the agent; press **Ctrl+]** first to return to Plous before exiting.

The binary is built at `bin/plous`. See [Getting started](docs/getting-started/README.md) for installing on PATH, choosing a context, and taking a first look at Code. Kubernetes, Helm, Kustomize, and Git libraries are embedded; configured Kubernetes authentication helpers and external assistant CLIs must be installed separately.

## MCP

Plous exposes its connection reads, saved evidence, and view controls to the Claude Code or Codex session inside the assistant panel through **Model Context Protocol (MCP)**. Select your installed agent in **Settings → Application → Agent**; Plous configures the tool connection automatically. The CLI keeps its own model and authentication settings. Built-in Plous provider connections are configured under **Settings → Agent → Plous**.

Ask “Why is this Pod restarting?” or “What changed between these Helm revisions?” The agent can read scoped evidence and open the relevant YAML, logs, or comparison beside the conversation.

The bridge lasts for the native session. There is no standalone `plous mcp` server or manual registration in another app. See the [MCP guide](docs/guides/plous-mcp.md) for setup and examples, and the [MCP reference](docs/reference/mcp.md) for tools, access limits, and transport details.

## Documentation

- [User guides](docs/README.md): navigation, workspaces, cluster operations, Code, Activity, and AI.
- [Keymap](docs/reference/keymap.md) and [Configuration](docs/reference/configuration.md).
- [Data and privacy](docs/reference/data-and-privacy.md): local storage, credentials, and provider requests.
- [Troubleshooting](docs/reference/troubleshooting.md).
- [Kubernetes playground](docs/integrations/kubernetes/playground.md) and [GitOps playground](docs/integrations/gitops/playground.md).

## Contributing

Bug reports, documentation improvements, and focused code contributions are welcome. [Open an issue](https://github.com/simjay/plous/issues) with reproduction steps, or read the [contributor guide](docs/contributing/README.md) before making a change. Discuss larger product or architecture changes first.

Run `make check` for documentation, formatting, vet, Go tests, and terminal tests. Documentation and terminal checks require Python 3.9+. [PlousBench](benchmark/README.md) covers tool correctness and performance through direct calls and MCP.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), and [Eino](https://github.com/cloudwego/eino).

## License

[Apache-2.0](LICENSE).

<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/lockup-dark.svg">
    <img src="assets/logo/lockup.svg" alt="Plous" width="320">
  </picture>
</h1>

A terminal workspace for infrastructure and deployment tools.

Plous brings tool connections, configuration history, and AI conversations into one terminal workspace. Its main views are **Overview**, **Explore**, and **Activity**. Each workspace can group several tools, contexts, and repositories.

**Status: `0.0.1` · early development · not ready to ship.**

[Get started](docs/getting-started.md) · [Documentation](docs/README.md) · [Keymap](docs/keymap.md) · [Troubleshooting](docs/troubleshooting.md) · [Development](docs/development.md)

## What works today

- Local workspaces with multiple tool connections and configuration history.
- Kubernetes resource lists, Describe, YAML, Events, grouped relationships, and Pod or merged workload logs.
- Plous Editor for YAML and read-only values, Diff Reviewer for validated changes and configuration history, and Log Viewer for logs and terminal review.
- Supported Kubernetes actions from object details, including editing, scaling, rollout restart, deletion, and container Exec.
- Saved configuration references for Helm, Kustomize, Terraform, Argo, and Kargo.
- OpenAI and Anthropic assistant with read-only Kubernetes investigation, searchable conversation records, automatic context compaction, and saved drafts.
- Panel search, keyboard commands, and adjustable panel visibility.

Deployment pipelines, remote workspaces, sharing, and assistant write operations are not available in this version. See the [documentation overview](docs/README.md#what-this-version-supports) for current scope.

## Build and start

Requires macOS or Linux, Go **1.26.4+**, Git, Make, and an interactive terminal at least **80 columns by 16 rows**.

```sh
git clone https://github.com/simjay/plous.git
cd plous
make run
```

On the first launch, press **Enter** to create a workspace. Enter a name and choose **Save**, then select a tool or press **Esc** to start empty.

The [getting started guide](docs/getting-started.md) walks through creating a workspace and choosing your tools. No cluster or AI account is required. See [Tool integrations](docs/integrations.md) for each tool's setup and current capabilities.

## Learn the interface

Use **Tab / Shift+Tab** to change panels, arrows and **Enter** to select items, **/** to search, and **Esc** to go back. **hh** opens Help while navigating. Type **:q** and press Enter to save and exit, or use **Ctrl+C** from anywhere.

- [Navigation](docs/navigation.md): panels, commands, search, and typing.
- [Workspaces](docs/workspaces.md): connections and settings across tools.
- [Activity](docs/activity.md): configuration history and revision comparisons.
- [AI chat](docs/ai-chat.md): connect a vendor, choose a model, and manage conversations.
- [Tool integrations](docs/integrations.md): capabilities, connection setup, and tool-specific workflows.

Plous stores data under `~/.plous/` by default. Use `--data-dir PATH` for another profile. API keys are kept in a separate plaintext file with owner-only permissions. [Data and privacy](docs/data-and-privacy.md) explains storage, backups, and what is sent to AI vendors.

## Development and contributing

Run `make check` for formatting, vet, Go tests, and executable/terminal tests. Terminal tests also need Python 3.9+. `make dist` builds local macOS and Linux archives without publishing them. See [Contributing](docs/contributing.md) and [Development](docs/development.md) for the contributor workflow and repository layout.

The documentation website builds from `docs/` with Astro Starlight. Use `make docs-install` and `make docs-dev` to preview it, or `make docs-check` to validate the production build. See [documentation maintenance](docs/documentation.md) for GitHub Pages setup.

The interface uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). AI execution uses [Eino](https://github.com/cloudwego/eino), with embedded [Starlark](https://github.com/google/starlark-go) for recursive investigation.

[Report a bug](https://github.com/simjay/plous/issues) with the version, OS, terminal, and reproduction steps. The [troubleshooting guide](docs/troubleshooting.md#report-a-bug) describes what to include.

## License

[Apache-2.0](LICENSE).

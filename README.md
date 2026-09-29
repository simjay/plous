# Plous

A terminal workspace for infrastructure and deployment tools.

Plous brings multiple workspaces, Kubernetes contexts, and tool configurations into a full-screen interface. A workspace tree sits beside four main views—Overview, Resources, Deployment, and Activity—with an AI assistant panel on the right.

**Status: `0.0.1` · early development · not ready to ship.** The workspace and configuration layer works today. Live resource browsing, deployment execution, and AI integration are not implemented yet.

[Documentation](docs/README.md) · [Keymap](docs/keymap.md) · [Issues](https://github.com/simjay/plous/issues)

## What works today

- Create multiple workspaces and manage their tool connections.
- Discover local Kubernetes contexts and add several to one workspace.
- Save configuration references for Helm, Kustomize, Terraform, Argo, and Kargo.
- Browse a collapsible workspace tree and switch between four main views.
- Inspect configuration changes in Activity, including before/after values.
- Keep a local chat draft for each workspace. Nothing is sent to an AI service.

## Quick start

Requires Go 1.26 or newer, Git, Make, and an interactive terminal on macOS or Linux.

```sh
git clone https://github.com/simjay/plous.git
cd plous
make run
```

Use a terminal at least **80 columns × 16 rows**. On first launch, press **Enter** to create a workspace, enter a name, then select **Save**. Choose **Add tool → Kubernetes** to select local kubecontexts, or start with an empty workspace.

**Tab / Shift+Tab** move between panels. With the main panel focused, type `:1`, `:2`, `:3`, or `:4` and press **Enter** to switch views. Type `:help` for help or `:quit` to save the session and exit.

See the [user guide](docs/README.md) for workspace setup and the [keymap](docs/keymap.md) for all current bindings.

## Local data

Plous saves workspace configuration, change history, navigation state, and chat drafts under `~/.plous/`. Kubernetes discovery reads your kubeconfig locally; it does not change the active context, contact clusters, or copy credentials into Plous.

To use a separate data directory:

```sh
make build
./bin/plous --data-dir /tmp/plous-demo
```

## Development

| Command | Purpose |
| --- | --- |
| `make run` | Build and open Plous. |
| `make build` | Build `bin/plous`. |
| `make check` | Check formatting, run vet and tests, build, and verify the version. |
| `make dist` | Package macOS and Linux binaries for amd64 and arm64, with checksums. |
| `make clean` | Remove generated binaries and archives. |

The application version comes from `cmd/plous/VERSION`. Packaging uses that version; it does not publish a release. CI runs checks on Linux and macOS and verifies distribution archives on Linux.

The terminal interface uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Contributing

Bug reports and focused improvements are welcome. [Open an issue](https://github.com/simjay/plous/issues) to describe a problem or discuss a feature before starting a larger change. Include your operating system, terminal, Plous version (`./bin/plous --version`), and steps to reproduce bugs.

For a pull request, keep changes small, update relevant documentation, and run `make check`. Run `make dist` when changing packaging. Use a temporary `--data-dir` for manual checks. Avoid including credentials or private configuration in reports or commits.

## License

Plous is licensed under [Apache-2.0](LICENSE).

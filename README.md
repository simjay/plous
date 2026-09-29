<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/lockup-dark.svg">
    <img src="assets/logo/lockup.svg" alt="Plous" width="320">
  </picture>
</h1>

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

**Tab / Shift+Tab** move between panels. With the main panel focused and no form open, typing `:` opens a command box at the bottom. Type `:1`, `:2`, `:3`, or `:4` and press **Enter** to switch views. Every command has a full name and a short alias: for example, `:overview` or `:o` opens Overview, `:help` or `:h` opens help, and `:quit` or `:q` saves the session and exits.

**Ctrl+C** saves the session and chat drafts, then exits from any panel. Unsaved form edits are not applied. Use **Esc** to cancel a form or command without exiting.

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
| `make test` | Run all Go unit tests and executable/terminal tests. |
| `make test-unit` | Run Go package tests. |
| `make test-e2e` | Build and test the actual executable in isolated terminals. |
| `make test-race` | Run Go tests with the race detector. |
| `make test-coverage` | Write Go coverage reports to `.cache/tests/`. |
| `make check` | Check formatting, run vet, and run all tests. |
| `make dist` | Package macOS and Linux binaries for amd64 and arm64, with checksums. |
| `make clean` | Remove generated binaries and archives. |

The application version comes from `VERSION`. Packaging uses that version; it does not publish a release. CI runs checks on Linux and macOS and verifies distribution archives on Linux.

`main.go` at the repository root is the executable entry point. Application code lives under `internal/`, with Go unit tests beside the code they test. `tests/e2e/` contains tests of the built executable. Terminal tests require Python 3.9+ and use only its standard library; Python is not needed to build or run Plous. See the [development and testing guide](docs/development.md) for the repository layout and test coverage.

The terminal interface uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Contributing

Bug reports and focused improvements are welcome. [Open an issue](https://github.com/simjay/plous/issues) to describe a problem or discuss a feature before starting a larger change. Include your operating system, terminal, Plous version (`./bin/plous --version`), and steps to reproduce bugs.

For a pull request, keep changes small, update relevant documentation, and run `make check`. Run `make dist` when changing packaging. Use a temporary `--data-dir` for manual checks. Avoid including credentials or private configuration in reports or commits.

## License

Plous is licensed under [Apache-2.0](LICENSE).

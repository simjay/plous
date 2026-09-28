# Plous

Plous `0.0.1` is an early development build, not ready to ship.

Plous opens a full-screen workspace manager with a collapsible workspace tree, four main-panel modes, and local chat drafts. It saves configuration and history under `~/.plous/`.

## Build and run

Requires Go 1.26 or newer and Make.

```sh
make run
```

Or use `make build` followed by `./bin/plous`. The terminal must be at least 80 columns by 16 rows; smaller windows show a resize message and disable hidden actions. Exiting restores the previous terminal screen.

On first launch, press Enter to create a workspace. Enter a name, then select Save. Choose **Add tool → Kubernetes** to discover local kubecontexts. Leave the kubeconfig path blank to use `KUBECONFIG` or `~/.kube/config`, or enter another file. Space selects multiple contexts; Enter adds the selected contexts (or the highlighted one). You can also start with an empty workspace.

Kubeconfig is read locally without changing its active context, contacting clusters, copying credentials, or running authentication plugins. Helm, Kustomize, Terraform, Argo, and Kargo entries save their relevant context, directory, or source references. They are saved configuration, not live connections.

## Navigation and settings

- **Tab / Shift+Tab:** move between tree, main, and chat panels. The highlighted border identifies focus.
- **Tree:** Up/Down selects rows; Left/Right collapses or expands groups; Enter opens a workspace, tool, connection, or settings. Changing the selected tool keeps the current main-panel mode. The tree has no numeric mode shortcuts.
- **Main navigation:** type a command and press Enter: `:1` Overview, `:2` Resources, `:3` Deployment, `:4` Activity, `:new`, `:settings`, `:help`, or `:quit`.
- **Forms:** Enter advances to the next field or activates Save/Cancel. Up/Down moves between fields and actions. Esc cancels. Tab still moves between panels and preserves unfinished form input.
- **Text entry:** letters and punctuation remain text. Paste never executes commands. Esc leaves chat editing or cancels a form/command. Ctrl+C cancels main input; use `:quit` from main navigation to exit.

Workspace settings lets you rename a workspace and add, edit, or remove tool connections. Removing a connection only removes its saved Plous entry. Each save creates a revision with exact before/after values. **Activity** lists those revisions; Enter opens a change and arrows/PageUp/PageDown scroll its details.

**Overview** shows saved configuration. **Resources** shows selected connection details; live resource browsing is not implemented. **Deployment** is an empty state; it does not execute deployments. There is no shared resource-action keymap.

The chat panel keeps a separate draft for each workspace. Type directly; Enter adds a newline while editing. `:` and `/` remain literal text. Drafts autosave every half second when changed and flush on normal exit. AI is not connected and nothing is sent. Form edits survive panel switches but only enter configuration history after Save.

## Local state and checks

`~/.plous/workspaces/<id>/revisions/` contains chronological revision directories, each with a structured `change.json` holding the before/after configuration. These committed records are authoritative; interrupted temporary writes are ignored. `~/.plous/session.json` stores selection, view positions, tree expansion, and drafts. Stable IDs preserve history across renames. Files are private to the local user; one Plous process owns a data directory at a time. Corrupt committed data is reported and preserved rather than silently reset.

Use an isolated directory when trying fixtures or keeping a separate local profile:

```sh
./bin/plous --data-dir /tmp/plous-demo
./bin/plous --version
make check
```

The application version remains `0.0.1`. `make check` checks formatting, runs Go vet and tests, builds the executable, and verifies its embedded version. Tests use temporary storage. Terminal rendering uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Repository

```text
cmd/plous/main.go          Entry point
cmd/plous/                Terminal UI, forms, input routing, and rendering
internal/workspace/       Persistent revisions and session storage
internal/kubeconfig/      Local kubecontext discovery
cmd/plous/VERSION          Single source of the application version
go.mod                    Go module
Makefile                  Build, run, check, package, clean
scripts/dist.sh           Cross-platform release packaging
.github/workflows/ci.yml   Checks on branches and pull requests
.github/workflows/release.yml  Tagged release distribution
```

## Distribution

Build release archives locally:

```sh
make dist VERSION=v0.0.1
```

`dist/` contains four `.tar.gz` archives for macOS (`darwin`) and Linux, each for x86-64 (`amd64`) and ARM64 (`arm64`), plus `checksums.txt`. Each archive contains `plous`, this README, and the Apache-2.0 license. `make dist` defaults to the version in `cmd/plous/VERSION`; an explicit `VERSION` must match it with a `v` prefix. `make clean` removes generated binaries and archives.

After downloading the archive matching your system and `checksums.txt` from a release:

```sh
# Example: Apple Silicon Mac. Replace the version/platform as needed.
grep 'plous_v0.0.1_darwin_arm64.tar.gz$' checksums.txt | shasum -a 256 -c -
tar -xzf plous_v0.0.1_darwin_arm64.tar.gz
./plous
```

To install it on your user PATH, place `plous` in `~/.local/bin` and add that directory to your PATH if necessary. Release binaries are not Apple-notarized or code-signed by this initial workflow.

## CI and releases

GitHub Actions runs `make check` on Linux and macOS for branch pushes and pull requests. Linux also builds and verifies all four distribution archives.

The release pipeline is available for a future owner-approved release. Do not create or push a release tag during development. When shipping is explicitly authorized, the matching tag would be:

```sh
git tag -a v0.0.1 -m 'Plous v0.0.1'
git push origin v0.0.1
```

The release workflow reruns CI, builds the archives, uploads them with checksums to a draft GitHub Release, then publishes it. It uses GitHub's built-in token; no personal token or external release service is required. Repository/organization policy must allow the release job's `contents: write` permission. Version tags use `vMAJOR.MINOR.PATCH` and must match the application version. Major or minor version changes require explicit owner direction; versions are never bumped automatically.

If upload fails, the release stays a draft and the same workflow can be rerun. Published releases are never overwritten by a rerun; use a new version. Releases will appear in [simjay/plous](https://github.com/simjay/plous/releases). No initial version tag or live release has been created as part of this bootstrap.

Workflow references: [GitHub's Go guide](https://docs.github.com/en/actions/tutorials/build-and-test-code/go), [GitHub CLI release creation](https://cli.github.com/manual/gh_release_create).

## Current scope

The current scope is the version `0.0.1` workspace, configuration history, navigation, and input layer. Live resource browsers, deployment execution, and AI behavior will be developed one step at a time.

Licensed under [Apache-2.0](LICENSE), preserving the license already present in the connected GitHub repository.

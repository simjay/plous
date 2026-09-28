# Plous

Plous `0.0.1` is an early development build, not ready to ship.

The executable opens a full-screen terminal with three panels: a narrow list on the left, the main view in the center, and an AI assistant chat placeholder on the right. These are empty sections for now; AI is not connected.

## Build and run

Requires Go 1.26 or newer and Make.

```sh
make build
./bin/plous
```

Use Tab / Shift+Tab to switch panel focus and `q` or Ctrl+C to quit. The layout follows terminal resizing and restores your previous terminal screen on exit. A terminal of at least 80 columns by 12 rows is required; smaller windows show a resize message.

`./bin/plous --version` prints `0.0.1` without opening the UI. `make check` checks formatting, runs Go vet and layout/navigation tests, builds the executable, and verifies the embedded version. Terminal rendering uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Repository

```text
cmd/plous/main.go          Entry point
cmd/plous/ui.go            Full-screen three-panel layout
cmd/plous/VERSION          Single source of the application version
go.mod                    Go module
Makefile                  Build, check, package, clean
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

The current scope is the version `0.0.1` terminal shell and existing build/distribution pipeline. Panel contents, deployment features, and AI behavior will be developed one step at a time.

Licensed under [Apache-2.0](LICENSE), preserving the license already present in the connected GitHub repository.

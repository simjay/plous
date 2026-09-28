# Plous

The initial executable prints one line:

```text
welcome to plous
```

## Build and run

Requires Go 1.26 or newer and Make.

```sh
make build
./bin/plous
```

`make check` checks formatting, runs Go vet/tests, builds the executable, and verifies its exact output. The application has no third-party dependencies or product features yet.

## Repository

```text
cmd/plous/main.go          Entry point
go.mod                    Go module
Makefile                  Build, check, package, clean
scripts/dist.sh           Cross-platform release packaging
.github/workflows/ci.yml   Checks on branches and pull requests
.github/workflows/release.yml  Tagged release distribution
```

## Distribution

Build release archives locally:

```sh
make dist VERSION=v0.1.0
```

`dist/` contains four `.tar.gz` archives for macOS (`darwin`) and Linux, each for x86-64 (`amd64`) and ARM64 (`arm64`), plus `checksums.txt`. Each archive contains `plous`, this README, and the Apache-2.0 license. `make dist` without a version creates development archives. `make clean` removes generated binaries and archives.

After downloading the archive matching your system and `checksums.txt` from a release:

```sh
# Example: Apple Silicon Mac. Replace the version/platform as needed.
grep 'plous_v0.1.0_darwin_arm64.tar.gz$' checksums.txt | shasum -a 256 -c -
tar -xzf plous_v0.1.0_darwin_arm64.tar.gz
./plous
```

To install it on your user PATH, place `plous` in `~/.local/bin` and add that directory to your PATH if necessary. Release binaries are not Apple-notarized or code-signed by this initial workflow.

## CI and releases

GitHub Actions runs `make check` on Linux and macOS for branch pushes and pull requests. Linux also builds and verifies all four distribution archives.

Once this repository is hosted on GitHub with Actions enabled, push a new version tag to publish a release:

```sh
git tag -a v0.1.0 -m 'Plous v0.1.0'
git push origin v0.1.0
```

The release workflow reruns CI, builds the archives, uploads them with checksums to a draft GitHub Release, then publishes it. It uses GitHub's built-in token; no personal token or external release service is required. Repository/organization policy must allow the release job's `contents: write` permission. Version tags use `vMAJOR.MINOR.PATCH`.

If upload fails, the release stays a draft and the same workflow can be rerun. Published releases are never overwritten by a rerun; use a new version. Releases will appear in [simjay/plous](https://github.com/simjay/plous/releases). No initial version tag or live release has been created as part of this bootstrap.

Workflow references: [GitHub's Go guide](https://docs.github.com/en/actions/tutorials/build-and-test-code/go), [GitHub CLI release creation](https://cli.github.com/manual/gh_release_create).

## Current scope

The current scope is a minimal executable and build/distribution pipeline. Product features and UI design will be discussed one step at a time.

Licensed under [Apache-2.0](LICENSE), preserving the license already present in the connected GitHub repository.

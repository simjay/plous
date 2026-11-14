# Installation and updates

[Get started](README.md) · [Choose your workflow](workflows.md)

## Requirements

- macOS or Linux. Native Windows is not supported.
- A terminal at least 80 columns wide and 16 rows tall.
- A kubeconfig for cluster access, including any authentication helper it needs.

Plous includes Kubernetes and Helm libraries. You do not need `kubectl` or `helm` for normal use. Plous is still in early development.

## Install with Homebrew

```sh
brew install simjay/tap/plous
plous --version
plous
```

Homebrew installs a bottle when available, or builds from source. See the [tap](https://github.com/simjay/homebrew-tap) for supported bottle platforms.

## Published releases and development docs

Choose website documentation matching `plous --version`. Development docs follow `main` and may describe unreleased features.

Standalone MCP and `plous setup` require v0.0.3 or newer. Starting with v0.0.4, standalone MCP follows kubeconfig’s current context by default.

## Build from source

You need Git, Make, and Go 1.26.4 or newer.

```sh
git clone https://github.com/simjay/plous.git
cd plous
make build
./bin/plous
```

This builds development `main`. Website tools are separate and are not needed to build Plous.

## Install on your PATH

Homebrew handles this automatically. For a source build, copy the executable to a directory on PATH:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 bin/plous "$HOME/.local/bin/plous"
```

Add `~/.local/bin` to PATH if needed, or launch `~/.local/bin/plous` directly.

## Update

Close Plous and [back up its data](../reference/data-and-privacy.md#back-up-or-recover-a-profile). Early development builds can change data formats.

For Homebrew:

```sh
brew update
brew upgrade plous
plous --version
```

For a source installation, update the checkout and run `make build`. Replace any executable you copied elsewhere. Rebuilding the checkout does not update that separate copy.

## Distribution archives

[GitHub releases](https://github.com/simjay/plous/releases) include macOS and Linux archives for amd64 and arm64. Each contains the executable, license, and Markdown guides.

Maintainers build them with `make dist`.

## Uninstall

```sh
brew uninstall plous
```

For a manual installation, remove the executable you installed.

Uninstalling keeps data in `~/.plous/` or the chosen `--data-dir`. That includes credentials and conversations. See [Data and privacy](../reference/data-and-privacy.md) before deleting a profile.

# Installation and updates

[Get started](README.md) · [Choose your workflow](workflows.md)

## Requirements

Use macOS or Linux and an interactive terminal at least **80 columns by 16 rows**. Plous is an early development build; native Windows is not supported.

Plous embeds the Go libraries for Kubernetes access and Helm. These features do not require `kubectl` or `helm` executables. A kubeconfig that uses an external authentication helper still requires that helper.

## Install with Homebrew

With Homebrew installed:

```sh
brew install simjay/tap/plous
plous --version
plous
```

Homebrew makes `plous` available on your PATH. It installs a compatible bottle when available and otherwise builds from source with its declared build dependencies. See the [tap](https://github.com/simjay/homebrew-tap) for platform coverage.

## Published releases and development docs

Homebrew installs the published release. Standalone `plous setup` and `plous mcp serve` are included starting with v0.0.3. On the website, select documentation matching `plous --version`. Development documentation follows `main` and can describe features ahead of the latest release.

## Build from source

Source builds require Git, Make, and Go **1.26.4 or newer**.

```sh
git clone https://github.com/simjay/plous.git
cd plous
make build
```

The executable is `bin/plous`. Run `./bin/plous` from the checkout or install it below. The website's Node and Rust tooling is separate from building Plous.

## Install on your PATH

Homebrew handles this automatically. For a source build, to make it available outside the checkout, copy it into a directory on your shell's `PATH`. For example:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 bin/plous "$HOME/.local/bin/plous"
```

If that directory is not on `PATH`, run `~/.local/bin/plous` or add the directory using your shell's configuration.

## Update

Close Plous and [back up its data](../reference/data-and-privacy.md#back-up-or-recover-a-profile) before trying a newer development build. Data formats can change during early development.

For a Homebrew installation:

```sh
brew update
brew upgrade plous
plous --version
```

For a source installation, update your checkout, run `make build`, and replace any copy you installed elsewhere. Check `--version` on the executable you actually launch. Rebuilding alone does not update a separate installed copy.

## Distribution archives

Maintainers can run `make dist` to build macOS and Linux archives for amd64 and arm64. Each contains the executable, license, and Markdown guides. Published archives are available from [Plous releases](https://github.com/simjay/plous/releases).

## Uninstall

For Homebrew:

```sh
brew uninstall plous
```

For a manual installation, remove the executable you installed. Workspace data remains in `~/.plous/`, or the directory supplied with `--data-dir`. Keep it if you plan to return; removing the executable does not remove credentials or conversations. See [Data and privacy](../reference/data-and-privacy.md) before deleting or moving a profile.

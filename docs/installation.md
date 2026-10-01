# Install Plous

[Documentation](README.md) · [First workspace](getting-started.md)

Plous is an early development build. Build it from source on macOS or Linux. Native Windows is not supported.

## Requirements

| Requirement | Used for |
| --- | --- |
| Go 1.26.4 or newer, Git, Make | Building the executable. |
| Interactive terminal, at least 80 columns × 16 rows | Running the interface. |
| Working kubeconfig and cluster access | Kubernetes features. Optional for workspace setup. |
| OpenAI or Anthropic API key | AI features. Optional for workspace and tool views. |

Kubernetes browsing uses the API directly; kubectl is not required. Install any credential helper referenced by your kubeconfig. Node.js is only needed to work on the documentation website.

## Build from source

```sh
git clone https://github.com/simjay/plous.git
cd plous
make build
./bin/plous --version
./bin/plous
```

The executable is `bin/plous`. Follow [First workspace](getting-started.md#create-your-first-workspace) after launching.

To make it available outside the checkout, copy it into a directory on your shell's `PATH`. For example:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 bin/plous "$HOME/.local/bin/plous"
```

If that directory is not on `PATH`, run `~/.local/bin/plous` or add the directory using your shell's configuration.

## Update

Close Plous and [back up its data](data-and-privacy.md#back-up-or-recover-a-profile) before trying a newer development build. Data formats can change during early development.

Update your source checkout, run `make build`, and replace any copy you installed elsewhere. Check `--version` on the executable you actually launch. Rebuilding alone does not update a separate installed copy.

## Distribution archives

Maintainers can run `make dist` to build macOS and Linux archives for amd64 and arm64. Each contains the executable, license, and Markdown guides. This does not imply that a release has been published. There is no Homebrew or other package-manager installation documented for this build.

## Uninstall

Remove the executable you installed. Workspace data remains in `~/.plous/`, or the directory supplied with `--data-dir`. Keep it if you plan to return; removing the executable does not remove credentials or conversations. See [Data and privacy](data-and-privacy.md) before deleting or moving a profile.

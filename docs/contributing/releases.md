# Releasing Plous

[Contributing](README.md) · [Installation](../getting-started/installation.md)

`cmd/plous/VERSION` is the application version source. A matching `vMAJOR.MINOR.PATCH` tag starts the release workflow.

## One-time setup

The Plous repository needs `HOMEBREW_TAP_TOKEN`, restricted to `simjay/homebrew-tap` with Contents and Pull requests read/write permissions.

Push the tap's test and publication workflows first. Repository rules must allow the workflow to publish its tested formula and bottle metadata to main. The tap uses its own `GITHUB_TOKEN` for bottles.

Configure [GitHub Pages](website.md#github-pages-setup) for documentation publication.

## Release a version

1. Set `cmd/plous/VERSION` to the authorized version, without `v`.
2. Run `make check` and `make dist`.
3. Commit the release on main. Inspect outgoing commits and the tree for local internals. Only root `.github/` is a permitted dot-directory.
4. Push main and check CI.
5. Create and push the matching tag.

For a version file containing `0.0.3`:

```sh
git tag -a v0.0.3 -m "Release v0.0.3"
git push origin v0.0.3
```

The workflow runs all CI jobs and builds tagged documentation. It checks the version and checksums, then publishes the verified archives and docs snapshot. It does not rebuild the distribution artifact during upload.

Only `vMAJOR.MINOR.PATCH` tags are supported. Prerelease suffixes are not supported.

After publication, the workflow opens a Homebrew formula PR and queues Pages. The tap tests bottles on Apple Silicon macOS, Linux ARM64, and Linux x86-64. Intel Mac binaries are available as release archives.

The formula uses the tagged source archive and its SHA-256. Its Go dependency follows the source `go.mod`. It does not override the embedded version.

The tap publishes only after all checks pass for the current PR commit. Do not merge the formula PR manually before bottle publication. Verify the formula and bottle assets afterward.

## Retry a failed update

| Failure | Recovery |
| --- | --- |
| Before GitHub release publication | Fix the cause and rerun failed jobs. Draft assets can be replaced. |
| Homebrew update after publication | Dispatch Update Homebrew with the published tag |
| Tap tests | Fix the formula or test problem and rerun checks |
| Bottle publication | Dispatch brew pr-pull with the PR number and expected head SHA |
| Pages | Rerun Documentation website on main |

Published application releases are not overwritten. See [website publication](website.md#publication-and-recovery) for docs archives and [documentation corrections](website.md#correct-released-docs) for writing or rendering fixes that need no binary release.

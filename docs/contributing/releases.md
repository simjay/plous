# Releasing Plous

[Contributing](README.md) · [Getting started](../getting-started/README.md)

`cmd/plous/VERSION` is the application version source. Releases use matching `vMAJOR.MINOR.PATCH` tags. The workflow publishes macOS and Linux archives for amd64 and arm64, then opens a Homebrew update in [simjay/homebrew-tap](https://github.com/simjay/homebrew-tap).

## One-time setup

Push the tap's test and publication workflows before releasing Plous. The Plous repository needs an Actions secret named `HOMEBREW_TAP_TOKEN`: a fine-grained token restricted to `simjay/homebrew-tap`, with Contents and Pull requests read/write permissions. Renew it before expiration. The tap uses its own `GITHUB_TOKEN` for bottle publication.

Both repositories must allow their declared GitHub Actions permissions. Tap publication pushes the tested formula and bottle metadata to `main`; repository rules must permit that workflow. No manual PR approval or GitHub auto-merge setting is required by this pipeline.

The formula is published in the tap. User installation and update instructions are in [Installation](../getting-started/installation.md#install-with-homebrew). A successful local check does not verify a new hosted publication.

## Release a version

1. Choose the version and update `cmd/plous/VERSION`, without a leading `v`.
2. Run `make check` and `make dist`.
3. Commit the intended release files on `main`. Inspect the outgoing commits and tree for local internals; root `.github/` is the only permitted dot-directory.
4. Push `main` and confirm CI succeeds.
5. Create and push the matching tag. For example, if the version file contains `0.0.3`:

   ```sh
   git tag -a v0.0.3 -m "Release v0.0.3"
   git push origin v0.0.3
   ```

The Release workflow runs CI, verifies the tag and archive checksums, uploads the verified distribution artifact and tagged documentation snapshot, and publishes the GitHub release with generated notes. Publication is automatic. Tags with prerelease suffixes are not supported.

Manually published GitHub releases also trigger the Homebrew update. Use the tag workflow above to run the full application checks and publish verified download archives as part of the release.

The Homebrew job downloads the tagged source archive, checks its embedded version, computes its SHA-256, and opens a formula PR. The build dependency follows the Go major/minor version declared in the archive's `go.mod`. It does not use `go install module@version` or override the embedded application version.

The tap builds and tests bottles on macOS 15 for Apple Silicon, and Linux for ARM64 and x86-64. Intel Mac release archives are published separately. Tests check the version, CLI behavior, and startup and exit in an isolated terminal. After every job succeeds, publication verifies the current PR commit, uploads the bottles, and pushes the formula with bottle metadata. Only owner-created release branches that change `Formula/plous.rb` qualify for automatic publication. The previous formula stays available if a build fails.

After the tap publishes, verify the formula and bottle metadata. Use the [Homebrew installation and update instructions](../getting-started/installation.md#install-with-homebrew) to check the published package. The tap README documents platform coverage; GitHub release archives remain available for direct download.

## Retry a failed update

- **Before GitHub publication:** fix the failure and rerun the failed Release jobs. Existing draft assets can be replaced. Published releases are never overwritten.
- **After GitHub publication:** run **Update Homebrew** from the Plous Actions tab and enter the published tag. This retries the tap update without publishing the release again. It preserves existing bottles and skips versions already published or superseded in the tap.
- **Tap tests:** fix the formula or test problem, then rerun the tap checks. Publication waits for success on the current PR commit.
- **Bottle publication:** run **brew pr-pull** from the tap Actions tab with the PR number and its expected head SHA. The workflow repeats its release and test checks before publishing.

Do not merge a release formula PR manually before bottle publication; the publishing workflow handles both together.

The release documentation build must pass before publication. A successful release also queues the main Pages workflow to publish the new snapshot alongside Development and older versions. See [Website publication](website.md#publication-and-recovery) for URLs, retained snapshots, and retries.

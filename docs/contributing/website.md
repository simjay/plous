# Documentation website

[Contributing](README.md) · [Documentation](../README.md)

Rune renders the website from Markdown in `docs/`. It exports static HTML, then starts the same view through WebAssembly and WebGPU where available. GitHub Pages serves the files without an application server.

## Preview and verify

You need Node.js 22.23.4+, npm, Rust 1.99.1, its Wasm target, and wasm-bindgen CLI 0.2.129. These are website tools, not Plous runtime requirements.

```sh
npm ci --prefix website
rustup toolchain install 1.99.1 \
  --profile minimal --target wasm32-unknown-unknown
cargo +1.99.1 install wasm-bindgen-cli --version 0.2.129 --locked
make website-check
make website-build
npm --prefix website run preview
```

Open `http://127.0.0.1:8046/plous/`. Rebuild after source edits. Stop the preview with Ctrl+C.

| Check | What it verifies |
| --- | --- |
| `make docs-check` | Source titles, links, anchors, navigation, prose punctuation |
| `make website-check` | Content mapping, examples, headings, redirects, publication rules, Rune compilation |
| `make website-build` | Full export, generated links, assets, search, syntax colors |

Review Quickstart, MCP setup, and a long reference at desktop and mobile widths. Check search, copy, theme, and code colors. Use `?static=1` for static HTML. The document's `data-runtime` reports the live Rune handoff.

Passing structural checks does not establish readability. Read each changed guide as a user following its steps.

## Layout and browser integration

| Source | Owns |
| --- | --- |
| `website/rune/src/lib.rs` | Layout, typography, colors, icons, Markdown rendering |
| `scripts/navigation.mjs` | Flat sidebar groups |
| `scripts/prepare.mjs` | Markdown, links, headings, compiler input |
| `scripts/build.mjs` | Native HTML export, Wasm, fonts, assets, search |
| `scripts/template.html` | Metadata and browser dialogs |
| `scripts/client.mjs` | Static controls, dialogs, themes, live startup |
| `scripts/publish.py` | Release archives, corrections, version assembly |

Paths in this table are under `website/`. Generated input, outputs, and dependencies are ignored by Git. The build includes Rune and Geist license files from the pinned dependency checkout.

Do not use `MarkdownStyle::shell_code` for syntax-colored examples. Rune treats that option as plain shell text. Use the normal code-block theme and syntax palette.

## Content and links

Keep one source under `docs/`. Relative guide links become website routes. For example, `docs/getting-started/README.md` becomes `/plous/docs/getting-started/` at the default alias.

Write one task per guide. Put exact commands and limits in references. Keep implementation details in contributor pages.

Use short sentences and concrete names. Avoid semicolons and em dashes in prose. Put warnings beside the action they affect. Code and quoted output keep their literal punctuation.

Label code fences by language. Format long argument arrays across lines. Keep plain-text output separate from commands. Verify colors in light and dark themes, static HTML, and live Rune.

Keep the approved flat navigation groups and separate Argo CD and Flux guides. Preserve public routes and heading links when moving content. Development source links use main. Release corrections link to their exact source commit.

## GitHub Pages setup

Enable **Settings → Pages → Source → GitHub Actions**. Allow the `github-pages` environment to deploy from main. Only the deployment job receives Pages write permissions.

The published site is [simjay.github.io/plous](https://simjay.github.io/plous/).

| Actions variable | Default |
| --- | --- |
| `PLOUS_SITE_URL` | `https://simjay.github.io` |
| `PLOUS_SITE_BASE` | `/plous` |

For a custom domain, configure Pages and DNS, then set the origin and base. Check domain-root builds locally:

```sh
PLOUS_SITE_URL=https://docs.example.com \
  PLOUS_SITE_BASE=/ make website-build
```

Pages receives `website/published/`. Build outputs, private data, and local internals stay out of source control.

## Publication and recovery

| Event | Result |
| --- | --- |
| Pull request | Validate and build a downloadable preview. No deployment. |
| Main push | Update Development and preserve published versions |
| Application release | Save the tagged docs archive and queue Pages |
| Documentation correction | Add a commit-identified correction and republish that version |

Default routes open the latest released version. `/plous/dev/` follows main. `/plous/v0.0.3/` serves docs for that application version. Search stays within the selected version.

Release archives and SHA-256 files live as GitHub release assets. They do not depend on expiring Actions artifacts. Each Pages build verifies and assembles all published versions.

### Correct released docs

Run **Correct released documentation** on main and enter the released version. For example:

```sh
gh workflow run docs-correction.yml --ref main \
  -f release_version=v0.0.3
```

The workflow rejects a draft release, a version mismatch, or application-source changes relative to the tag. It builds the current documentation and records its exact commit.

Corrections add new archives without replacing the original snapshot or application assets. Pages uses the newest complete correction. The application version and release tag do not change, and Homebrew is not triggered.

If the application has advanced beyond the target release, prepare the correction from matching application source. Do not relabel newer behavior as older-version docs.

### Recover a failed publication

Rerun failed jobs or dispatch Documentation website on main. An invalid archive or failed build leaves the current site available.

A correction retry reuses existing assets for that commit and finishes a partial upload. It does not overwrite published archives. The next successful Pages build includes it.

Confirm the workflow result and public URL before reporting deployment success.

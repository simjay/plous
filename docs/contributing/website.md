# Documentation website

[Contributing](README.md) · [Documentation](../README.md)

VitePress builds the website from Markdown in `docs/` and serves it as static files on GitHub Pages. The contributor guides stay in the repository.

## Preview and verify

You need Node.js 22.23.4 or newer, npm, and Python 3.11 or newer. Use Python 3.13 to assemble release archives locally.

```sh
npm ci --prefix website
make website-check
make website-build
npm --prefix website run preview
```

Open `http://127.0.0.1:8046/plous/`. Rebuild after source edits. Stop the preview with Ctrl+C.

| Check | What it verifies |
| --- | --- |
| `make docs-check` | Source titles, links, anchors, navigation, prose punctuation |
| `make website-check` | Guide mapping, examples, bookmarks, redirects, version publication |
| `make website-build` | VitePress build, generated links, images, search, and code examples |

Follow the homepage links at desktop and mobile widths. Check sidebar selection and Back navigation, version switching, search, theme, code copying, and keyboard navigation in the interactive examples.

Passing structural checks does not establish readability. Read each changed guide as a user following its steps.

## Layout and browser integration

| Source | Owns |
| --- | --- |
| `vitepress.config.mjs` | VitePress defaults, site navigation, search, and metadata |
| `home.md` | Homepage and entry links |
| `scripts/navigation.mjs` | Flat sidebar groups |
| `scripts/prepare.mjs` | Guide selection, Markdown links, and compatibility bookmarks |
| `scripts/build.mjs` | VitePress build, assets, redirects, and version manifest |
| `theme/` | Brand styles, version picker, screenshot, and guide tabs |
| `scripts/publish.py` | Release archives, corrections, version assembly |

Paths in this table are under `website/`. The build stages Markdown and the VitePress configuration in a temporary directory. The package lockfile pins the toolchain. VitePress 1.6 uses the patched Vite 6 build dependency selected in `package.json`.

## Content and links

Keep one source under `docs/`. Relative guide links become website routes. For example, `docs/getting-started/README.md` becomes `/plous/docs/getting-started/` at the default alias.

Write one task per guide. Put exact commands and limits in references. Keep implementation details in contributor pages.

Use short sentences and concrete names. Avoid semicolons and em dashes in prose. Put warnings beside the action they affect. Code and quoted output keep their literal punctuation.

Label code fences by language. Format long argument arrays across lines. Keep plain-text output separate from commands. Verify syntax colors in light and dark themes.

Place examples beside the steps they explain. The `plous-example` markers in Quickstart render the workspace screenshot and the interactive panel controls.

Group the user guides under Terminal UI and Coding agents. Keep assistant setup with the terminal UI. Put plous-debug setup first under Coding agents and keep manual MCP configuration there for users who need it. Preserve the shared installation, integration, and reference material and keep Argo CD and Flux guides separate.

Preserve public routes and heading links when moving content. Development source links use main. Release corrections link to their exact source commit.

The build excludes `docs/contributing/` from public guides and search. Old contributor URLs redirect to Markdown in the repository. Earlier release archives retain their original pages.

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

Run **Correct released documentation** from a revision whose application source matches the release. This command corrects v0.0.3 from a matching main revision.

```sh
gh workflow run docs-correction.yml --ref main \
  -f release_version=v0.0.3
```

The workflow rejects a draft release, a version mismatch, or application-source changes relative to the tag. It builds the current documentation and records its exact commit.

Corrections add new archives without replacing the original snapshot or application assets. Pages uses the newest complete correction. The application version and release tag do not change, and Homebrew is not triggered.

If main has advanced beyond the target release, start from that version's current documentation source and apply the relevant website and guide changes. Push a correction branch and pass it to `--ref`. Keep the release branch's application source and version-specific instructions intact.

### Recover a failed publication

Rerun failed jobs or dispatch Documentation website on main. An invalid archive or failed build leaves the current site available.

A correction retry reuses existing assets for that commit and finishes a partial upload. It does not overwrite published archives. The next successful Pages build includes it.

Confirm the workflow result and public URL before reporting deployment success.

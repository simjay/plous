# Documentation website

[Contributing](README.md) · [Documentation](../README.md)

The Plous website is a Rune application under `website/rune/`. Rune owns the homepage, documentation layout, navigation, typography, icons, tables, and code presentation. Markdown in `docs/` remains the content source for the site, GitHub readers, and distribution archives.

The same Rune view exports responsive semantic HTML and runs in the browser through WebAssembly and WebGPU. Static HTML is the first paint and remains usable without the live renderer. GitHub Pages serves the generated files; no server-side application is required.

## Preview and verify

Website development requires Node.js **22.23.4 or newer**, npm, Rust **1.99.1**, the `wasm32-unknown-unknown` target, and wasm-bindgen CLI **0.2.129**. These tools are separate from building or running Plous.

```sh
npm ci --prefix website
rustup toolchain install 1.99.1 --profile minimal --target wasm32-unknown-unknown
cargo +1.99.1 install wasm-bindgen-cli --version 0.2.129 --locked
make website-check
make website-build
npm --prefix website run preview
```

Open `http://127.0.0.1:8046/plous/`. The preview serves the last static build. Rebuild and reload after editing source files. `make website-dev` builds and then serves the result. Stop the preview with Ctrl+C.

The build pins Rune to one upstream revision and uses its licensed Geist and Geist Mono font files for both static and live rendering. Pagefind supplies local search. Rune renders every published page.

`make docs-check` validates source links, headings, and index reachability. `make website-check` tests content coverage, literal examples, heading offsets, entry paths, and compatibility redirects, then checks the Rust view. `make website-build` compiles Rune, exports every page, builds search, and checks generated links, anchors, image alternatives, source coverage, and runtime assets.

Check the homepage, a long guide, and a table-heavy reference at desktop and mobile widths. Verify navigation, search, code copying, themes, and heading links. Append `?static=1` to inspect the static export. The browser document's `data-runtime` attribute reports the live handoff. Local verification does not establish hosted publication or full screen-reader compatibility.

## Layout and browser integration

`website/rune/src/lib.rs` owns the Rune view, responsive columns, type scale, vector icons, and theme colors. `website/scripts/navigation.mjs` owns the flat navigation groups. `prepare.mjs` reads every guide, preserves code examples, and prepares headings and compiler input.

`build.mjs` invokes Rune's native HTML exporter and Wasm build, then packages assets and search in `website/dist/`. `template.html` supplies metadata and native search/navigation dialogs. `client.mjs` connects those dialogs, static controls, theme persistence, aliases, and the live renderer. Mobile navigation reuses the exported Rune sidebar. The static button adapter and inline-text CSS accommodate the pinned Rune export format.

Generated compiler input, build outputs, fonts copied from the dependency checkout, and Wasm files remain ignored by Git. Dependency sources, license files, and font files are packaged from the pinned Cargo checkout during builds.

## Content and links

`website/scripts/content.mjs` maps Markdown into website routes. For example, `docs/getting-started/README.md` becomes `/plous/docs/getting-started/`. Existing GitHub file paths stay intact. Relative documentation links become website links, assets are copied, and references to implementation files lead to GitHub. Development pages link to editable Markdown on main. Release pages link to the exact tagged source.

Keep one page per workflow. Put installation and examples in guides, controls and settings in reference pages, and contributor material here. The landing page introduces implemented features and clearly identifies the development build. Do not add release claims or installation channels before verifying them.

The homepage and documentation index offer three workflows: Terminal UI, MCP server, and Agent skills. The user sidebar has Get started, Terminal UI, MCP server, Agent skills, Tutorials, Integrations, and Reference. Each skill has one flat document under docs/agent-skills; plous-debug has no child directory. Argo CD and Flux have separate integration guides. Developers has its own navigation, with links to architecture, AI implementation, testing and benchmarks, releases, and this guide. Both areas use the same site, search, and Markdown source.

Keep Get started and the current section expanded by default. Keep other sections collapsed and avoid nested directory trees. Automatic sidebar-order pagination is disabled; finish guides with relevant next steps or reference links in the Markdown so GitHub readers receive the same guidance. Preserve existing URLs when changing navigation labels. Moved guides use static redirect pages with a canonical destination; the former combined GitOps URL routes controller-specific heading links to the corresponding guide.

## GitHub Pages setup

The repository owner must enable **Settings → Pages → Build and deployment → Source → GitHub Actions**. Allow the `github-pages` environment to deploy from `main` and permit the workflow's declared Pages permissions. No personal access token is required.

The default build targets `https://simjay.github.io/plous/`. That is the intended location, not a claim that it is already published. Keep the README's working Markdown links until hosted publication succeeds.

For a custom domain, configure the domain and its DNS in GitHub Pages, then set these repository Actions variables:

| Variable | Default | Custom domain example |
| --- | --- | --- |
| `PLOUS_SITE_URL` | `https://simjay.github.io` | `https://docs.example.com` |
| `PLOUS_SITE_BASE` | `/plous` | `/` |

Use the same variables locally to verify either deployment layout. For example:

```sh
PLOUS_SITE_URL=https://docs.example.com PLOUS_SITE_BASE=/ make website-build
```

The version assembler packages `website/published/` for Pages. Build outputs, dependency directories, private data, and local dot-directories stay out of source control and deployment artifacts.

## Publication and recovery

The **Documentation website** workflow validates and builds pull requests, pushes to `main`, and manual runs. Main publishes Development at `/plous/dev/`. Pull requests retain a downloadable site artifact for seven days and cannot deploy.

The Release workflow builds an additional snapshot from its exact tag. Before publishing the release it verifies and attaches `documentation-vMAJOR.MINOR.PATCH.tar.gz` and its SHA-256 file alongside application archives. Those release assets preserve the website independently of expiring Actions artifacts. Versioned website publication starts with v0.0.3; earlier releases retain their Markdown on GitHub.

After a successful release, the workflow queues Documentation website on main. That workflow builds current Development docs, downloads and checks every published documentation snapshot, and assembles one complete Pages artifact. Published snapshots live at `/plous/vMAJOR.MINOR.PATCH/`. Existing unversioned routes redirect to the latest release while preserving query strings and heading fragments. Before the first snapshot exists, they lead to Development. The version selector keeps the current guide when it exists in the selected version; search stays within that version.

Documentation-only pushes do not change `cmd/plous/VERSION` or publish an application release. They update Development while preserving released snapshots. Deployment runs are serialized, and only the deployment job receives Pages write permissions. Release-triggered deployment runs on main so the Pages environment can remain restricted to main.

A failed build or snapshot integrity check leaves the existing site available. Rerun failed jobs or dispatch Documentation website on main to rebuild. A failed documentation snapshot build blocks its application release. A failed Pages deployment after release can be retried without republishing the release or Homebrew package. Correct development content on main; published version snapshots stay tied to their release source.

Verify the deployment result and URL before reporting hosted publication. Keep the Markdown index and existing GitHub paths for source readers and offline archives.

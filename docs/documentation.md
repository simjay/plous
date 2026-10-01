# Maintain these docs

[Documentation](README.md) · [Contributing](contributing.md)

Edit Markdown in `docs/`. The same files are readable on GitHub, included in distribution archives, and built into the website. Do not edit generated website pages.

## Preview locally

Install Node.js 22.12 or newer and npm. From the repository root:

```sh
make docs-install
make docs-dev
```

Open the local URL printed by Astro, including `/plous/`. Changes to the Markdown reload the preview. Restart the preview after changing website configuration or navigation.

For the production build, including search:

```sh
make docs-check
npm --prefix docs-site run preview
```

Search is generated during the production build. It is not available in the development preview.

## Add or change a page

1. Add a Markdown file in `docs/`, starting with one `#` title.
2. Add it to `docs-site/pages.mjs`, which controls sidebar order and labels.
3. Link it from the documentation index or a relevant guide.
4. Run `make docs-check`.

Use relative Markdown links, including `.md` and section anchors. The build converts guide links to website routes and repository-file links to GitHub. Keep useful existing filenames and headings so incoming links continue to work. The build rejects missing pages, missing local targets, and links into local internals.

Use Markdown rather than embedded HTML. Tables work on GitHub and the site. Keep paragraphs short, spell controls exactly as they appear, and document implemented behavior. Avoid filler, em dashes, and duplicate specifications.

## Organize by product, then tool

**Start here** covers installation and a first workspace. **Using Plous** covers shared behavior: workspaces, navigation, Activity, and AI. **Integrations** holds tool-specific setup and workflows. **Reference** and **Contribute** apply across the product.

Keep first-run instructions independent of any tool. Add new integration guides beside Kubernetes as their capabilities become available. Until then, document their saved connection fields and limits in the integration overview. Do not add empty feature pages or present planned operations as available.

## What the build does

`docs-site/` contains Astro Starlight configuration, styling, and build scripts. Preparation reads `docs/`, removes the repeated title and breadcrumb from the website body, adds page metadata, and writes an ignored staging directory. Source guides remain unchanged.

The development banner reads `cmd/plous/VERSION`. Edit links lead to the original Markdown file. Search is built locally with the site; it does not require a hosted search service. Dependency versions are locked in `docs-site/package-lock.json`.

Geist and Geist Mono are bundled with the site. Visitors do not need to fetch fonts from an external service.

Checks cover navigation completeness, source links, rendered page links and anchors, local assets, the search index, and exclusion of local internals. They do not establish that external websites or hosted deployments are available.

## Publish with GitHub Pages

The workflow in [docs.yml](../.github/workflows/docs.yml) builds and checks pull requests. Pushes to `main` also deploy the checked output through GitHub Pages. Pull requests do not deploy.

For the repository owner, the one-time setup is **Settings → Pages → Build and deployment → Source → GitHub Actions**. Push the website files, then inspect the **Documentation** workflow. A successful deployment serves the site at `https://simjay.github.io/plous/`.

Local builds do not publish anything. Only `docs-site/dist/` is uploaded. No profile data or local decision records belong in that output. A failed check prevents deployment.

The site currently documents one development build. Add versioned documentation when released versions need different instructions, rather than keeping duplicate guides now.

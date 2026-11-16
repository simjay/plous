import { cp, mkdir, mkdtemp, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { build } from 'vitepress';
import { docs, href, repository, website, base, siteURL, sourceRef, versionLabel } from './content.mjs';
import { legacyRoutes } from './legacy-routes.mjs';
import { prepareContent } from './prepare.mjs';

const dist = path.join(website, 'dist');
const escape = (value) => value.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
const json = (value) => JSON.stringify(value).replaceAll('<', '\\u003c');

async function output(root, filename, contents) {
  const target = path.join(root, filename);
  await mkdir(path.dirname(target), {recursive: true});
  await writeFile(target, contents);
}

function redirect(target, anchors = {}) {
  const url = escape(target);
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<meta name="robots" content="noindex"><title>Documentation moved · Plous</title>
<link rel="canonical" href="${url}">
<script>
const anchors = ${json(anchors)};
const name = location.hash.slice(1);
const target = new URL(Object.hasOwn(anchors, name) ? anchors[name] : ${json(target)}, location.href);
if (location.search) target.search = location.search;
if (!Object.hasOwn(anchors, name) && !target.hash) target.hash = location.hash;
location.replace(target.href);
</script>
<noscript><meta http-equiv="refresh" content="0;url=${url}"></noscript>
</head><body><h1>Documentation moved</h1><a href="${url}">Open the current page</a></body></html>
`;
}

const { pages, retired } = await prepareContent();
const root = await mkdtemp(path.join(website, '.vitepress-build-'));
const rendered = path.join(root, 'dist');
try {
  await output(root, '.vitepress/config.mjs', "export { default } from '../../vitepress.config.mjs';\n");
  await cp(path.join(website, 'theme'), path.join(root, '.vitepress/theme'), {recursive: true});
  await cp(path.join(website, 'home.md'), path.join(root, 'index.md'));
  const assets = path.join(root, 'public/assets');
  await mkdir(assets, {recursive: true});
  for (const name of ['lockup.svg', 'lockup-dark.svg', 'icon.svg']) {
    await cp(path.join(repository, 'assets/logo', name), path.join(assets, name));
  }
  await cp(path.join(docs, 'assets/plous-overview.png'), path.join(assets, 'plous-overview.png'));
  for (const page of pages) {
    await output(root, page.route + 'index.md', `---\nsource: ${json(page.source)}\n---\n\n${page.body}`);
  }
  await build(root, {outDir: rendered});
  for (const route of legacyRoutes) {
    const anchors = Object.fromEntries(Object.entries(route.anchors).map(([key, value]) => [key, href(value)]));
    await output(rendered, `docs/${route.slug}/index.html`, redirect(href(route.destination), anchors));
  }
  await output(rendered, 'rune/index.html', redirect(href('docs/')));
  await output(rendered, 'rune/guides/investigate-restarting-pod/index.html', redirect(href('docs/guides/investigate-restarting-pod/')));
  for (const page of retired) {
    await output(rendered, page.route + 'index.html', redirect(page.destination));
  }

  const routes = ['', ...pages.map(page => page.route)];
  const sitemap = routes.map(route => `<url><loc>${escape(new URL(href(route), siteURL).href)}</loc></url>`).join('');
  await output(rendered, 'sitemap.xml', `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${sitemap}</urlset>\n`);
  await output(rendered, 'site.json', JSON.stringify({
    version: versionLabel,
    revision: process.env.PLOUS_DOCS_REVISION ?? null,
    source: sourceRef,
    base,
    renderer: 'vitepress',
    routes,
  }, null, 2) + '\n');
  await rm(dist, {recursive: true, force: true});
  await rename(rendered, dist);
} finally {
  await rm(root, {recursive: true, force: true});
}

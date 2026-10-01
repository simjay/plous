import { readFile, readdir, stat } from 'node:fs/promises';
import path from 'node:path';
import { load } from 'cheerio';
import { root } from './prepare.mjs';
import { base, pages, slug } from '../pages.mjs';

const directory = path.join(root, 'docs-site/dist');
const origin = 'https://simjay.github.io';
const failures = [];
const documents = new Map();
async function walk(dir) {
  const files = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const filename = path.join(dir, entry.name);
    if (entry.name.startsWith('.')) failures.push(`Private path in build: ${filename}`);
    files.push(...entry.isDirectory() ? await walk(filename) : [filename]);
  }
  return files;
}
for (const filename of await walk(directory)) {
  if (filename.endsWith('.html')) documents.set(filename, load(await readFile(filename, 'utf8')));
}
for (const [name] of pages) {
  const filename = path.join(directory, slug(name), 'index.html');
  if (!documents.has(filename)) failures.push(`Missing built page: ${name}`);
}
for (const [filename, $] of documents) {
  const pathname = base + path.relative(directory, filename).replace(/index\.html$/, '');
  if ($('h1').length !== 1) failures.push(`${pathname}: expected one page title`);
  for (const element of $('[href], [src]').toArray()) {
    // Starlight's fallback is 404.html, but its canonical URL uses /404/.
    if (path.basename(filename) === '404.html' && $(element).attr('rel') === 'canonical') continue;
    const href = $(element).attr('href') ?? $(element).attr('src');
    const url = new URL(href, origin + pathname);
    if (url.origin !== origin) continue;
    if (!url.pathname.startsWith(base)) {
      failures.push(`${pathname}: link outside Pages base: ${href}`);
      continue;
    }
    let target = path.join(directory, decodeURIComponent(url.pathname.slice(base.length)));
    const info = await stat(target).catch(() => null);
    if (info?.isDirectory()) target = path.join(target, 'index.html');
    if (!await stat(target).catch(() => null)) {
      failures.push(`${pathname}: missing target: ${href}`);
      continue;
    }
    if (url.hash && documents.has(target)) {
      const id = decodeURIComponent(url.hash.slice(1));
      if (!documents.get(target)('[id]').toArray().some(node => node.attribs.id === id)) {
        failures.push(`${pathname}: missing anchor: ${href}`);
      }
    }
  }
}
if (!await stat(path.join(directory, 'pagefind/pagefind.js')).catch(() => null)) failures.push('Search index missing.');
if (failures.length) throw new Error(failures.join('\n'));
console.log(`Checked ${documents.size} built pages, local links, anchors, assets, and search.`);

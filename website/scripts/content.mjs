import { readdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const website = fileURLToPath(new URL('../', import.meta.url));
export const repository = path.dirname(website);
export const docs = path.join(repository, 'docs');
export const base = `/${(process.env.PLOUS_SITE_BASE ?? '/plous').split('/').filter(Boolean).join('/')}`.replace(/\/$/, '') + '/';
export const sourceRef = process.env.PLOUS_DOCS_SOURCE_REF || process.env.PLOUS_DOCS_VERSION || 'main';
export const versionLabel = process.env.PLOUS_DOCS_VERSION || 'Development';
export const siteRoot = ('/' + (process.env.PLOUS_SITE_ROOT ?? process.env.PLOUS_SITE_BASE ?? '/plous').split('/').filter(Boolean).join('/')).replace(/\/$/, '') + '/';
export const href = (route = '') => base + route.replace(/^\/+/, '');

export function routeFor(relative) {
  const route = relative.replace(/\.md$/, '').replace(/(^|\/)README$/, '$1').replace(/\/$/, '');
  return `docs/${route ? route + '/' : ''}`;
}

function publishedURL(value, source) {
  if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(value)) return value;
  const [, pathname, suffix = ''] = value.match(/^([^?#]*)(.*)$/);
  const target = path.resolve(path.dirname(source), decodeURIComponent(pathname));
  const relative = path.relative(repository, target);
  if (relative.startsWith('..') || relative.split(path.sep).some((part) => part.startsWith('.'))) {
    throw new Error(`Documentation link leaves public repository content: ${value}`);
  }
  if (relative === 'README.md') return href() + suffix;
  if (target.startsWith(docs + path.sep)) {
    const entry = path.relative(docs, target).split(path.sep).join('/');
    if (entry.endsWith('.md')) return href(routeFor(entry)) + suffix;
    if (entry.startsWith('assets/')) return href(entry) + suffix;
  }
  return `https://github.com/simjay/plous/blob/${sourceRef}/${relative.split(path.sep).map(encodeURIComponent).join('/')}${suffix}`;
}

export function rewriteMarkdown(markdown, source) {
  let fence = '';
  return markdown.split('\n').map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = '';
      return line;
    }
    if (fence) return line;
    return line.replace(/(\]\()([^\s)]+)(\))/g, (_, before, url, after) => before + publishedURL(url, source) + after)
      .replace(/((?:href|src)=")([^"]+)(")/g, (_, before, url, after) => before + publishedURL(url, source) + after);
  }).join('\n');
}

export async function markdownFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(entries.filter((entry) => !entry.name.startsWith('.')).map((entry) => {
    const location = path.join(directory, entry.name);
    if (entry.isDirectory()) return markdownFiles(location);
    return entry.isFile() && entry.name.endsWith('.md') ? [location] : [];
  }));
  return files.flat().sort();
}

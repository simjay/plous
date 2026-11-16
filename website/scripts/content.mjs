import { readdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const website = fileURLToPath(new URL('../', import.meta.url));
export const repository = path.dirname(website);
export const docs = path.join(repository, 'docs');
export const base = `/${(process.env.PLOUS_SITE_BASE ?? '/').split('/').filter(Boolean).join('/')}`.replace(/\/$/, '') + '/';
export const sourceRef = process.env.PLOUS_DOCS_SOURCE_REF || process.env.PLOUS_DOCS_VERSION || 'main';
export const versionLabel = process.env.PLOUS_DOCS_VERSION || 'Development';
export const siteRoot = ('/' + (process.env.PLOUS_SITE_ROOT ?? process.env.PLOUS_SITE_BASE ?? '/').split('/').filter(Boolean).join('/')).replace(/\/$/, '') + '/';
export const siteURL = process.env.PLOUS_SITE_URL ?? 'https://plous.ai';
export const href = (route = '') => base + route.replace(/^\/+/, '');
export const isMaintainerGuide = (relative) => relative.startsWith('contributing/');
export const repositoryURL = (relative, action = 'blob') =>
  `https://github.com/simjay/plous/${action}/${sourceRef}/${relative.split('/').map(encodeURIComponent).join('/')}`;

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
  if (relative === 'README.md') return '/' + suffix;
  if (target.startsWith(docs + path.sep)) {
    const entry = path.relative(docs, target).split(path.sep).join('/');
    if (entry.endsWith('.md') && !isMaintainerGuide(entry)) return '/' + routeFor(entry) + suffix;
    if (entry.startsWith('assets/')) return '/' + entry + suffix;
  }
  return repositoryURL(relative.split(path.sep).join('/')) + suffix;
}

const exampleComponents = new Map([
  ['panel-focus', '<GuideTabs example="panel-focus" />'],
  ['workspace-overview', '<WorkspaceScreenshot />'],
  ['mcp-target', '<GuideTabs example="mcp-target" />'],
]);

export function rewriteMarkdown(markdown, source, renderExamples = false) {
  let fence = '';
  return markdown.split('\n').map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = '';
      return line;
    }
    if (fence) return line;
    const example = line.match(/^<!-- plous-example: ([a-z-]+) -->$/);
    if (example && renderExamples) {
      const name = example[1];
      if (!exampleComponents.has(name)) throw new Error(`Unknown example '${name}' in ${source}`);
      return exampleComponents.get(name);
    }
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

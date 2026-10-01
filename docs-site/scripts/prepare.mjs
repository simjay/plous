import { readFile, readdir, mkdir, writeFile, rm, copyFile, stat } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import remarkStringify from 'remark-stringify';
import { base, pages, repository, slug } from '../pages.mjs';

export const root = fileURLToPath(new URL('../../', import.meta.url));
const source = path.join(root, 'docs');
const output = path.join(root, 'docs-site/src/content/docs');
const markdown = unified().use(remarkParse).use(remarkGfm).use(remarkStringify);
const plain = (node) => node.value ?? node.children?.map(plain).join('') ?? '';

export async function rewriteLink(url, filename) {
  if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(url)) return url;
  const [, pathname, suffix = ''] = url.match(/^([^?#]*)(.*)$/);
  const target = path.resolve(source, path.dirname(filename), decodeURIComponent(pathname));
  const relative = path.relative(root, target).split(path.sep).join('/');
  if (relative.startsWith('..') || relative.split('/').some((part, index) => part.startsWith('.') && !(index === 0 && part === '.github'))) {
    throw new Error(`${filename}: link points outside public source: ${url}`);
  }
  const info = await stat(target).catch(() => { throw new Error(`${filename}: missing link: ${url}`); });
  if (path.dirname(target) === source && target.endsWith('.md')) {
    const name = path.basename(target, '.md');
    if (!pages.some(([entry]) => entry === name)) throw new Error(`Page missing from navigation: ${name}`);
    return `${base}${slug(name)}${slug(name) ? '/' : ''}${suffix}`;
  }
  return `${repository}/${info.isDirectory() ? 'tree' : 'blob'}/main/${relative}${suffix}`;
}

export async function prepare() {
  const files = (await readdir(source)).filter(name => name.endsWith('.md')).sort();
  const expected = pages.map(([name]) => `${name}.md`).sort();
  if (JSON.stringify(files) !== JSON.stringify(expected)) {
    throw new Error('Every docs/*.md page must appear exactly once in docs-site/pages.mjs.');
  }
  await mkdir(output, { recursive: true });
  for (const stale of await readdir(output)) {
    if (stale.endsWith('.md') && !expected.map(name => name === 'README.md' ? 'index.md' : name).includes(stale)) {
      await rm(path.join(output, stale));
    }
  }
  const version = (await readFile(path.join(root, 'cmd/plous/VERSION'), 'utf8')).trim();
  for (const filename of files) {
    const tree = markdown.parse(await readFile(path.join(source, filename), 'utf8'));
    const heading = tree.children.shift();
    if (heading?.type !== 'heading' || heading.depth !== 1) throw new Error(`${filename}: start with a title.`);
    if (tree.children[0]?.type === 'paragraph' && tree.children[0].children[0]?.url === 'README.md') {
      tree.children.shift();
    }
    const description = plain(tree.children.find(node => node.type === 'paragraph') ?? {}).slice(0, 200);
    async function visit(node) {
      if (node.type === 'html') throw new Error(`${filename}: use Markdown, not embedded HTML.`);
      if (node.url) node.url = await rewriteLink(node.url, filename);
      if (node.children) await Promise.all(node.children.map(visit));
    }
    await visit(tree);
    const metadata = {
      title: plain(heading), description,
      editUrl: `${repository}/edit/main/docs/${filename}`,
      banner: { content: `Development build · ${version} · Not ready for production` },
    };
    const content = `---\n${Object.entries(metadata).map(([key, value]) => `${key}: ${JSON.stringify(value)}`).join('\n')}\n---\n\n${markdown.stringify(tree)}`;
    const destination = path.join(output, filename === 'README.md' ? 'index.md' : filename);
    if (await readFile(destination, 'utf8').catch(() => '') !== content) await writeFile(destination, content);
  }
  const publicDir = path.join(root, 'docs-site/public');
  await mkdir(publicDir, { recursive: true });
  await copyFile(path.join(root, 'assets/logo/icon.svg'), path.join(publicDir, 'favicon.svg'));
  const licenses = path.join(publicDir, 'font-licenses');
  await mkdir(licenses, { recursive: true });
  for (const font of ['geist', 'geist-mono']) {
    await copyFile(path.join(root, `docs-site/node_modules/@fontsource-variable/${font}/LICENSE`), path.join(licenses, `${font}.txt`));
  }
  console.log(`Prepared ${files.length} documentation pages for Plous ${version}.`);
}

import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { base, docs, href, markdownFiles, routeFor, rewriteMarkdown, website, sourceRef, versionLabel, siteRoot } from './content.mjs';
import { groups } from './navigation.mjs';

export function headings(body) {
  const found = [], seen = new Map();
  let fence = '', offset = 0;
  for (const [line, value] of body.split('\n').entries()) {
    const marker = value.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = '';
    } else if (!fence) {
      const match = value.match(/^(#{2,6}) (.+)$/);
      if (match) {
        const title = match[2].replace(/\[([^]]+)\]\([^)]*\)/g, '$1').replace(/[`*_]/g, '');
        const slug = title.toLowerCase().replace(/[^\w\- ]/g, '').replaceAll(' ', '-');
        const count = seen.get(slug) ?? 0;
        seen.set(slug, count + 1);
        const anchor = slug + (count ? `-${count}` : '');
        const id = /^[a-z]/i.test(anchor) ? anchor : `section-${anchor}`;
        found.push({title, id, anchor, level:match[1].length, line, offset});
      }
    }
    offset += Buffer.byteLength(value + '\n');
  }
  return found;
}

export async function prepareContent() {
  const navigation = groups.map((group) => ({ label:group.label, items:group.items.map(([title, route]) => ({ title, href:href('docs/' + route.replace(/\/?(#|$)/, '/$1')) })) }));
  const index = rewriteMarkdown(await readFile(path.join(docs,'README.md'),'utf8'), path.join(docs,'README.md'));
  const paths = [...index.matchAll(/<a href="([^"]+)"><span class="docs-path-icon"[^>]*>.*?<\/span><strong>(.*?)<\/strong><span>(.*?)<\/span>/g)].map((m) => ({href:m[1],title:m[2],description:m[3]}));
  if (paths.length !== 3) throw new Error('The docs index must expose the three approved workflows.');
  const pages = [];
  for (const source of await markdownFiles(docs)) {
    const relative = path.relative(docs,source).split(path.sep).join('/');
    const raw = await readFile(source,'utf8');
    const title = raw.match(/^# (.+)$/m)?.[1];
    if (!title) throw new Error(`Missing page title: ${relative}`);
    let body = rewriteMarkdown(raw.replace(/^# .+\n\n(?:\[[^\n]+\n\n)?/,''),source);
    // Workflow cards are rendered as Rune components from this same source index.
    if (relative === 'README.md') body = body.slice(body.indexOf('## Get started'));
    body = body.replace(/^<span id="[^"]+"><\/span>\n/gm,'');
    const sections = headings(body);
    const aliases = Object.fromEntries(sections.filter((h) => h.anchor !== h.id).map((h) => [h.anchor,h.id]));
    aliases._top = 'top';
    if (relative === 'mcp/setup.md') aliases['register-a-standalone-server'] = 'configure-your-client';
    if (relative === 'getting-started/README.md') aliases['build-and-launch'] = 'install-and-launch';
    if (relative === 'guides/plous-mcp.md') aliases['use-plous-mcp'] = 'use-the-assistant-panel';
    const route = routeFor(relative);
    const group = navigation.find((group) => group.items.some((item) => item.href.split('#')[0] === href(route)));
    pages.push({route,href:href(route),title,body,headings:sections,aliases,section:group?.label ?? 'Documentation',developers:relative.startsWith('contributing/'),edit:`https://github.com/simjay/plous/${sourceRef === 'main' ? 'edit' : 'blob'}/${sourceRef}/docs/${relative}`});
  }
  pages.push({route:'404/',href:href('404.html'),title:'Page not found',body:'The page may have moved. [Open the documentation]('+href('docs/')+') or use Search to find a guide.',headings:[],aliases:{_top:'top'},section:'Documentation',developers:false,edit:'https://github.com/simjay/plous/tree/main/docs'});
  const routes = new Set(pages.map((page) => page.href));
  for (const group of navigation) for (const item of group.items) {
    if (!routes.has(item.href.split('#')[0])) throw new Error(`Navigation points to missing guide: ${item.href}`);
  }
  const content = {version:versionLabel,siteRoot,base,root:href(),navigation,paths,pages};
  await mkdir(path.join(website,'rune/.generated'),{recursive:true});
  const destination = path.join(website,'rune/.generated/content.json');
  const serialized = JSON.stringify(content,null,2)+'\n';
  if (await readFile(destination,'utf8').catch(()=> '') !== serialized) await writeFile(destination,serialized);
  return content;
}

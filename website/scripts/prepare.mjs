import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { docs, isMaintainerGuide, markdownFiles, repositoryURL, rewriteMarkdown, routeFor, versionLabel } from './content.mjs';
import { groups } from './navigation.mjs';

export function headingSlug(value) {
  const slug = value.toLowerCase().replace(/[^\w\- ]/g, '').replaceAll(' ', '-');
  return /^[a-z]/.test(slug) ? slug : 'section-' + slug;
}

const guideAliases = {
  'README.md': {setup: 'quick-links', reference: 'quick-links', 'why-plous': 'quick-links'},
  'getting-started/README.md': {'build-and-launch': 'install-and-launch', quickstart: 'terminal-quickstart'},
  'guides/navigation.md': {
    'learn-the-interface': 'explore-clusters-in-plous',
    'understand-the-workspace-tree': 'workspaces-and-connections',
    'run-a-command': 'move-search-and-go-back',
    'switch-between-tree-and-main': 'move-search-and-go-back',
    'know-when-you-are-typing': 'move-search-and-go-back',
    'search-and-clear-a-filter': 'move-search-and-go-back',
    'go-back-with-esc': 'move-search-and-go-back',
    'make-more-room': 'move-search-and-go-back',
    'use-help': 'move-search-and-go-back',
  },
  'guides/ai-chat.md': {
    'use-the-ai-assistant': 'use-an-agent-with-plous',
    'use-the-assistant-in-plous': 'use-an-agent-with-plous',
    'connect-the-plous-agent': 'choose-an-agent',
    'connect-a-vendor': 'choose-an-agent',
    'choose-a-model': 'choose-an-agent',
    'executables-and-authentication': 'run-claude-code-or-codex',
    'use-the-native-terminal': 'run-claude-code-or-codex',
    'session-scope-and-saved-evidence': 'run-claude-code-or-codex',
    'workspace-context': 'investigate-a-workload',
    'ask-the-assistant-to-show-a-view': 'investigate-a-workload',
    'manage-conversations': 'inspect-a-replys-sources',
    'compose-and-navigate': 'inspect-a-replys-sources',
    'limits-and-recovery': 'inspect-a-replys-sources',
    'credentials-and-advanced-settings': 'choose-an-agent',
    'next-steps': 'inspect-a-replys-sources',
  },
  'agent-skills/plous-debug.md': {
    'debug-with-your-coding-agent': 'debug-on-kubernetes-with-your-coding-agent',
    'investigate-a-checkout-error': 'follow-an-incident-from-cluster-to-code',
    'collect-evidence': 'follow-an-incident-from-cluster-to-code',
    'trace-the-configuration': 'follow-an-incident-from-cluster-to-code',
    'verify-after-deployment': 'follow-an-incident-from-cluster-to-code',
    'when-logs-arent-enough': 'follow-an-incident-from-cluster-to-code',
    'configuration-reference': 'what-setup-installs',
    'pin-a-context': 'what-setup-installs',
    'installed-files': 'what-setup-installs',
    'remove-the-integration': 'what-setup-installs',
    'next-steps': 'what-setup-installs',
  },
  'mcp/setup.md': {
    'configure-mcp-manually': 'mcp-setup-and-troubleshooting',
    'register-a-standalone-server': 'configure-your-client',
    'codex': 'configure-your-client',
    'claude-code': 'configure-your-client',
    'other-supported-clients': 'configure-your-client',
    'use-mcp-inside-the-tui': 'troubleshoot-the-connection',
  },
};

function addBookmarkAliases(source, relative) {
  let fence = '';
  const headings = new Set();
  const bookmark = (id) => `<span id="${id}" aria-hidden="true"></span>`;
  return source.split('\n').map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = '';
      return line;
    }
    if (fence) return line;
    const named = line.match(/^<a name="([\w-]+)"><\/a>$/);
    if (named) return bookmark(named[1]);

    const heading = line.match(/^(#{1,6}) (.+)$/);
    if (!heading) return line;
    const [, level, title] = heading;
    const text = title.replace(/\[([^]]+)\]\([^)]*\)/g, '$1').replace(/[`*_]/g, '');
    const slug = headingSlug(text);
    let id = slug;
    let count = 1;
    while (headings.has(id)) id = slug + '-' + count++;
    headings.add(id);
    const aliases = Object.entries(guideAliases[relative] ?? {})
      .filter(([, target]) => target === id).map(([alias]) => alias);
    if (level === '#') {
      for (const alias of ['top', '_top']) if (id !== alias) aliases.push(alias);
    }
    const plain = text.toLowerCase().replace(/[^\w\- ]/g, '').replaceAll(' ', '-');
    if (plain && !/^[a-z]/.test(plain)) aliases.push(id.slice('section-'.length));
    return aliases.length ? aliases.map(bookmark).join('') + '\n\n' + line : line;
  }).join('\n');
}

export async function prepareContent() {
  const pages = [];
  const retired = [];
  for (const filename of await markdownFiles(docs)) {
    const relative = path.relative(docs, filename).split(path.sep).join('/');
    const route = routeFor(relative);
    if (isMaintainerGuide(relative)) {
      retired.push({route, destination: repositoryURL('docs/' + relative)});
      continue;
    }
    const raw = await readFile(filename, 'utf8');
    const body = addBookmarkAliases(rewriteMarkdown(raw, filename, true), relative);
    const source = repositoryURL('docs/' + relative, versionLabel === 'Development' ? 'edit' : 'blob');
    pages.push({route, relative, body, source});
  }
  const quickstart = pages.find(page => page.route === 'docs/getting-started/');
  const command = quickstart.body.match(/^`{3}sh\n(brew install [^\n]+)\n/m)?.[1];
  if (!command) throw new Error('Quickstart must include the Homebrew install command');
  const index = pages.find(page => page.route === 'docs/');
  index.body = index.body.replace(/^## Install Plous$/m, ['## Install Plous', '', '```sh', command, '```'].join('\n'));
  const routes = new Set(pages.map(page => page.route));
  for (const group of groups) {
    for (const [, route] of group.items) {
      if (!routes.has(`docs/${route}/`)) throw new Error(`Sidebar points to missing guide: ${route}`);
    }
  }
  return {pages, retired};
}

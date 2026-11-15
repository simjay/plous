import { defineConfig } from 'vitepress';
import { base, siteRoot, versionLabel } from './scripts/content.mjs';
import { groups } from './scripts/navigation.mjs';
import { headingSlug } from './scripts/prepare.mjs';

const hostname = process.env.PLOUS_SITE_URL ?? 'https://simjay.github.io';

export default defineConfig({
  base,
  cleanUrls: true,
  title: 'Plous',
  description: 'Explore and troubleshoot Kubernetes from your terminal or coding agent.',
  head: [['link', {rel: 'icon', href: base + 'assets/icon.svg'}]],
  transformHead({pageData}) {
    if (pageData.relativePath === '404.md') return [['meta', {name: 'robots', content: 'noindex'}]];
    return [['link', {rel: 'canonical', href: new URL(base + pageData.relativePath.replace(/index\.md$/, ''), hostname).href}]];
  },
  transformHtml(html, _file, {page}) {
    if (page === '404.md') {
      return html.replace('</body>', `<noscript><h1>Page not found</h1><a href="${base}docs/">Open documentation</a></noscript></body>`);
    }
  },
  markdown: {
    anchor: {slugify: headingSlug, uniqueSlugStartIndex: 1},
    theme: {light: 'github-light', dark: 'github-dark'},
  },
  themeConfig: {
    siteTitle: false,
    logo: {light: '/assets/lockup.svg', dark: '/assets/lockup-dark.svg', alt: 'Plous home'},
    nav: [{text: 'Docs', link: '/docs/', activeMatch: '^/docs/'}],
    sidebar: {
      '/docs/': groups.map(({label, items}) => ({
        text: label,
        items: items.map(([text, route]) => ({text, link: `/docs/${route}/`})),
      })),
    },
    search: {provider: 'local'},
    docFooter: {prev: false, next: false},
    editLink: {
      pattern: ({frontmatter}) => frontmatter.source,
      text: versionLabel === 'Development' ? 'Edit on GitHub' : 'View source',
    },
    documentationVersion: versionLabel,
    versionsURL: siteRoot + 'versions.json',
  },
});

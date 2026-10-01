import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { base, repository, sidebar } from './pages.mjs';

export default defineConfig({
  site: 'https://simjay.github.io',
  base,
  trailingSlash: 'always',
  integrations: [starlight({
    title: 'Plous',
    description: 'A terminal workspace for infrastructure tools, configuration history, and AI assistance.',
    logo: { light: '../assets/logo/icon.svg', dark: '../assets/logo/icon-dark.svg' },
    favicon: '/favicon.svg',
    social: [{ icon: 'github', label: 'GitHub', href: repository }],
    editLink: { baseUrl: `${repository}/edit/main/` },
    sidebar: sidebar(),
    customCss: ['./src/styles.css'],
    tableOfContents: { minHeadingLevel: 2, maxHeadingLevel: 2 },
    expressiveCode: {
      themes: ['github-dark', 'github-light'],
      styleOverrides: { codeFontFamily: "'Geist Mono Variable', monospace" },
    },
  })],
});

import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { docs, routeFor } from '../../website/scripts/content.mjs';
import { legacyRoutes } from '../../website/scripts/legacy-routes.mjs';

test('moved website routes target current guides and preserve controller heading links', async () => {
  const files = (await readdir(docs, { recursive: true })).filter((file) => file.endsWith('.md'));
  const pages = new Map(files.map((file) => [routeFor(file), file]));
  for (const route of legacyRoutes) {
    assert.ok(!pages.has(`docs/${route.slug}/`), 'Moved guides must have one canonical source');
    for (const destination of [route.destination, ...Object.values(route.anchors)]) {
      const [url, anchor] = destination.split('#');
      assert.ok(pages.has(url), `Missing redirect target: ${destination}`);
      if (anchor) {
        const source = await readFile(path.join(docs, pages.get(url)), 'utf8');
        const headings = [...source.matchAll(/^## (.+)$/gm)].map((match) => match[1].toLowerCase().replace(/[^\w\- ]/g, '').replaceAll(' ', '-'));
        assert.ok(headings.includes(anchor), `Missing redirect heading: ${destination}`);
      }
    }
  }
  const gitops = legacyRoutes.find((route) => route.slug === 'integrations/gitops');
  assert.equal(gitops.anchors['flux-sources'], 'docs/integrations/flux/#flux-sources');
  assert.equal(gitops.anchors['argo-applications'], 'docs/integrations/argo/#argo-applications');
});

import assert from 'node:assert/strict';
import path from 'node:path';
import test from 'node:test';
import { docs, href, rewriteMarkdown, routeFor, sourceRef } from '../../website/scripts/content.mjs';

test('repository pages map to stable website routes', () => {
  assert.equal(routeFor('README.md'), 'docs/');
  assert.equal(routeFor('getting-started/README.md'), 'docs/getting-started/');
  assert.equal(routeFor('reference/keymap.md'), 'docs/reference/keymap/');
});

test('links preserve fragments, assets, and source references', () => {
  const result = rewriteMarkdown('[Guide](../getting-started/README.md#before-you-begin)\n![Screen](../assets/plous-overview.png)\n[Test](../../internal/app/tui/interaction_test.go)\n[Site](https://example.com/)\n[Here](#heading)', path.join(docs, 'guides/overview.md'));
  assert.ok(result.includes(`(${href('docs/getting-started/#before-you-begin')})`));
  assert.ok(result.includes(`(${href('assets/plous-overview.png')})`));
  assert.ok(result.includes(`(https://github.com/simjay/plous/blob/${sourceRef}/internal/app/tui/interaction_test.go)`));
  assert.ok(result.includes('(https://example.com/)'));
  assert.ok(result.includes('(#heading)'));
});

test('code examples remain literal', () => {
  const markdown = '```md\n[Example](../example.md)\n```\n~~~md\n[Example](../other.md)\n~~~';
  assert.equal(rewriteMarkdown(markdown, path.join(docs, 'guides/overview.md')), markdown);
});

test('links cannot publish local internals or leave the repository', () => {
  const source = path.join(docs, 'guides/overview.md');
  assert.throws(() => rewriteMarkdown('[Private](../../.decisions/ai.md)', source), /public repository content/);
  assert.throws(() => rewriteMarkdown('[Private](../../../secret.md)', source), /public repository content/);
});

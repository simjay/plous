import test from 'node:test';
import assert from 'node:assert/strict';
import { rewriteLink } from '../../docs-site/scripts/prepare.mjs';

test('source links become Pages routes without losing fragments', async () => {
  assert.equal(await rewriteLink('README.md', 'logs.md'), '/plous/');
  assert.equal(await rewriteLink('logs.md#controls', 'kubernetes-explore.md'), '/plous/logs/#controls');
  assert.equal(await rewriteLink('#controls', 'logs.md'), '#controls');
});

test('repository sources stay on GitHub', async () => {
  assert.equal(await rewriteLink('../LICENSE', 'contributing.md'), 'https://github.com/simjay/plous/blob/main/LICENSE');
  assert.equal(await rewriteLink('../.github/workflows/docs.yml', 'documentation.md'), 'https://github.com/simjay/plous/blob/main/.github/workflows/docs.yml');
  assert.equal(await rewriteLink('https://example.com/docs?q=1', 'logs.md'), 'https://example.com/docs?q=1');
});

test('missing files and local internals cannot become published links', async () => {
  for (const link of ['missing.md', '../.decisions/README.md', '../%2edecisions/README.md', '../../outside.md']) {
    await assert.rejects(rewriteLink(link, 'logs.md'));
  }
});

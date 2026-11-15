import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { docs, isMaintainerGuide, markdownFiles, routeFor } from '../../website/scripts/content.mjs';
import { groups } from '../../website/scripts/navigation.mjs';
import { prepareContent } from '../../website/scripts/prepare.mjs';

const {pages, retired} = await prepareContent();
const findPage = (route) => pages.find((page) => page.route === route);

test('publishes every product guide and keeps contributor pages out of the site', async () => {
  const sources = (await markdownFiles(docs)).map(file => path.relative(docs, file).split(path.sep).join('/'));
  assert.deepEqual(pages.map(page => page.route), sources.filter(file => !isMaintainerGuide(file)).map(routeFor));
  assert.deepEqual(retired.map(page => page.route), sources.filter(isMaintainerGuide).map(routeFor));
  assert.ok(retired.every(page => page.destination.startsWith('https://github.com/simjay/plous/blob/')));
  assert.deepEqual(groups.map(group => group.label), ['Start here', 'Use Plous', 'Reference']);
  assert.equal(groups.flatMap(group => group.items).length, 10);
});

test('preserves the literal contents of every guide’s fenced examples', async () => {
  const fences = text => [...text.matchAll(/```[^\n]*\n([\s\S]*?)```/g)].map(match => match[1]);
  for (const page of pages) {
    const source = await readFile(path.join(docs, page.relative), 'utf8');
    const expected = fences(source);
    if (page.route === 'docs/') expected.unshift(fences(findPage('docs/getting-started/').body)[0].split('\n')[0] + '\n');
    assert.deepEqual(fences(page.body), expected, page.relative);
  }
});

test('keeps compatibility bookmarks and places interactive examples where readers need them', () => {
  const tutorial = findPage('docs/guides/investigate-restarting-pod/').body;
  assert.match(tutorial, /id="1-create-the-example"[^]*## 1\. Create the example/);
  const quickstart = findPage('docs/getting-started/').body;
  assert.match(quickstart, /id="build-and-launch"[^]*## Install and launch/);
  assert.match(quickstart, /## Learn the basic controls[^]*<WorkspaceScreenshot \/>[^]*<GuideTabs example="panel-focus" \/>/);
  const setup = findPage('docs/mcp/setup/').body;
  assert.match(setup, /## Check the connection[^]*<GuideTabs example="mcp-target" \/>[^]*## Target and evidence options/);
});

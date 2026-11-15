import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { docs, href, markdownFiles, routeFor } from '../../website/scripts/content.mjs';
import { headings, prepareContent } from '../../website/scripts/prepare.mjs';
const content=await prepareContent();

test('Rune includes every source guide and preserves all fenced examples',async()=>{
  const files=await markdownFiles(docs);
  assert.equal(content.pages.filter(p=>p.route.startsWith('docs/')).length,files.length);
  const fences=text=>[...text.matchAll(/```[^\n]*\n([\s\S]*?)```/g)].map(m=>m[1]);
  for(const file of files){
    const source=await readFile(file,'utf8');
    const page=content.pages.find(p=>p.route===routeFor(path.relative(docs,file).split(path.sep).join('/')));
    assert.ok(page,`Missing guide: ${file}`);
    assert.deepEqual(fences(page.body),fences(source),`Changed example: ${file}`);
  }
});
test('all heading keys point at exact source byte and line boundaries',()=>{
  for(const page of content.pages){
    const body=Buffer.from(page.body);
    assert.equal(new Set(page.headings.map(h=>h.id)).size,page.headings.length);
    for(const h of page.headings){
      assert.match(h.id,/^[a-z][a-z0-9_-]*$/i);
      assert.ok(body.subarray(h.offset).toString().startsWith('#'.repeat(h.level)+' '));
      assert.equal(Buffer.byteLength(page.body.split('\n').slice(0,h.line).join('\n')+(h.line?'\n':'')),h.offset);
    }
  }
});
test('headings ignore fenced content and retain numeric and repeated anchor aliases',()=>{
  const result=headings('Text → α\n\n## 1. Start\n\n```md\n## Example only\n```\n\n## Again\n\n## Again\n');
  assert.deepEqual(result.map(h=>h.id),['section-1-start','again','again-1']);
  assert.equal(result[0].anchor,'1-start');
  const tutorial=content.pages.find(p=>p.route==='docs/guides/investigate-restarting-pod/');
  assert.equal(tutorial.aliases['1-create-the-example'],'section-1-create-the-example');
});
test('workflow links lead to the terminal UI and coding agent setup',()=>{
  const quickstart=href('docs/getting-started/');
  const setup=href('docs/agent-skills/plous-debug/');
  assert.deepEqual(content.paths.map(p=>p.href),[quickstart,setup]);
  assert.deepEqual(content.navigation.map(g=>g.label),['Get started','Terminal UI','Coding agents','Integrations','Reference','Developers']);
  const terminal=content.navigation.find(group=>group.label==='Terminal UI');
  assert.deepEqual(terminal.items.slice(0,3).map(item=>item.href),[quickstart,href('docs/guides/navigation/'),href('docs/guides/workspaces/')]);
  const coding=content.navigation.find(group=>group.label==='Coding agents');
  assert.deepEqual(coding.items.slice(0,2).map(item=>item.href),[setup,href('docs/mcp/troubleshooting/')]);
  assert.ok(coding.items.some(item=>item.href===href('docs/mcp/setup/')));
  const guide=content.pages.find(page=>page.href===setup);
  const install=guide.body.split('## Install\n')[1].split('\n## ')[0];
  for(const client of ['opencode','grok','agy','cursor','gemini','codex','claude']) {
    assert.ok(install.includes('`plous setup '+client+'`'),`Missing install command for ${client}`);
  }
});

test('the index groups shared links by intent and the MCP diagram follows the target check',()=>{
  assert.deepEqual(content.quickLinks.map(group=>[group.label,group.items.map(item=>item.href)]),[
    ['Setup',[href('docs/getting-started/installation/'),href('docs/guides/ai-chat/'),href('docs/integrations/')]],
    ['Reference',[href('docs/reference/keymap/'),href('docs/reference/mcp/'),href('docs/reference/troubleshooting/')]],
  ]);
  const setup=content.pages.find(page=>page.route==='docs/mcp/setup/');
  assert.deepEqual(setup.examples,['mcp-target']);
  assert.match(setup.body,/## Check the connection\n[^]*> plous-example:mcp-target\n[^]*## Target and evidence options/);
});

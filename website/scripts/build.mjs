import { spawn } from 'node:child_process';
import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { website, href } from './content.mjs';
import { prepareContent } from './prepare.mjs';
import { legacyRoutes } from './legacy-routes.mjs';

const rune=path.join(website,'rune');
function run(command,args,options={}) {
  return new Promise((resolve,reject)=>{
    const child=spawn(command,args,{cwd:rune,stdio:'inherit',...options});
    child.on('error',reject);child.on('exit',code=>code===0?resolve():reject(new Error(`${command} exited ${code}`)));
  });
}
function capture(command,args) {
  return new Promise((resolve,reject)=>{
    const child=spawn(command,args,{cwd:rune});let output='';
    child.stdout.on('data',chunk=>output+=chunk);child.stderr.pipe(process.stderr);
    child.on('error',reject);child.on('exit',code=>code===0?resolve(output):reject(new Error(`${command} exited ${code}`)));
  });
}
const escape=(value)=>value.replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;').replaceAll('>','&gt;');
const content=await prepareContent();
await run('cargo',['build','--locked']);
const metadata=JSON.parse(await capture('cargo',['metadata','--format-version','1','--no-deps']));
const host=(await capture('rustc',['-vV'])).match(/^host: (.+)$/m)[1];
const full=JSON.parse(await capture('cargo',['metadata','--format-version','1','--locked','--offline','--filter-platform',host]));
const dependency=full.packages.find(p=>p.name==='rune');
const upstream=path.resolve(path.dirname(dependency.manifest_path),'../..');
const dist=path.join(website,'dist');
await rm(dist,{recursive:true,force:true});
const assets=path.join(dist,'assets');await mkdir(assets,{recursive:true});
for(const name of ['lockup.svg','lockup-dark.svg','icon.svg']) await cp(path.join(website,'../assets/logo',name),path.join(assets,name));
await cp(path.join(website,'../docs/assets/plous-overview.png'),path.join(assets,'plous-overview.png'));
for(const name of ['geist.ttf','geist-mono.ttf','geist-OFL.txt']) await cp(path.join(upstream,'www/public/fonts',name),path.join(assets,name));
for(const name of ['Inter-Regular.ttf','Inter-SemiBold.ttf','Inter-LICENSE.txt']) await cp(path.join(upstream,'crates/rune/assets',name),path.join(assets,name));
await cp(path.join(upstream,'LICENSE'),path.join(assets,'Rune-LICENSE.txt'));
await cp(path.join(website,'scripts/client.mjs'),path.join(assets,'client.mjs'));
await run('cargo',['build','--locked','--target','wasm32-unknown-unknown','--release','--lib']);
await run('wasm-bindgen',['--target','web','--out-dir',assets,'--out-name','app','--remove-name-section',path.join(metadata.target_directory,'wasm32-unknown-unknown/release/plous_docs_rune.wasm')]);
const colors={'#ffffff':'bg','#f6f8fc':'surface','#172033':'ink','#56657a':'muted','#e2e8f0':'line','#326ce5':'blue','#2458b8':'code-blue','#edf3ff':'wash','#206a43':'green','#7045ba':'violet','#905500':'amber','#b63e50':'red'};
function controls(html) {
  // Rune's exported press targets retain their keys; use native buttons before the live handoff.
  const stack=[];
  return html.replace(/<\/?div\b[^>]*>/g,tag=>{
    if(tag.startsWith('</')) return stack.pop() ? '</button>' : tag;
    const label=tag.match(/aria-label="([^"]+)"/)?.[1]??'';
    if(label==='example-frames'){
      stack.push(false);
      return tag.replace('aria-label="example-frames"','id="example-frames" role="status"');
    }
    const frame=label.match(/^example-frame-(\d+)$/);
    if(frame){
      stack.push(false);
      return tag.replace(`aria-label="${label}"`,`id="${label}"${frame[1]==='0'?'':' hidden'}`);
    }
    const example=label.match(/^example-step-(\d+)$/);
    const button=example||/\brole="button"/.test(tag)||['Search documentation','Change color theme','Open navigation','Choose documentation version','On this page'].includes(label)||content.navigation.some(g=>escape(g.label)===label)||/^Copy code block \d+$/.test(label);
    stack.push(button);
    if(!button)return tag;
    let control=({'Search documentation':'search','Change color theme':'theme','Open navigation':'menu','Choose documentation version':'versions','On this page':'contents'})[label];
    const group=content.navigation.findIndex(group=>escape(group.label)===label);
    if(group>=0)control='group-'+group;
    const code=label.match(/^Copy code block (\d+)$/);
    if(code)control='copy-'+(Number(code[1])-1);
    if(example){
      control=label;
      tag=tag.replace(` aria-label="${label}"`,'');
    }
    const expanded = group >= 0 || control === 'contents';
    if (expanded) tag = tag.replace(/\saria-expanded="[^"]*"/, '');
    return tag.replace('<div',`<button type="button"${control?` data-control="${control}"`:''}${expanded?` aria-expanded="${group >= 0}"`:''}${control==='contents'?' aria-controls="contents-inline-links"':''}${example?` aria-expanded="${example[1]==='0'}" aria-controls="example-frame-${example[1]}"`:''}`);
  });
}
const template=await readFile(path.join(website,'scripts/template.html'),'utf8');
const pages=[{route:'',title:'Plous documentation',body:'Inspect Kubernetes clusters and investigate with Plous.',aliases:{_top:'top'}},...content.pages];
for(const page of pages) {
  const output=page.route==='404/'?path.join(dist,'404.html'):path.join(dist,page.route,'index.html');
  await mkdir(path.dirname(output),{recursive:true});
  const canonical=new URL(href(page.route==='404/'?'404.html':page.route),process.env.PLOUS_SITE_URL??'https://simjay.github.io').href;
  const description=page.route==='docs/'?content.indexIntro:page.body.replace(/[#*`\[\]]/g,'').split('\n').find(line=>line.length>30)?.slice(0,160)??page.title;
  const title=page.route==='docs/'?page.title:`${page.title} · Plous`;
  let frame=template.replaceAll('__BASE__',content.base).replaceAll('__SITE_ROOT__',content.siteRoot).replaceAll('__VERSION__',escape(content.version)).replaceAll('__PAGE__',escape(page.route)).replaceAll('__SECTION__',escape(page.section??'')).replaceAll('__TITLE__',escape(title)).replaceAll('__DESCRIPTION__',escape(description)).replaceAll('__CANONICAL__',escape(canonical));
  const templateFile=path.join(rune,'.generated/template.html');await writeFile(templateFile,frame);
  await run(path.join(metadata.target_directory,'debug/plous-docs-rune'),['--html',output,'--html-widths','360,700,1000,1280,1440','--html-template',templateFile],{cwd:dist,env:{...process.env,PLOUS_SITE_PAGE:page.route}});
  let html=controls(await readFile(output,'utf8'));
  if(page.route==='docs/') html=html.replace(/<a\b(?=[^>]*\bid="sidebar-root")/, '$& aria-current="page"');
  html=html.replace(/(<(?:nav|a|section)\b[^>]*id="(?:sidebar|nav-docs|nav-developers|contents)"[^>]*)\saria-hidden="true"/g,'$1');
  html=html.replaceAll('src="assets/',`src="${content.base}assets/`);
  html=html.replace(/<img\b[^>]*src="[^"]*\/lockup(?:-dark)?\.svg"[^>]*>/g,tag=>tag.replace('<img','<img data-wordmark'));
  html=html.replace(/<style data-rune-static>([\s\S]*?)<\/style>/,(_,css)=>`<style data-rune-static>${css.replaceAll('[data-rune-static]',':is([data-rune-static],[data-rune-menu])').replace(/#[a-f\d]{6}\b/gi,color=>colors[color.toLowerCase()]?`var(--${colors[color.toLowerCase()]})`:color)}</style>`);
  html=html.replace(/style="([^"]*)"/g,(_,style)=>`style="${style.replace(/#[a-f\d]{6}\b/gi,color=>colors[color.toLowerCase()]?`var(--${colors[color.toLowerCase()]})`:color)}"`);
  html=html.replace(/(fill|stroke)="(#[a-f\d]{6})"/gi,(_,attribute,color)=>`${attribute}="${colors[color.toLowerCase()]?`var(--${colors[color.toLowerCase()]})`:color}"`);
  // Exported image URLs preserve the same font/asset locations on nested routes.
  const nav=html.match(/<nav\b[^>]*id="sidebar"[^>]*>[\s\S]*?<\/nav>/)?.[0]??'';
  // Rune retires the initial stylesheet at handoff; the native menu keeps its exported styling.
  if(nav){
    const menuStyles=html.match(/<style data-rune-static>([\s\S]*?)<\/style>/)?.[1]??'';
    html=html.replace('</head>',`<style id="menu-styles">${menuStyles}</style></head>`);
  }
  html=html.replace('__MENU__',nav?`<div data-rune-menu>${nav.replace(/\bid="([^"]+)"/g,'id="mobile-$1"').replace(/\saria-hidden="true"/,'')}</div>`:`<p><a href="${href('docs/')}">Open documentation</a></p>`);
  html=html.replace('__ALIASES__',JSON.stringify(page.aliases).replaceAll('<','\\u003c'));
  const codes=[];let fence='',lines=[];
  for(const line of page.body.split('\n')){
    const marker=line.match(/^\s*(`{3,}|~{3,})/);
    if(!fence&&marker){fence=marker[1];lines=[]}
    else if(fence&&marker&&marker[1][0]===fence[0]&&marker[1].length>=fence.length){codes.push(lines.join('\n'));fence=''}
    else if(fence)lines.push(line);
  }
  html=html.replace('__CODE__',JSON.stringify(codes).replaceAll('<','\\u003c'));
  for(const [alias,id] of Object.entries(page.aliases)) {
    const marker=new RegExp(`(<h[1-6]\\b[^>]*id="${id}"[^>]*>)`);
    html=html.replace(marker,`$1<span id="${escape(alias)}" style="position:absolute;width:0;height:0" aria-hidden="true"></span>`);
  }
  html=html.replace('<main ',page.route==='404/'?'<main data-pagefind-ignore="all" ':'<main data-pagefind-body ');
  if(page.route==='404/') html=html.replace('<head>','<head><meta name="robots" content="noindex" />');
  await writeFile(output,html);
}
for(const route of [...legacyRoutes,{slug:'../rune',destination:'docs/',anchors:{}}]) {
  const relative=route.slug==='../rune'?'rune/':`docs/${route.slug}/`;
  const target=href(route.destination);
  const fragments=Object.fromEntries(Object.entries(route.anchors).map(([key,value])=>[key,href(value)]));
  const output=path.join(dist,relative,'index.html');await mkdir(path.dirname(output),{recursive:true});
  await writeFile(output,`<!doctype html><html lang="en" data-pagefind-ignore="all"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta name="robots" content="noindex"><title>Documentation moved · Plous</title><link rel="canonical" href="${target}"><noscript><meta http-equiv="refresh" content="0;url=${target}"></noscript><script>location.replace((${JSON.stringify(fragments)})[location.hash.slice(1)]??(${JSON.stringify(target)}+location.search+location.hash))</script></head><body><h1>Documentation moved</h1><a href="${target}">Open the current guide</a></body></html>`);
}
// Keep the previous evaluation guide URL usable after promoting Rune to the main site.
const evaluation=path.join(dist,'rune/guides/investigate-restarting-pod');await mkdir(evaluation,{recursive:true});
await writeFile(path.join(evaluation,'index.html'),`<!doctype html><html lang="en" data-pagefind-ignore="all"><head><meta charset="utf-8"><title>Guide moved · Plous</title><meta http-equiv="refresh" content="0;url=${href('docs/guides/investigate-restarting-pod/')}"></head><body><h1>Guide moved</h1><a href="${href('docs/guides/investigate-restarting-pod/')}">Open the guide</a></body></html>`);
await run(path.join(website,'node_modules/.bin/pagefind'),['--site',dist]);
await writeFile(path.join(dist,'sitemap.xml'),`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${pages.filter(p=>p.route!=='404/').map(p=>`<url><loc>${new URL(href(p.route),process.env.PLOUS_SITE_URL??'https://simjay.github.io').href}</loc></url>`).join('')}</urlset>`);
await writeFile(path.join(rune,'.generated/build.json'),JSON.stringify({base:content.base,guides:content.pages.filter(p=>p.route.startsWith('docs/')).length,rune:dependency.source.split('#').at(-1)},null,2)+'\n');
console.log(`Built ${pages.length} Rune pages and compatibility redirects in ${dist}`);

await writeFile(path.join(dist,'site.json'),JSON.stringify({version:content.version,revision:process.env.PLOUS_DOCS_REVISION??null,source:process.env.PLOUS_DOCS_SOURCE_REF??process.env.PLOUS_DOCS_VERSION??'main',base:content.base,routes:pages.filter(p=>p.route!=='404/').map(p=>p.route)},null,2)+'\n');

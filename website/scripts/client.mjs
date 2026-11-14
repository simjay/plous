const root=document.documentElement;
const base=root.dataset.base;
const search=document.querySelector('#doc-search');
const menu=document.querySelector('#mobile-menu');
const input=document.querySelector('#search-input');
const results=document.querySelector('#search-results');
const status=document.querySelector('#search-status');
const aliases=JSON.parse(document.querySelector('#page-aliases').textContent);
const codes=JSON.parse(document.querySelector('#page-code').textContent);
window.plousRunePage=()=>root.dataset.page;
window.plousRuneDark=()=>root.dataset.theme==='dark';
function syncWordmark(){
  for(const image of document.querySelectorAll('img[data-wordmark]')) image.src=base+'assets/'+(window.plousRuneDark()?'lockup-dark.svg':'lockup.svg');
}
window.plousSetDark=(dark)=>{
  root.dataset.theme=dark?'dark':'light';
  syncWordmark();
  try{localStorage.setItem('plous-docs-theme',root.dataset.theme)}catch{}
};
syncWordmark();
for(const button of document.querySelectorAll('button[data-control="theme"]')){
  const label=button.querySelector('.t');if(label)label.textContent=window.plousRuneDark()?'Light':'Dark';
}
for(const button of document.querySelectorAll('button[data-control^="group-"]')){
  const label=button.getAttribute('aria-label');
  if(label!=='Get started'&&label!==root.dataset.section&&label!=='Developers'){
    const items=button.nextElementSibling;
    if(items){button.setAttribute('aria-expanded','false');items.style.display='none';items.inert=true}
  }
}
window.plousOpenSearch=()=>{if(!search.open)search.showModal();input.focus()};
const versions=document.querySelector('#doc-versions');
window.plousOpenVersions=async()=>{
  if(!versions.open)versions.showModal();
  const status=document.querySelector('#version-status'),list=document.querySelector('#version-list');
  status.textContent='Current: '+root.dataset.version;list.replaceChildren();
  try{
    const response=await fetch(root.dataset.siteRoot+'versions.json',{cache:'no-cache'});
    if(!response.ok)throw new Error('Version list unavailable');
    const catalog=await response.json();
    for(const entry of catalog.versions){
      const li=document.createElement('li'),a=document.createElement('a');
      a.textContent=entry.version+(entry.version===catalog.latest?' (latest)':'');
      a.href=entry.base+(entry.routes.includes(root.dataset.page)?root.dataset.page:'docs/')+location.hash;
      if(entry.version===root.dataset.version)a.setAttribute('aria-current','true');
      li.append(a);list.append(li);
    }
  }catch{status.textContent+=' · Other versions are available on the published website.'}
};
document.querySelector('#close-versions').addEventListener('click',()=>versions.close());
window.plousOpenMenu=()=>{if(!menu.open)menu.showModal()};
// Rune's link bridge treats absolute HTTP URLs as browser navigation.
window.plousRuneMarkdown=(source)=>{
  let fence='';
  return source.split('\n').map(line=>{
    const marker=line.match(/^\s*(`{3,}|~{3,})/);
    if(marker){if(!fence)fence=marker[1];else if(marker[1][0]===fence[0]&&marker[1].length>=fence.length)fence='';return line}
    return fence?line:line.replace(/\]\(([/#][^\s)]*)\)/g,(_match,url)=>`](${new URL(url,location.href).href})`);
  }).join('\n');
};
document.querySelector('#close-search').addEventListener('click',()=>search.close());
document.querySelector('#close-menu').addEventListener('click',()=>menu.close());
function followAlias(){const target=aliases[decodeURIComponent(location.hash.slice(1))];if(target&&target!==location.hash.slice(1))location.replace('#'+target)}
followAlias();window.addEventListener('hashchange',followAlias);
document.addEventListener('keydown',event=>{
  if((event.metaKey||event.ctrlKey)&&event.key.toLowerCase()==='k'){event.preventDefault();window.plousOpenSearch()}
});
document.addEventListener('click',async event=>{
  const button=event.target.closest('button[data-control]');
  if(button&&(root.dataset.runtime!=='rune'||button.closest('#mobile-menu'))){
    const control=button.dataset.control;
    if(control==='versions')window.plousOpenVersions();
    if(control==='search')window.plousOpenSearch();
    if(control==='menu')window.plousOpenMenu();
    if(control==='theme'){
      window.plousSetDark(!window.plousRuneDark());
      const label=button.querySelector('.t');if(label)label.textContent=window.plousRuneDark()?'Light':'Dark';
    }
    if(control.startsWith('copy-')){
      try{await navigator.clipboard.writeText(codes[Number(control.slice(5))]??'');button.setAttribute('aria-label','Copied code')}catch{button.setAttribute('aria-label','Select the code to copy')}
    }
    if(control==='contents'||control.startsWith('group-')){
      const open=button.getAttribute('aria-expanded')!=='false';
      button.setAttribute('aria-expanded',String(!open));
      if(control!=='contents'){
        const items=button.nextElementSibling;
        if(items){items.style.display=open?'none':'flex';items.inert=open}
      }
    }
  }
  const link=event.target.closest('[data-rune-a11y] a[href]');
  if(link&&!event.metaKey&&!event.ctrlKey&&!event.shiftKey&&!event.altKey&&event.button===0){
    const target=new URL(link.href);
    if(target.origin===location.origin){event.preventDefault();event.stopImmediatePropagation();if(target.pathname===location.pathname&&target.hash)location.hash=target.hash;else location.assign(target.href)}
  }
},true);
let index,sequence=0;
input.addEventListener('input',async()=>{
  const current=++sequence,query=input.value.trim();results.replaceChildren();
  if(!query){status.textContent='Search the documentation.';return}
  status.textContent='Searching…';
  try{
    index??=import(`${base}pagefind/pagefind.js`).then(async module=>{await module.options({baseUrl:base});return module});
    const matches=await(await index).search(query);
    const items=await Promise.all(matches.results.slice(0,10).map(result=>result.data()));
    if(current!==sequence)return;
    status.textContent=items.length?`${matches.results.length} results for ${query}`:`No results for ${query}`;
    for(const item of items){const li=document.createElement('li'),a=document.createElement('a'),p=document.createElement('p');a.href=item.url;a.textContent=item.meta.title;p.innerHTML=item.excerpt;li.append(a,p);results.append(li)}
  }catch{if(current===sequence)status.textContent='Search could not load. Use the documentation navigation.'}
});
window.addEventListener('runepresent',()=>{root.dataset.runtime='rune'});
// Match Rune's own website: preserve the static first paint, then start the live view.
if(!new URLSearchParams(location.search).has('static')&&navigator.gpu){
  const start=async()=>{try{const app=await import(`${base}assets/app.js`);await app.default()}catch(error){console.error('Plous renderer unavailable; using static documentation.',error)}};
  if('requestIdleCallback'in window)requestIdleCallback(start,{timeout:1200});else requestAnimationFrame(start);
}

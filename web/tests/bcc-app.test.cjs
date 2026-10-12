'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const web=path.join(__dirname,'..'),read=f=>fs.readFileSync(path.join(web,f),'utf8');
const app=read('bcc-app.js'),html=read('bcc-app.html'),assets=read('assets.go');
const countriesGo=fs.readFileSync(path.join(web,'..','internal','bcc','countries.go'),'utf8');

const T=new Function('return '+app.slice(app.indexOf('const T=')+8,app.indexOf('\nconst lang='))
  .replace(/;\s*$/,''))();

test('Persian and English carry the same strings',()=>{
  const fa=Object.keys(T.fa).sort(),en=Object.keys(T.en).sort();
  assert.deepEqual(fa,en);
  assert.deepEqual(Object.keys(T.fa.nav).sort(),Object.keys(T.en.nav).sort());
  for(const l of ['fa','en'])for(const [k,v] of Object.entries(T[l]))if(typeof v==='string')assert.ok(v.trim(),l+'.'+k+' is empty');
});

test('every translated element in the page has a string',()=>{
  for(const [,k] of html.matchAll(/data-t="([^"]+)"/g))assert.ok(k in T.fa,'missing string: '+k);
});

test('the country list matches the server',()=>{
  const js=JSON.parse(app.match(/const COUNTRIES=(\[[^\]]*\])/)[1].replace(/'/g,'"'));
  const go=[...countriesGo.matchAll(/"([A-Z]{2})":\s*\{/g)].map(m=>m[1]).sort();
  assert.deepEqual([...js].sort(),go);
});

test('every menu route has a page and a name',()=>{
  const routes=JSON.parse(app.match(/const ROUTES=(\[[^\]]*\])/)[1].replace(/'/g,'"'));
  for(const r of routes)assert.ok(T.fa.nav[r]&&T.en.nav[r],'route without a name: '+r);
});

test('demo numbers appear only in the labelled preview',()=>{
  const uses=[...app.matchAll(/demoOverview\(\)(?!\{)/g)].map(m=>m.index);
  assert.equal(uses.length,1,'demoOverview is called from more than one place');
  assert.match(app.slice(uses[0]-40,uses[0]),/if\(preview\)\{overview=$/);
  assert.match(app,/preview\?' · DEMO'/);
});

test('text from the server is escaped before it reaches the page',()=>{
  const esc=new Function('return '+app.match(/const esc=(s=>[^\n]*?\);)\n/)[1].replace(/;$/,''))();
  assert.equal(esc('<img src=x onerror="a">\'&'),'&lt;img src=x onerror=&quot;a&quot;&gt;&#39;&amp;');
  assert.equal(esc(null),'');
});

test('the page only loads files that BCC embeds',()=>{
  const embed=assets.match(/\/\/go:embed (.*)/)[1].split(/\s+/);
  for(const [,f] of html.matchAll(/(?:src|href)="\.\/([^"]+)"/g)){
    assert.ok(embed.includes(f)||(f.startsWith('assets/')&&embed.includes('assets/*')),'not embedded: '+f);
    assert.ok(fs.existsSync(path.join(web,f)),'missing file: '+f);
  }
});

test('theme and language are applied before the first paint',()=>{
  const head=html.slice(0,html.indexOf('</head>'));
  assert.match(head,/localStorage\.getItem\('baft\.theme'\)/);
  assert.match(head,/localStorage\.getItem\('baft\.lang'\)/);
});

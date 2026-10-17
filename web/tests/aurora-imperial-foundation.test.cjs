'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const root=path.resolve(__dirname,'../..');
const css=fs.readFileSync(path.join(root,'web/aurora-imperial-tokens.css'),'utf8');
const assets=fs.readFileSync(path.join(root,'web/assets.go'),'utf8');
const html=fs.readFileSync(path.join(root,'web/bcc-app.html'),'utf8');
const map=fs.readFileSync(path.join(root,'web/bcc-map.js'),'utf8');
const earth=fs.readFileSync(path.join(root,'web/bcc-earth-v3.js'),'utf8');
const welcome=fs.readFileSync(path.join(root,'web/bcc-welcome-globe-v3.js'),'utf8');

test('Imperial tokens are opt-in and do not overwrite BCC global tokens',()=>{
  assert.match(css,/:root\[data-design="imperial"\]/);
  assert.match(css,/:root\[data-design="imperial"\]\[data-theme="light"\]/);
  assert.doesNotMatch(css,/(^|\n)\s*:root\s*\{/);
  assert.doesNotMatch(css,/\b--(?:bg|panel|gold|text|line):/);
});
test('Imperial style is embedded but only enabled for explicit preview opt-in',()=>{
  assert.match(assets,/aurora-imperial-tokens\.css/);
  assert.match(html,/href="\.\/aurora-imperial-tokens\.css"/);
  assert.match(html,/opt==='1'/);
  assert.match(html,/BAFT_UI\.mode==='preview'/);
  assert.match(assets,/bcc-map\.js/);
  assert.match(assets,/bcc-earth-v3\.js/);
  assert.match(assets,/bcc-welcome-globe-v3\.js/);
});
test('Existing geographic map and globe integration still present',()=>{
  assert.match(map,/window\.BAFT_MAP=\{create,flagOf,flagSupported\}/);
  assert.match(map,/opts\.onSelect/);
  assert.match(earth,/getContext\('webgl'/);
  assert.match(welcome,/welcomeGlobe/);
  assert.match(welcome,/pointerdown/);
});
test('Motion respects reduced-motion preference',()=>{
  assert.match(css,/prefers-reduced-motion:reduce/);
});
test('every Imperial rule is scoped to the opt-in design attribute',()=>{
  const body=css.replace(/\/\*[\s\S]*?\*\//g,'');
  const selectors=[...body.matchAll(/(^|\})\s*([^@{}][^{}]*)\{/g)].map(m=>m[2].trim()).filter(s=>!/^(@|from|to|\d)/.test(s));
  assert.ok(selectors.length>10);
  for(const group of selectors)for(const s of group.split(','))assert.match(s.trim(),/^:root\[data-design="imperial"\]/,s);
});
test('Imperial keeps nav focus visible and does not restyle map or globe canvases',()=>{
  assert.match(css,/#nav a:focus-visible/);
  assert.doesNotMatch(css.replace(/\/\*[\s\S]*?\*\//g,''),/canvas|#mapCanvas|#welcomeGlobe/);
});

'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const web=path.join(__dirname,'..'),app=fs.readFileSync(path.join(web,'bcc-app.js'),'utf8'),css=fs.readFileSync(path.join(web,'bcc-app.css'),'utf8');
const dash=fs.readFileSync(path.join(web,'..','internal','bcc','dashboard.go'),'utf8');
const ops=dash.slice(dash.indexOf('<script>')+8,dash.indexOf('</script>'));
const cards=app.slice(app.indexOf('/* Server and tunnel cards.'),app.indexOf('\nfunction tunnelsView('));

test('a server name on the Servers page opens its card',()=>{
  assert.match(app,/class="srv-open" data-server="'\+esc\(n\.id\)\+'"/);
  assert.match(app,/querySelectorAll\('\[data-server\]'\)\)b\.addEventListener\('click',\(\)=>openServerCard\(b\.dataset\.server\)\)/);
});

test('every card action is a reviewed operation of the Classic script',()=>{
  const used=new Set([...cards.matchAll(/\['(tn[A-Za-z]+)','c[A-Za-z]+'/g)].map(m=>m[1]));
  for(const m of cards.matchAll(/data-act="(rotateToken|revokeNode)"/g))used.add(m[1]);
  assert.deepEqual([...used].sort(),['revokeNode','rotateToken','tnCancel','tnDecommission','tnDrift','tnEdit','tnRotate']);
  for(const f of used)assert.match(ops,new RegExp('async function '+f+'\\('),f+' is not in the operations script');
});

test('tunnel actions follow the tunnel phase',()=>{
  const done=cards.match(/const TUN_DONE=(\[[^\]]*\])/)[1];
  const expr=cards.match(/const list=(p==='active'[^;]*);/)[1];
  const acts=p=>vm.runInNewContext('const TUN_DONE='+done+';('+expr+').map(a=>a[0]).join()',{p}).split(',').filter(Boolean);
  assert.deepEqual(acts('active'),['tnEdit','tnDrift','tnRotate','tnDecommission']);
  assert.deepEqual(acts('decommission_failed'),['tnDecommission']);
  assert.deepEqual(acts('committing_ir'),['tnCancel']);
  for(const p of ['decommissioned','rolled_back','superseded','rollback_failed'])assert.deepEqual(acts(p),[],p);
});

test('a revoked server or the preview never enables an action',()=>{
  assert.match(cards,/off=!live\|\|n\.revoked/);
  assert.match(cards,/off=!live\|\|n\?\.revoked/);
  assert.equal((cards.match(/\(off\?' disabled':''\)/g)||[]).length,4);
});

test('server values in the cards are escaped',()=>{
  // kv() escapes its values itself; everything else must call esc().
  assert.doesNotMatch(cards.replace(/kv\(\[[\s\S]*?\]\);/g,''),/'\+(x|n|det|c|v)\.[a-z_]+/,'a field is concatenated without esc()');
  assert.match(cards,/function kv\(rows\)\{[^}]*esc\(v\)/);
});

test('operation cards hide only their own title, not the server cards inside them',()=>{
  assert.match(css,/\.ops-card>\.top>b\{display:none\}/);
  assert.doesNotMatch(css,/\.ops-card \.top>b\{display:none\}/);
});

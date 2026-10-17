'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const go=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=go.slice(go.indexOf('<script>')+8,go.indexOf('</script>'));
const card=go.slice(go.indexOf('<div class="card aurora-tunnel-toolbox"'),go.indexOf('<div class="card aurora-tunnel-studio">'));
const library=js.slice(js.indexOf('// Universal Toolbox is backend-driven.'),js.indexOf('// Native BAFT Smart Tunnel Wizard.'));

test('universal engine library is mounted in BCC Aurora, not a separate fake preview',()=>{
 assert.match(card,/id="tlCatalog"/);
 assert.match(card,/id="tlSummary"/);
 assert.match(card,/جعبه‌ابزار همه‌کارهٔ تونل/);
 assert.match(js,/async function loadTunnelEngines\(/);
 assert.match(js,/loadTunnelEngines\(\),loadHealth\(\)/);
 assert.match(go,/\.tlCatalog\{display:grid/);
 assert.match(go,/@media\(max-width:620px\)/);
});
function harness(data,ok=true){
 const grid={innerHTML:'',replaceChildren(){this.innerHTML=''}};
 const summary={textContent:''};
 const requests=[];
 const q=id=>({'#tlCatalog':grid,'#tlSummary':summary})[id];
 const ctx={q,token:()=>'catalog-test-token',
  tnWizardText:(en,fa)=>en,tlPersian:{},document:{documentElement:{lang:'en'}},esc:x=>String(x).replace(/[<>&"']/g,c=>({'<':'&lt;','>':'&gt;','&':'&amp;','"':'&quot;',"'":'&#39;'}[c]))};
 ctx.fetch=async(u,opts)=>{requests.push([u,opts]);return {ok,status:ok?200:503,json:async()=>data}};
 vm.createContext(ctx);vm.runInContext(library,ctx);
 return {ctx,grid,summary,requests};
}
test('catalog requests use authenticated GET without a mutation body or token in the DOM',async()=>{
 const h=harness([{name:'baft',label:'BAFT',family:'direct',deployable:true}]);
 await vm.runInContext('loadTunnelEngines()',h.ctx);
 assert.equal(h.requests.length,1);
 const [url,options]=h.requests[0];
 assert.equal(url,'/api/tunnel-engines');
 assert.equal(options.method,'GET');
 assert.equal(options.headers.Authorization,'Bearer catalog-test-token');
 assert.equal(options.headers['Content-Type'],'application/json');
 assert.equal(options.body,undefined);
 assert.doesNotMatch(h.grid.innerHTML+h.summary.textContent,/catalog-test-token/);
});
test('catalog renders only backend-admitted BAFT actionable, with other engines locked',async()=>{
 const list=[
 {name:'baft',label:'BAFT Secure',family:'direct',direction:'ir_to_ex',deployable:true,description:'Native',requirements:[]},
 {name:'frp',label:'FRP',family:'reverse',direction:'ex_to_ir',deployable:false,description:'Planned',reason:'Not tested',requirements:['two-node E2E']},
 {name:'wss',label:'WSS',family:'carrier',direction:'configurable',deployable:false,description:'Carrier',reason:'Unavailable',requirements:['carrier E2E']}
 ];
 const h=harness(list);await vm.runInContext('loadTunnelEngines()',h.ctx);
 assert.deepEqual(h.requests.map(x=>x[0]),['/api/tunnel-engines']);
 assert.match(h.summary.textContent,/1 trusted engine/);
 assert.match(h.grid.innerHTML,/BAFT Secure/);
 assert.match(h.grid.innerHTML,/FRP/);
 assert.equal((h.grid.innerHTML.match(/onclick="tlPickEngine/g)||[]).length,1);
 assert.equal((h.grid.innerHTML.match(/disabled aria-disabled="true"/g)||[]).length,2);
 assert.match(h.grid.innerHTML,/EX → IR/);
});
test('catalog fetch failure never becomes a guessed working engine',async()=>{
 const h=harness([],false);h.grid.innerHTML='old card';
 await vm.runInContext('loadTunnelEngines()',h.ctx);
 assert.equal(h.grid.innerHTML,'');
 assert.match(h.summary.textContent,/unavailable/);
});

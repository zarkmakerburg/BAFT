// Run: node --test internal/bcc/testdata/tunnel_catalog_race.test.cjs
// Browser-like regression harness for dashboard's actual inline function.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../dashboard.go'),'utf8');
const start=source.indexOf('let tlCatalogRequestSeq=0;');
const end=source.indexOf('// Native BAFT Smart Tunnel Wizard.',start);
assert(start>=0&&end>start,'catalog implementation must be present');
const snippet=source.slice(start,end);
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}}
function harness(){
 let credential='a';
 const pending=[];
 const grid={innerHTML:'',replaceChildren(){this.innerHTML=''}};
 const summary={textContent:''};
 const select={options:[{value:'baft',disabled:false}],value:''};
 const context={
  q:(s)=>({'#tlCatalog':grid,'#tlSummary':summary,'#tnEngine':select})[s],
  token:()=>credential,ah:()=>({Authorization:'Bearer '+credential}),
  fetch:()=>{const d=deferred();pending.push(d);return d.promise},
  tnWizardText:(en)=>en,esc:(x)=>String(x??'').replace(/[&<>"]/g,''),
  tlPersian:{},document:{documentElement:{lang:'en'}},
  console
 };
 vm.createContext(context);
 vm.runInContext(snippet,context);
 const catalog=[{name:'baft',label:'BAFT',deployable:true,family:'direct',direction:'ir_to_ex',description:'native'}];
 const ok=(d,data=catalog)=>d.resolve({ok:true,json:async()=>data});
 return {context,pending,grid,summary,ok,setCredential:(v)=>credential=v};
}
test('late old success never overwrites newer catalog',async()=>{
 const h=harness();const a=h.context.loadTunnelEngines();const b=h.context.loadTunnelEngines();
 h.ok(h.pending[1]);await b;const current=h.grid.innerHTML;
 h.ok(h.pending[0],[]);await a;assert.equal(h.grid.innerHTML,current);
});
test('late old success never restores actions after newer HTTP failure',async()=>{
 const h=harness();const a=h.context.loadTunnelEngines();const b=h.context.loadTunnelEngines();
 h.pending[1].resolve({ok:false,status:503});await b;
 h.ok(h.pending[0]);await a;
 assert.equal(h.grid.innerHTML,'');assert.match(h.summary.textContent,/unavailable/);
});
test('credential rotation invalidates in-flight response',async()=>{
 const h=harness();const a=h.context.loadTunnelEngines();
 h.setCredential('b');h.ok(h.pending[0]);await a;
 assert.equal(h.grid.innerHTML,'');
});
test('network error fails closed',async()=>{
 const h=harness();const a=h.context.loadTunnelEngines();
 h.pending[0].reject(new Error('offline'));await a;
 assert.equal(h.grid.innerHTML,'');assert.match(h.summary.textContent,/unavailable/);
});

test('refresh immediately removes stale actionable cards',async()=>{
 const h=harness();
 const first=h.context.loadTunnelEngines();h.ok(h.pending[0]);await first;
 assert.match(h.grid.innerHTML,/data-engine="baft"/);
 const second=h.context.loadTunnelEngines();
 assert.equal(h.grid.innerHTML,'');
 assert.match(h.summary.textContent,/Checking supported drivers/);
 h.ok(h.pending[1]);await second;
 assert.match(h.grid.innerHTML,/data-engine="baft"/);
});
test('each request uses the token captured at dispatch',async()=>{
 const h=harness();const auth=[];
 h.context.fetch=(_url,options)=>{auth.push(options.headers.Authorization);const d=deferred();h.pending.push(d);return d.promise};
 const a=h.context.loadTunnelEngines();
 h.setCredential('b');
 const b=h.context.loadTunnelEngines();
 assert.deepEqual(auth,['Bearer a','Bearer b']);
 h.ok(h.pending[1]);await b;
 h.ok(h.pending[0]);await a;
 assert.match(h.grid.innerHTML,/data-engine="baft"/);
});

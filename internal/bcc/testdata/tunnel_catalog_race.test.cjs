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
const authStart=source.indexOf('function token(){');
const authEnd=source.indexOf('function ah(){',authStart);
assert(authStart>=0&&authEnd>authStart,'actual token and setToken implementations must be present');
const authSnippet=source.slice(authStart,authEnd);
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}}
function harness(){
 let credential='a',prompted='a',reloads=0;
 const pending=[];
 const grid={innerHTML:'',replaceChildren(){this.innerHTML=''}};
 const summary={textContent:''};
 const select={options:[{value:'baft',disabled:false}],value:''};
 const context={
  q:(s)=>({'#tlCatalog':grid,'#tlSummary':summary,'#tnEngine':select})[s],
  localStorage:{getItem:()=>credential,setItem:(_key,value)=>{credential=value}},
  prompt:()=>prompted,loadAll:()=>{reloads++},
  fetch:()=>{const d=deferred();pending.push(d);return d.promise},
  tnWizardText:(en)=>en,esc:(x)=>String(x??'').replace(/[&<>"]/g,''),
  tlPersian:{},document:{documentElement:{lang:'en'}},
  console
 };
 vm.createContext(context);
 vm.runInContext(authSnippet+'\n'+snippet,context);
 const catalog=[{name:'baft',label:'BAFT',deployable:true,family:'direct',direction:'ir_to_ex',description:'native'}];
 const ok=(d,data=catalog)=>d.resolve({ok:true,json:async()=>data});
 return {context,pending,grid,summary,ok,setCredential:(v)=>credential=v,
  rotateCredential(v){prompted=v;context.setToken()},get reloads(){return reloads}};
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

test('refresh, token rotation, and out-of-order success and failure preserve current catalog',async()=>{
 const h=harness();
 const old=h.context.loadTunnelEngines();
 const refresh=h.context.loadTunnelEngines();
 h.rotateCredential('b');
 assert.equal(h.reloads,1);
 assert.equal(h.grid.innerHTML,'');
 const current=h.context.loadTunnelEngines();
 h.ok(h.pending[2]);await current;
 const cards=h.grid.innerHTML,summary=h.summary.textContent;
 h.ok(h.pending[1],[]);await refresh;
 h.pending[0].reject(new Error('late failure'));await old;
 assert.equal(h.grid.innerHTML,cards);
 assert.equal(h.summary.textContent,summary);
});
test('token change clears rendered cards before scheduling the next reload',async()=>{
 const h=harness();
 const initial=h.context.loadTunnelEngines();h.ok(h.pending[0]);await initial;
 assert.match(h.grid.innerHTML,/data-engine="baft"/);
 h.rotateCredential('b');
 assert.equal(h.grid.innerHTML,'');
 assert.equal(h.summary.textContent,'');
 assert.equal(h.reloads,1);
});
test('logout blocks late responses and signed-out refresh makes no request',async()=>{
 const h=harness();
 const initial=h.context.loadTunnelEngines();h.ok(h.pending[0]);await initial;
 assert.match(h.grid.innerHTML,/data-engine="baft"/);
 const pending=h.context.loadTunnelEngines();
 h.rotateCredential('');
 assert.equal(h.grid.innerHTML,'');
 assert.equal(h.summary.textContent,'');
 await h.context.loadTunnelEngines();
 assert.equal(h.pending.length,2,'signed-out refresh must not make an API request');
 assert.match(h.summary.textContent,/Sign in/);
 h.ok(h.pending[1]);await pending;
 assert.equal(h.grid.innerHTML,'');
 assert.match(h.summary.textContent,/Sign in/);
});
test('rotating away and back to the same token still invalidates the old request',async()=>{
 const h=harness();const pending=h.context.loadTunnelEngines();
 h.rotateCredential('b');h.rotateCredential('a');
 h.ok(h.pending[0]);await pending;
 assert.equal(h.grid.innerHTML,'');
 assert.equal(h.summary.textContent,'');
 assert.equal(h.reloads,2);
});
test('late response body cannot overwrite a newer catalog',async()=>{
 const h=harness(),body=deferred(),reading=deferred();
 const old=h.context.loadTunnelEngines();
 h.pending[0].resolve({ok:true,json:()=>{reading.resolve();return body.promise}});
 await reading.promise;
 const current=h.context.loadTunnelEngines();h.ok(h.pending[1]);await current;
 const cards=h.grid.innerHTML,summary=h.summary.textContent;
 body.resolve([]);await old;
 assert.equal(h.grid.innerHTML,cards);
 assert.equal(h.summary.textContent,summary);
});
for(const change of ['refresh','rotate'])test('final DOM guard rejects a '+change+' during rendering',async()=>{
 const h=harness();let changed=false,current;
 const escape=h.context.esc;
 h.context.esc=value=>{
  if(!changed){changed=true;if(change==='refresh')current=h.context.loadTunnelEngines();else h.rotateCredential('b')}
  return escape(value);
 };
 const old=h.context.loadTunnelEngines();h.ok(h.pending[0]);await old;
 assert.equal(changed,true);
 assert.equal(h.grid.innerHTML,'');
 if(change==='refresh'){
  assert.match(h.summary.textContent,/Checking supported drivers/);
  h.ok(h.pending[1]);await current;
  assert.match(h.grid.innerHTML,/data-engine="baft"/);
 }else assert.equal(h.summary.textContent,'');
});

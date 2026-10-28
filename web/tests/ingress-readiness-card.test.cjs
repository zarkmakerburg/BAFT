'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const src=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=src.slice(src.indexOf('<script>')+8,src.indexOf('</script>'));
function controller(){const a=js.indexOf('let tnIngressReadinessSeq='),b=js.indexOf('async function tnRotate(',a);assert.ok(a>=0&&b>a,'missing readiness controller');return js.slice(a,b)}
function harness(lang='en'){
 const els=new Map(),requests=[];let reply=()=>{throw Error('unexpected API call')};
 const q=key=>{if(!els.has(key))els.set(key,{textContent:'',innerHTML:''});return els.get(key)};
 const c={q,fetch:(url,opts={})=>{requests.push({url,opts});return reply(url,opts)},ah:()=>({Authorization:'Bearer test'}),tnWizardText:(en,fa)=>lang==='fa'?fa:en,console};
 vm.createContext(c);vm.runInContext(controller(),c);
 return {c,q,requests,respond:fn=>reply=fn};
}
const SHA='a'.repeat(64);
const card={tunnel_id:'tun-1',ir_node:'ir-1',config_port:54619,public_port:54619,plan_sha256:SHA};
const ok=body=>async()=>({ok:true,text:async()=>JSON.stringify(body)});

test('markup has an escaped status area and card box, and a per-tunnel action',()=>{
 assert.match(src,/id="tnIngressReadinessState"/);
 assert.match(src,/id="tnIngressReadinessCard"/);
 assert.match(src,/onclick="tnIngressReadiness\(this.dataset.id\)"/);
});
test('card is rendered only for verified Ready, as text, from the authenticated API',async()=>{
 const h=harness();
 h.respond(ok({verified:true,ready:true,card}));
 await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 assert.equal(h.requests[0].url,'/api/ingress/readiness?tunnel_id=tun-1');
 assert.equal(h.requests[0].opts.headers.Authorization,'Bearer test');
 const t=h.q('#tnIngressReadinessCard').textContent;
 assert.match(t,/ir-1/);assert.match(t,/54619/);assert.ok(t.includes(SHA));
 assert.equal(h.q('#tnIngressReadinessCard').innerHTML,'');
 assert.match(h.q('#tnIngressReadinessState').textContent,/Verified/);
});
test('persian strings are used under fa',async()=>{
 const h=harness('fa');
 h.respond(ok({verified:true,ready:true,card}));
 await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 assert.match(h.q('#tnIngressReadinessState').textContent,/تأیید شد/);
 assert.match(h.q('#tnIngressReadinessCard').textContent,/پورت عمومی/);
});
test('not ready, unverified or ready-without-valid-card never shows a card',async()=>{
 const bad=[
  {ready:false,verified:false,reason:'awaiting independent verification',card},
  {ready:true,verified:false,card},
  {ready:true,verified:true},
  {ready:true,verified:true,card:{...card,tunnel_id:'other'}},
  {ready:true,verified:true,card:{...card,plan_sha256:'xyz'}},
  {ready:true,verified:true,card:{...card,public_port:0}},
  {ready:true,verified:true,card:{...card,config_port:'54619'}},
  {ready:'true',verified:true,card},
  null,[],'ready'
 ];
 for(const body of bad){
  const h=harness();h.q('#tnIngressReadinessCard').textContent='stale';
  h.respond(ok(body));
  await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
  assert.equal(h.q('#tnIngressReadinessCard').textContent,'',JSON.stringify(body));
  assert.match(h.q('#tnIngressReadinessState').textContent,/NOT ready/,JSON.stringify(body));
 }
});
test('server reason is shown as text only and HTTP/parse failures show no card',async()=>{
 let h=harness();
 h.respond(ok({ready:false,reason:'<img src=x onerror=alert(1)>'}));
 await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 assert.ok(h.q('#tnIngressReadinessState').textContent.includes('<img src=x onerror=alert(1)>'));
 assert.equal(h.q('#tnIngressReadinessState').innerHTML,'');
 h=harness();h.respond(async()=>({ok:false,text:async()=>'no'}));
 await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 assert.equal(h.q('#tnIngressReadinessCard').textContent,'');
 assert.match(h.q('#tnIngressReadinessState').textContent,/unavailable/);
 h=harness();h.respond(async()=>({ok:true,text:async()=>'{not json'}));
 await vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 assert.equal(h.q('#tnIngressReadinessCard').textContent,'');
});
test('an older slow response cannot overwrite a newer result',async()=>{
 const h=harness(),pending=new Map();
 h.respond(url=>new Promise(r=>pending.set(url,r)));
 const first=vm.runInContext('tnIngressReadiness("tun-1")',h.c);
 const urlFirst=h.requests[0].url;
 const second=vm.runInContext('tnIngressReadiness("tun-2")',h.c);
 pending.get('/api/ingress/readiness?tunnel_id=tun-2')({ok:true,text:async()=>JSON.stringify({ready:false,reason:'new'})});
 await second;
 pending.get(urlFirst)({ok:true,text:async()=>JSON.stringify({verified:true,ready:true,card})});
 await first;
 assert.equal(h.q('#tnIngressReadinessCard').textContent,'');
});

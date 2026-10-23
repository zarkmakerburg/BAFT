'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const src=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=src.slice(src.indexOf('<script>')+8,src.indexOf('</script>'));
const html=src.slice(src.indexOf('<div class="card aurora-tunnel-studio">'),src.indexOf('<div class="card"><div class="top"><b>Change History'));
function controller(){const a=js.indexOf('let tnIngressPreflightJobID='),b=js.indexOf('async function tnRotate(',a);assert.ok(a>=0&&b>a,'missing ingress preflight controller');return js.slice(a,b)}
function harness(port='54619'){
 const els=new Map(),requests=[];let reply=()=>{throw Error('unexpected API call')};
 const q=key=>{if(!els.has(key))els.set(key,{value:key==='#tnClientPort'?port:'',textContent:'',innerHTML:'',hidden:false,scrollIntoView(){this.scrolls=(this.scrolls||0)+1}});return els.get(key)};
 const c={q,fetch:(url,opts={})=>{requests.push({url,opts});return reply(url,opts)},ah:()=>({Authorization:'Bearer test'}),prompt:()=>port,tnWizardText:en=>en,console};
 vm.createContext(c);vm.runInContext(controller(),c);
 return {c,q,requests,respond:fn=>reply=fn};
}
test('studio renders read-only public ingress check and manual receipt refresh',()=>{
 assert.match(html,/id="tnIngressPreflightState"/);
 assert.match(html,/id="tnIngressPreflightRefresh"/);
 assert.match(js,/onclick="tnIngressPreflight\(this.dataset.id\)"/);
});
test('verified active tunnel sends exact read-only preflight request without apply',async()=>{
 const h=harness();
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-123'},read_only:true,ready_to_apply:false})};
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.deepEqual(h.requests.map(x=>x.url),['/api/tunnels?id=tun-active','/api/ingress/preflight']);
 assert.deepEqual(JSON.parse(h.requests[1].opts.body),{ir_node:'ir-1',tunnel_id:'tun-active',public_port:54619,local_target:'127.0.0.1:1443'});
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,false);
 assert.match(h.q('#tnIngressPreflightState').textContent,/read-only/i);
});
test('reserved public SSH port is refused without any mutation request',async()=>{
 const h=harness('22');
 h.respond(async()=>({ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})}));
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(h.requests.length,1);
 assert.match(h.q('#tnIngressPreflightState').textContent,/reserved|22/i);
});

test('read-only receipt displays limitations as text and never claims apply authorization',async()=>{
 const h=harness();
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-123'},read_only:true,ready_to_apply:false})};
  if(url==='/api/ingress/preflight?id=job-123')return {ok:true,text:async()=>JSON.stringify({job_id:'job-123',status:'succeeded',read_only:true,ready_to_apply:false,evidence:{public_port:54619,public_port_state:'free',target_listener_observed:true,ownership_verified:false,firewall_policy_verified:false,ready_to_apply:false,limitations:['<b>not verified</b>']}})};
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 await vm.runInContext('tnIngressPreflightRefresh()',h.c);
 assert.match(h.q('#tnIngressPreflightState').textContent,/NOT approved/);
 assert.match(h.q('#tnIngressPreflightState').textContent,/<b>not verified<\/b>/);
 assert.equal(h.q('#tnIngressPreflightState').innerHTML,'');
});
test('older tunnel GET response cannot schedule a job after a newer selection',async()=>{
 const h=harness(),pending=new Map();
 h.respond(url=>new Promise(resolve=>pending.set(url,resolve)));
 const first=vm.runInContext('tnIngressPreflight("tun-old")',h.c);
 const second=vm.runInContext('tnIngressPreflight("tun-new")',h.c);
 pending.get('/api/tunnels?id=tun-new')({ok:true,json:async()=>({id:'tun-new',phase:'preparing_ex',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})});
 await second;
 pending.get('/api/tunnels?id=tun-old')({ok:true,json:async()=>({id:'tun-old',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})});
 await first;
 assert.equal(h.requests.length,2);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
});
test('response that falsely claims ready-to-apply never reveals a refreshable job',async()=>{
 const h=harness();
 h.respond(async url=>url.startsWith('/api/tunnels?')?
  {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})}:
  {ok:true,text:async()=>JSON.stringify({job:{id:'job-evil'},read_only:true,ready_to_apply:true})});
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
 assert.match(h.q('#tnIngressPreflightState').textContent,/Untrusted/);
});

test('node-card preflight action brings its otherwise distant evidence panel into view',async()=>{
 const h=harness();
 h.respond(async()=>({ok:false}));
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(h.q('#tnIngressPreflightState').scrolls,1);
});

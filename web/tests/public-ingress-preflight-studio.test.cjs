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
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-123',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
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
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-123',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
  if(url==='/api/ingress/preflight?id=job-123')return {ok:true,text:async()=>JSON.stringify({job_id:'job-123',node_id:'ir-1',status:'succeeded',read_only:true,ready_to_apply:false,evidence:{schema_version:1,scope:'agent_network_namespace_snapshot',tunnel_id:'tun-active',local_target:'127.0.0.1:1443',public_port:54619,public_port_state:'not_observed',target_listener_observed:true,ownership_verified:false,firewall_policy_verified:false,ready_to_apply:false,limitations:['<b>not verified</b>']}})};
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

test('read-only receipt refuses wrong selected node or tunnel or public port or loopback route',async()=>{
 const expected={schema_version:1,tunnel_id:'tun-active',public_port:54619,local_target:'127.0.0.1:1443',scope:'agent_network_namespace_snapshot',public_port_state:'not_observed',target_listener_observed:true,ownership_verified:false,firewall_policy_verified:false,ready_to_apply:false,limitations:['manual firewall/ownership checks required']};
 for(const [key,wrong] of [['tunnel_id','tun-other'],['public_port',54620],['local_target','127.0.0.1:2443'],['scope','host_verified']]){
  const h=harness();
  h.respond(async url=>{
   if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
   if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-123',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
   if(url==='/api/ingress/preflight?id=job-123')return {ok:true,text:async()=>JSON.stringify({job_id:'job-123',node_id:'ir-1',status:'succeeded',read_only:true,ready_to_apply:false,evidence:{...expected,[key]:wrong}})};
   throw Error('unexpected '+url);
  });
  await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
  await vm.runInContext('tnIngressPreflightRefresh()',h.c);
  assert.match(h.q('#tnIngressPreflightState').textContent,/mismatch|Untrusted|Incomplete|not authorized|not verified/i,key);
  assert.doesNotMatch(h.q('#tnIngressPreflightState').textContent,/Read-only observation:/,key);
 }
});
test('read-only preflight rejects accepted job bound to a different IR node',async()=>{
 const h=harness();
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-cross-node',node_id:'ir-other'},read_only:true,ready_to_apply:false})};
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
 assert.match(h.q('#tnIngressPreflightState').textContent,/Untrusted|mismatch/i);
});

test('double-click during preflight POST preserves its original refreshable job without duplicate inspection',async()=>{
 const h=harness(),requests=[];
 let acceptPOST;
 h.respond(async url=>{
  requests.push(url);
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return requests.filter(x=>x===url).length===1?new Promise(resolve=>{acceptPOST=resolve}):{ok:false,text:async()=> 'already pending'};
  throw Error('unexpected '+url);
 });
 const first=vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 while(!acceptPOST)await Promise.resolve();
 const second=vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 await second;
 acceptPOST({ok:true,text:async()=>JSON.stringify({job:{id:'job-preserved',node_id:'ir-1'},read_only:true,ready_to_apply:false})});
 await first;
 assert.equal(requests.filter(x=>x==='/api/ingress/preflight').length,1);
 assert.equal(requests.filter(x=>x==='/api/tunnels?id=tun-active').length,1);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,false);
 assert.match(h.q('#tnIngressPreflightState').textContent,/job-preserved/);
});

test('failed read-only inspection submission releases the client-side pending gate for retry',async()=>{
 const h=harness();let posts=0;
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight'){
   posts++;
   return posts===1?{ok:false,text:async()=> 'temporary error'}:{ok:true,text:async()=>JSON.stringify({job:{id:'job-retried',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
  }
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(posts,2);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,false);
 assert.match(h.q('#tnIngressPreflightState').textContent,/job-retried/);
});

test('read-only ingress check refuses mismatched tunnel ID from detail endpoint before POST',async()=>{
 const h=harness();
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-selected')return {ok:true,json:async()=>({id:'tun-other',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  throw Error('unexpected POST for mismatched tunnel');
 });
 await vm.runInContext('tnIngressPreflight("tun-selected")',h.c);
 assert.equal(h.requests.length,1);
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
 assert.match(h.q('#tnIngressPreflightState').textContent,/Only an active tunnel/);
});

test('late older receipt refresh cannot replace newer completed inspection evidence',async()=>{
 const h=harness(),refreshes=[];
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-stable',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
  if(url==='/api/ingress/preflight?id=job-stable')return new Promise(resolve=>refreshes.push(resolve));
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 const older=vm.runInContext('tnIngressPreflightRefresh()',h.c);
 const newer=vm.runInContext('tnIngressPreflightRefresh()',h.c);
 const success={job_id:'job-stable',node_id:'ir-1',read_only:true,ready_to_apply:false,status:'succeeded',evidence:{schema_version:1,scope:'agent_network_namespace_snapshot',tunnel_id:'tun-active',public_port:54619,local_target:'127.0.0.1:1443',public_port_state:'not_observed',target_listener_observed:true,ownership_verified:false,firewall_policy_verified:false,ready_to_apply:false,limitations:['manual checks required']}};
 refreshes[1]({ok:true,text:async()=>JSON.stringify(success)});await newer;
 assert.match(h.q('#tnIngressPreflightState').textContent,/Read-only observation/);
 refreshes[0]({ok:true,text:async()=>JSON.stringify({...success,status:'queued',evidence:undefined})});await older;
 assert.match(h.q('#tnIngressPreflightState').textContent,/Read-only observation/);
});

test('public ingress inspection uses only the wizard client config port and never prompts for a second IR port',async()=>{
 const h=harness('24443');let prompts=0;
 h.c.prompt=()=>{prompts++;return '34567'};
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return {ok:true,text:async()=>JSON.stringify({job:{id:'job-config-port',node_id:'ir-1'},read_only:true,ready_to_apply:false})};
  throw Error('unexpected '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(prompts,0);
 assert.equal(h.requests.length,2);
 assert.equal(JSON.parse(h.requests[1].opts.body).public_port,24443);
});

test('no selected configuration port prevents read-only ingress preflight from guessing or prompting',async()=>{
 const h=harness('');let prompts=0;
 h.c.prompt=()=>{prompts++;return '443'};
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  throw Error('unexpected mutation request '+url);
 });
 await vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 assert.equal(prompts,0);
 assert.equal(h.requests.length,1);
 assert.match(h.q('#tnIngressPreflightState').textContent,/client configuration port|client port|config port|پورت/i);
});

test('changing the canonical client port invalidates an earlier pending IR inspection result',async()=>{
 const h=harness('24443');let finish;
 h.c.tnClearPlan=()=>{};
 // Load the real wizard input invalidation handler in the same controller context.
 const inputChanged=js.slice(js.indexOf('function tnWizardIngressChanged()'),js.indexOf('function tnWizardValidateStep(i)'));
 vm.runInContext(inputChanged,h.c);
 h.respond(async url=>{
  if(url==='/api/tunnels?id=tun-active')return {ok:true,json:async()=>({id:'tun-active',phase:'active',ir_node:'ir-1',route_listen:'127.0.0.1:1443'})};
  if(url==='/api/ingress/preflight')return new Promise(resolve=>{finish=resolve});
  throw Error('unexpected '+url);
 });
 const pending=vm.runInContext('tnIngressPreflight("tun-active")',h.c);
 while(!finish)await Promise.resolve();
 h.q('#tnClientPort').value='35555';
 vm.runInContext('tnWizardIngressChanged()',h.c);
 finish({ok:true,text:async()=>JSON.stringify({job:{id:'job-original-port',node_id:'ir-1'},read_only:true,ready_to_apply:false})});
 await pending;
 assert.equal(h.q('#tnIngressPreflightRefresh').hidden,true);
 assert.doesNotMatch(h.q('#tnIngressPreflightState').textContent,/job-original-port/);
});

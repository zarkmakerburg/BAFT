'use strict';
// Pure logic of the live Quick Tunnel wizard (web/bcc-action-wizard.js).
const test=require('node:test');
const assert=require('node:assert/strict');
const path=require('node:path');
const qt=require(path.resolve(__dirname,'../bcc-action-wizard.js'));

const now=Date.parse('2026-10-31T12:00:00Z');
const live=[
 {id:'t1',phase:'active',ex_node:'ex',ir_node:'ir',port:443,client_port:2053,route_listen:'127.0.0.1:1444',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302'},
 {id:'t2',phase:'rolled_back',ex_node:'ex',ir_node:'ir',port:8443,client_port:2096,route_listen:'127.0.0.1:1445'},
 {id:'t3',phase:'preparing_ir',ex_node:'ex',ir_node:'ir2',port:2087,client_port:2083,route_listen:'127.0.0.1:1444'},
];

test('nodes: online only with a fresh agent report; revoked hidden; roles split; online first',()=>{
 const fresh='2026-10-31T11:58:00Z',stale='2026-10-31T11:50:00Z';
 assert.equal(qt.nodeOnline({agent_seen:fresh},now),true);
 assert.equal(qt.nodeOnline({agent_seen:stale},now),false);
 assert.equal(qt.nodeOnline({agent_seen:fresh,revoked:true},now),false);
 assert.equal(qt.nodeOnline({},now),false);
 assert.equal(qt.nodeOnline({agent_seen:'2026-10-31T13:00:00Z'},now),false,'a clock far in the future is not "online"');
 const g=qt.splitNodes([{id:'b',role:'worker',agent_seen:stale},{id:'a',alias:'Tehran',role:'master',agent_seen:fresh},{id:'x',role:'foreign',agent_seen:fresh},{id:'r',role:'foreign',revoked:true},{id:'n',role:'other'}],now);
 assert.deepEqual(g.iran.map(n=>[n.id,n.name,n.online]),[['a','Tehran',true],['b','b',false]]);
 assert.deepEqual(g.foreign.map(n=>n.id),['x']);
});

test('client port: range, SSH and collisions with live tunnels on the same Iran server',()=>{
 assert.equal(qt.clientPortProblem('2096',live,'ir'),'','rolled-back tunnels free their port');
 assert.equal(qt.clientPortProblem('2053',live,'ir'),'taken');
 assert.equal(qt.clientPortProblem('2083',live,'ir'),'','another IR may use the same port');
 assert.equal(qt.clientPortProblem('2083',live,'ir2'),'taken','in-progress tunnels still hold their port');
 assert.equal(qt.clientPortProblem('22',live,'ir'),'ssh');
 for(const bad of ['0','65536','abc','','1e3',' ','-5','08080x'])assert.equal(qt.clientPortProblem(bad,live,'ir'),'range',bad);
});

test('test ports skip ports live or in-progress tunnels listen on and add the preferred port',()=>{
 // t1 (active) holds 443 and t3 (in progress) holds 2087 on ex; t2 rolled back, so 8443 is free.
 assert.deepEqual(qt.candidatePorts(live,'ex',''),[2053,2083,2096,8443,9443]);
 assert.deepEqual(qt.candidatePorts(live,'ex','31000'),[2053,2083,2096,8443,9443,31000]);
 assert.deepEqual(qt.candidatePorts(live,'ex','443'),[2053,2083,2096,8443,9443],'a taken preferred port is not tested');
 assert.deepEqual(qt.candidatePorts([],'ex','x'),qt.TEST_PORTS);
});

test('internal listeners avoid live tunnels on the same servers only',()=>{
 assert.deepEqual(qt.allocateInternal(live,'ex','ir'),{route_listen:'127.0.0.1:1445',ex_metrics_listen:'127.0.0.1:9203',ir_metrics_listen:'127.0.0.1:9303'});
 assert.deepEqual(qt.allocateInternal(live,'ex2','ir3'),{route_listen:'127.0.0.1:1444',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302'});
 assert.equal(qt.allocateInternal(live,'ex','ir2').route_listen,'127.0.0.1:1445');
});

test('request carries auto-select evidence only when it matches the pair and port',()=>{
 const base={ex:'ex',ir:'ir',port:'443',clientPort:'2096',target:'127.0.0.1:2096',instanceID:'baft-a',internal:{route_listen:'127.0.0.1:1445',ex_metrics_listen:'127.0.0.1:9203',ir_metrics_listen:'127.0.0.1:9303'}};
 const ev={source_node:'ir',destination_node:'ex',port:443,carrier_id:'h2'};
 const r=qt.buildRequest({...base,evidence:ev});
 assert.deepEqual(r,{ex_node:'ex',ir_node:'ir',port:443,client_port:2096,target:'127.0.0.1:2096',instance_id:'baft-a',route_listen:'127.0.0.1:1445',ex_metrics_listen:'127.0.0.1:9203',ir_metrics_listen:'127.0.0.1:9303',auto_select_evidence:ev});
 assert.equal(qt.buildRequest({...base,port:'8443',evidence:ev}).auto_select_evidence,undefined);
 assert.equal(qt.buildRequest({...base,evidence:{...ev,source_node:'other'}}).auto_select_evidence,undefined);
 assert.equal(qt.buildRequest({...base,evidence:null}).auto_select_evidence,undefined);
});

test('targets and instance IDs',()=>{
 assert.equal(qt.targetFor('127.0.0.1','2096'),'127.0.0.1:2096');
 assert.equal(qt.targetFor('::1',443),'[::1]:443');
 assert.equal(qt.targetFor('',80),'127.0.0.1:80');
 for(const ok of ['127.0.0.1','10.0.0.5','::1','2001:db8::7'])assert.equal(qt.validTargetHost(ok),true,ok);
 for(const bad of ['256.1.1.1','example.com','1.2.3','','[::1]','127.0.0.1:80'])assert.equal(qt.validTargetHost(bad),false,bad);
 const id=qt.instanceID(n=>Uint32Array.from({length:n},(_,i)=>[4294967295,123456][i]));
 assert.match(id,/^baft-[a-z0-9]+-[a-z0-9]+$/);
 assert.ok(id.length<=40,'instance_id must fit the 40-char server rule');
});

test('probe rows: only FULL_DATA passes; pending vs untested depends on phase',()=>{
 const rows={FULL_DATA:'ok',BYTE_CEILING:'partial',HANDSHAKE_ONLY:'partial',CONNECT_ONLY:'partial',TIMEOUT_UNREACHABLE:'blocked',LOCAL_CONFLICT:'busy',UNKNOWN:'unknown'};
 for(const [k,v] of Object.entries(rows))assert.equal(qt.probeRow({class:k},'running'),v,k);
 assert.equal(qt.probeRow({},'running'),'pending');
 assert.equal(qt.probeRow({},'complete'),'untested');
 assert.equal(qt.probeMetrics({connect_rtt_ms:41,connect_spread_ms:3,attempts:3,ack_successes:3}),'41 ms ±3 · 3/3');
 assert.equal(qt.probeMetrics({attempts:3}),'0/3');
 assert.equal(qt.probeMetrics({}),'');
});

test('progress never claims more than BCC recorded',()=>{
 let p=qt.progress({phase:'preparing_ex',client_port:2096});
 assert.deepEqual(p.steps,['now','idle','idle','idle','idle','idle']);assert.equal(p.outcome,'working');assert.equal(p.done,false);
 p=qt.progress({phase:'committing_ir',client_port:2096});
 assert.deepEqual(p.steps,['done','done','now','idle','idle','idle']);
 p=qt.progress({phase:'active',client_port:2096,ingress:{state:'queued',ready:false}});
 assert.equal(p.outcome,'active','active tunnel without verified ingress is not "ready"');assert.equal(p.ingressState,'pending');assert.equal(p.done,false);
 p=qt.progress({phase:'active',client_port:2096,ingress:{state:'applied',ready:false,reason:'awaiting independent verification'}});
 assert.equal(p.outcome,'active','an Agent receipt alone is not verification');
 p=qt.progress({phase:'active',client_port:2096,ingress:{state:'applied',ready:true}});
 assert.equal(p.outcome,'ready');assert.equal(p.done,true);assert.ok(p.steps.every(x=>x==='done'));
 p=qt.progress({phase:'active',client_port:2096,ingress:{state:'blocked',ready:false,reason:'signed window passed'}});
 assert.equal(p.outcome,'partial');assert.equal(p.reason,'signed window passed');assert.equal(p.done,true);
 p=qt.progress({phase:'active',client_port:0});
 assert.equal(p.outcome,'ready','no public port requested');
 p=qt.progress({phase:'rolling_back',error:'health check failed'});
 assert.equal(p.outcome,'rolling');assert.equal(p.done,false);assert.equal(p.error,'health check failed');
 assert.equal(qt.progress({phase:'rolled_back'}).outcome,'failed');
 assert.equal(qt.progress({phase:'rollback_failed'}).outcome,'attention');
 assert.equal(qt.progress(null).outcome,'working');
});

test('gates are ordered FAIL, WARN, PASS',()=>{
 assert.deepEqual(qt.gateOrder([{status:'PASS',name:'a'},{status:'FAIL',name:'b'},{status:'WARN',name:'c'},{status:'?',name:'d'}]).map(g=>g.name),['b','c','a','d']);
 assert.deepEqual(qt.gateOrder(null),[]);
});

test('throughput appears only when BCC derived it, in Mbps',()=>{
 assert.equal(qt.probeMetrics({connect_rtt_ms:41,attempts:3,ack_successes:3,throughput_kbps:85400}),'41 ms · 3/3 · ≈ 85 Mbps');
 assert.equal(qt.probeMetrics({throughput_kbps:4200}),'≈ 4.2 Mbps');
 assert.equal(qt.probeMetrics({throughput_kbps:0}),'');
 assert.equal(qt.mbps(10000),'10');
});

test('automatic public port only when the Iran agent freshly advertises operational-ingress-v1',()=>{
 const fresh='2026-10-31T11:58:00Z',stale='2026-10-31T11:50:00Z';
 assert.equal(qt.INGRESS_CAPABILITY,'operational-ingress-v1');
 assert.equal(qt.autoIngressAvailable({agent_seen:fresh,capabilities:['carrier-h2','operational-ingress-v1']},now),true);
 assert.equal(qt.autoIngressAvailable({agent_seen:fresh,capabilities:['carrier-h2']},now),false,'a host agent without the capability');
 assert.equal(qt.autoIngressAvailable({agent_seen:fresh},now),false);
 assert.equal(qt.autoIngressAvailable({agent_seen:stale,capabilities:['operational-ingress-v1']},now),false,'a stale report proves nothing');
 assert.equal(qt.autoIngressAvailable({agent_seen:fresh,revoked:true,capabilities:['operational-ingress-v1']},now),false);
 const g=qt.splitNodes([{id:'lab',role:'worker',agent_seen:fresh,capabilities:['operational-ingress-v1']},{id:'host',role:'worker',agent_seen:fresh,capabilities:['carrier-h2']}],now);
 assert.deepEqual(g.iran.map(n=>[n.id,n.autoIngress]),[['host',false],['lab',true]]);
});

test('safe host mode (operational-ingress-host-v1) is automatic but distinct from the LAB',()=>{
 const fresh='2026-10-31T11:58:00Z',stale='2026-10-31T11:50:00Z';
 assert.equal(qt.INGRESS_HOST_CAPABILITY,'operational-ingress-host-v1');
 assert.equal(qt.ingressMode({agent_seen:fresh,capabilities:['operational-ingress-host-v1']},now),'host');
 assert.equal(qt.ingressMode({agent_seen:fresh,capabilities:['operational-ingress-v1']},now),'lab');
 assert.equal(qt.ingressMode({agent_seen:fresh,capabilities:['carrier-h2']},now),'');
 assert.equal(qt.ingressMode({agent_seen:stale,capabilities:['operational-ingress-host-v1']},now),'','a stale report proves nothing');
 assert.equal(qt.autoIngressAvailable({agent_seen:fresh,capabilities:['operational-ingress-host-v1']},now),true);
 const g=qt.splitNodes([{id:'h',role:'worker',agent_seen:fresh,capabilities:['operational-ingress-host-v1']}],now);
 assert.equal(g.iran[0].ingressMode,'host');
});

test('the per-port acknowledgement is sent only with a client port and only when given',()=>{
 const base={ex:'ex',ir:'ir',port:'443',target:'127.0.0.1:2096',instanceID:'baft-x',internal:{route_listen:'127.0.0.1:1444',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302'}};
 assert.equal(qt.buildRequest({...base,clientPort:'2096',clientPortAck:true}).client_port_ack,true);
 assert.equal('client_port_ack' in qt.buildRequest({...base,clientPort:'2096',clientPortAck:false}),false);
 assert.equal('client_port_ack' in qt.buildRequest({...base,clientPort:'2096'}),false);
 assert.equal('client_port_ack' in qt.buildRequest({...base,clientPort:'',clientPortAck:true}),false,'no acknowledgement without a public port');
 assert.equal('client_port_ack' in qt.buildRequest({...base,clientPort:'2096',clientPortAck:'yes'}),false,'only a real boolean');
});

test('plan request omits client_port when no automatic public port is used',()=>{
 const base={ex:'ex',ir:'ir',port:'443',target:'127.0.0.1:2096',instanceID:'baft-x',internal:{route_listen:'127.0.0.1:1444',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302'}};
 assert.equal(qt.buildRequest({...base,clientPort:'2096'}).client_port,2096);
 for(const none of ['',undefined,null])assert.equal('client_port' in qt.buildRequest({...base,clientPort:none}),false,'clientPort='+none);
 // Without a public port the tunnel is "ready" once active: nothing is left pending forever.
 const p=qt.progress({phase:'active',route_listen:'127.0.0.1:1444'});
 assert.equal(p.outcome,'ready'); assert.equal(p.ingressState,'none'); assert.equal(p.done,true);
});

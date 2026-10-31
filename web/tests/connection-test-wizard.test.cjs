'use strict';
// v0.5.0: on-demand Connection Test in the Smart Tunnel Wizard (C-BAFT-style
// board, built on BAFT's signed path-probe jobs). The board must report what
// the probe actually observed, never promote a partial or pending port to
// "passes", and must hand selection to the evidence-checked auto-select.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const src=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const begin=src.indexOf('/* tnConnTest:begin');
const end=src.indexOf('/* tnConnTest:end */');
assert.ok(begin>=0&&end>begin,'connection test block missing');
const block=src.slice(src.indexOf('\n',begin)+1,end);
const html=src.slice(0,src.indexOf('<script>'));

function el(){return {value:'',textContent:'',innerHTML:'',hidden:false,disabled:false}}

function rig({fields={},responses=[]}={}){
 const els={tnEx:el(),tnIr:el(),tnPort:el(),tnConnFamily:el(),tnConnTestNote:el(),tnConnTestBoard:el(),tnConnTestCancel:el(),tnConnTestStart:el()};
 els.tnConnFamily.value='4';
 for(const [k,v] of Object.entries(fields))els[k].value=v;
 const calls=[],timers=[];let autoSelects=0;
 const ctx={
  q:sel=>els[sel.slice(1)],
  esc:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
  ah:()=>({Authorization:'Bearer t'}),
  tnWizardText:(en)=>en,
  tnAutoSelect:async()=>{autoSelects++},
  encodeURIComponent,
  setTimeout:(fn)=>{timers.push(fn);return timers.length},
  clearTimeout:()=>{},
  fetch:async(url,opts={})=>{calls.push({url,method:opts.method||'GET',body:opts.body?JSON.parse(opts.body):undefined});const res=responses.shift()||{status:500,text:'no response'};return{ok:res.status>=200&&res.status<300,status:res.status,json:async()=>res.json,text:async()=>res.text||''}},
 };
 vm.createContext(ctx);
 vm.runInContext(block,ctx);
 return {ctx,els,calls,timers,get autoSelects(){return autoSelects},run:code=>vm.runInContext(code,ctx)};
}

const probe=(phase,candidates,extra={})=>({id:'pp-1',source_node:'ir-1',destination_node:'ex-1',family:'4',phase,candidates,...extra});

test('wizard markup exposes the connection test controls',()=>{
 for(const id of ['tnConnTestBox','tnConnFamily','tnConnTestStart','tnConnTestCancel','tnConnTestNote','tnConnTestBoard'])assert.ok(html.includes('id="'+id+'"'),id+' missing');
 assert.match(html,/onclick="tnConnTestStart\(\)"/);
 assert.match(html,/it is not a tunnel health check/);
 assert.ok(src.includes('function tnWizardChanged(){if(typeof tnConnTestReset===\'function\')tnConnTestReset();'),'server change must invalidate the board');
});

test('candidate ports include the chosen EX port, stay unique, sorted and within the 16-port API bound',()=>{
 const r=rig();
 assert.deepEqual(Array.from(r.run('tnConnTestCandidates("8443")')),[443,2053,2083,2087,2096,8443,9443]);
 assert.deepEqual(Array.from(r.run('tnConnTestCandidates("31000")')),[443,2053,2083,2087,2096,8443,9443,31000]);
 for(const bad of ['"0"','"65536"','"abc"','""','"-1"'])assert.deepEqual(Array.from(r.run('tnConnTestCandidates('+bad+')')),[443,2053,2083,2087,2096,8443,9443]);
 assert.ok(r.run('tnConnTestCandidates("1")').length<=16);
});

test('start posts a signed-job path probe IR -> EX and refuses a missing or identical pair',async()=>{
 const same=rig({fields:{tnEx:'n1',tnIr:'n1',tnPort:'8443'}});
 await same.run('tnConnTestStart()');
 assert.equal(same.calls.length,0);
 assert.match(same.els.tnConnTestNote.textContent,/two different servers/);

 const r=rig({fields:{tnEx:'ex-1',tnIr:'ir-1',tnPort:'31000'},responses:[{status:202,json:probe('starting_listener',[{port:443},{port:31000}])}]});
 r.els.tnConnFamily.value='6';
 await r.run('tnConnTestStart()');
 assert.equal(r.calls.length,1);
 assert.equal(r.calls[0].url,'/api/path-probes');
 assert.equal(r.calls[0].method,'POST');
 assert.deepEqual(r.calls[0].body,{source_node:'ir-1',destination_node:'ex-1',family:'6',candidate_ports:[443,2053,2083,2087,2096,8443,9443,31000]});
 assert.equal(r.timers.length,1,'polling must be scheduled');
 assert.equal(r.els.tnConnTestStart.disabled,true);
 assert.equal(r.els.tnConnTestCancel.hidden,false);
 assert.match(r.els.tnConnTestBoard.innerHTML,/Waiting/);
});

test('board reports each class truthfully and never shows partial or pending as passing',()=>{
 const r=rig();
 const html=r.run('tnConnTestBoardHTML')(probe('running',[
  {port:443,class:'FULL_DATA',connect_rtt_ms:41},
  {port:2053,class:'BYTE_CEILING',byte_ceiling:16384},
  {port:2083,class:'HANDSHAKE_ONLY'},
  {port:2087,class:'TIMEOUT_UNREACHABLE'},
  {port:2096,class:'LOCAL_CONFLICT'},
  {port:8443},
 ]));
 const rows=html.split('<div class="tnConnRow"').slice(1);
 assert.equal(rows.length,6);
 assert.match(rows[0],/Data passes/);assert.match(rows[0],/41 ms/);
 assert.match(rows[1],/Cut off after 16384 bytes/);
 assert.match(rows[2],/Connects, but data is blocked/);
 assert.match(rows[3],/Blocked or unreachable/);
 assert.match(rows[4],/Port busy/);
 assert.match(rows[5],/Waiting/);
 assert.equal((html.match(/Data passes/g)||[]).length,1,'only FULL_DATA may read as passing');
 assert.doesNotMatch(html,/Best measured port/,'no recommendation before completion');
 const done=r.run('tnConnTestBoardHTML')(probe('complete',[{port:443,class:'FULL_DATA'},{port:8443}],{recommended_port:443}));
 assert.match(done,/Best measured port: <b dir="ltr">443/);
 assert.match(done,/Not tested/,'an untested row after completion is not pending');
});

test('board escapes node-controlled detail',()=>{
 const r=rig();
 const html=r.run('tnConnTestBoardHTML')(probe('running',[{port:'<img src=x onerror=alert(1)>',class:'"><script>'}]));
 assert.doesNotMatch(html,/<img|<script>/);
});

test('completion with a passing port re-runs the evidence-checked auto-select; no pass leaves selection alone',async()=>{
 const r=rig({fields:{tnEx:'ex-1',tnIr:'ir-1',tnPort:'8443'},responses:[
  {status:202,json:probe('queued',[{port:443},{port:8443}])},
  {status:200,json:probe('running',[{port:443,class:'FULL_DATA'},{port:8443}])},
  {status:200,json:probe('complete',[{port:443,class:'FULL_DATA'},{port:8443,class:'TIMEOUT_UNREACHABLE'}],{recommended_port:443})},
 ]});
 await r.run('tnConnTestStart()');
 await r.timers.shift()();
 assert.equal(r.autoSelects,0,'must not select while running');
 assert.equal(r.calls[1].url,'/api/path-probes?id=pp-1');
 await r.timers.shift()();
 assert.equal(r.autoSelects,1);
 assert.equal(r.timers.length,0,'polling stops at a terminal phase');
 assert.match(r.els.tnConnTestNote.textContent,/1 of 2 ports passed real data/);
 assert.equal(r.els.tnConnTestStart.disabled,false);
 assert.equal(r.els.tnConnTestCancel.hidden,true);

 const none=rig({fields:{tnEx:'ex-1',tnIr:'ir-1'},responses:[
  {status:202,json:probe('queued',[{port:443}])},
  {status:200,json:probe('complete',[{port:443,class:'BYTE_CEILING',byte_ceiling:9000}])},
 ]});
 await none.run('tnConnTestStart()');
 await none.timers.shift()();
 assert.equal(none.autoSelects,0,'a partial path must never be selected');
 assert.match(none.els.tnConnTestNote.textContent,/No tested port passed full data/);
});

test('failed probes surface the reason; server changes discard stale results',async()=>{
 const r=rig({fields:{tnEx:'ex-1',tnIr:'ir-1'},responses:[
  {status:202,json:probe('queued',[{port:443}])},
  {status:200,json:probe('failed',[{port:443}],{error:'listener job timed out'})},
 ]});
 await r.run('tnConnTestStart()');
 await r.timers.shift()();
 assert.match(r.els.tnConnTestNote.textContent,/Test failed: listener job timed out/);

 const stale=rig({fields:{tnEx:'ex-1',tnIr:'ir-1'},responses:[
  {status:202,json:probe('queued',[{port:443}])},
  {status:200,json:probe('complete',[{port:443,class:'FULL_DATA'}])},
 ]});
 await stale.run('tnConnTestStart()');
 const pending=stale.timers.shift();
 stale.run('tnConnTestReset()');
 await pending();
 assert.equal(stale.calls.length,1,'a reset test must not keep polling');
 assert.equal(stale.autoSelects,0);
 assert.equal(stale.els.tnConnTestBoard.innerHTML,'');
});

test('refused start reports BCC text and re-enables the button; cancel posts the probe id',async()=>{
 const refused=rig({fields:{tnEx:'ex-1',tnIr:'ir-1'},responses:[{status:400,text:'destination node is not an active enrolled node'}]});
 await refused.run('tnConnTestStart()');
 assert.match(refused.els.tnConnTestNote.textContent,/Test not started: destination node is not an active enrolled node/);
 assert.equal(refused.els.tnConnTestStart.disabled,false);
 assert.equal(refused.timers.length,0);

 const c=rig({fields:{tnEx:'ex-1',tnIr:'ir-1'},responses:[
  {status:202,json:probe('running',[{port:443}])},
  {status:202,json:probe('cancelled',[{port:443}])},
 ]});
 await c.run('tnConnTestStart()');
 await c.run('tnConnTestCancel()');
 assert.deepEqual(c.calls[1],{url:'/api/path-probes/cancel',method:'POST',body:{id:'pp-1'}});
 assert.match(c.els.tnConnTestNote.textContent,/Test cancelled/);
});

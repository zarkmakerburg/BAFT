'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const src=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=src.slice(src.indexOf('<script>')+8,src.indexOf('</script>'));
function fn(from,to){const start=js.indexOf(from),end=js.indexOf(to,start);assert.ok(start>=0&&end>start);return js.slice(start,end)}
function context(fields,prelude){
 const c={q:selector=>({get value(){return fields[selector.slice(1)]},set value(v){fields[selector.slice(1)]=v},get checked(){return fields[selector.slice(1)]},set checked(v){fields[selector.slice(1)]=v},get textContent(){return fields[selector.slice(1)]},set textContent(v){fields[selector.slice(1)]=v},get innerHTML(){return fields[selector.slice(1)]},set innerHTML(v){fields[selector.slice(1)]=v},scrollIntoView(){}}),document:{documentElement:{lang:'fa'}},alert(){},tnClearPlan(){},console};
 vm.createContext(c);vm.runInContext(fn('function tnWizardTargetChanged()','function tnWizardGenerateServicePort()')+prelude,c);return c;
}
const req=fn('function tnReq()','function tnModeChanged()');
const defaults={tnDirection:'ir-to-ex',tnEngine:'baft',tnEx:'ex-de-1',tnIr:'ir-arno-1',tnPort:'8444',tnTarget:'127.0.0.1:2443',tnMode:'independent',tnInstance:'baft-arno-de-2',tnRouteListen:'127.0.0.1:1444',tnExMetric:'127.0.0.1:9202',tnIrMetric:'127.0.0.1:9302',tnLegacyCoexist:true,tnClientPort:'443'};
test('wizard really submits isolated BAFT backend fields without application or public ingress settings',()=>{
 const f={...defaults};
 const c=context(f,'let tnEditBase=null;'+req);
 const result=JSON.parse(JSON.stringify(vm.runInContext('tnReq()',c)));
 assert.deepEqual(result,{ex_node:'ex-de-1',ir_node:'ir-arno-1',port:8444,target:'127.0.0.1:2443',instance_id:'baft-arno-de-2',route_listen:'127.0.0.1:1444',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302',allow_legacy_coexistence:true});
});
test('editing a selected managed instance preserves its backend identity',()=>{
 const old={instance_id:'existing-managed',ex_node:'ex-de-1',ir_node:'ir-arno-1',public_address:'203.0.113.5',port:8443,target:'127.0.0.1:2443',route_id:'service-main',route_listen:'127.0.0.1:1443',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302',record_shaping:true};
 const c=context({...defaults,tnPort:'2053',tnTarget:'127.0.0.1:3443'},'let tnEditBase='+JSON.stringify(old)+';'+req);
 const result=JSON.parse(JSON.stringify(vm.runInContext('tnReq()',c)));
 assert.equal(result.port,2053);
 assert.equal(result.target,'127.0.0.1:3443');
 assert.equal(result.instance_id,old.instance_id);
 assert.equal(result.public_address,old.public_address);
 assert.equal(result.route_id,old.route_id);
 assert.equal(result.record_shaping,true);
});
for(const target of ['127.0.0.1:30000','192.0.2.15:5432','[2001:db8::15]:8443'])test('generic destination survives request creation: '+target,()=>{
 const fields={...defaults,tnTarget:target};
 const c=context(fields,'let tnEditBase=null;'+req);
 const result=vm.runInContext('tnReq()',c);
 assert.equal(result.target,target);
 assert.equal(result.port,8444);
});
for(const instance of ['', 'existing-managed'])test('editing '+(instance?'managed':'legacy')+' tunnel keeps non-loopback IPv6 target reviewable',async()=>{
 const fields={...defaults};
 const old={id:'tun-existing',phase:'active',instance_id:instance,ex_node:'ex-de-1',ir_node:'ir-arno-1',public_address:'203.0.113.5',port:2053,target:'[2001:db8::15]:5432',route_id:'service-main',route_listen:'127.0.0.1:1443'};
 const c=context(fields,'let tnEditBase=null,tnPlanEpoch=0;const tnWizardText=(en,fa)=>fa;'+req+fn('async function tnEdit(id)','function tnEditEnd()')+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 c.fetch=async()=>({ok:true,json:async()=>old});c.ah=()=>({});c.esc=String;c.tnModeChanged=()=>{};c.tnWizardStep=()=>{};
 await vm.runInContext('tnEdit("tun-existing")',c);
 assert.equal(fields.tnTarget,old.target);
 assert.equal(fields.tnPort,old.port);
 fields.tnIngressConfirm=true;
 fields.tnClientPort='443'; // Edit resets client port; operator must select it again.
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
 const result=vm.runInContext('tnReq()',c);
 assert.equal(result.target,old.target);
 assert.equal(result.instance_id,instance);
 assert.equal(result.route_id,old.route_id);
});
test('review requires a valid BAFT port; empty destination uses the configured default',()=>{
 const fields={...defaults,tnIngressConfirm:true};
 const c=context(fields,'const tnWizardText=(en,fa)=>fa;'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 for(const port of ['', '0', '65536', '8.5']){
  fields.tnPort=port;assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 }
 fields.tnPort='8444';
 fields.tnTarget='  ';assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
 fields.tnTarget='[::1]:5432';assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
});
test('public IR ingress acknowledgement is required before review',()=>{
 const fields={...defaults,tnIngressConfirm:false};
 const c=context(fields,'const tnWizardText=(en,fa)=>fa;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 fields.tnIngressConfirm=true;
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
});

function portHarness(fields={},inventory=[],samples=[0]){
 const values={...defaults,...fields};let calls=0,clears=0;
 const c=context(values,'const tnWizardText=en=>en;let tnWizardTunnels='+JSON.stringify(inventory)+';'+fn('function tnWizardGenerateServicePort()','function tnWizardIngressChanged()')+fn('function tnWizardActive(t)','function tnWizardSharesLegacy()'));
 c.crypto={getRandomValues(bytes){const next=samples[calls++%samples.length];if(next instanceof Error)throw next;bytes[0]=next;return bytes}};
 c.tnClearPlan=()=>{clears++};
 return {c,values,generate:()=>vm.runInContext('tnWizardGenerateServicePort()',c),get calls(){return calls},get clears(){return clears}};
}
test('port suggestion avoids current and node inventory conflicts while preserving IPv6 host',()=>{
 const h=portHarness({tnPort:'20001',tnExMetric:'127.0.0.1:20003',tnTarget:'[2001:db8::15]:20004'},[
  {ex_node:'ex-de-1',phase:'active',port:2053,target:'192.0.2.8:20002',ex_metrics_listen:'127.0.0.1:20005'},
  {ex_node:'ex-other',phase:'active',target:'127.0.0.1:20006'},
  {ir_node:'ex-de-1',phase:'active',route_listen:'127.0.0.1:20007',ir_metrics_listen:'[::1]:20008'}
 ],[1,2,3,4,5,7,8,6]);
 const before={...h.values};h.generate();
 assert.equal(h.values.tnTarget,'[2001:db8::15]:20006');
 assert.equal(h.calls,8);assert.equal(h.clears,1);
 for(const id of ['tnPort','tnRouteListen','tnExMetric','tnIrMetric','tnInstance','tnClientPort'])assert.equal(h.values[id],before[id],id);
 assert.match(h.values.tnServicePortHint,/Suggested 20006/);
});
for(const phase of ['superseded','decommissioned','rolled_back'])test('released '+phase+' inventory does not reserve a suggestion',()=>{
 const h=portHarness({},[{ex_node:'ex-de-1',phase,port:20001,target:'127.0.0.1:20001'}],[1]);h.generate();
 assert.equal(h.values.tnTarget,'127.0.0.1:20001');assert.equal(h.calls,1);
});
test('failed decommission still reserves resources until they are proven released',()=>{
 const h=portHarness({},[{ex_node:'ex-de-1',phase:'decommission_failed',target:'127.0.0.1:20001'}],[1,2]);h.generate();
 assert.equal(h.values.tnTarget,'127.0.0.1:20002');assert.equal(h.calls,2);
});
test('explicit suggestion can fill an empty destination without a silent default',()=>{
 const h=portHarness({tnTarget:''},[],[39999]);assert.equal(h.values.tnTarget,'');h.generate();
 assert.equal(h.values.tnTarget,'127.0.0.1:59999');assert.equal(h.clears,1);
});
test('invalid destination text is preserved without consuming randomness',()=>{
 const h=portHarness({tnTarget:'https://example.invalid:443'});h.generate();
 assert.equal(h.values.tnTarget,'https://example.invalid:443');assert.equal(h.calls,0);assert.equal(h.clears,0);
 assert.match(h.values.tnServicePortHint,/fixed destination IP:port/);
});
test('suggestion requires a selected foreign node',()=>{
 const h=portHarness({tnEx:''});h.generate();assert.equal(h.calls,0);assert.equal(h.clears,0);
 assert.match(h.values.tnServicePortHint,/Choose the foreign server/);
});
test('secure randomness failure leaves the chosen destination and reviewed plan intact',()=>{
 const h=portHarness({},[],[new Error('unavailable')]),target=h.values.tnTarget;h.generate();
 assert.equal(h.values.tnTarget,target);assert.equal(h.clears,0);
 assert.match(h.values.tnServicePortHint,/Secure port suggestion unavailable/);
});
test('random sampling rejects the biased tail before choosing a port',()=>{
 const h=portHarness({},[],[0xffffffff,0]);h.generate();
 assert.equal(h.values.tnTarget,'127.0.0.1:20000');assert.equal(h.calls,2);
});
test('port suggestion has a bounded retry budget and preserves input on exhaustion',()=>{
 const h=portHarness({tnPort:'20001'},[],[1]),target=h.values.tnTarget;h.generate();
 assert.equal(h.calls,64);assert.equal(h.values.tnTarget,target);assert.equal(h.clears,0);
 assert.match(h.values.tnServicePortHint,/Could not suggest a port/);
});
test('manual target edit clears an obsolete suggestion hint and invalidates the plan',()=>{
 const h=portHarness({tnTarget:'192.0.2.8:53001',tnServicePortHint:'Old suggestion'});
 vm.runInContext('tnWizardTargetChanged()',h.c);
 assert.equal(h.values.tnTarget,'192.0.2.8:53001');assert.equal(h.values.tnServicePortHint,'');assert.equal(h.clears,1);
});
test('reverse topology fails before plan creation; engine is fixed at baft by hidden input',()=>{
 const fields={...defaults,tnDirection:'ex-to-ir'};
 const c=context(fields,'let tnEditBase=null;const tnWizardText=en=>en;'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 assert.equal(vm.runInContext('tnWizardValidateStep(0)',c),false);
 fields.tnDirection='ir-to-ex';assert.equal(vm.runInContext('tnWizardValidateStep(0)',c),true);
});
test('new tunnel resets direction and engine and requires an explicit destination',()=>{
 const fields={...defaults,tnDirection:'ex-to-ir',tnEngine:'frp',tnServicePortHint:'Old suggestion'};
 const c=context(fields,'let tnEditBase={};'+fn('function tnNew()','async function tnEdit(id)'));
 c.tnModeChanged=()=>{};c.tnWizardStep=()=>{};vm.runInContext('tnNew()',c);
 assert.equal(fields.tnDirection,'ir-to-ex');assert.equal(fields.tnEngine,'baft');
 assert.equal(fields.tnTarget,'');assert.equal(fields.tnServicePortHint,'');
});

test('blocked backend plan cannot leave a deployable plan hash',()=>{
 const fields={tnPlan:''};const c=context(fields,'let tnPlanHash="stale",tnPlanReq={stale:true},tnPlanReplaces="old",tnPlanEpoch=0;const esc=String;'+fn('function tnShowPlan(p)','function tnClearPlan()'));
 vm.runInContext('tnShowPlan({ok:false,hash:"blocked",request:{},gates:[],nodes:[],steps:[],verify:[],rollback:[]})',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
});

test('stale async plan response cannot restore deploy state after user edits',async()=>{
 const fields={...defaults,tnPlan:'',tnIngressConfirm:true};let resolveFetch;
 const c=context(fields,'const esc=String;let tnEditBase=null;let tnWizardTunnels=[];const tnWizardText=en=>en;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+req+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)')+'let tnPlanHash="",tnPlanReq=null,tnPlanReplaces="",tnPlanEpoch=0;'+fn('function tnShowPlan(p)','async function tnDeploy()'));
 c.fetch=()=>new Promise(resolve=>{resolveFetch=resolve});c.ah=()=>({});c.loadAudit=()=>{};
 const pending=vm.runInContext('tnPlan()',c);
 vm.runInContext('tnClearPlan()',c);
 resolveFetch({ok:true,text:async()=>JSON.stringify({ok:true,hash:'stale-hash',request:{},gates:[],nodes:[],steps:[],verify:[],rollback:[]})});
 await pending;
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
});

test('starting a new backend plan invalidates previously deployable plan immediately',async()=>{
 const fields={...defaults,tnPlan:'old rendered plan',tnIngressConfirm:true};let resolveFetch;
 const c=context(fields,'const esc=String;let tnEditBase=null;let tnWizardTunnels=[];const tnWizardText=en=>en;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+req+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)')+'let tnPlanHash="old-hash",tnPlanReq={old:true},tnPlanReplaces="",tnPlanEpoch=0;'+fn('function tnShowPlan(p)','async function tnDeploy()'));
 c.fetch=()=>new Promise(resolve=>{resolveFetch=resolve});c.ah=()=>({});c.loadAudit=()=>{};
 const pending=vm.runInContext('tnPlan()',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
 resolveFetch({ok:false,text:async()=> 'unavailable'});await pending;
});
test('double-click deploy submits only one backend mutation while pending',async()=>{
 const fields={...defaults,tnPlan:''};let resolveFetch;let calls=0;
 const c=context(fields,'let tnPlanHash="valid",tnPlanReq={instance_id:"test"},tnPlanReplaces="",tnPlanEpoch=0,tnDeployPending=false;'+fn('async function tnDeploy()','async function tnRotate(id)'));
 c.confirm=()=>true;c.ah=()=>({});c.fetch=()=>{calls++;return new Promise(resolve=>{resolveFetch=resolve})};c.tnClearPlan=()=>{};c.tnEditEnd=()=>{};c.loadTunnels=()=>{};c.loadAudit=()=>{};
 const first=vm.runInContext('tnDeploy()',c);
 const second=vm.runInContext('tnDeploy()',c);
 assert.equal(calls,1);
 resolveFetch({ok:true,status:200,text:async()=>'{"id":"job"}'});
 await Promise.all([first,second]);
});
test('stale deploy conflict response cannot resurrect plan after form change',async()=>{
 const fields={...defaults,tnPlan:''};let resolveFetch;
 const c=context(fields,'const esc=String;let tnPlanHash="reviewed",tnPlanReq={instance_id:"test"},tnPlanReplaces="",tnPlanEpoch=0,tnDeployPending=false;'+fn('function tnShowPlan(p)','async function tnRotate(id)'));
 c.confirm=()=>true;c.ah=()=>({});c.fetch=()=>new Promise(resolve=>{resolveFetch=resolve});c.loadAudit=()=>{};c.loadTunnels=()=>{};c.tnEditEnd=()=>{};
 const pending=vm.runInContext('tnDeploy()',c);
 vm.runInContext('tnClearPlan()',c);
 resolveFetch({ok:false,status:409,text:async()=>JSON.stringify({error:'stale',plan:{ok:true,hash:'outdated',request:{},gates:[],nodes:[],steps:[],verify:[],rollback:[]}})});
 await pending;
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
});

test('Stop editing discards the reviewed replacement plan and its credentials',()=>{
 const fields={...defaults,tnPlan:'reviewed replacement'};
 const c=context(fields,'let tnEditBase={instance_id:"old"},tnPlanHash="replacement",tnPlanReq={instance_id:"old"},tnPlanReplaces="tun-old",tnPlanEpoch=0;'+fn('function tnEditEnd()','function tnShowPlan(p)')+fn('function tnClearPlan()','async function tnPlan()'));
 vm.runInContext('tnEditEnd()',c);
 assert.equal(vm.runInContext('tnEditBase',c),null);
 assert.equal(fields.tnTarget,'');
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
 assert.equal(vm.runInContext('tnPlanReplaces',c),'');
});

test('choosing Edit invalidates previous deployable plan before detail request returns',async()=>{
 const fields={...defaults,tnPlan:'previous plan'};
 let resolveFetch;
 const c=context(fields,'let tnEditBase=null,tnPlanHash="old-plan",tnPlanReq={old:true},tnPlanReplaces="",tnPlanEpoch=0;'+fn('async function tnEdit(id)','function tnEditEnd()')+fn('function tnClearPlan()','async function tnPlan()'));
 c.ah=()=>({});c.fetch=()=>new Promise(resolve=>{resolveFetch=resolve});
 const pending=vm.runInContext('tnEdit("tun-selected")',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
 resolveFetch({ok:false,text:async()=> 'unable to load'});
 await pending;
});

test('late Edit response never overwrites a newer selected tunnel',async()=>{
 const fields={...defaults,tnTarget:'not yet edited'};
 const inflight=new Map();
 const c=context(fields,'let tnEditBase=null,tnPlanHash="",tnPlanReq=null,tnPlanReplaces="",tnPlanEpoch=0;const esc=String;'+fn('async function tnEdit(id)','function tnEditEnd()')+fn('function tnClearPlan()','async function tnPlan()'));
 c.ah=()=>({});c.tnModeChanged=()=>{};c.tnWizardStep=()=>{};
 c.fetch=url=>new Promise(resolve=>inflight.set(decodeURIComponent(url.split('id=')[1]),resolve));
 const older=vm.runInContext('tnEdit("tun-old")',c);
 const newer=vm.runInContext('tnEdit("tun-new")',c);
 const tunnel=(id,target)=>({id:'tun-'+id,phase:'active',instance_id:id,ex_node:'ex-de-1',ir_node:'ir-arno-1',port:8443,target});
 inflight.get('tun-new')({ok:true,json:async()=>tunnel('new','192.0.2.20:4000')});
 await newer;
 inflight.get('tun-old')({ok:true,json:async()=>tunnel('old','192.0.2.20:3000')});
 await older;
 assert.equal(fields.tnTarget,'192.0.2.20:4000');
 assert.equal(vm.runInContext('tnEditBase.instance_id',c),'new');
});

test('new independent Wizard plan cannot replace a tunnel unless Edit explicitly selected it',()=>{
 const fields={tnPlan:''};
 const c=context(fields,'let tnEditBase=null,tnEditSelectedID="",tnPlanHash="",tnPlanReq=null,tnPlanReplaces="";const esc=String;'+fn('function tnShowPlan(p)','function tnClearPlan()'));
 const p={ok:true,hash:'backend-ready',request:{instance_id:'reused'},replaces:'tun-active',gates:[],nodes:[],steps:[],verify:[],rollback:[]};
 vm.runInContext('tnShowPlan('+JSON.stringify(p)+')',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
 assert.match(fields.tnPlan,/disabled onclick="tnDeploy\(\)"/);
 assert.match(fields.tnPlan,/Choose Edit on that exact tunnel/);
});

test('review may replace only the precise selected Edit tunnel, not a different active tunnel',()=>{
 const fields={tnPlan:''};
 const c=context(fields,'let tnEditBase={instance_id:"managed"},tnEditSelectedID="tun-first",tnPlanHash="",tnPlanReq=null,tnPlanReplaces="";const esc=String;'+fn('function tnShowPlan(p)','function tnClearPlan()'));
 const p={ok:true,hash:'valid',request:{instance_id:'managed'},replaces:'tun-other',gates:[],nodes:[],steps:[],verify:[],rollback:[]};
 vm.runInContext('tnShowPlan('+JSON.stringify(p)+')',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 p.replaces='tun-first';vm.runInContext('tnShowPlan('+JSON.stringify(p)+')',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'valid');
 assert.equal(vm.runInContext('tnPlanReplaces',c),'tun-first');
 assert.doesNotMatch(fields.tnPlan,/disabled onclick="tnDeploy\(\)"/);
});

test('changing from Edit to new mode revokes replacement selection',()=>{
 const fields={...defaults,tnMode:'replace'};
 const c=context(fields,'let tnEditBase={instance_id:"managed"},tnEditSelectedID="tun-first";'+fn('function tnModeChanged()','function tnNew()'));
 vm.runInContext('tnModeChanged()',c);
 assert.equal(vm.runInContext('tnEditBase',c),null);
 assert.equal(vm.runInContext('tnEditSelectedID',c),'');
});

test('Edit review refuses a plan that no longer replaces the selected active tunnel',()=>{
 const fields={tnPlan:''};
 const c=context(fields,'let tnEditBase={instance_id:"managed"},tnEditSelectedID="tun-first",tnPlanHash="",tnPlanReq=null,tnPlanReplaces="";const esc=String;'+fn('function tnShowPlan(p)','function tnClearPlan()'));
 const p={ok:true,hash:'ready-but-unrelated',request:{instance_id:'managed'},replaces:'',gates:[],nodes:[],steps:[],verify:[],rollback:[]};
 vm.runInContext('tnShowPlan('+JSON.stringify(p)+')',c);
 assert.equal(vm.runInContext('tnPlanHash',c),'');
 assert.equal(vm.runInContext('tnPlanReq',c),null);
 assert.match(fields.tnPlan,/disabled onclick="tnDeploy\(\)"/);
 assert.match(fields.tnPlan,/no longer the target/);
});

test('Edit refuses non-active or mismatched tunnel API response without selecting a replacement',async()=>{
 for(const result of [
  {id:'tun-other',phase:'active',instance_id:'managed'},
  {id:'tun-selected',phase:'decommission_failed',instance_id:'managed'},
  {id:'tun-selected',phase:'rolled_back',instance_id:'managed'},
  null
 ]){
  const fields={...defaults,tnTarget:'127.0.0.1:9000',tnPlan:''};
  let alerted=0;
  const c=context(fields,'let tnEditBase=null,tnEditSelectedID="",tnPlanHash="",tnPlanReq=null,tnPlanReplaces="",tnPlanEpoch=0;'+fn('async function tnEdit(id)','function tnEditEnd()')+fn('function tnClearPlan()','async function tnPlan()'));
  c.ah=()=>({});c.alert=()=>{alerted++};
  c.fetch=async()=>({ok:true,json:async()=>result});c.esc=String;
  c.tnWizardStep=()=>{};c.tnModeChanged=()=>{};
  await vm.runInContext('tnEdit("tun-selected")',c);
  assert.equal(vm.runInContext('tnEditBase',c),null);
  assert.equal(vm.runInContext('tnEditSelectedID',c),'');
  assert.equal(fields.tnTarget,'127.0.0.1:9000');
  assert.equal(alerted,1);
 }
});

test('Wizard requires one explicit usable IR client configuration port before reviewed planning',()=>{
 const fields={...defaults,tnIngressConfirm:true};
 const c=context(fields,'const tnWizardText=en=>en;'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 for(const input of ['', '0', '22', '65536', '8.5', '  ', '0x50']){
  fields.tnClientPort=input;
  assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false,'should refuse client config port '+input);
 }
 for(const port of ['443', '2053', '24443']){
  fields.tnClientPort=port;
  assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true,'valid free candidate '+port);
 }
});

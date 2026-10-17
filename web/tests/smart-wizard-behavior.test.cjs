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
 const c={q:selector=>({get value(){return fields[selector.slice(1)]},set value(v){fields[selector.slice(1)]=v},get checked(){return fields[selector.slice(1)]},set checked(v){fields[selector.slice(1)]=v},scrollIntoView(){}}),document:{documentElement:{lang:'fa'}},alert(){},tnClearPlan(){},console};
 vm.createContext(c);vm.runInContext(prelude,c);return c;
}
const req=fn('function tnReq()','function tnModeChanged()');
const defaults={tnEx:'ex-de-1',tnIr:'ir-arno-1',tnPort:'8444',tnTarget:'127.0.0.1:2443',tnMode:'independent',tnInstance:'baft-arno-de-2',tnRouteListen:'127.0.0.1:1444',tnExMetric:'127.0.0.1:9202',tnIrMetric:'127.0.0.1:9302',tnLegacyCoexist:true,tnClientPort:'443'};
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
 const old={instance_id:instance,ex_node:'ex-de-1',ir_node:'ir-arno-1',public_address:'203.0.113.5',port:2053,target:'[2001:db8::15]:5432',route_id:'service-main',route_listen:'127.0.0.1:1443'};
 const c=context(fields,'let tnEditBase=null;const tnWizardText=(en,fa)=>fa;'+req+fn('async function tnEdit(id)','function tnEditEnd()')+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 c.fetch=async()=>({ok:true,json:async()=>old});c.ah=()=>({});c.esc=String;c.tnModeChanged=()=>{};c.tnWizardStep=()=>{};
 await vm.runInContext('tnEdit("tun-existing")',c);
 assert.equal(fields.tnTarget,old.target);
 assert.equal(fields.tnPort,old.port);
 fields.tnIngressConfirm=true;
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
 const result=vm.runInContext('tnReq()',c);
 assert.equal(result.target,old.target);
 assert.equal(result.instance_id,instance);
 assert.equal(result.route_id,old.route_id);
});
test('review requires an explicit service destination and valid BAFT port',()=>{
 const fields={...defaults,tnIngressConfirm:true};
 const c=context(fields,'const tnWizardText=(en,fa)=>fa;'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 for(const port of ['', '0', '65536', '8.5']){
  fields.tnPort=port;assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 }
 fields.tnPort='8444';
 fields.tnTarget='  ';assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 fields.tnTarget='[::1]:5432';assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
});
test('public IR ingress acknowledgement is required before review',()=>{
 const fields={...defaults,tnIngressConfirm:false};
 const c=context(fields,'const tnWizardText=(en,fa)=>fa;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 fields.tnIngressConfirm=true;
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
});

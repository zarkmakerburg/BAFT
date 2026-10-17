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
 const c={q:selector=>({get value(){return fields[selector.slice(1)]},set value(v){fields[selector.slice(1)]=v},get checked(){return fields[selector.slice(1)]},set checked(v){fields[selector.slice(1)]=v},get textContent(){return fields[selector.slice(1)]},set textContent(v){fields[selector.slice(1)]=v}}),document:{documentElement:{lang:'fa'}},alert(){},tnClearPlan(){},console};
 vm.createContext(c);vm.runInContext(prelude,c);return c;
}
const req=fn('function tnReq()','function tnModeChanged()');
const defaults={tnEx:'ex-de-1',tnIr:'ir-arno-1',tnPort:'8444',tnXrayPort:'2443',tnTarget:'127.0.0.1:2443',tnMode:'independent',tnInstance:'baft-arno-de-2',tnRouteListen:'127.0.0.1:1444',tnExMetric:'127.0.0.1:9202',tnIrMetric:'127.0.0.1:9302',tnLegacyCoexist:true,tnUserProtocol:'VLESS',tnClientPort:'443'};
test('wizard really submits isolated BAFT backend fields, never unprovisioned Xray/IR ingress',()=>{
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
test('actual Xray destination port edits the BAFT EX target, not BAFT outer port',()=>{
 const fields={...defaults,tnXrayPort:'30000'};
 const c=context(fields,'let tnClearPlan=()=>{};'+fn('function tnWizardTargetSync()','function tnWizardIngressChanged()'));
 vm.runInContext('tnWizardTargetSync()',c);
 assert.equal(fields.tnTarget,'127.0.0.1:30000');
 assert.equal(fields.tnPort,'8444');
});
test('public IR ingress acknowledgement is required before review',()=>{
 const fields={...defaults,tnIngressConfirm:false};
 const c=context(fields,'const tnWizardText=(en,fa)=>fa;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),false);
 fields.tnIngressConfirm=true;
 assert.equal(vm.runInContext('tnWizardValidateStep(2)',c),true);
});
test('random Xray port avoids active EX inventory and maps only EX service target',()=>{
 const fields={...defaults,tnXrayPort:'2443',tnPort:'20001'};
 const c=context(fields,
  'let tnClearPlan=()=>{};const tnWizardText=(en,fa)=>fa;'+
  'const tnWizardActive=t=>t.phase==="active";'+
  'let tnWizardTunnels='+JSON.stringify([
    {ex_node:'ex-de-1',phase:'active',port:2053,target:'127.0.0.1:20002',ex_metrics_listen:'127.0.0.1:9202'},
    {ex_node:'ex-other',phase:'active',target:'127.0.0.1:20003'}
  ])+';'+
  fn('function tnWizardTargetSync()','function tnWizardIngressChanged()'));
 let index=0;
 const samples=[1,2,4]; // 20001 used as BAFT port, 20002 used Xray port, 20004 accepted.
 c.crypto={getRandomValues(bytes){bytes[0]=samples[index++];return bytes}};
 vm.runInContext('tnWizardGenerateXrayPort()',c);
 assert.equal(fields.tnXrayPort,'20004');
 assert.equal(fields.tnTarget,'127.0.0.1:20004');
 assert.equal(fields.tnPort,'20001');
 assert.match(fields.tnXrayPortHint,/پیشنهادی/);
 assert.equal(index,3);
});
test('manual Xray port entry stays authoritative and generator does not configure Xray',()=>{
 const fields={...defaults,tnXrayPort:'53001'};
 const c=context(fields,'let tnClearPlan=()=>{};'+fn('function tnWizardTargetSync()','function tnWizardGenerateXrayPort()'));
 vm.runInContext('tnWizardTargetSync()',c);
 assert.equal(fields.tnTarget,'127.0.0.1:53001');
 assert.equal(fields.tnPort,'8444');
});

test('future reverse topology is rejected before creating a native BAFT plan',()=>{
 const fields={...defaults,tnDirection:'ex-to-ir',tnEngine:'baft'};
 let warnings=[];
 const c=context(fields,'let tnEditBase=null;const tnWizardText=(en,fa)=>fa;const tnWizardSharesLegacy=()=>false;const tnWizardEnsureID=()=>{};'+fn('function tnWizardValidateStep(i)','function tnWizardStep(i)'));
 c.alert=x=>warnings.push(x);
 assert.equal(vm.runInContext('tnWizardValidateStep(0)',c),false);
 assert.match(warnings.join(''),/معکوس/);
 fields.tnDirection='ir-to-ex';
 assert.equal(vm.runInContext('tnWizardValidateStep(0)',c),true);
 fields.tnEngine='frp';
 assert.equal(vm.runInContext('tnWizardValidateStep(0)',c),false);
});

'use strict';
// Execute the actual inline controllers together. API responses and DOM are
// deterministic fixtures; these checks are not browser or live-node E2E.
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const go=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=go.slice(go.indexOf('<script>')+8,go.indexOf('</script>'));
function section(from,to){const a=js.indexOf(from),b=js.indexOf(to,a);assert.ok(a>=0&&b>a,from);return js.slice(a,b)}
const controllers=section('function token(){','function ago(')+section('// Universal Toolbox is backend-driven.','// Native BAFT Smart Tunnel Wizard.')+section('// Native BAFT Smart Tunnel Wizard.','async function tnCancel(');
function harness(lang='en'){
 const defaults={tnDirection:'ir-to-ex',tnEngine:'baft',tnEx:'ex-test',tnIr:'ir-test',tnMode:'independent',tnInstance:'test-instance',tnTarget:'[2001:db8::15]:5432',tnPort:'8443',tnRouteListen:'127.0.0.1:1444',tnExMetric:'127.0.0.1:9202',tnIrMetric:'127.0.0.1:9302',tnClientPort:''};
 const elements=new Map(),requests=[],warnings=[];
 const element=id=>{
  if(!elements.has(id))elements.set(id,{value:defaults[id]||'',textContent:'',innerHTML:'',checked:id==='tnIngressConfirm',hidden:false,options:[],children:[],replaceChildren(){this.innerHTML=''},scrollIntoView(){}});
  return elements.get(id);
 };
 element('tnEngine').options=[{value:'baft',disabled:false},{value:'frp',disabled:true}];
 const stages=Array.from({length:5},(_,i)=>({dataset:{tnStep:String(i)},hidden:i!==0}));
 element('tnWizardSteps').children=stages.map(s=>({dataset:{tnNav:s.dataset.tnStep},classList:{toggle(){}}}));
 let credential='integration-credential',plans=0;
 const context={q:selector=>element(selector.slice(1)),
  localStorage:{getItem:()=>credential,setItem:(_key,value)=>{credential=value}},prompt:()=>'',
  document:{documentElement:{lang},querySelector:()=>element('studio'),querySelectorAll:()=>stages,addEventListener(){}},
  crypto:{getRandomValues(bytes){bytes.fill(7);return bytes}},
  alert:message=>warnings.push(message),confirm:()=>true,loadAudit(){},loadTunnels(){},loadAll(){},
  fetch:async(url,options)=>{
   const request={url,method:options.method||'GET',headers:{...options.headers},body:options.body?JSON.parse(options.body):undefined};
   requests.push(request);
   let data;
   if(url==='/api/tunnel-engines')data=[{name:'baft',label:'BAFT Secure',family:'direct',direction:'ir_to_ex',deployable:true},{name:'frp',label:'FRP',family:'reverse',deployable:false}];
   else if(url==='/api/tunnels/plan')data={hash:'reviewed-'+(++plans),ok:true,request:request.body,gates:[],nodes:[],steps:[],verify:[],rollback:[]};
   else if(url==='/api/tunnels')data={id:'fixture-tunnel'};
   else throw new Error('unexpected API request: '+url);
   return {ok:true,status:200,json:async()=>data,text:async()=>JSON.stringify(data)};
  }
 };
 vm.createContext(context);vm.runInContext(controllers,context);
 return {context,element,requests,warnings};
}
test('catalog to Studio to generated IPv6 destination to reviewed deployment uses one request contract',async()=>{
 const h=harness(),c=h.context;
 await c.loadTunnelEngines();
 assert.equal((h.element('tlCatalog').innerHTML.match(/onclick="tlPickEngine/g)||[]).length,1);
 h.element('tnDirection').value='ex-to-ir';c.tlPickEngine('baft');
 assert.equal(h.element('tnDirection').value,'ir-to-ex');
 c.tnWizardGenerateServicePort();
 assert.equal(h.element('tnTarget').value,'[2001:db8::15]:20007');
 await c.tnPlan();await c.tnDeploy();
 assert.deepEqual(h.requests.map(r=>[r.method,r.url]),[['GET','/api/tunnel-engines'],['POST','/api/tunnels/plan'],['POST','/api/tunnels']]);
 const [catalog,plan,create]=h.requests;
 assert.equal(catalog.body,undefined);
 assert.deepEqual(create.body,{...plan.body,plan_hash:'reviewed-1'});
 assert.equal(create.body.target,'[2001:db8::15]:20007');
 assert.equal(create.body.port,8443);
 assert.equal(create.body.instance_id,'test-instance');
 assert.deepEqual(Object.keys(plan.body).sort(),['allow_legacy_coexistence','ex_metrics_listen','ex_node','instance_id','ir_metrics_listen','ir_node','port','route_listen','target'].sort());
 for(const request of h.requests)assert.equal(request.headers.Authorization,'Bearer integration-credential');
 assert.doesNotMatch(h.element('tlCatalog').innerHTML+h.element('tnPlan').innerHTML,/integration-credential/);
});
test('suggesting a destination after review invalidates the old plan before any deployment',async()=>{
 const h=harness(),c=h.context;
 await c.tnPlan();assert.match(h.element('tnPlan').innerHTML,/reviewed-1/);
 c.tnWizardGenerateServicePort();await c.tnDeploy();
 assert.equal(h.element('tnPlan').innerHTML,'');
 assert.equal(h.requests.filter(r=>r.url==='/api/tunnels').length,0);
 await c.tnPlan();await c.tnDeploy();
 const create=h.requests.find(r=>r.url==='/api/tunnels');
 assert.equal(create.body.plan_hash,'reviewed-2');
 assert.equal(create.body.target,'[2001:db8::15]:20007');
});
test('unsupported direction or engine cannot send a plan or mutation request',async()=>{
 const h=harness(),c=h.context;
 h.element('tnDirection').value='ex-to-ir';await c.tnPlan();
 h.element('tnDirection').value='ir-to-ex';h.element('tnEngine').value='frp';await c.tnPlan();
 assert.equal(h.requests.length,0);assert.equal(h.warnings.length,2);
 h.element('tnEngine').value='baft';c.tlPickEngine('frp');
 assert.equal(h.element('tnEngine').value,'baft');assert.equal(h.requests.length,0);
});
for(const lang of ['fa','en'])test('combined '+lang+' review keeps endpoint direction and application-neutral copy',()=>{
 const h=harness(lang);h.context.tnWizardSummary();
 const html=h.element('tnWizardSummary').innerHTML;
 assert.match(html,/<bdi dir="ltr">\[2001:db8::15\]:5432<\/bdi>/);
 assert.match(html,/<bdi dir="ltr">IR \(dialer\) → EX \(listener\)<\/bdi>/);
 assert.doesNotMatch(html,/Xray|VLESS|VMess|Trojan|integration-credential/i);
 assert.equal(h.requests.length,0);
});

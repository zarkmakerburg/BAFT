'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const src=fs.readFileSync(path.resolve(__dirname,'../../internal/bcc/dashboard.go'),'utf8');
const js=src.slice(src.indexOf('<script>')+8,src.indexOf('</script>'));
const start=js.indexOf('async function tnAutoSelect()');
assert.ok(start>=0,'tnAutoSelect missing');
const body=js.slice(start,js.indexOf('\n',start));

function run(fields,response){
 const calls=[];let cleared=0;
 const c={
  q:sel=>{const k=sel.slice(1);return{get value(){return fields[k]},set value(v){fields[k]=v},get textContent(){return fields[k]},set textContent(v){fields[k]=v}}},
  tnWizardText:en=>en,ah:()=>({}),tnClearPlan:()=>{cleared++},
  fetch:async(url,opts)=>{calls.push({url,body:JSON.parse(opts.body),method:opts.method});return{ok:response.ok!==false,status:response.status||200,json:async()=>response.json,text:async()=>response.text||''}},
 };
 vm.createContext(c);vm.runInContext(body,c);
 return vm.runInContext('tnAutoSelect()',c).then(()=>({calls,cleared}));
}

test('auto-select asks for h2 on the chosen pair and fills only the EX port',async()=>{
 const f={tnEx:'ex-1',tnIr:'ir-1',tnPort:'8443',tnAutoSelectNote:''};
 const r=await run(f,{json:{selected:{port:2087,carrier_id:'h2',connect_rtt_ms:14},reason:''}});
 assert.equal(r.calls.length,1);
 assert.equal(r.calls[0].url,'/api/tunnels/auto-select');
 assert.equal(r.calls[0].method,'POST');
 assert.deepEqual(r.calls[0].body,{ex_node:'ex-1',ir_node:'ir-1',family:'4',carrier:'h2'});
 assert.equal(String(f.tnPort),'2087');
 assert.equal(r.cleared,1,'a changed port must invalidate any reviewed plan');
 assert.match(f.tnAutoSelectNote,/2087/);
});

test('a refusal keeps the port and shows the reason as text',async()=>{
 const f={tnEx:'ex-1',tnIr:'ir-1',tnPort:'8443',tnAutoSelectNote:''};
 await run(f,{json:{reason:'no path evidence',rejected:[{port:443,carrier_id:'h2',reason:'<img src=x onerror=alert(1)>'}]}});
 assert.equal(f.tnPort,'8443');
 assert.match(f.tnAutoSelectNote,/no path evidence/);
 assert.match(f.tnAutoSelectNote,/<img/,'reasons are assigned as textContent, never as HTML');
});

test('nothing is requested before both servers are chosen',async()=>{
 const f={tnEx:'',tnIr:'ir-1',tnPort:'8443',tnAutoSelectNote:''};
 const r=await run(f,{json:{}});
 assert.equal(r.calls.length,0);
 assert.equal(f.tnPort,'8443');
});

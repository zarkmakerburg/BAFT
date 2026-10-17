'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const root=path.resolve(__dirname,'../..');
const app=fs.readFileSync(path.join(root,'web/bcc-app.js'),'utf8');
const mount=fs.readFileSync(path.join(root,'web/bcc-operations-v3.js'),'utf8');
const operations=fs.readFileSync(path.join(root,'internal/bcc/dashboard.go'),'utf8');
const server=fs.readFileSync(path.join(root,'internal/bcc/server.go'),'utf8');

// Evaluate only simple literal map data from our own repository; do not run
// the dashboard code, timers, privileged operations or any API requests.
function literalMap(source,name){
 const match=source.match(new RegExp('const '+name+'=(\\{[^;]+\\});'));
 assert.ok(match,'missing '+name+' route map');
 return vm.runInNewContext('('+match[1]+')',Object.create(null));
}
function classicCardHeadings(){
 const html=operations.slice(operations.indexOf('<body>'),operations.indexOf('<script>'));
 const starts=[...html.matchAll(/<div class="card(?:\s[^"]*)?"[^>]*>/g)];
 return starts.map((m,i)=>{
   const card=html.slice(m.index,starts[i+1]?.index??html.length);
   const heading=card.match(/<b(?:\s[^>]*)?>([\s\S]*?)<\/b>/);
   assert.ok(heading,'operational card has no heading');
   return heading[1].replace(/<[^>]*>/g,'').trim();
 });
}
test('all command-center tools resolve to actually embedded operation cards',()=>{
 const nav=literalMap(app,'OPS');
 const routes=literalMap(mount,'routes');
 for(const [tool,heading] of Object.entries(nav)){
   assert.ok(Object.hasOwn(routes,heading),tool+' points to missing operation card '+heading);
   assert.ok(classicCardHeadings().some(h=>h.startsWith(heading)),tool+' card heading is not embedded in BCC');
 }
});
test('every Classic operation card remains reachable from an Aurora tool on the right page',()=>{
 const nav=literalMap(app,'OPS'),routes=literalMap(mount,'routes'),headings=classicCardHeadings();
 assert.ok(headings.length>=17,'Classic capabilities were removed');
 for(const heading of headings){
   const keys=Object.keys(routes).filter(k=>heading.startsWith(k));
   assert.equal(keys.length,1,'unregistered or ambiguous Classic card: '+heading);
   const tools=Object.keys(nav).filter(k=>nav[k]===keys[0]);
   assert.equal(tools.length,1,'card is parked but has no unique Aurora tool: '+heading);
   const index=app.indexOf("tool('"+tools[0]+"'");
   assert.ok(index>=0,'tool is registered but never rendered: '+tools[0]);
   const page=[...app.slice(0,index).matchAll(/(?:async )?function (\w+)View\(/g)].at(-1)?.[1];
   assert.equal(routes[keys[0]],page,'card route metadata differs from its rendered page: '+heading);
 }
});
test('tunnel tool mounts Aurora Studio and retains authenticated plan/deploy API',()=>{
 const nav=literalMap(app,'OPS');
 const routes=literalMap(mount,'routes');
 assert.equal(nav.tunnels,'Aurora · Tunnel Control Studio');
 assert.equal(routes[nav.tunnels],'tunnels');
 assert.match(app,/tool\('tunnels'/);
 assert.match(mount,/card\.dataset\.operationalRoute=routes\[key\]/);
 assert.match(operations,/async function tnPlan\(/);
 assert.match(operations,/async function tnDeploy\(/);
 assert.match(server,/\/api\/tunnels\/plan/);
});

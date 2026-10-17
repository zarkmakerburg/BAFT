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
test('all command-center tools resolve to actually embedded operation cards',()=>{
 const nav=literalMap(app,'OPS');
 const routes=literalMap(mount,'routes');
 for(const [tool,heading] of Object.entries(nav)){
   assert.ok(Object.hasOwn(routes,heading),tool+' points to missing operation card '+heading);
   assert.ok(operations.includes('<b>'+heading),tool+' card heading is not embedded in BCC');
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

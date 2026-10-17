'use strict';
const assert = require('node:assert/strict');
const {test}=require('node:test');
const fs=require('node:fs');
const path=require('node:path');
const root=path.resolve(__dirname,'../..');
const classic=fs.readFileSync(path.join(root,'internal/bcc/dashboard.go'),'utf8');
const fusion=fs.readFileSync(path.join(root,'web/bcc-operations-v3.js'),'utf8');
const css=fs.readFileSync(path.join(root,'web/bcc-app.css'),'utf8');

test('Aurora Tunnel Studio mounts into the existing BCC tunnel route',()=>{
 assert.match(classic,/Aurora · Tunnel Control Studio/);
 assert.match(fusion,/'Aurora · Tunnel Control Studio':'tunnels'/);
 assert.match(css,/\.aurora-tunnel-studio/);
});
test('independent tunnel request includes instance and isolated listeners',()=>{
 for(const s of ['id="tnInstance"','id="tnRouteListen"','id="tnExMetric"','id="tnIrMetric"','id="tnLegacyCoexist"','allow_legacy_coexistence','tnModeChanged()'])assert.ok(classic.includes(s),s);
 assert.match(classic,/r\.instance_id=q\('#tnInstance'\)/);
 assert.match(classic,/r\.ex_metrics_listen=q\('#tnExMetric'\)/);
 assert.match(classic,/r\.ir_metrics_listen=q\('#tnIrMetric'\)/);
});
test('old tunnel cannot be silently replaced and safe remove still uses reviewed decommission',()=>{
 assert.match(classic,/tnPlanReplaces&&!confirm/);
 assert.match(classic,/tnPlanReq\.allow_legacy_coexistence&&!confirm/);
 assert.match(classic,/tnDecommission\(this\.dataset\.id\)/);
 assert.match(classic,/api\/tunnels\/decommission\/plan/);
 assert.match(classic,/plan_hash:p\.hash/);
});

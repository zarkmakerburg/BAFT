'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const root=path.resolve(__dirname,'../..');
const go=fs.readFileSync(path.join(root,'internal/bcc/dashboard.go'),'utf8');
const js=go.slice(go.indexOf('<script>')+8,go.indexOf('</script>'));
const html=go.slice(go.indexOf('<div class="card aurora-tunnel-studio">'),go.indexOf('<div class="card"><div class="top"><b>Change History'));
const styles=fs.readFileSync(path.join(root,'web/bcc-app.css'),'utf8');
test('guided five-step UI remains mounted on real operational BCC card',()=>{
 assert.match(html,/Aurora · Tunnel Control Studio/);
 assert.equal((html.match(/data-tn-step="[0-4]"/g)||[]).length,5);
 for(const id of ['tnEngine','tnMode','tnEx','tnIr','tnPort','tnXrayPort','tnClientPort','tnUserProtocol','tnIngressConfirm','tnLegacyCoexist','tnInstance','tnRouteListen','tnExMetric','tnIrMetric','tnTarget','tnWizardSummary','tnPlan']){
  assert.match(html,new RegExp('id="'+id+'"'),'missing '+id);
 }
 assert.match(js,/function tnWizardStep\(/);
 assert.match(js,/function tnWizardValidateStep\(/);
 assert.match(js,/function tnWizardSummary\(/);
 assert.match(js,/async function tnPlan\(/);
 assert.match(js,/async function tnDeploy\(/);
 assert.match(styles,/\.tnWizardSteps/);
});
test('only Native BAFT is selectable: future drivers cannot silently become live',()=>{
 assert.match(html,/<option value="baft">BAFT Secure/);
 for(const driver of ['frp','rathole','wss'])assert.match(html,new RegExp('<option value="'+driver+'" disabled>'));
 assert.match(js,/q\('#tnEngine'\)\.value!=='baft'/);
});
test('IR public client port is visibly unmanaged and never sent as BAFT tunnel API data',()=>{
 assert.match(html,/BAFT.*does NOT create the public IR client listener/);
 assert.match(html,/id="tnClientPort"/);
 assert.match(html,/id="tnIngressConfirm"/);
 assert.match(js,/if\(!q\('#tnIngressConfirm'\)\.checked\)/);
 const request=js.slice(js.indexOf('function tnReq()'),js.indexOf('function tnModeChanged()'));
 assert.doesNotMatch(request,/tnClientPort|tnUserProtocol|tnIngressConfirm|client_listen_port|public_ir_port/);
 assert.match(request,/r\.route_listen=/);
 assert.match(request,/r\.ex_metrics_listen=/);
 assert.match(request,/r\.ir_metrics_listen=/);
 assert.match(request,/allow_legacy_coexistence/);
 assert.match(js,/function tnWizardTargetSync\(/);
 assert.match(js,/q\('#tnTarget'\)\.value='127\.0\.0\.1:'\+n/);
});
test('deployment preview is real, reviewed, authorized, and blocks superseding a legacy tunnel by default',()=>{
 assert.match(js,/fetch\('\/api\/tunnels\/plan'/);
 assert.match(js,/fetch\('\/api\/tunnels'/);
 assert.match(js,/plan_hash:tnPlanHash/);
 assert.match(js,/tnWizardSharesLegacy\(\)&&!q\('#tnLegacyCoexist'\)\.checked/);
 assert.match(js,/tnPlanReplaces&&!confirm/);
 assert.match(js,/tnPlanReq\.allow_legacy_coexistence&&!confirm/);
 assert.match(js,/tnWizardTunnels=Array\.isArray\(ts\)/);
});
test('wizard js compiles and does not introduce inline secrets or unmanaged engine API fields',()=>{
 new vm.Script(js);
 assert.doesNotMatch(js.slice(js.indexOf('function tnWizardText('),js.indexOf("let tnPlanHash=''")),/fetch\(|localStorage|private_key/);
});
test('manual-or-random Xray port follows explicit external Sanaei/3x-ui setup',()=>{
 assert.match(html,/id="tnGenerateXrayPort"/);
 assert.match(html,/onclick="tnWizardGenerateXrayPort\(\)"/);
 assert.match(html,/Generate random/);
 assert.match(html,/تولید تصادفی/);
 assert.match(html,/3x-ui/);
 assert.match(html,/دقیقاً با همان پورت/);
 assert.match(js,/crypto\.getRandomValues\(bytes\)/);
 assert.match(js,/function tnWizardGenerateXrayPort\(/);
 assert.match(js,/let n=Number\(q\('#tnXrayPort'\)\.value\)/);
 assert.match(js,/q\('#tnTarget'\)\.value='127\.0\.0\.1:'\+n/);
 assert.match(js,/Xray or IR public listener is created by BAFT/);
 const request=js.slice(js.indexOf('function tnReq()'),js.indexOf('function tnModeChanged()'));
 assert.doesNotMatch(request,/tnGenerateXrayPort|tnUserProtocol|tnClientPort/);
});

test('new tunnel starts with an explicit user-chosen Xray port, never silently uses a default',()=>{
 assert.match(html,/id="tnXrayPort"[^>]+placeholder="e.g. 2443"/);
 assert.doesNotMatch(html,/id="tnXrayPort"[^>]+value="2443"/);
 assert.match(js,/q\('#tnTarget'\)\.value='';q\('#tnXrayPort'\)\.value=''/);
 assert.match(js,/const raw=String\(q\('#'\+id\)\.value\)\.trim\(\),n=Number\(raw\)/);
});

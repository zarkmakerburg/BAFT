const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../..');
const dashboard = fs.readFileSync(path.join(root, 'internal/bcc/dashboard.go'), 'utf8');
const api = fs.readFileSync(path.join(root, 'internal/bcc/public_ingress_deploy_http.go'), 'utf8');

test('active tunnel UI exposes reviewed ingress deploy and status without claiming queued success', () => {
  assert.match(dashboard, /onclick="tnIngressDeploy\(this\.dataset\.id\)"/);
  assert.match(dashboard, /onclick="tnIngressOperationCheck\(this\.dataset\.id\)"/);
  assert.match(dashboard, /id="tnIngressOperationRefresh"/);
  assert.match(dashboard, /id="tnIngressOperationRetire"/);
  assert.match(dashboard, /\/api\/ingress\/deploy\?tunnel_id=/);
  assert.match(dashboard, /\/api\/ingress\/deploy/);
  assert.match(dashboard, /\/api\/ingress\/retire/);
  assert.match(dashboard, /result\.ready_to_apply!==false/);
  assert.match(dashboard, /op\.receipt_verified&&op\.receipt_phase==='applied'/);
  assert.match(dashboard, /op\.receipt_verified&&op\.receipt_phase==='retired'/);
  assert.match(dashboard, /Not ready until the receipt is authenticated and verified/);
});

test('deploy and retire status endpoints keep readiness false and require verified Apply before Retire', () => {
  assert.match(api, /if r\.Method == http\.MethodGet/);
  assert.match(api, /ReadyToApply: false/);
  assert.match(api, /retirement requires the exact reviewed operation and authenticated applied receipt/);
  assert.match(api, /ReceiptVerified = receipt\.ValidateForCommand\(\*command\) == nil/);
});

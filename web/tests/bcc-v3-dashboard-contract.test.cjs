'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const go=fs.readFileSync(path.join(__dirname,'..','..','internal','bcc','dashboard.go'),'utf8');
test('BCC dashboard keeps all existing operational sections',()=>{
for(const name of ['Live Monitoring','7-Day Traffic & Uptime','Register / update node','Add a server over SSH','Existing tunnels','Layered health','Change History','Safe SSH Port Migration','Route Doctor','Automatic Path Discovery','Connectivity Matrix','Cluster Nodes','Recent Jobs','Immutable Audit Log','Node Finance','Finance Reports'])assert.ok(go.includes(name),'Missing: '+name);
});
test('BCC visual tokens and focus states are present',()=>{for(const token of ['--bcc-bg','--bcc-panel','--bcc-line','--bcc-gold','--bcc-focus'])assert.ok(go.includes(token));assert.ok(go.includes('button:focus-visible'));assert.ok(go.includes('prefers-reduced-motion'))});
test('BCC dashboard remains wired to existing operational loaders',()=>{for(const fn of ['loadNodes','loadMonitoring','loadJobs','loadAudit','loadFinance','loadHistory','loadTunnels','loadAll'])assert.ok(go.includes('function '+fn+'('),'Missing loader: '+fn)});

test('operational dashboard includes focus-visible and disabled control styles',()=>{assert.match(go,/button:focus-visible/);assert.match(go,/button:disabled/);assert.match(go,/@media\s*\(max-width:/);});

test('SSH enrollment always restores controls and clears credentials',()=>{
  const body=go.slice(go.indexOf('async function bsRun()'),go.indexOf('function bsTunnel()'));
  assert.match(body,/try\{/);assert.match(body,/finally\{/);assert.match(body,/controller\.abort/);
  assert.match(body,/body\.password=body\.private_key=body\.passphrase=''/);
  assert.match(body,/bsPass'\)\.value=q\('#bsKey'\)\.value=q\('#bsPhrase'\)\.value=''/);
  assert.match(body,/controls\.forEach\(\(e,i\)=>e\.disabled=previous\[i\]\)/);
});

test('Doctor cached PASS expires to NOT_ASSESSED at display time',()=>{
  const source=go.slice(go.indexOf('function rdCurrent('),go.indexOf('let rdResult='));
  const rdCurrent=new Function(source+'; return rdCurrent')();
  const names=['node_service','local_listener','tcp_path','tls13','carrier_handshake','route_authorization','target_reachability','application_flow','consistency'];
  const created='2026-10-14T20:00:00Z',stages=names.map(stage=>({stage,state:'PASS',observed_at:created}));
  const report={schema_version:2,created_at:created,fresh_for_seconds:180,verdict:'PASS',stages};
  assert.equal(rdCurrent(report,Date.parse(created)+179000).verdict,'PASS');
  const expired=rdCurrent(report,Date.parse(created)+180000);
  assert.equal(expired.verdict,'NOT_ASSESSED');assert.equal(expired.stale,true);
  assert.ok(expired.stages.every(s=>s.state==='NOT_ASSESSED'));
  assert.equal(rdCurrent({...report,stages:[...stages,stages[0]]},Date.parse(created)+1000).verdict,'NOT_ASSESSED');
});

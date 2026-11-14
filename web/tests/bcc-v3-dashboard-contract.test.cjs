'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const go=fs.readFileSync(path.join(__dirname,'..','..','internal','bcc','dashboard.go'),'utf8');
test('BCC dashboard keeps all existing operational sections',()=>{
for(const name of ['Live Monitoring','7-Day Traffic & Uptime','Register / update node','Add a server over SSH','Existing tunnels','Layered health','Change History','Safe SSH Port Migration','Route Doctor','Automatic Path Discovery','Connectivity Matrix','Cluster Nodes','Recent Jobs','Immutable Audit Log','Node Finance','Finance Reports'])assert.ok(go.includes(name),'Missing: '+name);
});
test('BCC visual tokens and focus states are present',()=>{for(const token of ['--bcc-bg','--bcc-panel','--bcc-line','--bcc-gold','--bcc-focus'])assert.ok(go.includes(token));assert.ok(go.includes('button:focus-visible'));assert.ok(go.includes('prefers-reduced-motion'))});
test('BCC dashboard remains wired to existing operational loaders',()=>{for(const fn of ['loadNodes','loadMonitoring','loadJobs','loadAudit','loadFinance','loadHistory','loadTunnels','loadAll'])assert.ok(go.includes('function '+fn+'('),'Missing loader: '+fn)});

test('operational dashboard includes focus-visible and disabled control styles',()=>{assert.match(go,/button:focus-visible/);assert.match(go,/button:disabled/);assert.match(go,/@media\s*\(max-width:/);});

test('SSH enrollment explains first-server prerequisites in both languages (UX-0.1)',()=>{
const start=go.indexOf('<details class="bs-guide" id="bsGuide" open>'),end=go.indexOf('</details>',start);
assert.ok(start>0&&end>start,'enrollment guide is present and open by default');
const guide=go.slice(start,end);
assert.ok(start<go.indexOf('<input id="bsAlias"'),'guide is shown above the SSH form');
for(const lang of ['en','fa'])assert.match(guide,new RegExp('data-ui-lang="'+lang+'"'),'guide has '+lang+' text');
for(const term of ['foreign, EX','Iran, IR','worker','master','SSH port','root','sudo','203.0.113.10','26-p1d-ssh-bootstrap.md'])assert.ok(guide.includes(term),'guide explains '+term);
assert.ok(!/password\s*[:=]|BEGIN [A-Z ]*PRIVATE KEY/i.test(guide),'guide never shows credential examples');
for(const ip of guide.match(/\b\d{1,3}(?:\.\d{1,3}){3}\b/g)||[])assert.match(ip,/^(192\.0\.2|198\.51\.100|203\.0\.113)\./,'example address is documentation-only: '+ip);
assert.match(go,/html\[lang=fa\] \[data-ui-lang=en\],html:not\(\[lang=fa\]\) \[data-ui-lang=fa\]\{display:none!important\}/);
});

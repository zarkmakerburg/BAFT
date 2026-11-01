'use strict';
// Contract for the live Quick Tunnel wizard (web/bcc-action-wizard.js, v0.5.0).
// It replaced the preview-only floating wizard: it must stay wired into the
// Command Center, keep its accessible modal behaviour, and act only through
// BCC's reviewed, CSRF-protected APIs. Behaviour is covered by
// quick-tunnel-wizard.test.cjs (logic) and web/qa/quick-tunnel-cdp.cjs (browser).
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path');
const web=path.resolve(__dirname,'..');
const html=fs.readFileSync(path.join(web,'bcc-app.html'),'utf8');
const css=fs.readFileSync(path.join(web,'bcc-action-wizard.css'),'utf8');
const js=fs.readFileSync(path.join(web,'bcc-action-wizard.js'),'utf8');
const app=fs.readFileSync(path.join(web,'bcc-app.js'),'utf8');

test('wizard assets and launchers are wired',()=>{
 assert.match(html,/bcc-action-wizard\.css/);
 assert.match(html,/bcc-action-wizard\.js/);
 assert.match(html,/id="baw-open"/);
 assert.match(js,/root\.BAFTActionWizard = Object\.freeze\(/);
 assert.match(js,/launch\.addEventListener\('click', open\)/);
 assert.match(js,/closest\('\[data-baw-open\]'\)/);
 assert.match(app,/dataset\.bawOpen='true'/);
 assert.match(app,/launcher\.setAttribute\('aria-haspopup','dialog'\)/);
});

test('glass shell keeps blur, reduced motion and mobile sheet',()=>{
 assert.match(css,/backdrop-filter:blur/);
 assert.match(css,/prefers-reduced-motion/);
 assert.match(css,/\.baw-dialog/);
 assert.match(css,/@media\(max-width:600px\)/);
 assert.match(css,/\[aria-invalid="true"\]/);
});

test('modal is accessible: labelled, Escape closes, focus is trapped and restored',()=>{
 assert.match(js,/'aria-modal': 'true'/);
 assert.match(js,/'aria-labelledby': 'baw-heading'/);
 assert.match(js,/e\.key === 'Escape'/);
 assert.match(js,/doc\.addEventListener\('focusin', onFocus\)/);
 assert.match(js,/doc\.removeEventListener\('focusin', onFocus\)/);
 assert.match(js,/returnFocus\.focus\(\)/);
 assert.match(js,/doc\.body\.style\.overflow = 'hidden'/);
 assert.match(js,/doc\.documentElement\.dir/);
 assert.match(js,/'aria-invalid'/);
 assert.match(js,/role: 'alert'/);
});

test('acts only through reviewed BCC APIs with CSRF; never creates without a plan hash',()=>{
 assert.match(js,/headers\[c\.csrfHeader\] = c\.csrf/);
 assert.match(js,/credentials: 'same-origin', redirect: 'error'/);
 for(const p of ["api('nodes')","api('tunnels')","api('path-probes'","api('tunnels/auto-select'","api('tunnels/plan'"])assert.ok(js.includes(p),'missing '+p);
 const create=js.slice(js.indexOf('async function deploy()'),js.indexOf('function watch('));
 assert.match(create,/if \(!s\.plan \|\| !s\.plan\.ok \|\| s\.deployBusy\) return;/);
 assert.match(create,/plan_hash: s\.plan\.hash/);
 assert.match(create,/r\.status === 409/);
 assert.doesNotMatch(js,/XMLHttpRequest|sendBeacon|innerHTML|eval\(|new Function/);
 assert.doesNotMatch(js,/localStorage|sessionStorage|Authorization/);
});

test('signed-out visitors are sent to sign in instead of a fake flow',()=>{
 assert.match(js,/const live = \(\) => cfg\(\)\.mode === 'dashboard'/);
 assert.match(js,/if \(!live\(\)\) body\.append\(signinView\(\)\)/);
 assert.match(js,/if \(live\(\)\) load\(\)/);
});

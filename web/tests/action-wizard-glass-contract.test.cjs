'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path');
const web=path.resolve(__dirname,'..');
const html=fs.readFileSync(path.join(web,'bcc-app.html'),'utf8');
const css=fs.readFileSync(path.join(web,'bcc-action-wizard.css'),'utf8');
const js=fs.readFileSync(path.join(web,'bcc-action-wizard.js'),'utf8');
test('wizard assets and preview launcher are wired',()=>{
 assert.match(html,/bcc-action-wizard\.css/);
 assert.match(html,/bcc-action-wizard\.js/);
 assert.match(html,/id="baw-open"/);
 assert.match(js,/BAFTActionWizard/);
 assert.match(js,/addEventListener\('click',open\)/);
});
test('floating glass backdrop, focus and motion preferences exist',()=>{
 assert.match(css,/backdrop-filter:blur/);
 assert.match(css,/prefers-reduced-motion/);
 assert.match(css,/\.baw-dialog/);
 assert.match(js,/aria-modal/);
 assert.match(js,/e\.key==='Escape'/);
});
test('preview cannot submit deployment',()=>{
 assert.match(js,/Preview only/);
 assert.doesNotMatch(js,/fetch\(|XMLHttpRequest|sendBeacon/);
 assert.match(js,/Reverse capability is not yet verified/);
});

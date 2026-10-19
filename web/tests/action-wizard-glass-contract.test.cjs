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
 assert.match(js,/compatibleEngine/);
 assert.match(js,/validPort/);
 assert.match(js,/engineCatalog/);
});

test('localized modal keeps focus and background scroll under control',()=>{
 assert.match(js,/document\.documentElement\.lang/);
 assert.match(js,/document\.documentElement\.dir/);
 assert.match(js,/previousOverflow/);
 assert.match(js,/document\.body\.style\.overflow='hidden'/);
 assert.match(js,/returnFocus\.focus\(\)/);
});
test('unsupported reverse is blocked and port validation is live',()=>{
 assert.match(js,/reverse:false/);
 assert.match(js,/!compatibleEngine\(model\.engine,model\.topology\)/);
 assert.match(js,/!validPort\(model\.port\)/);
 assert.match(js,/data-baw-next|dataset\.bawNext/);
});

test('tunnel page can open preview without replacing operational wizard',()=>{
 const app=fs.readFileSync(path.join(web,'bcc-app.js'),'utf8');
 assert.match(app,/dataset\.bawOpen='true'/);
 assert.match(js,/\[data-baw-open\]/);
 assert.match(js,/validNodes/);
 assert.match(js,/stageNames/);
 assert.match(js,/tr\('notice'\)/);
});

test('modal exposes accessible labels and guarded navigation',()=>{
 assert.match(js,/aria-labelledby/);
 assert.match(js,/aria-describedby/);
 assert.match(js,/aria-haspopup/);
 assert.match(js,/function canAdvance\(stage\)/);
 assert.match(js,/else if\(canAdvance\(index\)\)/);
 assert.match(js,/field\.required=true/);
 const app=fs.readFileSync(path.join(web,'bcc-app.js'),'utf8');
 assert.match(app,/launcher\.setAttribute\('aria-haspopup','dialog'\)/);
});

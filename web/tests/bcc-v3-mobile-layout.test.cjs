// Run: node --test web/tests/bcc-v3-mobile-layout.test.cjs
'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const css=fs.readFileSync(path.join(__dirname,'..','bcc-product-v3.css'),'utf8');

// An unconditional desktop sidebar width later in the file used to override
// the phone rule, so on a 390px screen the menu took most of the width and
// every page was squeezed into a narrow column. The last word on .layout at
// phone width must be a single column, after the last desktop width.
test('phone layout puts the menu above the page, not beside it',()=>{
  const desktop=css.lastIndexOf('.layout{grid-template-columns:238px');
  assert.ok(desktop>=0,'desktop sidebar width not found');
  const phone=css.indexOf('@media(max-width:760px)',desktop);
  assert.ok(phone>desktop,'no phone media block after the desktop sidebar width');
  const block=css.slice(phone,css.indexOf('\n}',phone));
  assert.match(block,/\.layout\{grid-template-columns:minmax\(0,1fr\)\}/);
  assert.match(block,/\.layout nav\{[^}]*flex-direction:row[^}]*overflow-x:auto/);
});

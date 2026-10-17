'use strict';
// Visual check of the Command Center in Chrome: every page, both languages,
// both themes, desktop and phone width. It only signs in and takes
// screenshots, so it is safe to point at a local or staging BCC.
//
//   BCC_URL=http://127.0.0.1:18772/<secret-path>/ BCC_USER=admin BCC_PASS=... \
//   CHROME=/path/to/chrome OUT=/tmp/bcc-shots node web/qa/bcc-app-chrome.cjs [filter]
//
// The BCC limits each IP to 120 requests a minute; run one filter at a time
// (desk-fa, desk-en, mob) if you see 429 responses.
const {chromium}=require('playwright-core');
const fs=require('node:fs'),path=require('node:path');
const base=process.env.BCC_URL,user=process.env.BCC_USER,pass=process.env.BCC_PASS,out=process.env.OUT||'bcc-shots',only=process.argv[2]||'';
if(!base||!user||!pass){console.error('set BCC_URL, BCC_USER and BCC_PASS');process.exit(2)}
const runs=require('./bcc-browser-matrix.cjs').runs.filter(r=>!only||r[0].includes(only));
(async()=>{
  fs.mkdirSync(out,{recursive:true});
  const browser=await chromium.launch(process.env.CHROME?{executablePath:process.env.CHROME}:{});
  let failed=false;
  for(const [name,viewport,lang,theme,routes] of runs){
    const ctx=await browser.newContext({viewport});
    await ctx.addInitScript(([l,t])=>{localStorage.setItem('baft.lang',l);localStorage.setItem('baft.theme',t)},[lang,theme]);
    const page=await ctx.newPage(),errors=[];
    page.on('pageerror',e=>errors.push(e.message));
    page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});
    const shot=async r=>{
      if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)){failed=true;console.log(name,r,'page scrolls sideways')}
      await page.screenshot({path:path.join(out,name+'-'+r+'.png'),fullPage:r!=='dashboard'&&r!=='landing'});
    };
    await page.goto(base,{waitUntil:'networkidle'});await shot('landing');
    await page.goto(base+'login',{waitUntil:'networkidle'});
    await page.fill('#loginUsername',user);await page.fill('#loginPassword',pass);
    await Promise.all([page.waitForNavigation({waitUntil:'networkidle'}).catch(()=>{}),page.click('#loginForm [type="submit"]')]);
    if(await page.locator('#app').isHidden()){console.error(name,'sign-in failed');process.exit(1)}
    for(const r of routes){await page.evaluate(r=>{location.hash=r},r);await page.waitForTimeout(r==='dashboard'?2500:1200);await shot(r)}
    if(errors.length){failed=true;console.log(name,'errors',errors.slice(0,6))}
    console.log(name,'done');await ctx.close();
  }
  await browser.close();process.exit(failed?1:0);
})().catch(e=>{console.error(e);process.exit(1)});

'use strict';
// Globe and layout check of the static DEMO page (no sign-in, no server state):
// drag and touch turn the landing globe, reduced motion keeps it still and idle,
// it still draws without WebGL, and every design/lang/theme/width has no
// sideways scroll or console errors.
//
//   CHROME=/path/to/chrome node web/qa/bcc-globe-motion-chrome.cjs web /tmp/out
const {chromium}=require('playwright-core');
const http=require('node:http'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(process.argv[2]||'web'),out=process.argv[3]||'bcc-globe-shots';fs.mkdirSync(out,{recursive:true});
const types={'.html':'text/html','.css':'text/css','.js':'text/javascript','.png':'image/png','.jpg':'image/jpeg','.json':'application/json','.txt':'text/plain','.svg':'image/svg+xml','.webp':'image/webp'};
const srv=http.createServer((q,r)=>{let p=decodeURIComponent(new URL(q.url,'http://x').pathname);if(p==='/')p='/bcc-app.html';const f=path.join(root,p);if(!f.startsWith(root)||!fs.existsSync(f)||fs.statSync(f).isDirectory()){r.writeHead(404);return r.end()}r.writeHead(200,{'content-type':types[path.extname(f)]||'application/octet-stream'});fs.createReadStream(f).pipe(r)});
const res={};
async function canvasHash(page,sel){return page.evaluate(s=>{const c=document.querySelector(s);if(!c)return null;const d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let h=0;for(let i=0;i<d.length;i+=4*97)h=(h*31+d[i]+d[i+1]*3+d[i+2]*7)>>>0;return h},sel)}
async function diffFrac(page,sel,a){return page.evaluate(([s,a])=>{const c=document.querySelector(s),d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;if(!a){window.__snap=Array.from(d.filter((_,i)=>i%16===0));return 0}let n=0,m=0,k=0;for(let i=0;i<d.length;i+=16){if(Math.abs(d[i]-window.__snap[k])>8)n++;k++;m++}return n/m},[sel,a])}
(async()=>{
  await new Promise(r=>srv.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+srv.address().port+'/';
  const browser=await chromium.launch(process.env.CHROME?{executablePath:process.env.CHROME}:{});
  async function open(opts,q='',hash=''){const ctx=await browser.newContext({viewport:opts.vp,reducedMotion:opts.reduce?'reduce':'no-preference',hasTouch:!!opts.touch,isMobile:!!opts.touch,deviceScaleFactor:1});
    await ctx.addInitScript(([l,t,nogl])=>{localStorage.setItem('baft.lang',l);localStorage.setItem('baft.theme',t);if(nogl){const g=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(k,...a){return /webgl/.test(k)?null:g.call(this,k,...a)}}},[opts.lang||'fa',opts.theme||'dark',!!opts.nogl]);
    const page=await ctx.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});
    await page.goto(base+q+hash,{waitUntil:'networkidle'});await page.waitForTimeout(1500);return {ctx,page,errors}}
  // 1. Drag rotates the rendered landing globe (real mouse input through CDP).
  {const {ctx,page,errors}=await open({vp:{width:1440,height:1000},reduce:true});
   const box=await page.locator('#welcomeGlobe').boundingBox();await diffFrac(page,'#welcomeGlobe',false);
   await page.waitForTimeout(1200);const still=await diffFrac(page,'#welcomeGlobe',true);
   const cx=box.x+box.width/2,cy=box.y+box.height/2;await page.mouse.move(cx,cy);await page.mouse.down();for(let i=1;i<=10;i++){await page.mouse.move(cx+i*20,cy);await page.waitForTimeout(16)}await page.mouse.up();await page.waitForTimeout(400);
   const dragged=await diffFrac(page,'#welcomeGlobe',true);
   const raf=await page.evaluate(()=>new Promise(r=>{let n=0;const o=window.requestAnimationFrame;let draws=0;const c=document.getElementById('welcomeGlobe'),g=c.getContext('2d'),cl=g.clearRect.bind(g);g.clearRect=(...a)=>{draws++;return cl(...a)};setTimeout(()=>{g.clearRect=cl;r(draws)},2000)}));
   res.dragReduce={stillDiffNoInput:still,diffAfterDrag:dragged,redrawsIn2sIdleReduce:raf,errors};await page.screenshot({path:out+'/landing-drag-reduce.png'});await ctx.close()}
  // 2. Without reduce, the globe spins on its own.
  {const {ctx,page,errors}=await open({vp:{width:1440,height:1000}});await diffFrac(page,'#welcomeGlobe',false);await page.waitForTimeout(2500);res.autoSpin={diff2_5s:await diffFrac(page,'#welcomeGlobe',true),errors};await ctx.close()}
  // 3. Touch drag on a phone.
  {const {ctx,page,errors}=await open({vp:{width:390,height:844},touch:true,reduce:true});const box=await page.locator('#welcomeGlobe').boundingBox();
   await diffFrac(page,'#welcomeGlobe',false);const cdp=await ctx.newCDPSession(page);const x=box.x+box.width/2,y=box.y+box.height/2;
   await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x,y}]});for(let i=1;i<=8;i++){await cdp.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:x+i*12,y}]});await page.waitForTimeout(16)}await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});await page.waitForTimeout(400);
   res.touch={globeBox:box,diffAfterTouchDrag:await diffFrac(page,'#welcomeGlobe',true),scrollW:await page.evaluate(()=>document.documentElement.scrollWidth),errors};await page.screenshot({path:out+'/landing-touch-390.png'});await ctx.close()}
  // 4. No WebGL: fallback globe still draws.
  {const {ctx,page,errors}=await open({vp:{width:1440,height:1000},nogl:true});const nonblank=await page.evaluate(()=>{const c=document.getElementById('welcomeGlobe'),d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let n=0;for(let i=3;i<d.length;i+=4*50)if(d[i]>0)n++;return n});res.noWebGL={nonTransparentSamples:nonblank,errors};await page.screenshot({path:out+'/landing-nowebgl.png'});await ctx.close()}
  // 5. Matrix.
  res.matrix=[];
  for(const design of ['classic','imperial'])for(const lang of ['fa','en'])for(const theme of ['dark','light'])for(const vp of [{width:1440,height:1000},{width:390,height:844}]){
    const {ctx,page,errors}=await open({vp,lang,theme},design==='imperial'?'?imperial=1':'','#dashboard');await page.waitForTimeout(1200);
    const info=await page.evaluate(()=>({design:document.documentElement.dataset.design||'none',dir:document.dir,lang:document.documentElement.lang,theme:document.documentElement.dataset.theme,innerW:innerWidth,scrollW:document.documentElement.scrollWidth,map:(()=>{const c=document.getElementById('mapCanvas');if(!c)return null;const b=c.getBoundingClientRect();return[Math.round(b.width),Math.round(b.height)]})(),overflowing:[...document.querySelectorAll('body *')].filter(e=>{const b=e.getBoundingClientRect();return b.width>0&&(b.right>innerWidth+1||b.left<-1)&&!e.closest('[hidden]')&&getComputedStyle(e).position!=='fixed'}).slice(0,5).map(e=>e.tagName+'.'+e.className)}));
    const name=[design,lang,theme,vp.width].join('-');await page.screenshot({path:out+'/'+name+'.png'});res.matrix.push({name,...info,errors});await ctx.close()}
  const fail=[];
  if(res.dragReduce.redrawsIn2sIdleReduce>2)fail.push('idle globe keeps drawing with reduced motion');
  if(res.dragReduce.diffAfterDrag<.05)fail.push('mouse drag did not turn the globe');
  if(res.touch.diffAfterTouchDrag<.05)fail.push('touch drag did not turn the globe');
  if(res.autoSpin.diff2_5s<.02)fail.push('globe does not spin with normal motion');
  if(!res.noWebGL.nonTransparentSamples)fail.push('globe is blank without WebGL');
  for(const r of [res.dragReduce,res.autoSpin,res.touch,res.noWebGL,...res.matrix])if(r.errors.length)fail.push((r.name||'landing')+' errors: '+r.errors.slice(0,3).join(' | '));
  for(const m of res.matrix){if(m.scrollW>m.innerW+1)fail.push(m.name+' scrolls sideways');if(!m.map)fail.push(m.name+' has no map')}
  res.fail=fail;fs.writeFileSync(out+'/result.json',JSON.stringify(res,null,1));console.log(JSON.stringify(res,null,1));await browser.close();srv.close();
  if(fail.length){console.error('FAIL\n'+fail.join('\n'));process.exit(1)}console.log('PASS')})().catch(e=>{console.error(e);process.exit(1)});

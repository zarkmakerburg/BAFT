'use strict';
// Dependency-free, deliberately narrower fallback for the Playwright suite.
// Node >=22, Chrome, and BCC_FIXTURE_BINARY are required. Disposable QA only.
const fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {spawn}=require('node:child_process');
const {createInterface}=require('node:readline');
const assert=require('node:assert/strict');
const {runs}=require('./bcc-browser-matrix.cjs');
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const out=process.env.OUT||'/tmp/bcc-cdp-smoke';
const summary={source:process.env.BCC_SOURCE_SHA||'local',scope:'Chromium DOM smoke only; not full Playwright acceptance or carrier admission',profiles:[]};
fs.mkdirSync(out,{recursive:true});
async function until(fn,label){for(let i=0;i<100;i++){const v=await fn();if(v)return v;await sleep(100)}throw Error('timeout: '+label)}
async function stop(child){if(!child||!child.pid||child.exitCode!==null||child.signalCode!==null)return;const ended=new Promise(r=>child.once('close',r));child.kill('SIGTERM');await Promise.race([ended,sleep(2000)]);if(child.exitCode===null){child.kill('SIGKILL');await ended}}
async function fixture(){
  const child=spawn(process.env.BCC_FIXTURE_BINARY,[],{stdio:['ignore','pipe','ignore']});
  const lines=createInterface({input:child.stdout});
  try{const data=await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(Error('fixture startup timeout')),10000);child.once('error',e=>{clearTimeout(timer);reject(e)});child.once('exit',()=>{clearTimeout(timer);reject(Error('fixture exited'))});lines.once('line',line=>{clearTimeout(timer);try{resolve(JSON.parse(line))}catch{reject(Error('invalid fixture response'))}})});assert.equal(new URL(data.base).hostname,'127.0.0.1');return {...data,child,lines}}catch(e){await stop(child);throw e}
}
async function connect(url){
  const ws=new WebSocket(url),pending=new Map();let serial=0;
  await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',()=>j(Error('CDP connection failed')),{once:true})});
  ws.addEventListener('message',event=>{const msg=JSON.parse(event.data);if(msg.id){const p=pending.get(msg.id);if(p){pending.delete(msg.id);clearTimeout(p.timer);msg.error?p.reject(Error(msg.error.message)):p.resolve(msg.result)}}});
  return {close:()=>ws.close(),send(method,params={}){return new Promise((resolve,reject)=>{const id=++serial,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout: '+method))},10000);pending.set(id,{resolve,reject,timer});ws.send(JSON.stringify({id,method,params}))})}};
}
async function profile([name,viewport,lang,theme,pages]){
  const result={name,pages:[],checks:[],failures:[]};summary.profiles.push(result);
  let f,chrome,cdp,dir;
  try{
    f=await fixture();dir=fs.mkdtempSync(path.join(os.tmpdir(),'baft-cdp-'));
    chrome=spawn(process.env.CHROME||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+dir,'--no-first-run','--no-default-browser-check','--disable-background-networking','about:blank'],{stdio:'ignore'});
    let launchError;chrome.once('error',()=>{launchError=true});
    const portFile=path.join(dir,'DevToolsActivePort');await until(()=>{if(launchError||chrome.exitCode!==null)throw Error('Chrome failed to start');return fs.existsSync(portFile)},'Chrome startup');
    const port=fs.readFileSync(portFile,'utf8').split('\n')[0];
    const target=await (await fetch('http://127.0.0.1:'+port+'/json/new?about:blank',{method:'PUT'})).json();
    cdp=await connect(target.webSocketDebuggerUrl);
    const browser=await cdp.send('Browser.getVersion');summary.browser={product:browser.product,protocolVersion:browser.protocolVersion,jsVersion:browser.jsVersion};
    await cdp.send('Page.enable');await cdp.send('Runtime.enable');
    const ev=async expression=>{const r=await cdp.send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error('page evaluation exception');return r.result.value};
    await cdp.send('Emulation.setDeviceMetricsOverride',{...viewport,deviceScaleFactor:1,mobile:name.startsWith('mob')});
    await cdp.send('Page.addScriptToEvaluateOnNewDocument',{source:`localStorage.setItem('baft.lang',${JSON.stringify(lang)});localStorage.setItem('baft.theme',${JSON.stringify(theme)})`});
    await cdp.send('Page.navigate',{url:f.base+'login'});
    await until(()=>ev('Boolean(document.querySelector("#loginUsername"))'),'login DOM');
    await ev(`document.querySelector('#loginUsername').value=${JSON.stringify(f.username)};document.querySelector('#loginPassword').value=${JSON.stringify(f.password)};document.querySelector('#loginForm [type="submit"]').click();true`);
    await until(()=>ev('Boolean(document.querySelector("#app")&&!document.querySelector("#app").hidden&&window.BAFT_OPS?.cards&&document.querySelector("#ops_tlCatalog [data-engine=baft]"))'),'authenticated app');
    result.checks.push('real DOM login');
    const attrs=await ev('({lang:document.documentElement.lang,dir:document.documentElement.dir,theme:document.documentElement.dataset.theme})');
    assert.deepEqual(attrs,{lang,dir:lang==='fa'?'rtl':'ltr',theme});result.checks.push('language direction theme');
    const shotDir=path.join(out,name);fs.mkdirSync(shotDir,{recursive:true});
    for(const route of pages){
      await ev(`(()=>{const m=document.querySelector('#menuToggle');if(m&&getComputedStyle(m).display!=='none'&&m.getAttribute('aria-expanded')!=='true')m.click();document.querySelector('#nav a[href="#${route}"]').click();return true})()`);
      await until(()=>ev(`document.querySelector('#main')?.dataset.route===${JSON.stringify(route)}`),'route '+route);
      await sleep(150);
      const state=await ev(`({focus:document.activeElement.id,overflow:document.documentElement.scrollWidth>innerWidth+1,app:!document.querySelector('#app').hidden,route:document.querySelector('#main').dataset.route})`);
      assert.equal(state.focus,'main',route+' focus');assert.equal(state.overflow,false,route+' overflow');assert.equal(state.app,true);
      const shot=await cdp.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(shotDir,route+'.png'),Buffer.from(shot.data,'base64'));
      result.pages.push(route);
    }
    await ev(`document.querySelector('#nav a[href="#tunnels"]').click();true`);
    await until(()=>ev('Boolean(document.querySelector("#ops_tlCatalog [data-engine=baft]"))'),'catalog');
    await ev(`document.querySelector('#ops_tlCatalog [data-engine=baft]').click();true`);
    const wizard=await ev(`({open:document.querySelector('details[data-tool="tunnels"]').open,engine:document.querySelector('#ops_tnEngine').value,next:!!document.querySelector('#ops_tnWizardNext'),overflow:document.documentElement.scrollWidth>innerWidth+1})`);
    assert.equal(wizard.open,true);assert.equal(wizard.engine,'baft');assert.equal(wizard.next,true);assert.equal(wizard.overflow,false);result.checks.push('native wizard opens without deployment');
  }catch(e){let error=String(e.message);if(f)for(const secret of [f.base,f.username,f.password])error=error.replaceAll(secret,'<redacted>');result.failures.push(error)}
  finally{cdp?.close();await stop(chrome);if(f){f.lines.close();await stop(f.child)}if(dir)try{fs.rmSync(dir,{recursive:true,force:true,maxRetries:15,retryDelay:100})}catch{result.failures.push('temporary Chrome profile cleanup failed')};fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n')}
  console.log(name+': '+result.pages.length+' pages, '+result.checks.length+' checks, '+result.failures.length+' failures');
}
(async()=>{assert.ok(process.env.BCC_FIXTURE_BINARY,'Set BCC_FIXTURE_BINARY');for(const run of runs)await profile(run);const failures=summary.profiles.reduce((n,r)=>n+r.failures.length,0);console.log('profiles '+summary.profiles.length+', failures '+failures);process.exitCode=failures?1:0})().catch(e=>{console.error(e.message);process.exitCode=1});

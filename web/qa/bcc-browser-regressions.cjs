'use strict';
// Real Chromium + real authenticated BCC, one disposable loopback server per
// profile. Only the two explicitly marked failure/race cases stub responses.
// Build ./web/qa/fixture and set BCC_FIXTURE_BINARY. No production URL is used.
const assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {spawn}=require('node:child_process');
const {chromium}=require('playwright-core');
const {runs,pages}=require('./bcc-browser-matrix.cjs');
const binary=process.env.BCC_FIXTURE_BINARY,out=process.env.OUT||'bcc-browser-evidence';
if(!binary)throw Error('Set BCC_FIXTURE_BINARY to the built disposable QA fixture');
const summary={source:process.env.BCC_SOURCE_SHA||'local',browser:null,profiles:[],scope:'real-browser / real-BCC / disposable state; no real agents or carrier admission'};
fs.mkdirSync(out,{recursive:true});
const save=()=>fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n');
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function fixture(){
  const child=spawn(binary,[],{stdio:['ignore','pipe','pipe']});
  let stderr='';child.stderr.on('data',b=>{stderr+=b.toString()});
  const exited=new Promise(resolve=>child.once('close',resolve));
  const lines=readline.createInterface({input:child.stdout});
  const data=await new Promise((resolve,reject)=>{
    const timeout=setTimeout(()=>reject(Error('fixture startup timeout')),20000);
    const fail=()=>{clearTimeout(timeout);reject(Error('fixture exited before ready: '+stderr.slice(-500)))};
    child.once('error',reject);child.once('exit',fail);
    lines.once('line',line=>{clearTimeout(timeout);child.off('exit',fail);try{resolve(JSON.parse(line))}catch(e){reject(e)}});
  }).catch(async e=>{child.kill('SIGTERM');await exited;throw e});
  assert.equal(new URL(data.base).hostname,'127.0.0.1');
  return {...data,async stop(){lines.close();child.kill('SIGTERM');await Promise.race([exited,sleep(3000).then(()=>child.kill('SIGKILL'))])}};
}
async function expiredSession(browser,viewport,lang,theme,trigger,dir){
  // Repeated full-page logins share the production 120-request/minute budget.
  // Isolate each auth path in a fresh real server; never raise/disable its guard.
  const f=await fixture(),origin=new URL(f.base).origin,basePath=new URL(f.base).pathname;
  const ctx=await browser.newContext({viewport}),errors=[],limited=[];
  try{
    await ctx.addInitScript(([l,t])=>{localStorage.setItem('baft.lang',l);localStorage.setItem('baft.theme',t)},[lang,theme]);
    await ctx.route('**/*',r=>{const q=r.request(),u=new URL(q.url());if(u.origin!==origin||(u.pathname.includes('/api/')&&!['GET','HEAD'].includes(q.method()))){errors.push('unexpected network request: '+q.method()+' '+u.pathname.slice(basePath.length));return r.abort()}return r.continue()});
    const page=await ctx.newPage();page.setDefaultTimeout(10000);
    page.on('pageerror',e=>errors.push(e.message));
    page.on('response',r=>{if(r.status()===429)limited.push(new URL(r.url()).pathname.slice(basePath.length))});
    await page.goto(f.base+'login',{waitUntil:'networkidle'});
    await page.locator('#loginUsername').fill(f.username);await page.locator('#loginPassword').fill(f.password);
    await Promise.all([page.waitForURL(f.base),page.locator('#loginForm [type="submit"]').click()]);
    await page.waitForFunction(()=>Boolean(window.BAFT_OPS?.cards&&document.querySelector('#ops_tlCatalog [data-engine="baft"]')));
    const openNav=async()=>{const menu=page.locator('#menuToggle');if(await menu.isVisible()&&await menu.getAttribute('aria-expanded')!=='true')await menu.click()};
    if(trigger==='catalog'){
      await openNav();await page.locator('#nav a[href="#tunnels"]').click();
      await page.locator('[onclick="loadTunnelEngines()"]:visible').waitFor();
    }else await openNav();
    await ctx.clearCookies();
    const endpoint=f.base+'api/'+(trigger==='catalog'?'tunnel-engines':'nodes');
    const [response]=await Promise.all([
      page.waitForResponse(r=>r.url()===endpoint&&r.status()===401),page.waitForURL(f.base+'login'),
      (trigger==='catalog'?page.locator('[onclick="loadTunnelEngines()"]'):page.locator('#nav a[href="#servers"]')).click()
    ]);
    assert.equal(response.request().method(),'GET');await page.locator('#loginForm').waitFor({state:'visible'});
    assert.equal(await page.locator('#app').isHidden(),true);assert.equal(await page.locator('.ops-card').count(),0);
    assert.deepEqual(limited,[],'auth fixture must stay within the unchanged rate limit');assert.deepEqual(errors,[]);
    await page.screenshot({path:path.join(dir,'session-expired-'+trigger+'.png'),fullPage:true,animations:'disabled'});
    return {trigger,status:response.status(),method:response.request().method(),operationalCards:0,rateLimited:limited.length};
  }catch(e){throw Error(String(e.stack||e).replaceAll(f.base,'<auth-bcc>/').replaceAll(basePath,'/<auth-bcc>/').replaceAll(f.password,'<password>').replaceAll(f.username,'<user>'))}
  finally{await ctx.close();await f.stop()}
}
async function runProfile(browser,[name,viewport,lang,theme]){
  const f=await fixture(),origin=new URL(f.base).origin;
  const result={name,viewport,lang,theme,pages:[],cards:[],checks:[],failures:[]};summary.profiles.push(result);
  const dir=path.join(out,name);fs.mkdirSync(dir,{recursive:true});
  const ctx=await browser.newContext({viewport});
  const scrub=s=>String(s).replaceAll(f.base,'<bcc>/').replaceAll(new URL(f.base).pathname,'/<bcc>/').replaceAll(f.password,'<password>').replaceAll(f.username,'<user>');
  let page,apiRequests=[];
  const cards=new Set();
  const fail=(test,e)=>{result.failures.push({test,error:scrub(e.stack||e)});console.error(name,test,scrub(e.message||e))};
  const check=async(test,fn)=>{try{await fn();result.checks.push(test)}catch(e){fail(test,e)}finally{save()}};
  try{
    await ctx.addInitScript(([l,t])=>{localStorage.setItem('baft.lang',l);localStorage.setItem('baft.theme',t)},[lang,theme]);
    // Refuse non-loopback network access and unintended state-changing actions.
    await ctx.route('**/*',async route=>{
      const r=route.request(),url=new URL(r.url());
      if(url.origin!==origin){fail('network boundary',Error(url.origin));return route.abort()}
      if(r.method()==='POST'&&url.pathname.includes('/api/')&&!['tunnels/plan','nodes/location'].some(p=>url.pathname===new URL(f.base).pathname+'api/'+p)){
        fail('unintended mutation',Error(url.pathname.slice(new URL(f.base).pathname.length)));return route.abort();
      }
      return route.continue();
    });
    page=await ctx.newPage();page.setDefaultTimeout(10000);
    page.on('pageerror',e=>fail('uncaught JavaScript',e));
    const dismissDialog=d=>d.dismiss();page.on('dialog',dismissDialog);
    page.on('response',r=>{if(r.status()===429){result.rateLimited??=[];result.rateLimited.push({path:new URL(r.url()).pathname.slice(new URL(f.base).pathname.length),type:r.request().resourceType()})}});
    page.on('request',r=>{const u=new URL(r.url());if(u.pathname.includes('/api/'))apiRequests.push({path:u.pathname.slice(new URL(f.base).pathname.length),method:r.method(),headers:r.headers()})});
    const shot=async label=>{
      await page.evaluate(()=>scrollTo(0,0));
      await page.evaluate(()=>document.fonts.ready);
      await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
      const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
      await page.screenshot({path:path.join(dir,label+'.png'),fullPage:true,animations:'disabled'});
      if(overflow){
        result.layout??={};result.layout[label]=await page.evaluate(()=>({viewport:innerWidth,width:document.documentElement.scrollWidth,offenders:[...document.querySelectorAll('#public *,#main *')].map(n=>({tag:n.tagName,id:n.id,classes:n.className,rect:n.getBoundingClientRect().toJSON()})).filter(n=>n.rect.width>0&&(n.rect.right>innerWidth+1||n.rect.left< -1)).slice(0,25)}));
      }
      assert.equal(overflow,false,'document has horizontal overflow');
    };
    const openNav=async()=>{
      const menu=page.locator('#menuToggle');
      if(await menu.isVisible()&&await menu.getAttribute('aria-expanded')!=='true')await menu.click();
    };
    const nav=async route=>{
      if(await page.locator('#main').getAttribute('data-route')===route)return;
      const link=page.locator(`#nav a[href="#${route}"]`);
      await openNav();
      await link.click();
      await page.waitForFunction(r=>document.querySelector('#main')?.dataset.route===r,route);
      assert.equal(await page.evaluate(()=>document.activeElement.id),'main','route must focus main');
    };
    await page.goto(f.base,{waitUntil:'networkidle'});
    await check('public access boundary',async()=>{
      assert.equal(await page.evaluate(()=>Boolean(window.BAFT_UI.operationsHTML||window.BAFT_UI.csrf)),false);
      assert.equal(await page.locator('.ops-card').count(),0);
      for(const control of ['#baw-open','[data-lang=fa]','[data-lang=en]','[data-go=signin]']){
        const item=page.locator('#public '+control).first();assert.equal(await item.isVisible(),true,'public control '+control+' must remain visible');
        const box=await item.boundingBox();assert.ok(box&&box.x>=-1&&box.x+box.width<=viewport.width+1,'public control '+control+' must fit viewport');
      }
      await shot('welcome');
    });
    await page.goto(f.base+'login',{waitUntil:'networkidle'});await shot('signin');
    await page.locator('#loginUsername').fill(f.username);await page.locator('#loginPassword').fill(f.password);
    await Promise.all([page.waitForURL(f.base),page.locator('#loginForm [type="submit"]').click()]);
    await page.locator('#app').waitFor({state:'visible'});
    await page.waitForFunction(()=>Boolean(window.BAFT_OPS?.cards&&document.querySelector('#ops_tlCatalog [data-engine="baft"]')));
    await check('authenticated session and direction',async()=>{
      assert.equal(await page.locator('html').getAttribute('lang'),lang);
      assert.equal(await page.locator('html').getAttribute('dir'),lang==='fa'?'rtl':'ltr');
      assert.equal(await page.locator('html').getAttribute('data-theme'),theme);
      const cookie=(await ctx.cookies()).find(c=>c.httpOnly);assert.ok(cookie,'HTTP-only session cookie missing');
      assert.ok(['Strict','Lax'].includes(cookie.sameSite));
      assert.equal(await page.evaluate(()=>Object.keys(localStorage).some(k=>/token|password|csrf/i.test(k))),false);
    });
    for(const route of pages)await check('page '+route,async()=>{
      await nav(route);
      for(const detail of await page.locator('#main details.tool').all()){
        if(!await detail.evaluate(d=>d.open))await detail.locator(':scope > summary').click();
        const card=detail.locator('.ops-card');await card.waitFor({state:'visible'});
        assert.equal(await card.evaluate(c=>c.hidden),false);cards.add(await detail.getAttribute('data-tool'));
      }
      assert.equal((await page.locator('#main').textContent()).includes('0001-01-01'),false,'unset timestamps must not appear as real observations');
      if(route==='monitoring'){
        const observations=await (await ctx.request.get(f.base+'api/monitoring')).json();
        const expected={kNodes:observations.length,kUp:observations.filter(n=>n.status==='up').length,kDown:observations.filter(n=>n.status==='down').length,kUnknown:observations.filter(n=>n.status==='unknown').length};
        for(const [id,n] of Object.entries(expected))assert.equal((await page.locator('#ops_'+id).textContent()).trim(),String(n),'localized KPI must preserve observed value');
        const nodes=await (await ctx.request.get(f.base+'api/nodes')).json();
        for(const n of nodes)assert.ok((await page.locator('#ops_monitorRows').textContent()).includes(n.alias),'API alias must survive localization');
      }
      if(route==='updates'){
        const jobs=await (await ctx.request.get(f.base+'api/jobs')).json();assert.equal(jobs.length,0);
        await page.waitForFunction(()=>/No jobs|کاری ثبت نشده/.test(document.querySelector('#ops_jobs')?.textContent||''));
      }
      await shot(route);result.pages.push(route);
    });
    result.cards=[...cards].sort();
    await check('complete card and page coverage',async()=>{assert.equal(cards.size,18);assert.equal(result.pages.length,10)});
    await check('native toolbox opens collapsed Studio without deploy',async()=>{
      await nav('tunnels');
      const studio=page.locator('details[data-tool="tunnels"]');
      if(await studio.evaluate(d=>d.open))await studio.locator(':scope > summary').click();
      const before=apiRequests.filter(x=>x.method!=='GET').length;
      assert.equal(await page.locator('#ops_tlCatalog button:not(:disabled)').count(),1);
      await page.locator('#ops_tlCatalog [data-engine="baft"]').click();
      assert.equal(await studio.evaluate(d=>d.open),true);
      assert.equal(apiRequests.filter(x=>x.method!=='GET').length,before);
      assert.equal(await page.locator('#ops_tnEngine').inputValue(),'baft');
      assert.equal(await page.locator('.tnWizardSteps .selected').evaluate(n=>getComputedStyle(n).color),await page.locator('body').evaluate(n=>getComputedStyle(n).color),'active step must use the current theme text color');
      assert.equal(await page.locator('#ops_tnEngine').getAttribute('type'),'hidden');
      assert.equal(await page.locator('#ops_tnDirection option[value="ex-to-ir"]').isDisabled(),true);
    });
    await check('shared listener Join is explicit and plans only the new tenant',async()=>{
      const tunnels=f.base+'api/tunnels',planEndpoint=f.base+'api/tunnels/plan';
      const owner={id:'qa-owner',phase:'active',ex_node:'qa-ex',ir_node:'qa-ir-owner',instance_id:'qa-shared',public_address:'203.0.113.20',port:8443,target:'127.0.0.1:2443',route_listen:'127.0.0.1:1443',route_id:'service-main',ex_metrics_listen:'127.0.0.1:9202',ir_metrics_listen:'127.0.0.1:9302',record_shaping:false};
      const child={...owner,id:'qa-child',ir_node:'qa-ir-child',join_owner_id:'qa-owner'};
      await page.route(tunnels,r=>r.fulfill({json:[owner,child]}));
      await nav('tunnels');const originalTunnels=await page.evaluate(()=>tnWizardTunnels);
      await page.evaluate(()=>loadTunnels());
      assert.equal(await page.locator('#ops_tnJoinOwner option[value="qa-owner"]').count(),1);
      assert.equal(await page.locator('#ops_tnJoinOwner option[value="qa-child"]').count(),0);
      await page.locator('#ops_tnMode').selectOption('join');
      await page.locator('#ops_tnWizardNext').click();
      await page.locator('#ops_tnJoinOwner').selectOption('qa-owner');
      await page.locator('#ops_tnIr').selectOption('qa-ir');
      assert.equal(await page.locator('#ops_tnExField').isHidden(),true);
      assert.equal(await page.locator('#ops_tnPort').inputValue(),'8443');
      assert.equal(await page.locator('#ops_tnPort').getAttribute('readonly'),'');
      await page.locator('#ops_tnWizardNext').click();
      await page.locator('#ops_tnTarget').fill('127.0.0.1:2444');
      await page.locator('#ops_tnClientPort').fill('2444');
      await page.locator('#ops_tnIngressConfirm').check();
      await page.locator('#ops_tnWizardNext').click();
      await page.locator('#ops_tnJoinConfirm').check();
      await page.locator('#ops_tnWizardNext').click();
      let captured;
      await page.route(planEndpoint,async r=>{
        captured=r.request().postDataJSON();
        await r.fulfill({status:200,contentType:'application/json',body:JSON.stringify({
          request:captured,ok:true,hash:'qa-shared-join-plan',replaces:'',
          gates:[{name:'fixture',status:'PASS',detail:'reviewed'}],
          nodes:[{node_id:'qa-ex',role:'ex',generation_from:1,generation_to:2,changes:['preserve shared listener']},{node_id:'qa-ir',role:'ir',generation_from:0,generation_to:1,changes:['add new IR tenant']}],
          steps:['prepare','commit'],verify:['owner remains active'],rollback:['remove only new tenant']
        })});
      });
      const review=page.locator('[onclick="tnPlan()"]');
      const [response]=await Promise.all([page.waitForResponse(r=>r.url()===planEndpoint&&r.request().method()==='POST'),review.click()]);
      assert.equal(response.status(),200);
      assert.equal(captured.join_existing_tunnel_id,'qa-owner');
      assert.equal(captured.ex_node,'qa-ex');assert.equal(captured.ir_node,'qa-ir');
      assert.equal(captured.port,8443);assert.equal(captured.instance_id,'qa-shared');
      assert.equal(captured.ex_metrics_listen,'127.0.0.1:9202');
      assert.equal(captured.target,'127.0.0.1:2444');
      assert.equal(captured.client_port,2444);
      assert.ok(captured.route_listen.startsWith('127.0.0.1:'));
      await page.waitForFunction(()=>document.querySelector('#ops_tnPlan')?.textContent.includes('qa-shared-join-plan'));
      assert.equal(await page.locator('#ops_tnPlan [onclick="tnDeploy()"]').isEnabled(),true);
      assert.equal(await page.locator('#ops_tnPlan').textContent().then(t=>t.includes('Replaces')),false);
      await page.unroute(planEndpoint);await page.unroute(tunnels);
      await page.evaluate(ts=>{tnWizardTunnels=ts;tnFill();tnNew()},originalTunnels);
    });
    await check('real reviewed plan, failed preview invalidation, edit invalidation',async()=>{
      await nav('tunnels');
      const next=page.locator('#ops_tnWizardNext');await next.click();
      await page.locator('#ops_tnIr').selectOption('qa-ir');await page.locator('#ops_tnEx').selectOption('qa-ex');await next.click();
      await page.locator('#ops_tnPort').fill('8443');
      await page.locator('#ops_tnTarget').fill('[::1]:2443');
      await page.locator('[onclick="tnWizardGenerateServicePort()"]').click();
      assert.match(await page.locator('#ops_tnTarget').inputValue(),/^\[::1\]:[1-9][0-9]*$/);
      await page.locator('#ops_tnClientPort').fill('443');await page.locator('#ops_tnIngressConfirm').check();
      await next.click();await page.locator('#ops_tnLegacyCoexist').evaluate(el=>{el.checked=true;el.dispatchEvent(new Event('change',{bubbles:true}))});await next.click();
      const review=page.locator('[onclick="tnPlan()"]');
      const [response]=await Promise.all([page.waitForResponse(r=>r.url()===f.base+'api/tunnels/plan'&&r.request().method()==='POST'),review.click()]);
      assert.equal(response.status(),200);const plan=await response.json();assert.ok(plan.hash);assert.equal(plan.request.target.startsWith('[::1]:'),true);
      result.plan={ok:plan.ok,gates:plan.gates.map(g=>({name:g.name,status:g.status}))};
      assert.equal(plan.ok,true,'simulated agent contact must permit a ready reviewed plan');
      await page.waitForFunction(h=>document.querySelector('#ops_tnPlan')?.textContent.includes(h),plan.hash);
      assert.equal(await page.locator('#ops_tnPlan [onclick="tnDeploy()"]').isEnabled(),Boolean(plan.ok));
      await shot('reviewed-plan');
      const endpoint=f.base+'api/tunnels/plan';
      await page.route(endpoint,r=>r.fulfill({status:503,contentType:'text/plain',body:'QA failure fixture'}));
      await Promise.all([page.waitForResponse(r=>r.url()===endpoint&&r.status()===503),review.click()]);
      // Failed re-planning must invalidate the old hash and its deploy action.
      await page.waitForFunction(hash=>!(document.querySelector('#ops_tnPlan')?.textContent||'').includes(hash),plan.hash);
      assert.equal((await page.locator('#ops_tnPlan').textContent()).includes(plan.hash),false);
      assert.equal(await page.locator('#ops_tnPlan [onclick="tnDeploy()"]').count(),0);
      await page.unroute(endpoint);
      await page.locator('#ops_tnWizardBack').click();await page.locator('#ops_tnWizardBack').click();await page.locator('#ops_tnTarget').fill('[::1]:5432');
      assert.equal(await page.locator('#ops_tnPlan [onclick="tnDeploy()"]').count(),0);
      assert.equal(apiRequests.filter(r=>r.path==='api/tunnels'&&r.method==='POST').length,0);
    });
    await check('catalog failure and late success stay unavailable',async()=>{
      await nav('tunnels');const endpoint=f.base+'api/tunnel-engines';
      const caps=await (await ctx.request.get(endpoint)).json();
      let seen,release;const first=new Promise(r=>seen=r),gate=new Promise(r=>release=r);let count=0;
      await page.route(endpoint,async r=>{if(++count===1){seen();await gate;await r.fulfill({json:caps})}else await r.fulfill({status:503,body:'QA catalog failure'})});
      const refresh=page.locator('[onclick="loadTunnelEngines()"]');await refresh.click();await first;await refresh.click();
      await page.waitForFunction(()=>/unavailable|در دسترس نیست/.test(document.querySelector('#ops_tlSummary')?.textContent||''));
      const late=page.waitForResponse(r=>r.url()===endpoint&&r.status()===200);
      release();await (await late).finished();
      await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
      assert.equal(await page.locator('#ops_tlCatalog button:not(:disabled)').count(),0);
      await shot('catalog-unavailable');await page.unroute(endpoint);await refresh.click();
      await page.locator('#ops_tlCatalog [data-engine="baft"]').waitFor({state:'visible'});
    });
    await check('CSRF refusal and real UI metadata write',async()=>{
      const denied=await page.evaluate(async base=>{const r=await fetch(base+'api/nodes/location',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({node_id:'qa-ex',country:'US'})});return r.status},f.base);
      assert.equal(denied,401);await nav('servers');
      const select=page.locator('select[data-country="qa-ex"]');await select.waitFor({state:'visible'});
      page.off('dialog',dismissDialog);page.once('dialog',d=>{d.accept();page.on('dialog',dismissDialog);});
      const [response]=await Promise.all([page.waitForResponse(r=>r.url()===f.base+'api/nodes/location'&&r.request().method()==='POST'),select.selectOption('US')]);
      assert.equal(response.status(),200);const headers=response.request().headers();assert.ok(headers['x-baft-csrf']);assert.equal(headers.authorization,undefined);
      const nodes=await page.evaluate(async base=>(await fetch(base+'api/nodes')).json(),f.base);assert.equal(nodes.find(n=>n.id==='qa-ex').country,'US');
    });
    await check('language, theme, keyboard focus and reduced motion',async()=>{
      await nav('settings');const other=lang==='fa'?'en':'fa';
      await page.locator(`#main [data-lang-switch] [data-lang="${other}"]`).click();assert.equal(await page.locator('html').getAttribute('lang'),other);
      await page.locator(`#main [data-lang-switch] [data-lang="${lang}"]`).click();
      const alternate=theme==='dark'?'light':'dark';await page.locator(`#main [data-theme-set="${alternate}"]`).click();assert.equal(await page.locator('html').getAttribute('data-theme'),alternate);await page.locator(`#main [data-theme-set="${theme}"]`).click();
      await openNav();
      await page.locator('#nav a[href="#servers"]').focus();await page.keyboard.press('Enter');await page.waitForFunction(()=>document.querySelector('#main')?.dataset.route==='servers');
      assert.equal(await page.evaluate(()=>document.activeElement.id),'main');await page.keyboard.press('Tab');
      assert.equal(await page.evaluate(()=>Boolean(document.activeElement.closest('#main'))),true);
      assert.notEqual(await page.evaluate(()=>getComputedStyle(document.activeElement).outlineStyle),'none');
      await nav('tunnels');
      const labelChanges=await page.locator('#ops_tnTarget').evaluate(async n=>{
        const frame=()=>new Promise(r=>requestAnimationFrame(r));await frame();await frame();
        let changes=0;const observer=new MutationObserver(ms=>{changes+=ms.length});observer.observe(n.previousElementSibling,{childList:true,subtree:true});
        for(let i=0;i<5;i++)await frame();observer.disconnect();return changes;
      });
      assert.equal(labelChanges,0,'idle localization must settle without rewriting field labels');
      await page.emulateMedia({reducedMotion:'reduce'});assert.equal(await page.locator('.tlTile').first().evaluate(n=>getComputedStyle(n).transitionDuration),'0s');await shot('reduced-motion');
    });
    await check('real logout and unauthenticated catalog',async()=>{
      await page.locator('#avatar').click();
      await page.locator('#userPop').waitFor({state:'visible'});
      await Promise.all([page.waitForURL(f.base),page.locator('#logout').click()]);
      assert.equal(await page.locator('#app').isHidden(),true);assert.equal(await page.locator('.ops-card').count(),0);
      assert.equal((await ctx.request.get(f.base+'api/tunnel-engines')).status(),401);
    });
    await check('expired session removes operational UI',async()=>{
      // Exercise both application fetch paths through their actual UI controls.
      // An arbitrary absolute-URL fetch bypasses their session handling.
      result.auth=[];
      for(const trigger of ['catalog','servers']){
        result.auth.push(await expiredSession(browser,viewport,lang,theme,trigger,dir));
      }
      assert.deepEqual(result.rateLimited||[],[],'main fixture must stay within the unchanged rate limit');
    });
    assert.equal(apiRequests.filter(r=>r.path==='api/tunnels'&&r.method==='POST').length,0);
  }catch(e){fail('profile setup or completion',e);if(page)await page.screenshot({path:path.join(dir,'failure.png'),fullPage:true}).catch(()=>{})}
  finally{await ctx.close();await f.stop();save()}
  const status=result.pages.length+' pages; '+result.cards.length+' cards; '+result.checks.length+' checks; '+result.failures.length+' failures';
  console.log('::notice title=BCC browser '+name+'::'+status);
}
(async()=>{
  const browser=await chromium.launch(process.env.CHROME?{executablePath:process.env.CHROME}:{});
  summary.browser=browser.version();
  try{for(const run of runs)await runProfile(browser,run)}finally{await browser.close();fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n')}
  assert.equal(summary.profiles.length,8,'all eight profiles must execute');
  const failures=summary.profiles.reduce((n,p)=>n+p.failures.length,0);
  console.log('Chromium',summary.browser,'profiles',summary.profiles.length,'failures',failures);process.exitCode=failures?1:0;
})().catch(e=>{console.error(e.message);process.exitCode=1});

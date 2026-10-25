'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const web=path.join(__dirname,'..'),root=path.join(web,'..');
const read=f=>fs.readFileSync(path.join(web,f),'utf8');
const src=read('bcc-updatecenter.js'),app=read('bcc-app.js'),html=read('bcc-app.html'),assets=read('assets.go');
const planGo=fs.readFileSync(path.join(root,'internal','bcc','updatecenter_plan.go'),'utf8');
const ucGo=fs.readFileSync(path.join(root,'internal','bcc','updatecenter.go'),'utf8');

/* ---------- a tiny DOM, enough to run the page and read back what it put on screen ---------- */
class TextNode{constructor(t){this.text=String(t);this.nodeType=3}get textContent(){return this.text}}
class El{
  constructor(tag){this.tag=tag;this.children=[];this.attrs={};this.listeners={};this.className='';this.hidden=false;this.disabled=false;this._value='';this.files=null;
    this.classList={toggle:(c,on)=>{const s=new Set(this.className.split(' ').filter(Boolean));if(on)s.add(c);else s.delete(c);this.className=[...s].join(' ')}}}
  get textContent(){return this.children.map(c=>c.textContent).join('')}
  set textContent(v){this.children=[new TextNode(v)]}
  append(...n){for(const x of n)this.children.push(x)}
  replaceChildren(...n){this.children=[...n]}
  setAttribute(k,v){this.attrs[k]=String(v);if(k==='hidden')this.hidden=true;if(k==='disabled')this.disabled=true;if(k==='class')this.className=String(v)}
  getAttribute(k){return this.attrs[k]}
  addEventListener(e,f){(this.listeners[e]=this.listeners[e]||[]).push(f)}
  async fire(e){for(const f of this.listeners[e]||[])await f({target:this})}
  get value(){if(this.tag==='select'&&!this._value){const o=this.children.find(c=>c.tag==='option');return o?o.attrs.value:''}return this._value}
  set value(v){this._value=v}
  all(p,out=[]){if(p(this))out.push(this);for(const c of this.children)if(c instanceof El)c.all(p,out);return out}
}
function setup({fetchImpl,lang='en'}){
  const reg={};
  const doc={documentElement:{lang},createElement:t=>new El(t),createTextNode:t=>new TextNode(t),getElementById:id=>reg[id]||null,_reg:reg};
  global.document=doc;global.window={dispatchEvent(){},addEventListener(){}};global.Event=class{constructor(t){this.type=t}};
  Object.defineProperty(global,'navigator',{value:{clipboard:{written:[],async writeText(t){this.written.push(t)}}},configurable:true,writable:true});
  global.fetch=fetchImpl;
  delete require.cache[require.resolve('../bcc-updatecenter.js')];
  return{doc,U:require('../bcc-updatecenter.js')};
}
const flush=()=>new Promise(r=>setImmediate(r));
const ctx=(lang='en')=>({live:true,cfg:{base:'/s3cr3t/',csrfHeader:'X-CSRF',csrf:'tok'},lang:()=>lang,when:x=>'WHEN:'+x});
const findBtn=(r,text)=>r.all(e=>e.tag==='button'&&e.textContent===text)[0];
const resp=(status,body)=>({status,ok:status>=200&&status<300,text:async()=>typeof body==='string'?body:JSON.stringify(body)});
const none=async()=>resp(200,{});

const VIEW={root_pinned:true,floor:'v0.4.0',revocation_sequence:2,revocation_expired:false,notice_count:1,latest:{digest:'d2',version:'v0.4.1'},
  releases:[{digest:'d2',version:'v0.4.1',commit:'b'.repeat(40),channel:'stable',signature_status:'verified',revoked:false,rollback_only:false,received_at:'2026-10-20T00:00:00Z'},
            {digest:'d1',version:'v0.4.0',commit:'a'.repeat(40),channel:'stable',signature_status:'verified',revoked:false,rollback_only:false,received_at:'2026-10-19T00:00:00Z'}],
  nodes:[{node_id:'ex-1',alias:'<img src=x onerror=alert(1)>',role:'foreign',installed_version:'v0.4.0',latest_version:'v0.4.1',status:'update-available',notice_code:'update_available',notice:'x',last_report_age_seconds:60,components:[]},
         {node_id:'ex-2',role:'foreign',installed_version:'v0.4.1',latest_version:'v0.4.1',status:'current',notice_code:'current',notice:'x',components:[]}]};

/* ---------- static contract ---------- */
test('Persian and English carry the same Update Center strings',()=>{
  const {U}=setup({fetchImpl:none});
  const fa=Object.keys(U.UC.fa).sort(),en=Object.keys(U.UC.en).sort();
  assert.deepEqual(fa,en);
  for(const k of fa){assert.equal(typeof U.UC.fa[k],typeof U.UC.en[k],k);if(typeof U.UC.fa[k]==='string')assert.ok(U.UC.fa[k].trim()&&U.UC.en[k].trim(),k)}
});

test('every status, notice code and plan step the server can send has a translation',()=>{
  const {U}=setup({fetchImpl:none});
  const goNotices=[...ucGo.slice(ucGo.indexOf('var updateNoticeText')).split('}\n')[0].matchAll(/^\s*"([a-z_]+)":/gm)].map(m=>m[1]);
  assert.ok(goNotices.length>=10,'notices found: '+goNotices.length);
  for(const c of goNotices){assert.ok(U.NOTICES.includes(c),'notice not handled by UI: '+c);for(const l of ['fa','en'])assert.ok(U.UC[l]['n_'+c],l+' n_'+c)}
  const goSteps=new Set([...planGo.matchAll(/(?:st\(|Code:\s*)"([a-z_]+)"/g)].map(m=>m[1]));
  assert.ok(goSteps.size>=15,'steps found: '+goSteps.size);
  for(const c of goSteps)for(const l of ['fa','en'])assert.equal(typeof U.UC[l]['s_'+c],'function',l+' step '+c);
  for(const s of U.STATUSES)for(const l of ['fa','en'])assert.ok(U.UC[l][U.statusKey(s)],l+' '+s);
  for(const code of ['f400','f409','f413','f503'])for(const l of ['fa','en'])assert.ok(U.UC[l][code]);
});

test('server text never reaches the page through innerHTML or inline handlers',()=>{
  const bad=[/\binnerHTML\b/,/insertAdjacentHTML/,/outerHTML/,/document\.write/,/\beval\s*\(/,/new Function/,/\son[a-z]+\s*=/i,/javascript:/i];
  for(const b of bad)assert.doesNotMatch(src,b,String(b));
});

test('the page is embedded, loaded before the app and hooked in minimally',()=>{
  assert.ok(assets.match(/\/\/go:embed (.*)/)[1].split(/\s+/).includes('bcc-updatecenter.js'));
  assert.ok(html.indexOf('./bcc-updatecenter.js')>0&&html.indexOf('./bcc-updatecenter.js')<html.indexOf('./bcc-app.js'));
  assert.match(html,/id="updBadge"[^>]*hidden/);
  assert.match(app,/BAFT_UPDATES\?\.mount\(uc,\{live,cfg,lang,when\}\)/);
  assert.match(app,/api\('updates'\)/);
});

test('a CSRF header and credentials accompany every state-changing call',()=>{
  assert.match(src,/hd\[cfg\.csrfHeader\]=cfg\.csrf/);
  assert.match(src,/credentials:'same-origin'/);
  assert.match(src,/redirect:'error'/);
});

/* ---------- behaviour ---------- */
test('the page renders servers and releases as text, never as markup',async()=>{
  const {doc,U}=setup({fetchImpl:async()=>resp(200,VIEW)});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  const text=main.textContent;
  assert.match(text,/<img src=x onerror=alert\(1\)>/);
  assert.equal(main.all(e=>e.tag==='img').length,0);
  assert.match(text,/Update available/);assert.match(text,/v0\.4\.1/);
  doc._reg.updBadge=new El('a');doc._reg.updBadgeN=new El('span');
  U.set(VIEW);assert.equal(doc._reg.updBadge.hidden,false);assert.match(doc._reg.updBadgeN.textContent,/1 update/);
  U.set({...VIEW,notice_count:0});assert.equal(doc._reg.updBadge.hidden,true);
  U.set(null);assert.equal(U.count,0);
});

test('Persian view uses Persian labels and keeps identifiers left-to-right',async()=>{
  const {U}=setup({fetchImpl:async()=>resp(200,VIEW),lang:'fa'});
  const main=new El('main');U.mount(main,ctx('fa'));await flush();await flush();
  assert.match(main.textContent,/به‌روزرسانی موجود/);
  assert.ok(main.all(e=>e.className.includes('ltr')).length>0);
});

test('the preview never calls the server',async()=>{
  let calls=0;const {U}=setup({fetchImpl:async()=>{calls++;return resp(200,{})}});
  const main=new El('main');U.mount(main,{...ctx(),live:false});await flush();
  assert.equal(calls,0);assert.match(main.textContent,/Demo preview/);
});

test('import posts the chosen file with CSRF and reports a refusal',async()=>{
  const seen=[];
  const {U}=setup({fetchImpl:async(u,o)=>{seen.push([u,o]);if(u.endsWith('api/updates'))return resp(200,VIEW);return resp(400,'invalid bundle JSON\n')}});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  const fileIn=main.all(e=>e.tag==='input'&&e.attrs.type==='file')[0];
  fileIn.files=[{size:50,text:async()=>JSON.stringify({schema_version:1,revocations:{}})}];
  await fileIn.fire('change');await flush();
  const post=seen.find(([u])=>u.endsWith('updates/import'));
  assert.ok(post);assert.equal(post[1].method,'POST');assert.equal(post[1].headers['X-CSRF'],'tok');assert.equal(post[1].credentials,'same-origin');
  assert.match(main.textContent,/Import refused/);assert.match(main.textContent,/invalid, unsigned or tampered/);
  seen.length=0;fileIn.files=[{size:300000,text:async()=>'{}'}];await fileIn.fire('change');
  assert.equal(seen.filter(([u])=>u.endsWith('updates/import')).length,0);assert.match(main.textContent,/larger than the limit/);
  fileIn.files=[{size:5,text:async()=>'nope'}];await fileIn.fire('change');
  assert.equal(seen.filter(([u])=>u.endsWith('updates/import')).length,0);
});

test('an older-than-floor refusal offers an explicit rollback-only import with the exact confirmation header',async()=>{
  const seen=[];const manifest={payload:Buffer.from(JSON.stringify({version:'v0.3.7',commit:'c'.repeat(40)})).toString('base64')};
  const bundle={schema_version:1,manifest,cert:{},revocations:{}};
  const {U}=setup({fetchImpl:async(u,o)=>{seen.push([u,o]);if(u.endsWith('api/updates'))return resp(200,VIEW);
    return o.headers['X-BAFT-Downgrade-Confirmation']?resp(201,{status:'imported',version:'v0.3.7',rollback_only:true}):resp(409,'release is older than the current floor')}});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  const fileIn=main.all(e=>e.tag==='input'&&e.attrs.type==='file')[0];
  fileIn.files=[{size:99,text:async()=>JSON.stringify(bundle)}];await fileIn.fire('change');await flush();
  const rb=findBtn(main,'Import as rollback-only');assert.ok(rb&&!rb.hidden);
  await rb.fire('click');await flush();
  const last=seen.filter(([u])=>u.endsWith('updates/import')).pop();
  assert.equal(last[1].headers['X-BAFT-Downgrade-Confirmation'],'DOWNGRADE v0.3.7 '+'c'.repeat(40));
  assert.match(main.textContent,/Imported: v0\.3\.7 \(rollback only\)/);
});

test('plan view lists gates, steps and rollback; apply needs the exact typed confirmation and the plan hash',async()=>{
  const plan={hash:'h'.repeat(64),ok:true,node_id:'ex-1',kind:'upgrade',from_version:'v0.4.0',to_version:'v0.4.1',target_digest:'d2',executor:'agent_job',
    gates:[{name:'release signature',status:'PASS',detail:''},{name:'version order',status:'WARN',detail:'<b>x</b>'}],
    steps:[{code:'agent_job_sign',text:'t'},{code:'brand_new_step',text:'FALLBACK <b>text</b>'}],verify:[{code:'verify_report',text:'t'}],
    rollback:[{code:'rollback_installer',text:'t',cmd:'sudo bash install.sh --offline . --allow-downgrade --yes'}],notes:[{code:'note_never_auto',text:'t'}],
    apply:{endpoint:'/api/updates/apply',confirmation:'UPDATE ex-1 v0.4.1'}};
  const seen=[];
  const {U}=setup({fetchImpl:async(u,o)=>{seen.push([u,o]);if(u.endsWith('api/updates'))return resp(200,VIEW);if(u.endsWith('updates/plan'))return resp(200,plan);return resp(202,[{id:'1'}])}});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  await findBtn(main,'Upgrade plan').fire('click');await flush();await flush();
  const t=main.textContent;
  assert.match(t,/Plan for ex-1 · v0\.4\.0 → v0\.4\.1/);assert.match(t,/release signature/);assert.match(t,/<b>x<\/b>/);
  assert.match(t,/BCC signs one update_baft job for ex-1 asking for v0\.4\.1/);
  assert.match(t,/FALLBACK <b>text<\/b>/);
  assert.equal(main.all(e=>e.tag==='b'&&['x','text'].includes(e.textContent)).length,0);
  assert.match(t,/sudo bash install\.sh --offline \. --allow-downgrade --yes/);
  const planBody=JSON.parse(seen.find(([u])=>u.endsWith('updates/plan'))[1].body);
  assert.deepEqual(planBody,{node_id:'ex-1',kind:'upgrade',target_digest:'d2'});
  const input=main.all(e=>e.tag==='input'&&e.className.includes('uc-confirm'))[0];
  const apply=findBtn(main,'Queue the signed update job');assert.ok(apply&&apply.disabled);
  input.value='update ex-1 v0.4.1';await input.fire('input');assert.equal(apply.disabled,true);
  input.value='UPDATE ex-1 v0.4.1';await input.fire('input');assert.equal(apply.disabled,false);
  await apply.fire('click');await flush();
  const body=JSON.parse(seen.find(([u])=>u.endsWith('updates/apply'))[1].body);
  assert.deepEqual(body,{node_id:'ex-1',kind:'upgrade',target_digest:'d2',plan_hash:plan.hash,confirmation:'UPDATE ex-1 v0.4.1'});
  assert.match(main.textContent,/queued for the agent/);
});

test('a blocked plan offers no apply, and rollback is installer-only',async()=>{
  const plan={hash:'h',ok:false,node_id:'ex-2',kind:'rollback',from_version:'v0.4.1',to_version:'v0.4.0',target_digest:'d1',executor:'installer',
    gates:[{name:'x',status:'FAIL',detail:'no'}],steps:[],verify:[],rollback:[],notes:[],installer_preview:'sudo bash install.sh --offline . --allow-downgrade --plan',installer_command:'sudo bash install.sh --offline . --allow-downgrade --yes'};
  const {U}=setup({fetchImpl:async(u)=>u.endsWith('updates/plan')?resp(200,plan):resp(200,VIEW)});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  const roll=findBtn(main,'Rollback plan');assert.ok(roll);
  await roll.fire('click');await flush();await flush();
  assert.ok(!findBtn(main,'Queue the signed update job'));
  assert.match(main.textContent,/cannot be applied from BCC/);assert.match(main.textContent,/verified installer on the server itself/);
});

test('an apply refused for drift tells the operator to plan again',async()=>{
  const plan={hash:'h',ok:true,node_id:'ex-1',kind:'upgrade',from_version:'v0.4.0',to_version:'v0.4.1',target_digest:'d2',executor:'agent_job',gates:[],steps:[],verify:[],rollback:[],notes:[],apply:{endpoint:'/x',confirmation:'UPDATE ex-1 v0.4.1'}};
  const {U}=setup({fetchImpl:async(u)=>u.endsWith('updates/plan')?resp(200,plan):u.endsWith('updates/apply')?resp(409,'plan drifted'):resp(200,VIEW)});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  await findBtn(main,'Upgrade plan').fire('click');await flush();await flush();
  const input=main.all(e=>e.tag==='input'&&e.className.includes('uc-confirm'))[0];input.value='UPDATE ex-1 v0.4.1';await input.fire('input');
  await findBtn(main,'Queue the signed update job').fire('click');await flush();
  assert.match(main.textContent,/plan changed since you reviewed it/);
});

test('without a pinned root the import is disabled and the page says why',async()=>{
  const {U}=setup({fetchImpl:async()=>resp(200,{...VIEW,root_pinned:false,releases:[],nodes:[]})});
  const main=new El('main');U.mount(main,ctx());await flush();await flush();
  assert.equal(findBtn(main,'Import signed bundle').disabled,true);assert.match(main.textContent,/No release root is pinned/);
});

test('age formatting and step fallback',()=>{
  const {U}=setup({fetchImpl:none});
  assert.equal(U.ago('en',60),'60 s ago');assert.equal(U.ago('en',600),'10 min ago');assert.equal(U.ago('en',7200),'2 h ago');
  assert.equal(U.ago('en',undefined),'—');assert.equal(U.ago('en',-1),'—');
  assert.equal(U.stepText('fa',{},{code:'zzz',text:'raw'}),'raw');
  assert.equal(U.bundleIdentity({manifest:{payload:'###'}}),null);
});

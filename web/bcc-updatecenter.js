/* BAFT Update Center page (v0.4.0).
   Everything that comes from the server is put on the page with textContent
   only: this file builds markup with DOM calls, never from HTML strings, and has no inline handlers,
   so it is safe under the Command Center CSP. BCC never updates by itself: the
   page imports a signed bundle, shows verified versions and prepares a reviewed
   plan; the update itself is run by the existing signed agent job or by the
   verified installer on the server. */
(()=>{
'use strict';
const UC={
fa:{
 title:'مرکز به‌روزرسانی',sub:'نسخه‌های امضاشدهٔ BAFT، وضعیت هر سرور و برنامهٔ بازبینی‌شدهٔ ارتقا یا بازگشت',
 never:'BCC هرگز خودش به‌روزرسانی نمی‌کند. فقط بستهٔ امضاشده با کلید Root پین‌شده پذیرفته می‌شود و هر ارتقا را شما، برای یک سرور، پس از بازبینی برنامه اجرا می‌کنید.',
 import:'درون‌ریزی بستهٔ امضاشده',importing:'در حال بررسی امضا…',importHint:'فایل JSON ساخته‌شده با scripts/release/update_center_bundle.sh از بستهٔ آفلاین',
 importOk:'درون‌ریزی شد: ',importSame:'بدون تغییر (قبلاً درون‌ریزی شده بود): ',importRev:'فهرست ابطال به‌روز شد',
 importFail:'درون‌ریزی رد شد',f400:'بسته نامعتبر، بدون امضا یا دست‌کاری‌شده است',f409:'رد شد (ابطال‌شده، قدیمی‌تر از کف مجاز، یا نسخهٔ تکراری)',f413:'فایل بزرگ‌تر از حد مجاز است',f503:'کلید Root روی این BCC پین نشده است',
 asRollback:'درون‌ریزی فقط به‌عنوان گزینهٔ بازگشت',rollbackOnlyNote:'فقط برای بازگشت',
 noRoot:'کلید Root انتشار روی BCC پین نشده؛ درون‌ریزی غیرفعال است (baft-bcc --update-root-pub).',
 root:'کلید Root',pinned:'پین‌شده',floor:'کف نسخه',revSeq:'شمارهٔ فهرست ابطال',revExpired:'فهرست ابطال منقضی شده؛ بستهٔ تازه درون‌ریزی کنید.',latest:'آخرین نسخهٔ تأییدشده',none:'—',
 releases:'نسخه‌های درون‌ریزی‌شده',noReleases:'هنوز نسخه‌ای درون‌ریزی نشده. بستهٔ آفلاین امضاشده را درون‌ریزی کنید.',
 colVersion:'نسخه',colChannel:'کانال',colCommit:'کامیت',colSig:'امضا',colState:'وضعیت',colReceived:'دریافت',
 sig_verified:'تأییدشده',sig_invalid:'نامعتبر',sig_unpinned:'بدون Root',revoked:'ابطال‌شده',stable:'پایدار',prerelease:'پیش‌انتشار',
 servers:'سرورها',noServers:'سروری ثبت نشده.',colServer:'سرور',colRole:'نقش',colInstalled:'نصب‌شده',colLatest:'آخرین',colStatus:'وضعیت',colReport:'آخرین گزارش',colAction:'عملیات',
 st_current:'به‌روز',st_update_available:'به‌روزرسانی موجود',st_downgrade_blocked:'جلوتر از فهرست',st_unknown:'نامشخص',st_revoked:'نسخهٔ ابطال‌شده',
 n_update_available:'نسخهٔ تأییدشدهٔ جدیدتری هست؛ پیش از اجرا برنامه را بازبینی کنید.',n_current:'آخرین نسخهٔ تأییدشده اجرا می‌شود.',n_revoked_installed:'نسخهٔ نصب‌شده با کلید ابطال‌شده امضا شده؛ به نسخهٔ تأییدشده بروید.',n_ahead_of_catalog:'نسخهٔ نصب‌شده از آخرین نسخهٔ درون‌ریزی‌شده جلوتر است؛ کاهش نسخه پیشنهاد نمی‌شود.',n_node_revoked:'دسترسی این سرور قطع شده است.',n_no_platform:'سیستم‌عامل/معماری تأییدشده توسط اپراتور ثبت نشده.',n_no_report:'هنوز گزارش اجزا از این سرور نرسیده.',n_stale_report:'آخرین گزارش برای اعتماد بیش از حد قدیمی است.',n_unmatched_digest:'باینری نصب‌شده با هیچ نسخهٔ امضاشدهٔ درون‌ریزی‌شده نمی‌خواند.',n_no_release:'نسخهٔ تأییدشده‌ای درون‌ریزی نشده.',n_no_root:'کلید Root پین نشده است.',
 ago_s:s=>s+' ثانیه پیش',ago_m:m=>m+' دقیقه پیش',ago_h:h=>h+' ساعت پیش',ago_d:d=>d+' روز پیش',
 plan:'برنامهٔ ارتقا',rollback:'برنامهٔ بازگشت',rollbackPick:'نسخهٔ مقصد بازگشت',noRollbackTarget:'نسخهٔ قدیمی‌تری درون‌ریزی نشده.',planning:'در حال ساخت برنامه…',planFail:'ساخت برنامه ناموفق بود.',
 planTitle:'برنامه برای',gates:'بررسی‌ها',steps:'مراحل',verify:'راستی‌آزمایی',rollbackGuide:'راهنمای بازگشت',notes:'نکته‌ها',planHash:'هش برنامه',
 preview:'پیش‌نمایش نصب‌کننده (چیزی تغییر نمی‌کند)',command:'دستور دقیق',copy:'کپی',copied:'کپی شد',close:'بستن',
 pass:'قبول',warn:'هشدار',failG:'ناموفق',planBlocked:'این برنامه گیت ناموفق دارد و از BCC اجرا نمی‌شود.',
 confirmLabel:'برای اجرا این عبارت را دقیق تایپ کنید:',apply:'ارسال کار به‌روزرسانی امضاشده',applying:'در حال ارسال…',applied:'کار update_baft امضاشده در صف عامل قرار گرفت. عامل خودش دانلود و راستی‌آزمایی می‌کند.',applyDrift:'برنامه از زمان بازبینی تغییر کرده؛ دوباره برنامه بسازید.',applyFail:'ارسال ناموفق بود: ',
 rollbackOnly2:'بازگشت همیشه با نصب‌کنندهٔ راستی‌آزمایی‌شده روی خود سرور انجام می‌شود؛ BCC آن را اجرا نمی‌کند.',
 s_agent_job_sign:(p)=>'BCC یک کار امضاشدهٔ update_baft برای '+p.node_id+' با نسخهٔ '+p.to_version+' می‌سازد. چیزی «فشار داده» نمی‌شود؛ عامل کار را می‌گیرد.',
 s_agent_job_verify:(p)=>'عامل '+p.to_version+' را از آدرس انتشار پیکربندی‌شدهٔ خودش می‌گیرد و با release.VerifyDir در برابر کلید Root پین‌شده، فهرست ابطال و وضعیت اعتماد نصب (بدون کاهش نسخه و بازبرچسب) راستی‌آزمایی می‌کند. اگر تأیید نشود چیزی تغییر نمی‌کند.',
 s_agent_job_swap:()=>'baft و baft-pair جایگزین و سرویس راه‌اندازی مجدد می‌شود؛ نسخهٔ قبلی با پسوند .prev می‌ماند. baft-agent و baft-bcc با این کار عوض نمی‌شوند.',
 s_agent_job_health:()=>'اگر سرویس بعد از زمان تثبیت فعال نباشد، عامل فایل‌های .prev را برمی‌گرداند، دوباره راه می‌اندازد، شکست را گزارش می‌کند و وضعیت اعتماد را جلو نمی‌برد.',
 s_rollback_auto:()=>'خودکار: اگر سرویس بالا نیاید نسخهٔ قبلی برگردانده می‌شود (بدون اقدام شما).',
 s_rollback_prev:()=>'روی سرور، پس از ارتقای موفق اما ناخواسته: baft.prev و baft-pair.prev کنار باینری‌ها هستند؛ نصب‌کنندهٔ آفلاین زیر کل نسخهٔ قبلی را با راستی‌آزمایی خودش برمی‌گرداند.',
 s_rollback_installer:(p)=>'بازگشت با نصب‌کنندهٔ آفلاین به '+(p.from_version||'—')+' (بستهٔ آن نسخه لازم است):',
 s_installer_obtain:(p)=>'بستهٔ baft-offline-'+p.to_version+'.tar.gz و فایل .sha256 آن را از مسیر مطمئن بگیرید و checksum را مقایسه کنید.',
 s_installer_extract:()=>'روی سرور باز کنید:',
 s_installer_preview:()=>'پیش‌نمایش تغییرات (چیزی عوض نمی‌شود). همان گزینه‌های نقش نصب اولیه را بدهید (baft status نشان می‌دهد):',
 s_installer_run:()=>'اجرا. نصب‌کننده نسخه را در برابر کلید Root داخل install.sh و فهرست ابطال همراه بسته راستی‌آزمایی می‌کند، پشتیبان می‌گیرد، بالا ماندن سرویس را می‌سنجد و در شکست فایل‌های قبلی را برمی‌گرداند:',
 s_rollback_installer_auto:()=>'اگر هر مرحله شکست بخورد نصب‌کننده خودش فایل‌ها و وضعیت سرویس را برمی‌گرداند و نسخه را فقط پس از سالم بودن سرویس ثبت می‌کند.',
 s_rollback_forward:()=>'اگر نسخهٔ برگشتی هم مشکل داشت، نصب‌کننده را با بستهٔ جدیدتر (بدون --allow-downgrade) اجرا کنید؛ کلیدها، جفت‌سازی و تونل‌ها حفظ می‌شوند.',
 s_verify_report:(p)=>'ظرف ۱۵ دقیقه سرور هش باینری‌ها را گزارش می‌دهد. این ردیف فقط وقتی «به‌روز» می‌شود که هش baft گزارش‌شده با هش امضاشدهٔ '+p.to_version+' برابر باشد.',
 s_verify_health:()=>'پیش از سرور بعدی می‌توانید یک کار سلامت (baft doctor) بفرستید. سرورها را یکی‌یکی به‌روز کنید.',
 s_note_agent_config:()=>'عامل باید به‌روزرسانی انتشار را پیکربندی داشته باشد (آدرس پایهٔ انتشار و Root پین‌شده). اگر نداشته باشد کار بدون آسیب شکست می‌خورد؛ از مسیر نصب‌کننده استفاده کنید. سرورهای بدون دسترسی به آدرس انتشار از بستهٔ آفلاین استفاده می‌کنند.',
 s_note_downgrade_gate:()=>'وضعیت ضد-کاهش‌نسخهٔ خود نصب‌کننده نسخهٔ قدیمی‌تر را بدون --allow-downgrade رد می‌کند. BCC این را برنمی‌دارد؛ عمدی است.',
 s_note_never_auto:()=>'BCC هرگز خودش به‌روزرسانی نمی‌کند؛ این برنامه فقط مراحل بازبینی‌شده را آماده می‌کند.',
 s_note_bcc_self:()=>'مرکز به‌روزرسانی خود BCC را عوض نمی‌کند. baft-bcc را با نصب‌کننده روی میزبان BCC ارتقا دهید؛ تغییر schema پایگاه‌داده یا پشتیبان بدون نسخهٔ پشتیبان تأییدشدهٔ پیش از ارتقا قابل برگشت نیست (KNOWN-LIMITATIONS).',
 s_note_unknown_installed:()=>'نسخهٔ نصب‌شده نامشخص است؛ این برنامه از BCC قابل اجرا نیست.',
 badge:n=>n+' به‌روزرسانی',badgeLabel:'اعلان به‌روزرسانی',demo:'پیش‌نمایش نمایشی؛ مرکز به‌روزرسانی فقط روی BCC زنده کار می‌کند.',loadFail:'دریافت اطلاعات ناموفق بود؛ دوباره تلاش کنید.',refresh:'تازه‌سازی'
},
en:{
 title:'Update Center',sub:'Signed BAFT releases, the state of every server, and reviewed upgrade and rollback plans',
 never:'BCC never updates by itself. Only a bundle signed under the pinned release root is accepted, and you run each upgrade for one server after reviewing its plan.',
 import:'Import signed bundle',importing:'Verifying signature…',importHint:'JSON file made by scripts/release/update_center_bundle.sh from an offline bundle',
 importOk:'Imported: ',importSame:'Unchanged (already imported): ',importRev:'Revocation list updated',
 importFail:'Import refused',f400:'The bundle is invalid, unsigned or tampered with',f409:'Refused (revoked, older than the floor, or a duplicate version)',f413:'The file is larger than the limit',f503:'The release root is not pinned on this BCC',
 asRollback:'Import as rollback-only',rollbackOnlyNote:'rollback only',
 noRoot:'No release root is pinned on this BCC; imports are disabled (baft-bcc --update-root-pub).',
 root:'Release root',pinned:'pinned',floor:'Version floor',revSeq:'Revocation list',revExpired:'The revocation list expired; import a fresh bundle.',latest:'Latest verified release',none:'—',
 releases:'Imported releases',noReleases:'No release imported yet. Import a signed offline bundle.',
 colVersion:'Version',colChannel:'Channel',colCommit:'Commit',colSig:'Signature',colState:'State',colReceived:'Received',
 sig_verified:'Verified',sig_invalid:'Invalid',sig_unpinned:'No root',revoked:'Revoked',stable:'Stable',prerelease:'Pre-release',
 servers:'Servers',noServers:'No servers enrolled.',colServer:'Server',colRole:'Role',colInstalled:'Installed',colLatest:'Latest',colStatus:'Status',colReport:'Last report',colAction:'Action',
 st_current:'Current',st_update_available:'Update available',st_downgrade_blocked:'Ahead of catalog',st_unknown:'Unknown',st_revoked:'Revoked release',
 n_update_available:'A newer verified release is available; review the plan before applying it.',n_current:'Running the latest verified release.',n_revoked_installed:'The installed release was signed with a revoked key; move to a verified release.',n_ahead_of_catalog:'Installed version is newer than the latest imported release; no downgrade is offered.',n_node_revoked:'Access for this server is revoked.',n_no_platform:'Operator-confirmed platform is missing; confirm OS/architecture first.',n_no_report:'No component report from this server yet.',n_stale_report:'The last component report is too old to trust.',n_unmatched_digest:'Installed binary does not match any imported signed release.',n_no_release:'No verified release is imported; import a signed bundle.',n_no_root:'The release root is not pinned on this BCC.',
 ago_s:s=>s+' s ago',ago_m:m=>m+' min ago',ago_h:h=>h+' h ago',ago_d:d=>d+' d ago',
 plan:'Upgrade plan',rollback:'Rollback plan',rollbackPick:'Rollback target',noRollbackTarget:'No older release is imported.',planning:'Building plan…',planFail:'Could not build the plan.',
 planTitle:'Plan for',gates:'Checks',steps:'Steps',verify:'Verify',rollbackGuide:'Rollback guidance',notes:'Notes',planHash:'Plan hash',
 preview:'Installer preview (changes nothing)',command:'Exact command',copy:'Copy',copied:'Copied',close:'Close',
 pass:'PASS',warn:'WARN',failG:'FAIL',planBlocked:'This plan has failing checks and cannot be applied from BCC.',
 confirmLabel:'To apply, type this exactly:',apply:'Queue the signed update job',applying:'Sending…',applied:'The signed update_baft job is queued for the agent. The agent downloads and verifies it itself.',applyDrift:'The plan changed since you reviewed it; plan again.',applyFail:'Could not queue: ',
 rollbackOnly2:'Rollback is always done with the verified installer on the server itself; BCC does not run it.',
 s_agent_job_sign:(p)=>'BCC signs one update_baft job for '+p.node_id+' asking for '+p.to_version+'. Nothing is pushed: the agent pulls the job.',
 s_agent_job_verify:(p)=>'The agent downloads '+p.to_version+' from its own configured release base URL and verifies it with release.VerifyDir against its pinned release root, the current revocation list and its install trust state (no downgrade, no re-tag). A release that does not verify changes nothing.',
 s_agent_job_swap:()=>'baft and baft-pair are replaced and the service is restarted; the previous binaries are kept as .prev. baft-agent and baft-bcc are not replaced by this job.',
 s_agent_job_health:()=>'If the service is not active after the settle time, the agent restores the .prev binaries, restarts, reports the job as failed, and does not advance its trust state.',
 s_rollback_auto:()=>'Automatic: a failed start restores the previous binaries (no operator action).',
 s_rollback_prev:()=>'On the node, after a successful but unwanted update: the previous baft and baft-pair are next to the binaries as baft.prev and baft-pair.prev; the offline installer below restores the whole previous release with its own verification.',
 s_rollback_installer:(p)=>'Offline installer rollback to '+(p.from_version||'—')+' (needs that release\'s bundle):',
 s_installer_obtain:(p)=>'Get baft-offline-'+p.to_version+'.tar.gz and its .sha256 through a channel you trust and compare the checksum.',
 s_installer_extract:()=>'Unpack it on the server:',
 s_installer_preview:()=>'Preview what it would change (changes nothing). Use the same role flags as the original install of this server (baft status shows them):',
 s_installer_run:()=>'Apply it. The installer verifies the release against the root pinned in install.sh and the bundled revocation list, keeps backups, checks the service stays up and restores the old files if not:',
 s_rollback_installer_auto:()=>'The installer restores the previous files and service state itself if any step fails, and records the release only after the service is healthy.',
 s_rollback_forward:()=>'If the rolled-back release is itself wrong, rerun the installer with the newest bundle (no --allow-downgrade): secrets, pairing and tunnels are kept.',
 s_verify_report:(p)=>'Within 15 minutes the node reports its binary digests. This row turns to current only when the reported baft digest equals the signed digest for '+p.to_version+'.',
 s_verify_health:()=>'Optionally dispatch a health job to the node (runs baft doctor) before the next server. Update servers one at a time.',
 s_note_agent_config:()=>'The agent must have release updates configured (release base URL and pinned release root). If it does not, the job fails harmlessly; use the installer path instead. Servers that cannot reach the release URL use the offline bundle.',
 s_note_downgrade_gate:()=>'The installer\'s own anti-downgrade state refuses an older release unless --allow-downgrade is given. BCC cannot lift that for you; this is deliberate.',
 s_note_never_auto:()=>'BCC never applies an update by itself: this plan only prepares reviewed steps.',
 s_note_bcc_self:()=>'The Update Center does not update BCC itself. To upgrade baft-bcc, use the installer on the BCC host; a SQLite or backup schema change cannot be downgraded without a verified pre-upgrade snapshot (see KNOWN-LIMITATIONS).',
 s_note_unknown_installed:()=>'The installed version is unknown, so this plan cannot be applied from BCC.',
 badge:n=>n+' update'+(n===1?'':'s'),badgeLabel:'Update notices',demo:'Demo preview; the Update Center works only on a live BCC.',loadFail:'Could not load data. Try again.',refresh:'Refresh'
}};

const STATUSES=['current','update-available','downgrade-blocked','unknown','revoked'];
const statusKey=s=>'st_'+String(s).replace(/-/g,'_');
const NOTICES=['update_available','current','revoked_installed','ahead_of_catalog','node_revoked','no_platform','no_report','stale_report','unmatched_digest','no_release','no_root'];
const STEP_CODES=Object.keys(UC.en).filter(k=>k.startsWith('s_')).map(k=>k.slice(2));

function tr(l,k,...a){const v=(UC[l]&&UC[l][k]!==undefined)?UC[l][k]:UC.en[k];return typeof v==='function'?v(...a):(v===undefined?String(k):v)}
function ago(l,sec){if(typeof sec!=='number'||!(sec>=0))return'—';if(sec<90)return tr(l,'ago_s',sec);if(sec<5400)return tr(l,'ago_m',Math.round(sec/60));if(sec<129600)return tr(l,'ago_h',Math.round(sec/3600));return tr(l,'ago_d',Math.round(sec/86400))}
/* A server-supplied step is shown in the operator's language when its code is known, else as the server's own text. */
function stepText(l,plan,step){const k='s_'+step.code;const f=UC[l][k];return typeof f==='function'?f(plan):String(step.text||'')}
function bundleIdentity(b){try{const m=JSON.parse(atob(b.manifest.payload));return{version:String(m.version||''),commit:String(m.commit||'')}}catch{return null}}
function errKey(status){return status===400||status===409||status===413||status===503?'f'+status:null}

const api={
  _ctx:null,
  count:0,
  data:null
};

function h(tag,props,...kids){const n=document.createElement(tag);if(props)for(const[k,v]of Object.entries(props)){if(v==null||v===false)continue;if(k==='class')n.className=v;else if(k==='text')n.textContent=v;else if(k==='on'){for(const[ev,fn]of Object.entries(v))n.addEventListener(ev,fn)}else n.setAttribute(k,v===true?'':String(v))}
  for(const c of kids.flat()){if(c==null||c===false)continue;n.append(typeof c==='string'?document.createTextNode(c):c)}return n}
const badge=(cls,text)=>h('span',{class:'badge'+(cls?' '+cls:''),text});
function table(cols,rows){return h('div',{class:'table-wrap'},h('table',null,h('thead',null,h('tr',null,cols.map(c=>h('th',{text:c})))),h('tbody',null,rows)))}
const td=(x,cls)=>h('td',{class:cls||null},x);

function paintBadge(){const a=document.getElementById('updBadge'),n=document.getElementById('updBadgeN');if(!a||!n)return;const c=api.count,l=api._ctx?api._ctx.lang():'fa';a.hidden=!(c>0);if(c>0){n.textContent=tr(l,'badge',c);a.setAttribute('aria-label',tr(l,'badgeLabel')+': '+c)}}
function set(view){api.data=view&&typeof view==='object'?view:null;api.count=api.data&&Number.isFinite(api.data.notice_count)?api.data.notice_count:0;paintBadge()}

function mount(main,ctx){
  api._ctx=ctx;
  const L=()=>ctx.lang(),T=(k,...a)=>tr(L(),k,...a);
  const status=h('div',{class:'status-line',role:'status','aria-live':'polite'});
  const body=h('div',{class:'stack uc-body'});
  const fileIn=h('input',{type:'file',accept:'application/json,.json',class:'uc-file',hidden:true,'aria-label':T('import')});
  const importBtn=h('button',{type:'button',class:'btn gold small',text:T('import'),title:T('importHint')});
  const refreshBtn=h('button',{type:'button',class:'btn small',text:T('refresh')});
  const planBox=h('section',{class:'card uc-plan',hidden:true,'aria-live':'polite'});
  const pending={bundle:null};
  const asRollbackBtn=h('button',{type:'button',class:'btn small',text:T('asRollback'),hidden:true});

  const root=h('div',{class:'uc'},
    h('section',{class:'card'},
      h('div',{class:'card-head'},h('div',null,h('h2',{text:T('title')}),h('p',{class:'sub',text:T('sub')})),h('div',{class:'page-actions'},refreshBtn,importBtn,fileIn)),
      h('p',{class:'muted uc-never',text:T('never')}),status,asRollbackBtn),
    body,planBox);
  main.append(root);

  if(!ctx.live){status.textContent=T('demo');return}

  function setStatus(msg,bad){status.textContent=msg||'';status.classList.toggle('bad',!!bad)}

  async function call(path,method,payload,headers){
    const cfg=ctx.cfg,hd={Accept:'application/json'};
    if(method!=='GET'){hd['Content-Type']='application/json';hd[cfg.csrfHeader]=cfg.csrf}
    if(headers)Object.assign(hd,headers);
    const r=await fetch(cfg.base+'api/'+path,{method,credentials:'same-origin',cache:'no-store',redirect:'error',headers:hd,body:payload!=null?JSON.stringify(payload):undefined});
    if(r.status===401){window.dispatchEvent(new Event('baft:session-expired'));throw Object.assign(Error('SESSION'),{status:401})}
    const text=await r.text();let json=null;try{json=JSON.parse(text)}catch{}
    return{status:r.status,ok:r.ok,json,text:text.slice(0,300)}}

  async function load(){
    try{const r=await call('updates','GET');if(!r.ok||!r.json)throw Error('HTTP');set(r.json);paint(r.json)}
    catch(e){if(e.message!=='SESSION'){body.replaceChildren(h('div',{class:'empty',text:T('loadFail')}))}}}

  function paint(v){
    const rel=Array.isArray(v.releases)?v.releases:[],nodes=Array.isArray(v.nodes)?v.nodes:[];
    const facts=[];
    facts.push(h('div',{class:'uc-fact'},h('b',{text:T('root')}),' ',v.root_pinned?badge('up',T('pinned')):badge('down',T('none'))));
    facts.push(h('div',{class:'uc-fact'},h('b',{text:T('floor')}),' ',h('span',{class:'ltr num',text:v.floor||T('none')})));
    facts.push(h('div',{class:'uc-fact'},h('b',{text:T('revSeq')}),' ',h('span',{class:'ltr num',text:String(v.revocation_sequence||0)})));
    facts.push(h('div',{class:'uc-fact'},h('b',{text:T('latest')}),' ',h('span',{class:'ltr num',text:v.latest?v.latest.version:T('none')})));
    const warn=[];
    if(!v.root_pinned)warn.push(h('p',{class:'uc-warn',text:T('noRoot')}));
    if(v.revocation_expired)warn.push(h('p',{class:'uc-warn',text:T('revExpired')}));
    importBtn.disabled=!v.root_pinned;

    const relRows=rel.map(r=>h('tr',null,
      td(r.version,'ltr num'),td(T(r.channel==='prerelease'?'prerelease':'stable')),td(String(r.commit||'').slice(0,12),'ltr num'),
      td(badge(r.signature_status==='verified'?'up':'down',T('sig_'+r.signature_status))),
      td([r.revoked?badge('down',T('revoked')):null,r.rollback_only?badge('gold',T('rollbackOnlyNote')):null]),
      td(ctx.when(r.received_at))));
    const relCard=h('section',{class:'card'},h('div',{class:'card-head'},h('h2',{text:T('releases')})),
      rel.length?table([T('colVersion'),T('colChannel'),T('colCommit'),T('colSig'),T('colState'),T('colReceived')],relRows):h('div',{class:'empty',text:T('noReleases')}));

    const srvRows=nodes.map(n=>{
      const cls=n.status==='current'?'up':n.status==='update-available'?'gold':n.status==='revoked'?'down':n.status==='downgrade-blocked'?'blue':'';
      const notice=NOTICES.includes(n.notice_code)?T('n_'+n.notice_code):String(n.notice||'');
      const acts=h('div',{class:'uc-acts'});
      if(n.status==='update-available'&&v.latest)acts.append(h('button',{type:'button',class:'btn small',text:T('plan'),on:{click:()=>openPlan(n,'upgrade',v.latest.digest)}}));
      const older=rel.filter(r=>r.signature_status==='verified'&&!r.revoked&&n.installed_version&&cmp(r.version,n.installed_version)<0);
      if(n.installed_version&&older.length)acts.append(rollbackPicker(n,older));
      return h('tr',null,
        td([h('b',{text:n.alias||n.node_id}),h('br'),h('small',{class:'ltr',text:n.node_id})]),
        td(n.role==='foreign'?'EX':n.role==='master'?'Master':'IR'),
        td(n.installed_version||T('none'),'ltr num'),td(n.latest_version||T('none'),'ltr num'),
        td([badge(cls,T(statusKey(n.status))),h('br'),h('small',{text:notice})]),
        td(ago(L(),n.last_report_age_seconds)),td(acts));});
    const srvCard=h('section',{class:'card'},h('div',{class:'card-head'},h('h2',{text:T('servers')})),
      nodes.length?table([T('colServer'),T('colRole'),T('colInstalled'),T('colLatest'),T('colStatus'),T('colReport'),T('colAction')],srvRows):h('div',{class:'empty',text:T('noServers')}));
    body.replaceChildren(h('section',{class:'card uc-facts'},facts,warn),srvCard,relCard);
  }

  function cmp(a,b){const p=s=>String(s).replace(/^v/,'').split('-')[0].split('.').map(Number),x=p(a),y=p(b);for(let i=0;i<3;i++){if((x[i]||0)!==(y[i]||0))return(x[i]||0)<(y[i]||0)?-1:1}
    const pa=String(a).includes('-'),pb=String(b).includes('-');return pa===pb?0:pa?-1:1}

  function rollbackPicker(n,older){
    const sel=h('select',{'aria-label':T('rollbackPick'),class:'uc-select'},older.map(r=>h('option',{value:r.digest,text:r.version})));
    const btn=h('button',{type:'button',class:'btn small danger',text:T('rollback'),on:{click:()=>openPlan(n,'rollback',sel.value)}});
    return h('span',{class:'uc-roll'},sel,btn)}

  async function openPlan(n,kind,digest){
    planBox.hidden=false;planBox.replaceChildren(h('p',{class:'muted',text:T('planning')}));
    try{
      const r=await call('updates/plan','POST',{node_id:n.node_id,kind,target_digest:digest});
      if(!r.ok||!r.json){planBox.replaceChildren(h('p',{class:'uc-warn',text:T('planFail')+' '+(r.text||'')}),closeBtn());return}
      paintPlan(r.json)}
    catch(e){if(e.message!=='SESSION')planBox.replaceChildren(h('p',{class:'uc-warn',text:T('planFail')}),closeBtn())}}

  function closeBtn(){return h('button',{type:'button',class:'btn small',text:T('close'),on:{click:()=>{planBox.hidden=true;planBox.replaceChildren()}}})}

  function cmd(label,c){
    const code=h('code',{class:'ltr uc-cmd',dir:'ltr',text:c});
    const copy=h('button',{type:'button',class:'btn small',text:T('copy'),on:{click:async()=>{try{await navigator.clipboard.writeText(c);copy.textContent=T('copied')}catch{}}}});
    return h('div',{class:'uc-cmdrow'},label?h('small',{text:label}):null,h('div',{class:'uc-cmdline'},code,copy))}

  function stepList(plan,steps){return h('ol',{class:'uc-steps'},(steps||[]).map(s=>h('li',null,h('span',{text:stepText(L(),plan,s)}),s.cmd?cmd('',s.cmd):null)))}

  function paintPlan(p){
    const gates=h('ul',{class:'uc-gates'},(p.gates||[]).map(g=>h('li',null,badge(g.status==='PASS'?'up':g.status==='WARN'?'gold':'down',T(g.status==='PASS'?'pass':g.status==='WARN'?'warn':'failG')),' ',h('b',{text:g.name}),g.detail?h('span',{text:' — '+g.detail}):null)));
    const kids=[
      h('div',{class:'card-head'},h('h2',{text:T('planTitle')+' '+p.node_id+' · '+(p.from_version||'—')+' → '+p.to_version}),closeBtn()),
      h('p',{class:'muted ltr',text:T('planHash')+': '+p.hash}),
      h('h3',{text:T('gates')}),gates];
    if(!p.ok)kids.push(h('p',{class:'uc-warn',text:T('planBlocked')}));
    if(p.executor==='installer')kids.push(h('p',{class:'muted',text:T('rollbackOnly2')}));
    kids.push(h('h3',{text:T('steps')}),stepList(p,p.steps));
    if(p.installer_preview||p.installer_command){kids.push(cmd(T('preview'),p.installer_preview||''),cmd(T('command'),p.installer_command||''))}
    kids.push(h('h3',{text:T('verify')}),stepList(p,p.verify),h('h3',{text:T('rollbackGuide')}),stepList(p,p.rollback),h('h3',{text:T('notes')}),stepList(p,p.notes));
    if(p.ok&&p.apply)kids.push(applyBox(p));
    planBox.replaceChildren(...kids)}

  function applyBox(p){
    const out=h('div',{class:'status-line',role:'status'});
    const input=h('input',{type:'text',class:'uc-confirm ltr',dir:'ltr',autocomplete:'off',spellcheck:'false','aria-label':T('confirmLabel')});
    const btn=h('button',{type:'button',class:'btn gold',text:T('apply'),disabled:true});
    input.addEventListener('input',()=>{btn.disabled=input.value!==p.apply.confirmation});
    btn.addEventListener('click',async()=>{
      if(input.value!==p.apply.confirmation)return;btn.disabled=true;out.textContent=T('applying');
      try{
        const r=await call('updates/apply','POST',{node_id:p.node_id,kind:p.kind,target_digest:p.target_digest,plan_hash:p.hash,confirmation:input.value});
        if(r.status===202){out.textContent=T('applied');load()}
        else if(r.status===409){out.textContent=T('applyDrift')+' '+(r.text||'')}
        else out.textContent=T('applyFail')+r.status+' '+(r.text||'')}
      catch(e){if(e.message!=='SESSION')out.textContent=T('applyFail')}});
    return h('div',{class:'uc-apply'},h('p',null,T('confirmLabel')+' '),h('code',{class:'ltr uc-cmd',dir:'ltr',text:p.apply.confirmation}),input,btn,out)}

  async function doImport(text,forceRollback){
    let bundle;try{bundle=JSON.parse(text)}catch{setStatus(T('importFail')+': '+T('f400'),true);return}
    pending.bundle=bundle;asRollbackBtn.hidden=true;setStatus(T('importing'));importBtn.disabled=true;
    try{
      const id=bundle&&bundle.manifest?bundleIdentity(bundle):null;
      const hdr=forceRollback&&id?{'X-BAFT-Downgrade-Confirmation':'DOWNGRADE '+id.version+' '+id.commit}:null;
      const r=await call('updates/import','POST',bundle,hdr);
      if(r.ok&&r.json){
        const j=r.json;let msg=j.status==='imported'?(j.version?T('importOk')+j.version+(j.rollback_only?' ('+T('rollbackOnlyNote')+')':''):T('importRev')):(j.version?T('importSame')+j.version:T('importSame'));
        setStatus(msg);await load()}
      else{
        const k=errKey(r.status);setStatus(T('importFail')+': '+(k?T(k):String(r.status))+(r.text?' — '+r.text.trim():''),true);
        if(r.status===409&&id&&/floor|older/.test(r.text))asRollbackBtn.hidden=false}}
    catch(e){if(e.message!=='SESSION')setStatus(T('importFail'),true)}
    finally{importBtn.disabled=false;fileIn.value=''}}

  importBtn.addEventListener('click',()=>fileIn.click());
  refreshBtn.addEventListener('click',load);
  asRollbackBtn.addEventListener('click',()=>{if(pending.bundle)doImport(JSON.stringify(pending.bundle),true)});
  fileIn.addEventListener('change',async()=>{const f=fileIn.files&&fileIn.files[0];if(!f)return;
    if(f.size>262144){setStatus(T('importFail')+': '+T('f413'),true);fileIn.value='';return}
    doImport(await f.text(),false)});
  load();
}

const exported={UC,tr,ago,stepText,statusKey,bundleIdentity,STATUSES,NOTICES,STEP_CODES,mount,set,paintBadge,get count(){return api.count}};
if(typeof module!=='undefined'&&module.exports)module.exports=exported;
else window.BAFT_UPDATES=exported;
})();

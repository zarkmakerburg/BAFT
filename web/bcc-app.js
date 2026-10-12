/* BAFT Command Center. The page is served by BCC with window.BAFT_UI; reads
   use the session cookie and every write sends the session's CSRF header.
   Opened as a static file (no BAFT_UI) it shows a clearly marked demo. */
(()=>{'use strict';
const cfg=window.BAFT_UI||{mode:'preview',base:''},live=cfg.mode==='dashboard',preview=cfg.mode==='preview';
const assetBase=cfg.base?cfg.base+'v3/':'./';
const $=id=>document.getElementById(id),root=document.documentElement;
const store={get(k){try{return localStorage.getItem(k)}catch{return null}},set(k,v){try{localStorage.setItem(k,v)}catch{}}};

const T={fa:{skip:'رفتن به محتوای اصلی',home:'خانه',docs:'مستندات',signin:'ورود',heroLead:'بافت؛',hero:'زیرساخت تاب‌آور برای مسیرهای چندگانه',intro:'شبکه‌ای مقاوم با تونل‌سازی هوشمند، بازیابی خودکار مسیرها، اجرای پایدار ECRL و نظارت متمرکز در مرکز فرمان.',start:'شروع سریع',techDocs:'مستندات فنی',stripLang:'فارسی و انگلیسی',stripSecure:'دسترسی امن',stripMonitor:'پایش زنده',
loginTitle:'ورود به مرکز فرمان',loginText:'با اطلاعات دسترسی ساخته‌شده هنگام نصب وارد شوید.',user:'نام کاربری',password:'رمز عبور',show:'نمایش',hide:'پنهان',secure:'دسترسی با مسیر محرمانه و نشست امن',back:'بازگشت به خانه',enterUsername:'نام کاربری را وارد کنید.',enterPassword:'رمز عبور را وارد کنید.',submitting:'در حال ورود…',loginBad:'نام کاربری یا رمز عبور نادرست است.',loginInvalid:'درخواست ورود قابل پردازش نبود؛ دوباره وارد کنید.',loginBusy:'ورود موقتاً محدود شده است؛ کمی بعد دوباره تلاش کنید.',previewLogin:'در نسخهٔ نمایشی ورود غیرفعال است.',
settings:'تنظیمات',classic:'داشبورد کلاسیک',logout:'خروج',search:'جستجو…',theme:'تغییر پوسته',lightOn:'فعال‌کردن حالت روشن',darkOn:'فعال‌کردن حالت تاریک',menu:'منو',alertsTitle:'هشدارهای فعال',noAlerts:'هشدار فعالی وجود ندارد.',account:'حساب کاربری',
nav:{dashboard:'داشبورد',servers:'سرورها',tunnels:'تونل‌ها',monitoring:'مانیتورینگ',security:'امنیت',ssl:'گواهی SSL',backups:'بکاپ و بازیابی',updates:'به‌روزرسانی',finance:'مالی',settings:'تنظیمات'},
desc:{servers:'سرورهای ثبت‌شده، وضعیت و موقعیت آن‌ها روی نقشه',tunnels:'ساخت، ویرایش و بازبینی تونل‌های IR به EX',monitoring:'هشدارها، پایش زنده و سلامت لایه‌ها',security:'رویدادهای ممیزی و دسترسی',ssl:'چرخش گواهی تونل‌ها',backups:'نسخه‌های پشتیبان رمزگذاری‌شده و بررسی بازیابی',updates:'استقرار نسخه روی سرورها و کارهای اخیر',finance:'ترافیک، هزینه و درآمد هر سرور',settings:'ظاهر، زبان و اطلاعات نسخه'},
mapTitle:'وضعیت لحظه‌ای شبکه',mapSub:'ارتباط سرورها و تونل‌های فعال روی نقشه',worldView:'نمای جهان',simpleMap:'نقشهٔ ساده',demo:'داده‌های نمونه',
kServers:'سرورهای فعال',kTunnels:'تونل‌های فعال',kLatency:'میانگین تأخیر',kTraffic:'ترافیک جاری',kAlerts:'هشدارها',serverStatus:'وضعیت سرورها',viewAll:'مشاهدهٔ همه',traffic:'ترافیک شبکه',in:'ورودی',out:'خروجی',r24:'۲۴ ساعت',r7:'۷ روز',
up:'آنلاین',down:'آفلاین',unknown:'نامشخص',revoked:'لغوشده',noServers:'هنوز سروری ثبت نشده است.',addServer:'افزودن سرور',noTraffic:'هنوز ترافیکی گزارش نشده است.',noLocation:n=>n+' سرور موقعیت ندارد؛ کشور را در صفحهٔ سرورها انتخاب کنید',
netOk:'شبکه فعال',netOkSub:'همهٔ سرورها آنلاین',netWarn:n=>n+' هشدار فعال',netWarnSub:'جزئیات در مانیتورینگ',netPart:(a,b)=>a+' از '+b+' سرور آنلاین',netPartSub:'برخی سرورها پاسخ نمی‌دهند',netNone:'شبکه خالی است',netNoneSub:'اولین سرور را اضافه کنید',netWait:'در حال دریافت وضعیت…',
loading:'در حال دریافت…',error:'دریافت اطلاعات ناموفق بود؛ دوباره تلاش کنید.',expired:'نشست منقضی شده؛ دوباره وارد شوید.',refresh:'تازه‌سازی',updated:'به‌روز شده',
roleIR:'ایران (IR)',roleEX:'خارج (EX)',roleMaster:'مرکز (IR)',colServer:'سرور',colRole:'نقش',colStatus:'وضعیت',colLatency:'تأخیر',colSeen:'آخرین گزارش',colLocation:'موقعیت روی نقشه',colAddress:'آدرس مدیریت',noCountry:'بدون موقعیت',saved:'ذخیره شد',failed:'عملیات ناموفق بود؛ دسترسی و ورودی‌ها را بررسی کنید.',
tSsh:'افزودن سرور با SSH',tSshSub:'کلید میزبان را بررسی و عامل BAFT را نصب کنید',tManual:'ثبت یا ویرایش دستی سرور',tManualSub:'برای سرورهایی که عامل را خودتان نصب کرده‌اید',tMigrate:'مهاجرت امن پورت SSH',tMigrateSub:'تغییر پورت SSH با تأیید و بازگشت خودکار',
tTunnels:'تونل‌ها',tTunnelsSub:'پیش‌نمایش برنامه، استقرار با هش و ویرایش',tDiscovery:'کشف تونل‌های موجود',tDiscoverySub:'فقط گزارش؛ چیزی تغییر نمی‌کند',tDoctor:'عیب‌یاب مسیر',tDoctorSub:'اولین مرحلهٔ مسدودکننده را پیدا می‌کند',tPaths:'کشف خودکار مسیر',tPathsSub:'اندازه‌گیری مسیرهای IPv4 و IPv6 بین سرورها',tChanges:'تاریخچهٔ تغییرات',tChangesSub:'تغییرات تأییدشده، فقط‌افزودنی',
tLive:'پایش زنده',tLiveSub:'وضعیت و نشست‌های هر سرور',tHistory:'ترافیک و دسترس‌پذیری ۷ روزه',tHistorySub:'تاریخچهٔ هر سرور',tHealth:'سلامت لایه‌ها',tHealthSub:'وضعیت هر لایهٔ ارتباط و تغییرات اخیر',activeAlerts:'هشدارهای فعال',colAlert:'هشدار',colNode:'سرور',colSeverity:'شدت',colTime:'زمان',critical:'بحرانی',warning:'هشدار',
tAudit:'رویدادهای ممیزی',tAuditSub:'زنجیرهٔ هش‌شده و فقط‌خواندنی',sessionTitle:'نشست فعلی',sessionText:'ورود با مسیر محرمانه، نشست امن و توکن CSRF برای هر تغییر. نشست با خروج یا انقضا بسته می‌شود.',
certTitle:'چرخش‌های گواهی',certEmpty:'هنوز چرخش گواهی‌ای انجام نشده است.',certHint:'چرخش گواهی از ردیف هر تونل فعال در صفحهٔ تونل‌ها شروع می‌شود.',colTunnel:'تونل',colEpoch:'نسل',colPhase:'مرحله',colStarted:'شروع',goTunnels:'رفتن به تونل‌ها',
backupTitle:'نسخه‌های پشتیبان',backupCreate:'ایجاد بکاپ رمزگذاری‌شده',backupPreview:'بررسی بازیابی',backupEmpty:'هنوز بکاپی ساخته نشده است.',backupNote:'ایجاد و بررسی بکاپ در این صفحه انجام می‌شود. بازیابی واقعی به روش آفلاین روی میزبان نیاز دارد.',backupGuide:'راهنمای بازیابی آفلاین',colName:'نام',colSize:'حجم',colModified:'تاریخ',backupMade:'بکاپ ساخته شد: ',backupUncertain:'نتیجهٔ بکاپ نامشخص است؛ فایل و رویداد ممیزی را بررسی کنید: ',verified:'اعتبار تأیید شد',refused:'بازیابی مجاز نیست',nodes:'سرورها',
tDeploy:'استقرار روی سرورها',tDeploySub:'انتخاب سرورها و استقرار نسخه، چرخش یا لغو توکن',tJobs:'کارهای اخیر',tJobsSub:'آخرین کارهای فرستاده‌شده به عامل‌ها',tFinance:'مالی هر سرور',tFinanceSub:'نرخ نسخه‌دار، ترافیک، هزینه و سود',tReports:'گزارش‌های مالی',tReportsSub:'گزارش روزانه یا ماهانه و خروجی CSV',
appearance:'پوسته',appearanceSub:'تیره یا روشن؛ انتخاب شما در این مرورگر می‌ماند',dark:'تیره',light:'روشن',language:'زبان',languageSub:'فارسی راست‌به‌چپ یا انگلیسی چپ‌به‌راست',mapStyle:'نقشهٔ داشبورد',mapStyleSub:'نمای جهان یا نقشهٔ ساده',classicSub:'صفحهٔ تک‌صفحه‌ای قبلی، برای دوره‌ی گذار',open:'باز کردن',about:'دربارهٔ این نسخه',version:'نسخهٔ BCC',motion:'حرکت و انیمیشن',motionSub:'تنظیم «کاهش حرکت» سیستم‌عامل رعایت می‌شود',toolMissing:'این ابزار در این نسخهٔ BCC در دسترس نیست.'},
en:{skip:'Skip to main content',home:'Home',docs:'Docs',signin:'Sign in',heroLead:'BAFT:',hero:'resilient infrastructure for multiple paths',intro:'A resilient network with smart tunnelling, automatic path recovery, steady ECRL runtime and central monitoring in one command center.',start:'Quick start',techDocs:'Technical docs',stripLang:'Persian and English',stripSecure:'Secure access',stripMonitor:'Live monitoring',
loginTitle:'Sign in to Command Center',loginText:'Use the access credentials created during installation.',user:'Username',password:'Password',show:'Show',hide:'Hide',secure:'Secret-path access and a secure session',back:'Back to home',enterUsername:'Enter your username.',enterPassword:'Enter your password.',submitting:'Signing in…',loginBad:'Incorrect username or password.',loginInvalid:'The sign-in request could not be processed. Enter your details again.',loginBusy:'Sign-in is temporarily limited. Try again shortly.',previewLogin:'Sign-in is disabled in the demo.',
settings:'Settings',classic:'Classic dashboard',logout:'Sign out',search:'Search…',theme:'Switch theme',lightOn:'Switch to light theme',darkOn:'Switch to dark theme',menu:'Menu',alertsTitle:'Active alerts',noAlerts:'No active alerts.',account:'Account',
nav:{dashboard:'Dashboard',servers:'Servers',tunnels:'Tunnels',monitoring:'Monitoring',security:'Security',ssl:'SSL Certificates',backups:'Backups & Restore',updates:'Updates',finance:'Finance',settings:'Settings'},
desc:{servers:'Enrolled servers, their state and where they sit on the map',tunnels:'Build, edit and review IR to EX tunnels',monitoring:'Alerts, live monitoring and layered health',security:'Audit events and access',ssl:'Tunnel certificate rotations',backups:'Encrypted backups and restore checks',updates:'Deploy versions to servers and recent jobs',finance:'Traffic, cost and revenue per server',settings:'Appearance, language and version'},
mapTitle:'Network Overview',mapSub:'Servers and active tunnels on the map',worldView:'World Map',simpleMap:'Simple Map',demo:'Sample data',
kServers:'Active Servers',kTunnels:'Active Tunnels',kLatency:'Average Latency',kTraffic:'Current Traffic',kAlerts:'Alerts',serverStatus:'Server Status',viewAll:'View all',traffic:'Network Traffic',in:'In',out:'Out',r24:'24 Hours',r7:'7 Days',
up:'Online',down:'Offline',unknown:'Unknown',revoked:'Revoked',noServers:'No servers enrolled yet.',addServer:'Add server',noTraffic:'No traffic reported yet.',noLocation:n=>n+(n===1?' server has':' servers have')+' no location; pick a country on the Servers page',
netOk:'Network Online',netOkSub:'All servers online',netWarn:n=>n+(n===1?' active alert':' active alerts'),netWarnSub:'Details in Monitoring',netPart:(a,b)=>a+' of '+b+' servers online',netPartSub:'Some servers are not responding',netNone:'No servers yet',netNoneSub:'Add your first server',netWait:'Loading status…',
loading:'Loading…',error:'Could not load data. Try again.',expired:'Session expired. Sign in again.',refresh:'Refresh',updated:'Updated',
roleIR:'Iran (IR)',roleEX:'Exit (EX)',roleMaster:'Master (IR)',colServer:'Server',colRole:'Role',colStatus:'Status',colLatency:'Latency',colSeen:'Last report',colLocation:'Map location',colAddress:'Management address',noCountry:'No location',saved:'Saved',failed:'Operation failed. Check permissions and inputs.',
tSsh:'Add a server over SSH',tSshSub:'Check the host key and install the BAFT agent',tManual:'Register or edit a server',tManualSub:'For servers where you installed the agent yourself',tMigrate:'Safe SSH port migration',tMigrateSub:'Move the SSH port with verification and automatic rollback',
tTunnels:'Tunnels',tTunnelsSub:'Preview the plan, deploy by hash, edit',tDiscovery:'Discover existing tunnels',tDiscoverySub:'Report only; nothing is changed',tDoctor:'Route Doctor',tDoctorSub:'Finds the first blocking stage',tPaths:'Automatic path discovery',tPathsSub:'Measures IPv4 and IPv6 paths between servers',tChanges:'Change history',tChangesSub:'Confirmed changes, append-only',
tLive:'Live monitoring',tLiveSub:'State and sessions of each server',tHistory:'7-day traffic and uptime',tHistorySub:'History of each server',tHealth:'Layered health',tHealthSub:'Each connection layer and recent transitions',activeAlerts:'Active alerts',colAlert:'Alert',colNode:'Server',colSeverity:'Severity',colTime:'Time',critical:'Critical',warning:'Warning',
tAudit:'Audit events',tAuditSub:'Hash-chained and read-only',sessionTitle:'Current session',sessionText:'Secret-path sign-in, a secure session and a CSRF token on every change. The session ends on sign-out or expiry.',
certTitle:'Certificate rotations',certEmpty:'No certificate rotation yet.',certHint:'Start a rotation from an active tunnel’s row on the Tunnels page.',colTunnel:'Tunnel',colEpoch:'Epoch',colPhase:'Phase',colStarted:'Started',goTunnels:'Go to Tunnels',
backupTitle:'Backups',backupCreate:'Create encrypted backup',backupPreview:'Preview restore',backupEmpty:'No backup yet.',backupNote:'Create and check backups here. A real restore uses the offline host procedure.',backupGuide:'Offline restore guide',colName:'Name',colSize:'Size',colModified:'Date',backupMade:'Backup created: ',backupUncertain:'Backup outcome uncertain; reconcile the file and audit log: ',verified:'Verified',refused:'Restore refused',nodes:'Servers',
tDeploy:'Deploy to servers',tDeploySub:'Select servers to deploy, rotate or revoke tokens',tJobs:'Recent jobs',tJobsSub:'Latest jobs sent to agents',tFinance:'Finance per server',tFinanceSub:'Versioned rates, traffic, cost and profit',tReports:'Finance reports',tReportsSub:'Daily or monthly reports and CSV export',
appearance:'Theme',appearanceSub:'Dark or light; kept in this browser',dark:'Dark',light:'Light',language:'Language',languageSub:'Persian right-to-left or English left-to-right',mapStyle:'Dashboard map',mapStyleSub:'World map or simple map',classicSub:'The previous single-page dashboard, kept for the transition',open:'Open',about:'About this version',version:'BCC version',motion:'Motion',motionSub:'The system reduce-motion setting is respected',toolMissing:'This tool is not available in this BCC build.'}};
const lang=()=>root.lang==='en'?'en':'fa',t=k=>T[lang()][k],nav=()=>T[lang()].nav;

/* Countries the map can place; must match internal/bcc/countries.go. */
const COUNTRIES=['AE','AM','AR','AT','AU','AZ','BE','BG','BH','BR','CA','CH','CL','CN','CY','CZ','DE','DK','EE','EG','ES','FI','FR','GB','GE','GR','HK','HR','HU','ID','IE','IL','IN','IQ','IR','IS','IT','JP','KR','KW','KZ','LT','LU','LV','MD','MX','MY','NL','NO','NZ','OM','PK','PL','PT','QA','RO','RS','RU','SA','SE','SG','SI','SK','TH','TR','TW','UA','US','UZ','VN','ZA'];
let regionNames=null;
function countryName(code){if(!code)return'';try{if(!regionNames||regionNames.lang!==lang()){regionNames=new Intl.DisplayNames([lang()],{type:'region'});regionNames.lang=lang()}return regionNames.of(code)}catch{return code}}

const ICONS={dashboard:'<path d="M3 11l9-8 9 8v9a1 1 0 01-1 1h-5v-6h-6v6H4a1 1 0 01-1-1z"/>',servers:'<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',tunnels:'<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="18" r="2.5"/><circle cx="18" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><path d="M8 8l8 8M16 8l-8 8"/>',monitoring:'<path d="M3 12h4l2-6 4 12 2-6h6"/>',security:'<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M9 12l2 2 4-4"/>',ssl:'<rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3M12 14v3"/>',backups:'<path d="M3 12a9 9 0 1015-6.7M18 3v4h-4"/><path d="M12 8v4l3 2"/>',updates:'<path d="M20 11a8 8 0 00-14.9-3M4 4v4h4M4 13a8 8 0 0014.9 3M20 20v-4h-4"/>',finance:'<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',settings:'<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 01-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 010-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 014 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 010 4h-.1a1.7 1.7 0 00-1.5 1z"/>',
latency:'<path d="M4 18a8 8 0 1116 0"/><path d="M12 18l4-6"/><path d="M8 18h.01M16 18h.01"/>',bars:'<path d="M6 20V12M10 20V8M14 20v-5M18 20V5"/>',bell:'<path d="M6 16V11a6 6 0 0112 0v5l2 2H4zM10 20a2 2 0 004 0"/>',chev:'<path d="M6 9l6 6 6-6"/>',globe:'<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.7 3.8 5.7 3.8 9s-1.3 6.3-3.8 9c-2.5-2.7-3.8-5.7-3.8-9S9.5 5.7 12 3z"/>',map:'<path d="M9 4L3 6v14l6-2 6 2 6-2V4l-6 2z"/><path d="M9 4v14M15 6v14"/>',plus:'<path d="M12 5v14M5 12h14"/>',key:'<circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M17 6l3 3"/>',route:'<circle cx="6" cy="19" r="2"/><circle cx="18" cy="5" r="2"/><path d="M8 19h6a4 4 0 000-8H10a4 4 0 010-8h6"/>',search:'<circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/>',doc:'<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 13h6M9 17h6"/>',jobs:'<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"/>'};
const svg=(name,cls)=>'<svg viewBox="0 0 24 24" aria-hidden="true"'+(cls?' class="'+cls+'"':'')+'>'+(ICONS[name]||'')+'</svg>';
const ROUTES=['dashboard','servers','tunnels','monitoring','security','ssl','backups','updates','finance','settings'];
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function el(tag,cls,html){const n=document.createElement(tag);if(cls)n.className=cls;if(html!=null)n.innerHTML=html;return n}

/* Numbers stay in Latin digits, as in the design; dates follow the language. */
const nf=(v,d=0)=>new Intl.NumberFormat('en-US',{maximumFractionDigits:d,minimumFractionDigits:d}).format(v);
function bps(v){if(v==null||!Number.isFinite(v))return null;if(v>=1e9)return[nf(v/1e9,v<1e10?2:1),'Gbps'];if(v>=1e6)return[nf(v/1e6,v<1e7?1:0),'Mbps'];if(v>=1e3)return[nf(v/1e3,0),'Kbps'];return[nf(v,0),'bps']}
const pad2=n=>String(n).padStart(2,'0');
function ago(ts){const d=Date.parse(ts);if(!Number.isFinite(d))return'—';const s=Math.round((d-Date.now())/1000),rtf=new Intl.RelativeTimeFormat(lang(),{numeric:'auto'});const a=Math.abs(s);return a<60?rtf.format(s,'second'):a<3600?rtf.format(Math.round(s/60),'minute'):a<86400?rtf.format(Math.round(s/3600),'hour'):rtf.format(Math.round(s/86400),'day')}
function when(ts){const d=Date.parse(ts);return Number.isFinite(d)?new Intl.DateTimeFormat(lang()==='fa'?'fa-IR':'en-GB',{dateStyle:'medium',timeStyle:'short'}).format(new Date(d)):'—'}
function size(b){return!Number.isFinite(b)?'—':b>=1<<30?nf(b/(1<<30),1)+' GiB':b>=1<<20?nf(b/(1<<20),1)+' MiB':nf(b/1024,0)+' KiB'}
function toast(msg,bad){const n=el('div','toast'+(bad?' bad':''));n.setAttribute('role','status');n.textContent=msg;document.body.append(n);setTimeout(()=>n.remove(),4200)}

/* API */
async function api(path,opts={}){if(!live)throw Error('PREVIEW');const method=(opts.method||'GET').toUpperCase(),headers={Accept:'application/json'};if(method!=='GET'){headers['Content-Type']='application/json';headers[cfg.csrfHeader]=cfg.csrf}
  const r=await fetch(cfg.base+'api/'+path,{method,credentials:'same-origin',cache:'no-store',redirect:'error',headers,body:opts.body!=null?JSON.stringify(opts.body):undefined});
  if(r.status===401){window.dispatchEvent(new Event('baft:session-expired'));throw Error('SESSION')}
  if(!r.ok){const e=Error('HTTP');e.status=r.status;e.response=r;throw e}return r.json()}

/* ---------- Theme and language ---------- */
function setTheme(next){root.dataset.theme=next;store.set('baft.theme',next);paintChrome();map?.theme();renderTraffic()}
function setLang(next){root.lang=next;root.dir=next==='fa'?'rtl':'ltr';store.set('baft.lang',next);render()}
document.addEventListener('click',ev=>{const th=ev.target.closest('[data-theme-toggle],[data-theme-set]'),lg=ev.target.closest('[data-lang]');
  if(th){setTheme(th.dataset.themeSet||(root.dataset.theme==='light'?'dark':'light'))}
  if(lg&&lg.closest('[data-lang-switch]'))setLang(lg.dataset.lang)});
function paintChrome(){for(const b of document.querySelectorAll('[data-lang-switch] [data-lang]'))b.setAttribute('aria-pressed',String(b.dataset.lang===lang()));
  for(const b of document.querySelectorAll('[data-theme-toggle]')){b.setAttribute('aria-label',root.dataset.theme==='light'?t('darkOn'):t('lightOn'));b.title=b.getAttribute('aria-label');if(b.classList.contains('icon-btn'))b.innerHTML=root.dataset.theme==='light'?'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 019.5 4 8 8 0 1020 14.5z"/></svg>':'<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>'}
  for(const b of document.querySelectorAll('[data-theme-set]'))b.setAttribute('aria-pressed',String(b.dataset.themeSet===root.dataset.theme));
  for(const n of document.querySelectorAll('[data-t]')){const v=t(n.dataset.t);if(typeof v==='string')n.textContent=v}
  for(const s of document.querySelectorAll('[data-lang-switch]'))s.setAttribute('aria-label',t('language'))}

/* ---------- Public pages ---------- */
const form=$('loginForm');
function loginError(){if(!cfg.error)return'';return t(/too many|busy/i.test(cfg.error)?'loginBusy':/bad request/i.test(cfg.error)?'loginInvalid':'loginBad')}
function showPublic(view){$('app').hidden=true;$('public').hidden=false;$('landing').hidden=view!=='landing';$('signin').hidden=view!=='signin';
  if(view==='signin'){const u=$('loginUsername');if(!u.disabled)u.focus()}window.scrollTo(0,0)}
form.addEventListener('submit',ev=>{const u=$('loginUsername'),p=$('loginPassword');$('loginError').textContent='';
  if(preview){ev.preventDefault();$('loginError').textContent=t('previewLogin');return}
  if(!u.value.trim()){ev.preventDefault();$('loginError').textContent=t('enterUsername');u.setAttribute('aria-invalid','true');u.focus();return}
  if(!p.value){ev.preventDefault();$('loginError').textContent=t('enterPassword');p.setAttribute('aria-invalid','true');p.focus();return}
  const b=form.querySelector('[type=submit]');b.disabled=true;b.textContent=t('submitting');form.setAttribute('aria-busy','true')});
for(const id of ['loginUsername','loginPassword'])$(id).addEventListener('input',ev=>{ev.target.removeAttribute('aria-invalid');$('loginError').textContent=''});
$('showPassword').addEventListener('click',()=>{const p=$('loginPassword'),shown=p.type==='password';p.type=shown?'text':'password';$('showPassword').setAttribute('aria-pressed',String(shown));$('showPassword').dataset.t=shown?'hide':'show';$('showPassword').textContent=t($('showPassword').dataset.t)});
document.addEventListener('click',ev=>{const g=ev.target.closest('[data-go]');if(!g||live)return;ev.preventDefault();location.hash=g.dataset.go==='signin'?'signin':'home'});

/* ---------- App shell ---------- */
let route='dashboard',overview=null,alerts=[],range=store.get('baft.range')==='7d'?'7d':'24h',mapMode=store.get('baft.map')==='simple'?'simple':'world',map=null,pollTimer=null,loadSeq=0,loadState='loading';
function buildNav(){const n=$('nav');n.replaceChildren();for(const r of ROUTES){const a=el('a',null,svg(r)+'<span>'+esc(nav()[r])+'</span>');a.href='#'+r;a.dataset.route=r;if(r==='monitoring'&&alerts.length)a.append(el('span','count',String(alerts.length)));if(r===route){a.classList.add('active');a.setAttribute('aria-current','page')}n.append(a)}}
function paintNetState(){const box=$('netState');let cls='',title=t('netWait'),sub='';if(overview){const s=overview.servers,a=overview.alerts.active;if(!s.total){title=t('netNone');sub=t('netNoneSub')}else if(a>0){cls=overview.alerts.critical?'bad':'warn';title=t('netWarn')(a);sub=t('netWarnSub')}else if(s.online===s.total){cls='ok';title=t('netOk');sub=t('netOkSub')}else{cls=s.online?'warn':'bad';title=t('netPart')(s.online,s.total);sub=t('netPartSub')}}
  box.className='net-state '+cls;box.innerHTML='<i></i><div><b>'+esc(title)+'</b><small>'+esc(sub)+'</small></div>';$('version').textContent=overview?(/^\d/.test(overview.version)?'v'+overview.version:overview.version):'';$('bellDot').hidden=!alerts.length}
function chrome(){paintChrome();buildNav();paintNetState();$('search').placeholder=t('search');$('search').setAttribute('aria-label',t('search'));$('menuToggle').setAttribute('aria-label',t('menu'));$('bell').setAttribute('aria-label',t('alertsTitle'));$('avatar').setAttribute('aria-label',t('account'));
  document.title=(route==='dashboard'?'BAFT Command Center':nav()[route]+' · BAFT')+(preview?' · DEMO':'')}
$('menuToggle').addEventListener('click',()=>{const s=$('app').classList.toggle('menu-open');$('menuToggle').setAttribute('aria-expanded',String(s))});
$('nav').addEventListener('click',()=>{$('app').classList.remove('menu-open');$('menuToggle').setAttribute('aria-expanded','false')});
function popover(btn,pop){btn.addEventListener('click',ev=>{ev.stopPropagation();const open=pop.hidden;for(const p of document.querySelectorAll('.popover'))p.hidden=true;for(const b of document.querySelectorAll('[aria-haspopup]'))b.setAttribute('aria-expanded','false');pop.hidden=!open;btn.setAttribute('aria-expanded',String(open));if(open&&pop.id==='alertsPop')paintAlertsPop()})}
popover($('bell'),$('alertsPop'));popover($('avatar'),$('userPop'));
document.addEventListener('click',ev=>{if(!ev.target.closest('.pop-wrap'))for(const p of document.querySelectorAll('.popover'))p.hidden=true});
document.addEventListener('keydown',ev=>{if(ev.key==='Escape'){for(const p of document.querySelectorAll('.popover'))p.hidden=true;$('app').classList.remove('menu-open')}});
function alertRow(a){const crit=a.severity==='critical';return'<div class="alert-item"><span class="pill '+(crit?'down':'warn')+'"><i></i></span><div><b>'+esc(a.node_alias||a.node_id)+'</b> · '+esc(a.message||a.type)+'</div><small>'+esc(ago(a.timestamp))+'</small></div>'}
function paintAlertsPop(){const p=$('alertsPop');p.innerHTML='<h3>'+esc(t('alertsTitle'))+'</h3>'+(alerts.length?alerts.slice(0,12).map(alertRow).join(''):'<p class="muted" style="margin:6px 8px 10px">'+esc(t('noAlerts'))+'</p>')}
$('logout').addEventListener('click',()=>{if(!live)return;const f=el('form');f.method='post';f.action=cfg.base+'logout';document.body.append(f);f.submit()});
if(cfg.base)$('classicLink').href=cfg.base+'classic/';
$('search').addEventListener('input',applySearch);
function applySearch(){const q=$('search').value.trim().toLowerCase();for(const r of $('main').querySelectorAll('.server-row,tbody tr'))r.hidden=!!q&&!r.textContent.toLowerCase().includes(q)}
window.addEventListener('hashchange',()=>{const h=location.hash.slice(1);if(!live){showPublic(h==='signin'?'signin':'landing');return}go(ROUTES.includes(h)?h:'dashboard')});
let sessionGone=false;window.addEventListener('baft:session-expired',()=>{if(!live||sessionGone)return;sessionGone=true;stopPolling();$('main').replaceChildren();$('app').hidden=true;location.replace(cfg.base+'login')});

/* ---------- Data ---------- */
async function refresh(){const seq=++loadSeq;if(preview){overview=demoOverview();alerts=[];loadState='ready';paintData();return}
  try{const [o,a]=await Promise.allSettled([api('overview?window='+range),api('alerts')]);if(seq!==loadSeq)return;
    if(o.status==='fulfilled'){overview=o.value;loadState='ready'}else if(o.reason?.message!=='SESSION')loadState='error';
    if(a.status==='fulfilled'&&a.value&&typeof a.value==='object')alerts=Object.values(a.value).filter(x=>x&&x.status!=='resolved').sort((x,y)=>Date.parse(y.timestamp)-Date.parse(x.timestamp))}
  catch{if(seq===loadSeq)loadState='error'}paintData()}
function paintData(){paintNetState();buildNav();if(route==='dashboard')paintDashboard();if(route==='servers')paintServers()}
function startPolling(){stopPolling();if(document.hidden||!live)return;pollTimer=setInterval(refresh,15000)}
function stopPolling(){if(pollTimer)clearInterval(pollTimer);pollTimer=null}
document.addEventListener('visibilitychange',()=>{if(document.hidden)stopPolling();else if(live){refresh();startPolling()}});

/* ---------- Dashboard ---------- */
function dashboardView(){const v=el('div','dash');v.innerHTML=
 '<section class="card map-card"><canvas id="mapCanvas" role="img"></canvas><div class="map-head"><div><h2>'+esc(t('mapTitle'))+'</h2><p>'+esc(t('mapSub'))+'</p></div><div class="map-modes"><button type="button" data-map="world">'+svg('globe')+'<span>'+esc(t('worldView'))+'</span></button><button type="button" data-map="simple">'+svg('map')+'<span>'+esc(t('simpleMap'))+'</span></button></div></div>'
 +(preview?'<span class="map-note">DEMO · '+esc(t('demo'))+'</span>':'')+'<a class="map-note" id="mapNote" href="#servers" hidden></a><div class="map-tip" id="mapTip" hidden></div><div class="kpis" id="kpis"></div></section>'
 +'<div class="dash-side"><section class="card"><div class="card-head"><h2>'+esc(t('serverStatus'))+'</h2><a href="#servers">'+esc(t('viewAll'))+'</a></div><div class="server-list" id="serverList"></div></section>'
 +'<section class="card"><div class="card-head"><h2>'+esc(t('traffic'))+'</h2><select class="range" id="range" aria-label="'+esc(t('traffic'))+'"><option value="24h">'+esc(t('r24'))+'</option><option value="7d">'+esc(t('r7'))+'</option></select></div><svg class="traffic-chart" id="trafficChart" viewBox="0 0 320 190" role="img" aria-label="'+esc(t('traffic'))+'"></svg><div class="legend"><span><i style="background:var(--blue)"></i>'+esc(t('in'))+'</span><span><i style="background:var(--gold)"></i>'+esc(t('out'))+'</span></div></section></div>';
 return v}
function mountDashboard(main){main.append(dashboardView());$('range').value=range;$('range').addEventListener('change',ev=>{range=ev.target.value;store.set('baft.range',range);refresh()});
  for(const b of main.querySelectorAll('[data-map]')){b.setAttribute('aria-pressed',String(b.dataset.map===mapMode));b.addEventListener('click',()=>{mapMode=b.dataset.map;store.set('baft.map',mapMode);for(const x of main.querySelectorAll('[data-map]'))x.setAttribute('aria-pressed',String(x===b));map.mode(mapMode)})}
  const tip=$('mapTip');
  map=window.BAFT_MAP.create($('mapCanvas'),{assetBase,countryName,unknown:()=>t('unknown'),onSelect:()=>{location.hash='servers'},
    onHover(n,x,y){if(!n){tip.hidden=true;return}tip.hidden=false;tip.innerHTML='<b>'+esc(n.alias)+'</b> · '+esc(t(n.status)||n.status)+(n.latency_ms!=null?' · <span class="ltr">'+n.latency_ms+' ms</span>':'');tip.style.left=(x+14)+'px';tip.style.top=(y+14)+'px'}});
  map.mode(mapMode);paintDashboard()}
function kpi(cls,icon,label,value,extra=''){return'<div class="kpi'+(cls?' '+cls:'')+'"><span class="ic '+icon[1]+'">'+svg(icon[0])+'</span><div class="body"><span>'+esc(label)+'</span><strong class="num">'+value+'</strong>'+extra+'</div></div>'}
function sparkPath(vals,wd,ht){const pts=vals.filter(Number.isFinite);if(pts.length<2)return'';const mx=Math.max(...pts),mn=Math.min(...pts),r=mx-mn||1;return pts.map((v,i)=>(i?'L':'M')+(i*wd/(pts.length-1)).toFixed(1)+' '+(ht-2-(v-mn)/r*(ht-4)).toFixed(1)).join(' ')}
function paintDashboard(){const k=$('kpis');if(!k)return;const o=overview,dash='—';
  if(!o){k.innerHTML=kpi('',['servers','gold'],t('kServers'),dash)+kpi('',['tunnels','green'],t('kTunnels'),dash)+kpi('',['latency','amber'],t('kLatency'),dash)+kpi('traffic',['bars','blue'],t('kTraffic'),dash)+kpi('',['bell','red'],t('kAlerts'),dash);$('serverList').innerHTML='<div class="empty">'+esc(loadState==='error'?t('error'):t('loading'))+'</div>';renderTraffic();return}
  const tun=o.tunnels,cur=o.traffic,tot=cur.in_bps!=null?bps(cur.in_bps+cur.out_bps):null,series=o.series.points.map(p=>p.in_bps!=null?p.in_bps+p.out_bps:NaN);
  const sp=sparkPath(series.slice(-30),70,30);
  k.innerHTML=kpi('',['servers','gold'],t('kServers'),pad2(o.servers.online)+' <small>/ '+pad2(o.servers.total)+'</small>')
   +kpi('',['tunnels','green'],t('kTunnels'),pad2(tun.active)+' <small>/ '+pad2(tun.active+tun.building)+'</small>')
   +kpi('',['latency','amber'],t('kLatency'),o.latency_ms!=null?nf(o.latency_ms)+' <small>ms</small>':dash)
   +kpi('traffic',['bars','blue'],t('kTraffic'),tot?tot[0]+' <small>'+tot[1]+'</small>':dash,tot?'<div class="flow"><b>↑ '+bps(cur.out_bps).join(' ')+'</b><b>↓ '+bps(cur.in_bps).join(' ')+'</b></div>':'')
   +kpi('',['bell',o.alerts.active?'red':'gray'],t('kAlerts'),pad2(o.alerts.active));
  if(sp)k.querySelector('.kpi.traffic').insertAdjacentHTML('beforeend','<svg class="spark" viewBox="0 0 70 30" preserveAspectRatio="none" aria-hidden="true"><path d="'+sp+'" stroke-width="1.6"/></svg>');
  // Map
  const nodes=o.nodes.filter(n=>!n.revoked),missing=nodes.filter(n=>n.lat==null).length;
  map?.set({nodes:nodes.map(n=>({...n})),links:o.links});const note=$('mapNote');note.hidden=!missing||!live;note.textContent=missing?t('noLocation')(missing):'';
  $('mapCanvas').setAttribute('aria-label',t('mapTitle')+': '+o.servers.online+'/'+o.servers.total);
  // Server list
  const list=$('serverList');if(!nodes.length){list.innerHTML='<div class="empty"><b>'+esc(t('noServers'))+'</b><a class="btn gold small" href="#servers">'+svg('plus')+esc(t('addServer'))+'</a></div>'}
  else list.innerHTML=nodes.slice(0,7).map(serverRow).join('');
  renderTraffic();applySearch()}
function flagHTML(code){if(!code)return'<span class="flag code">—</span>';return window.BAFT_MAP.flagSupported()?'<span class="flag" title="'+esc(countryName(code))+'">'+window.BAFT_MAP.flagOf(code)+'</span>':'<span class="flag code" title="'+esc(countryName(code))+'">'+esc(code)+'</span>'}
function statusPill(s){const c=s==='up'?'up':s==='down'?'down':s==='revoked'?'revoked':'unknown';return'<span class="pill '+c+'"><i></i>'+esc(t(c))+'</span>'}
function barsHTML(vals){const v=(vals||[]).slice(-12);if(!v.length)return'<span class="bars empty">'+'<i style="height:30%"></i>'.repeat(12)+'</span>';const mx=Math.max(...v,1);return'<span class="bars" aria-hidden="true">'+v.map(x=>'<i style="height:'+Math.max(12,Math.round(x/mx*100))+'%"></i>').join('')+'</span>'}
function serverRow(n){return'<a class="server-row" href="#servers">'+flagHTML(n.country)+'<div><b>'+esc(n.country?countryName(n.country):n.alias)+'</b><small>'+esc(n.country?n.alias:n.id)+'</small></div>'+statusPill(n.status)+'<span class="ms">'+(n.latency_ms!=null?n.latency_ms+' ms':'—')+'</span>'+barsHTML(n.spark)+'</a>'}
function renderTraffic(){const s=$('trafficChart');if(!s)return;const W=320,H=190,L=34,R=8,Tp=10,B=24;s.innerHTML='';const pts=overview?.series?.points||[],has=pts.some(p=>p.in_bps!=null);
  const grid=[0,1,2,3].map(i=>Tp+i*(H-Tp-B)/3);let out='';
  if(!has){out+=grid.map(y=>'<path d="M'+L+' '+y+'H'+(W-R)+'" stroke="var(--line)"/>').join('')+'<text x="'+(W/2)+'" y="'+(H/2)+'" text-anchor="middle" style="font-family:var(--font);font-size:12px">'+esc(overview?t('noTraffic'):loadState==='error'?t('error'):t('loading'))+'</text>';s.innerHTML=out;return}
  const mx=Math.max(1,...pts.map(p=>Math.max(p.in_bps||0,p.out_bps||0))),unit=mx>=1e9?[1e9,'Gbps']:mx>=1e6?[1e6,'Mbps']:mx>=1e3?[1e3,'Kbps']:[1,'bps'],raw=mx/unit[0]/3,mag=10**Math.floor(Math.log10(raw)),step=[1,2,2.5,5,10].map(m=>m*mag).find(v=>v>=raw),top=step*3,x=i=>L+i*(W-L-R)/Math.max(1,pts.length-1),y=v=>H-B-(v/unit[0])/top*(H-Tp-B);
  grid.forEach((gy,i)=>{out+='<path d="M'+L+' '+gy+'H'+(W-R)+'" stroke="var(--line)"/><text x="'+(L-6)+'" y="'+(gy+3)+'" text-anchor="end">'+nf(step*(3-i),step<1?1:0)+'</text>'});
  out+='<text x="'+(W-R)+'" y="'+(Tp-2)+'" text-anchor="end" style="font-size:9px">'+unit[1]+'</text>';
  for(const[key,color]of[['in_bps','var(--blue)'],['out_bps','var(--gold)']]){let d='',area='',run=[];const flush=()=>{if(run.length>1){d+=run.map((p,i)=>(i?'L':'M')+p[0].toFixed(1)+' '+p[1].toFixed(1)).join(' ');area+='M'+run[0][0].toFixed(1)+' '+(H-B)+run.map(p=>'L'+p[0].toFixed(1)+' '+p[1].toFixed(1)).join('')+'L'+run.at(-1)[0].toFixed(1)+' '+(H-B)+'Z'}else if(run.length===1)d+='M'+(run[0][0]-1)+' '+run[0][1]+'h2';run=[]};
    pts.forEach((p,i)=>{if(p[key]==null){flush();return}run.push([x(i),y(p[key])])});flush();out+='<path d="'+area+'" fill="'+color+'" opacity=".12"/><path d="'+d+'" stroke="'+color+'" stroke-width="1.8" fill="none"/>'}
  const fmt=new Intl.DateTimeFormat('en-GB',range==='7d'?{day:'numeric',month:'short'}:{hour:'2-digit',minute:'2-digit'});[0,1,2,3].forEach(i=>{const p=pts[Math.round(i*(pts.length-1)/3)];out+='<text x="'+x(Math.round(i*(pts.length-1)/3))+'" y="'+(H-6)+'" text-anchor="'+(i===0?'start':i===3?'end':'middle')+'">'+fmt.format(new Date(p.t))+'</text>'});
  s.innerHTML=out}

/* ---------- Inner pages ---------- */
function pageHead(r,actions=''){return'<div class="page-head"><div><h1>'+esc(nav()[r])+'</h1><p>'+esc(T[lang()].desc[r]||'')+'</p></div><div class="page-actions">'+actions+'</div></div>'}
const OPS={ssh:'Add a server over SSH',manual:'Register / update node',migrate:'Safe SSH Port Migration',tunnels:'Tunnels',discovery:'Existing tunnels (discovery)',doctor:'Route Doctor',paths:'Automatic Path Discovery',changes:'Change History',live:'Live Monitoring',history:'7-Day Traffic & Uptime',health:'Layered health',audit:'Immutable Audit Log',deploy:'Cluster Nodes',jobs:'Recent Jobs',finance:'Node Finance',reports:'Finance Reports'};
function tool(key,icon,title,sub,open){const d=el('details','tool');d.open=!!open;d.dataset.tool=key;d.innerHTML='<summary><span class="ic '+icon[1]+'">'+svg(icon[0])+'</span><span>'+esc(t(title))+'<small>'+esc(t(sub))+'</small></span>'+svg('chev','chev')+'</summary><div class="tool-body"></div>';mountOps(d);return d}
function mountOps(d){const body=d.querySelector('.tool-body'),card=window.BAFT_OPS?.cards?.[OPS[d.dataset.tool]];if(card){card.hidden=false;card.classList.add('ops-card');body.append(card)}else if(!live)body.innerHTML='<p class="ops-missing">'+esc('DEMO · '+t('demo'))+'</p>';else if(!window.BAFT_OPS)body.innerHTML='<p class="ops-missing">'+esc(t('loading'))+'</p>';else body.innerHTML='<p class="ops-missing">'+esc(t('toolMissing'))+'</p>'}
window.addEventListener('baft:ops-ready',()=>{for(const d of document.querySelectorAll('details.tool'))if(!d.querySelector('.ops-card'))mountOps(d)});
/* Before leaving a page, hand its operation cards back so polling keeps finding their elements. */
function parkOps(){const holder=$('operationalRoot');if(!holder)return;for(const c of $('main').querySelectorAll('.ops-card')){c.hidden=true;holder.append(c)}}
function table(cols,rows){return'<div class="table-wrap"><table><thead><tr>'+cols.map(c=>'<th>'+esc(c)+'</th>').join('')+'</tr></thead><tbody>'+rows.join('')+'</tbody></table></div>'}
function roleLabel(r){return r==='foreign'?t('roleEX'):r==='master'?t('roleMaster'):t('roleIR')}
function roleBadge(r){return'<span class="badge '+(r==='foreign'?'blue':'gold')+'">'+esc(roleLabel(r))+'</span>'}

let nodeDetail={};
async function serversView(main){main.insertAdjacentHTML('beforeend',pageHead('servers','<button type="button" class="btn gold small" id="addServer">'+svg('plus')+esc(t('addServer'))+'</button>')+'<div class="stack"><section class="card"><div id="serverTable"></div><div class="status-line" id="serverStatus" role="status"></div></section></div>');
  const stack=main.querySelector('.stack');stack.append(tool('ssh',['plus','gold'],'tSsh','tSshSub',false),tool('manual',['servers','blue'],'tManual','tManualSub',false),tool('migrate',['key','amber'],'tMigrate','tMigrateSub',false));
  $('addServer').addEventListener('click',()=>{const d=main.querySelector('[data-tool=ssh]');d.open=true;d.scrollIntoView({behavior:'smooth',block:'start'})});
  if(live)api('nodes').then(list=>{nodeDetail=Object.fromEntries((Array.isArray(list)?list:[]).map(n=>[n.id,n]));paintServers()}).catch(()=>{});paintServers()}
function paintServers(){const host=$('serverTable');if(!host)return;const nodes=overview?.nodes||[];
  if(!overview){host.innerHTML='<div class="empty">'+esc(loadState==='error'?t('error'):t('loading'))+'</div>';return}
  if(!nodes.length){host.innerHTML='<div class="empty"><b>'+esc(t('noServers'))+'</b></div>';const d=document.querySelector('[data-tool=ssh]');if(d)d.open=true;return}
  const opts=c=>'<option value="">'+esc(t('noCountry'))+'</option>'+COUNTRIES.map(x=>[x,countryName(x)]).sort((a,b)=>a[1].localeCompare(b[1],lang())).map(([x,name])=>'<option value="'+x+'"'+(x===c?' selected':'')+'>'+esc(name)+'</option>').join('');
  host.innerHTML=table([t('colServer'),t('colRole'),t('colStatus'),t('colLatency'),t('colSeen'),t('colLocation'),t('colAddress')],nodes.map(n=>'<tr><td><div class="cell">'+flagHTML(n.country)+'<div><b>'+esc(n.alias)+'</b><br><small class="ltr">'+esc(n.id)+'</small></div></div></td><td>'+roleBadge(n.role)+'</td><td>'+statusPill(n.status)+'</td><td class="ltr">'+(n.latency_ms!=null?n.latency_ms+' ms':'—')+'</td><td>'+esc(n.last_seen?ago(n.last_seen):'—')+'</td><td><select data-country="'+esc(n.id)+'" aria-label="'+esc(t('colLocation')+' '+n.alias)+'"'+(live&&!n.revoked?'':' disabled')+'>'+opts(n.country)+'</select></td><td class="ltr"><small>'+esc(nodeDetail[n.id]?.address||'—')+'</small></td></tr>'));
  for(const s of host.querySelectorAll('select[data-country]'))s.addEventListener('change',async ev=>{const sel=ev.target;sel.disabled=true;try{await api('nodes/location',{method:'POST',body:{node_id:sel.dataset.country,country:sel.value}});$('serverStatus').textContent=t('saved');await refresh()}catch(e){if(e.message!=='SESSION')toast(t('failed'),true)}finally{sel.disabled=false}});applySearch()}

function tunnelsView(main){main.insertAdjacentHTML('beforeend',pageHead('tunnels')+'<div class="stack"></div>');main.querySelector('.stack').append(tool('tunnels',['tunnels','green'],'tTunnels','tTunnelsSub',true),tool('discovery',['search','blue'],'tDiscovery','tDiscoverySub',false),tool('doctor',['route','amber'],'tDoctor','tDoctorSub',false),tool('paths',['globe','gold'],'tPaths','tPathsSub',false),tool('changes',['doc','gray'],'tChanges','tChangesSub',false))}

function monitoringView(main){main.insertAdjacentHTML('beforeend',pageHead('monitoring')+'<div class="stack"><section class="card"><div class="card-head"><h2>'+esc(t('activeAlerts'))+'</h2></div><div id="alertTable"></div></section></div>');
  const sev=a=>a.severity==='critical'?'<span class="badge red">'+esc(t('critical'))+'</span>':'<span class="badge amber">'+esc(t('warning'))+'</span>';
  $('alertTable').innerHTML=alerts.length?table([t('colAlert'),t('colNode'),t('colSeverity'),t('colTime')],alerts.map(a=>'<tr><td>'+esc(a.message||a.type)+'</td><td>'+esc(a.node_alias||a.node_id)+'</td><td>'+sev(a)+'</td><td>'+esc(ago(a.timestamp))+'</td></tr>')):'<div class="empty">'+esc(t('noAlerts'))+'</div>';
  main.querySelector('.stack').append(tool('live',['monitoring','green'],'tLive','tLiveSub',true),tool('history',['bars','blue'],'tHistory','tHistorySub',false),tool('health',['security','amber'],'tHealth','tHealthSub',false))}

function securityView(main){main.insertAdjacentHTML('beforeend',pageHead('security')+'<div class="stack"><section class="card"><div class="card-head"><h2>'+esc(t('sessionTitle'))+'</h2></div><p class="muted" style="margin:0">'+esc(t('sessionText'))+'</p></section></div>');main.querySelector('.stack').append(tool('audit',['doc','gold'],'tAudit','tAuditSub',true))}

async function sslView(main){main.insertAdjacentHTML('beforeend',pageHead('ssl','<a class="btn small" href="#tunnels">'+esc(t('goTunnels'))+'</a>')+'<section class="card"><div class="card-head"><div><h2>'+esc(t('certTitle'))+'</h2><p class="sub">'+esc(t('certHint'))+'</p></div></div><div id="certTable"><div class="empty">'+esc(live?t('loading'):'DEMO · '+t('demo'))+'</div></div></section>');
  if(!live)return;try{const rows=await api('cert-rotations'),list=Array.isArray(rows)?rows:[];if(!$('certTable'))return;$('certTable').innerHTML=list.length?table([t('colTunnel'),t('colEpoch'),t('colPhase'),t('colStarted')],list.sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at)).map(r=>'<tr><td class="ltr">'+esc(r.tunnel_id)+'<br><small>'+esc(r.ir_node+' → '+r.ex_node)+'</small></td><td class="num">'+esc(r.epoch)+'</td><td><span class="badge '+(r.phase==='completed'||r.phase==='done'?'green':r.error?'red':'gold')+'">'+esc(r.phase)+'</span>'+(r.error?'<br><small>'+esc(r.error)+'</small>':'')+'</td><td>'+esc(when(r.created_at))+'</td></tr>')):'<div class="empty">'+esc(t('certEmpty'))+'</div>'}catch(e){if($('certTable')&&e.message!=='SESSION')$('certTable').innerHTML='<div class="empty">'+esc(t('error'))+'</div>'}}

async function backupsView(main){const fa=lang()==='fa';main.insertAdjacentHTML('beforeend',pageHead('backups','<button type="button" class="btn gold small" id="backupCreate"'+(live?'':' disabled')+'>'+esc(t('backupCreate'))+'</button>')+'<section class="card"><div class="card-head"><div><h2>'+esc(t('backupTitle'))+'</h2><p class="sub">'+esc(t('backupNote'))+' <a class="text-link" target="_blank" rel="noopener noreferrer" href="https://github.com/zarkmakerburg/baft/blob/main/docs/'+(fa?'fa':'en')+'/29-bcc-backup-restore-preview.md">'+esc(t('backupGuide'))+'</a></p></div></div><div id="backupTable"><div class="empty">'+esc(live?t('loading'):'DEMO · '+t('demo'))+'</div></div><div class="status-line" id="backupResult" role="status"></div></section>');
  $('backupCreate').addEventListener('click',async()=>{const b=$('backupCreate');b.disabled=true;$('backupResult').textContent=t('loading');try{const r=await fetch(cfg.base+'api/backups/create',{method:'POST',credentials:'same-origin',redirect:'error',headers:{'Content-Type':'application/json',[cfg.csrfHeader]:cfg.csrf},body:'{}'});if(r.status===401){window.dispatchEvent(new Event('baft:session-expired'));return}
    if(r.status!==201){const p=await r.json().catch(()=>null);if(p?.reconciliation_required&&typeof p.name==='string'){$('backupResult').textContent=t('backupUncertain')+p.name;await loadBackups();return}throw Error('CREATE')}const d=await r.json();await loadBackups();$('backupResult').textContent=t('backupMade')+d.name}catch{$('backupResult').textContent=t('error')}finally{b.disabled=false}});
  if(live)loadBackups()}
async function loadBackups(){try{const d=await api('backups'),list=(Array.isArray(d.backups)?d.backups:[]).filter(x=>typeof x.name==='string');if(!$('backupTable'))return;
  $('backupTable').innerHTML=list.length?table([t('colName'),t('colSize'),t('colModified'),''],list.sort((a,b)=>Date.parse(b.modified_at)-Date.parse(a.modified_at)).map(x=>'<tr><td class="ltr">'+esc(x.name)+'</td><td class="num">'+esc(size(x.size))+'</td><td>'+esc(when(x.modified_at))+'</td><td>'+(x.name.endsWith('.baftbak')?'<button type="button" class="btn small" data-preview="'+esc(x.name)+'">'+esc(t('backupPreview'))+'</button>':'')+'</td></tr>')):'<div class="empty">'+esc(t('backupEmpty'))+'</div>';
  for(const b of $('backupTable').querySelectorAll('[data-preview]'))b.addEventListener('click',async()=>{b.disabled=true;$('backupResult').textContent=t('loading');try{const r=await api('backups/restore-preview',{method:'POST',body:{filename:b.dataset.preview}});$('backupResult').textContent=(r.verified&&!r.would_refuse?t('verified'):t('refused'))+(r.problem||r.would_refuse?' · '+(r.problem||r.would_refuse):'')+' · '+t('nodes')+' '+(r.nodes?.current??'—')+' → '+(r.nodes?.after_restore??'—')}catch(e){if(e.message!=='SESSION')$('backupResult').textContent=t('error')}finally{b.disabled=false}})}
  catch(e){if($('backupTable')&&e.message!=='SESSION')$('backupTable').innerHTML='<div class="empty">'+esc(t('error'))+'</div>'}}

function updatesView(main){main.insertAdjacentHTML('beforeend',pageHead('updates')+'<div class="stack"></div>');main.querySelector('.stack').append(tool('deploy',['updates','gold'],'tDeploy','tDeploySub',true),tool('jobs',['jobs','blue'],'tJobs','tJobsSub',true))}
function financeView(main){main.insertAdjacentHTML('beforeend',pageHead('finance')+'<div class="stack"></div>');main.querySelector('.stack').append(tool('finance',['finance','green'],'tFinance','tFinanceSub',true),tool('reports',['doc','gold'],'tReports','tReportsSub',false))}
function settingsView(main){const seg=(attr,vals)=>'<div class="seg" role="group"'+(attr==='lang'?' data-lang-switch':'')+'>'+vals.map(([v,l])=>'<button type="button" data-'+(attr==='lang'?'lang':attr==='theme'?'theme-set':'map-set')+'="'+v+'">'+esc(l)+'</button>').join('')+'</div>';
  main.insertAdjacentHTML('beforeend',pageHead('settings')+'<div class="stack"><section class="card">'
   +'<div class="settings-row"><div><b>'+esc(t('appearance'))+'</b><p>'+esc(t('appearanceSub'))+'</p></div>'+seg('theme',[['dark',t('dark')],['light',t('light')]])+'</div>'
   +'<div class="settings-row"><div><b>'+esc(t('language'))+'</b><p>'+esc(t('languageSub'))+'</p></div>'+seg('lang',[['fa','فارسی'],['en','English']])+'</div>'
   +'<div class="settings-row"><div><b>'+esc(t('mapStyle'))+'</b><p>'+esc(t('mapStyleSub'))+'</p></div>'+seg('map',[['world',t('worldView')],['simple',t('simpleMap')]])+'</div>'
   +'<div class="settings-row"><div><b>'+esc(t('motion'))+'</b><p>'+esc(t('motionSub'))+'</p></div><span class="badge">OS</span></div></section>'
   +'<section class="card"><div class="card-head"><h2>'+esc(t('about'))+'</h2></div><div class="settings-row"><div><b>'+esc(t('version'))+'</b></div><span class="ltr num">'+esc(overview?overview.version:'—')+'</span></div>'
   +'<div class="settings-row"><div><b>'+esc(t('classic'))+'</b><p>'+esc(t('classicSub'))+'</p></div><a class="btn small" href="'+esc(cfg.base?cfg.base+'classic/':'#settings')+'">'+esc(t('open'))+'</a></div>'
   +'<div class="settings-row"><div><b>'+esc(t('docs'))+'</b></div><a class="btn small" target="_blank" rel="noopener noreferrer" href="https://github.com/zarkmakerburg/baft/tree/main/docs">'+esc(t('open'))+'</a></div></section></div>');
  for(const b of main.querySelectorAll('[data-map-set]')){b.setAttribute('aria-pressed',String(b.dataset.mapSet===mapMode));b.addEventListener('click',()=>{mapMode=b.dataset.mapSet;store.set('baft.map',mapMode);for(const x of main.querySelectorAll('[data-map-set]'))x.setAttribute('aria-pressed',String(x===b))})}
  paintChrome()}
const VIEWS={servers:serversView,tunnels:tunnelsView,monitoring:monitoringView,security:securityView,ssl:sslView,backups:backupsView,updates:updatesView,finance:financeView,settings:settingsView};

function go(next,focus=true){route=next;const main=$('main');parkOps();map=null;main.replaceChildren();main.dataset.route=route;chrome();
  if(route==='dashboard')mountDashboard(main);else VIEWS[route](main);
  if(focus){main.focus({preventScroll:true});window.scrollTo(0,0)}window.dispatchEvent(new CustomEvent('baft:route',{detail:route}))}
function render(){paintChrome();if(!$('app').hidden)go(route,false);else if(!preview)$('loginError').textContent=loginError()}

/* ---------- Demo data for the static preview only ---------- */
function demoOverview(){const now=Date.now(),pt=(lat,lon)=>({lat,lon}),n=(id,alias,role,country,ms,ll)=>({id,alias,role,status:'up',latency_ms:ms,country,...ll,spark:Array.from({length:16},(_,i)=>ms+Math.round(Math.sin(i+ms)*8)),last_seen:new Date(now-30000).toISOString()});
  const nodes=[n('ir-1','IR-01','worker','IR',null,pt(35.69,51.39)),n('ex-1','EX-01','foreign','DE',71,pt(50.11,8.68)),n('ex-2','EX-02','foreign','NL',84,pt(52.37,4.9)),n('ex-3','EX-03','foreign','US',72,pt(40.71,-74.01)),n('ex-4','EX-04','foreign','CA',68,pt(43.65,-79.38)),n('ex-5','EX-05','foreign','GB',92,pt(51.51,-.13)),n('ex-6','EX-06','foreign','SG',148,pt(1.35,103.82))];
  nodes[0].latency_ms=12;const points=Array.from({length:97},(_,i)=>{const v=(110+60*Math.sin(i/9)+i*.9)*1e6;return{t:new Date(now-86400000+i*900000).toISOString(),in_bps:v,out_bps:v*.55}});
  return{demo:true,version:'demo',servers:{online:7,total:7},tunnels:{active:6,building:0,failed:0},latency_ms:89,traffic:{in_bps:160e6,out_bps:88e6,source:'foreign'},alerts:{active:0,critical:0},nodes,links:nodes.slice(1).map(x=>({tunnel_id:'t-'+x.id,ir_node:'ir-1',ex_node:x.id,phase:'active',port:8443})),series:{window:'24h',bucket_seconds:900,points}}}

/* ---------- Start ---------- */
paintChrome();
if(live||preview&&ROUTES.includes(location.hash.slice(1))){$('public').hidden=true;$('app').hidden=false;const h=location.hash.slice(1);route=ROUTES.includes(h)?h:'dashboard';go(route,false);refresh();startPolling();
  if(preview)window.addEventListener('hashchange',()=>{const h=location.hash.slice(1);if(ROUTES.includes(h)){$('public').hidden=true;$('app').hidden=false;go(h)}})}
else{showPublic(location.hash==='#signin'||cfg.mode==='signin'?'signin':'landing');$('loginError').textContent=loginError();
  if(preview){for(const i of form.querySelectorAll('input'))i.disabled=true;form.querySelector('[type=submit]').disabled=true;$('loginError').textContent=t('previewLogin');const d=el('a','btn ghost block',esc('DEMO · '+t('demo')));d.href='#dashboard';d.addEventListener('click',ev=>{ev.preventDefault();$('public').hidden=true;$('app').hidden=false;location.hash='dashboard';go('dashboard');refresh()});form.insertBefore(d,form.querySelector('.hint'))}}
})();

package bcc

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>BAFT Command Center</title>
<style>
:root{color-scheme:dark;font-family:Inter,system-ui,sans-serif;background:#090b10;color:#eef1f6}
body{margin:0;background:radial-gradient(circle at top,#182238,#090b10 42%);min-height:100vh}
.wrap{max-width:1280px;margin:auto;padding:24px}.top{display:flex;justify-content:space-between;align-items:center;gap:12px}
h1{font-size:24px;margin:0}.muted{color:#8e9aaf}.card{background:#111722;border:1px solid #263147;border-radius:16px;padding:18px;margin-top:16px}
.grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.grid input,.grid select,select{background:#0b1019;border:1px solid #2a3650;color:#fff;border-radius:10px;padding:10px}
button{background:#e8b54a;color:#16100a;border:0;border-radius:10px;padding:10px 14px;font-weight:700;cursor:pointer}button.alt{background:#25324a;color:#e8eef8}
table{width:100%;border-collapse:collapse;margin-top:12px}th,td{text-align:left;padding:10px;border-bottom:1px solid #222c3e;font-size:13px;vertical-align:top}
.badge{display:inline-block;padding:4px 9px;border-radius:999px;background:#263147;text-transform:uppercase;font-size:11px}.up{background:#123e2c;color:#92f0bf}.down{background:#4b2026;color:#ffabb6}.unknown{background:#37333a;color:#d9cedd}
.route{display:flex;gap:6px;align-items:center;margin:3px 0;white-space:nowrap}.actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:12px}
.kpis{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.kpi{background:#0b1019;border-radius:12px;padding:12px}.kpi b{display:block;font-size:20px;margin-top:4px}
canvas{width:100%;height:240px;background:#0b1019;border-radius:12px;margin-top:12px}
@media(max-width:820px){.grid,.kpis{grid-template-columns:1fr 1fr}.wrap{padding:12px}.tablewrap{overflow:auto}}@media(max-width:520px){.grid,.kpis{grid-template-columns:1fr}}

/* BCC V3 visual harmonization foundation — presentation-only; no API or auth changes. */
:root{--bcc-bg:#090c12;--bcc-panel:#141a24;--bcc-field:#0c121c;--bcc-line:#34343b;--bcc-text:#f1eee8;--bcc-muted:#a4a8b1;--bcc-gold:#d5b578;--bcc-focus:#e8c887}
body{background:radial-gradient(ellipse at 70% 0%,#202b38,var(--bcc-bg) 54%);color:var(--bcc-text);line-height:1.55}
.card{background:var(--bcc-panel);border-color:var(--bcc-line);border-radius:16px;box-shadow:0 8px 28px #0002}
.muted{color:var(--bcc-muted)}.kpi{background:var(--bcc-field);border:1px solid var(--bcc-line)}
button{min-height:40px;border-radius:10px;background:var(--bcc-gold);color:#18130d;transition:filter .15s ease}button.alt{background:#253044;color:var(--bcc-text);border:1px solid var(--bcc-line)}
button:hover:not(:disabled){filter:brightness(1.1)}button:disabled{opacity:.52;cursor:not-allowed}
button:focus-visible,input:focus-visible,select:focus-visible,textarea:focus-visible,a:focus-visible,summary:focus-visible{outline:2px solid var(--bcc-focus);outline-offset:3px}
.grid input,.grid select,select,textarea{background:var(--bcc-field);border:1px solid var(--bcc-line);color:var(--bcc-text);min-height:40px}
input::placeholder,textarea::placeholder{color:var(--bcc-muted);opacity:.9}
table{width:100%}th{color:var(--bcc-muted);font-weight:650}th,td{border-color:var(--bcc-line)}
.tablewrap{max-width:100%;overflow-x:auto}details>summary{cursor:pointer}
@media(max-width:820px){.top{flex-wrap:wrap}.grid{grid-template-columns:repeat(2,minmax(0,1fr))}.actions{flex-wrap:wrap}}
@media(max-width:520px){.grid,.kpis{grid-template-columns:minmax(0,1fr)}.card{padding:14px}.wrap{padding:12px}button{max-width:100%}}
@media(prefers-reduced-motion:reduce){*,*::before,*::after{animation-duration:.01ms!important;transition-duration:.01ms!important;scroll-behavior:auto!important}}

/* Native BAFT Smart Tunnel Wizard: bilingual, mobile-first, classic + Aurora. */
html[lang="fa"] .tn-en,html:not([lang="fa"]) .tn-fa{display:none!important}
.tnWizardSteps{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:7px;margin:18px 0}
.tnWizardSteps span[data-tn-nav]{border:1px solid var(--bcc-line,#34343b);border-radius:12px;padding:10px 5px;font-size:12px;text-align:center;color:var(--bcc-muted,#9aa5b7)}
.tnWizardSteps span[data-tn-nav].selected{color:var(--bcc-text,#fff);border-color:var(--bcc-gold,#d5b578);background:rgba(213,181,120,.12)}
.tnWizardStage[hidden],.tnWizardControls [hidden]{display:none!important}
.tnWizardStage{padding:14px 0;min-height:190px}
.tnWizardStage h3{font-size:17px;font-weight:700;margin:0 0 12px}
.tnWizardStage select{width:100%;max-width:550px}
.tnLabel{display:flex;flex-direction:column;gap:7px;margin:10px 0;font-size:13px;font-weight:650}
.tnLabel input,.tnLabel select{box-sizing:border-box;min-width:0;width:100%;font-weight:400}
.tnFields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 20px}
.tnConfirm{display:flex;align-items:flex-start;gap:10px;margin:15px 0;line-height:1.6;font-size:13px}
.tnConfirm input{width:19px;height:19px;flex:none;accent-color:var(--bcc-gold,#d5b578)}
.tnNotice{border-inline-start:3px solid var(--bcc-gold,#d5b578);background:rgba(213,181,120,.09);padding:13px;border-radius:8px;margin:12px 0;font-size:13px;line-height:1.8}
.tnNotice p{margin:6px 0 0}
.tnAdvanced{margin-top:14px;border:1px solid var(--bcc-line,#34343b);padding:12px;border-radius:10px}
.tnReview{display:grid;gap:9px;padding:14px;border:1px solid var(--bcc-line,#34343b);border-radius:12px;margin-bottom:12px;overflow-wrap:anywhere}
.tnWizardControls{display:flex;gap:10px;flex-wrap:wrap;margin:18px 0}
.tnFlowPreview{border:1px solid var(--bcc-line,#34343b);border-radius:12px;padding:12px;margin:10px 0 16px;background:rgba(213,181,120,.05)}
.tnFlowPreview p{font-size:12px;line-height:1.7;color:var(--bcc-muted,#9aa5b7);margin:7px 0 0}
.tnFlowTrack{display:flex;align-items:center;gap:9px;flex-wrap:wrap;margin-top:10px;font-size:13px}
.tnFlowTrack span:nth-child(odd){border:1px solid var(--bcc-line,#34343b);padding:7px;border-radius:9px}
.tnPortPicker{display:flex;flex-wrap:wrap;gap:9px;min-width:0}
.tnPortPicker input{flex:1 1 170px;min-width:0;width:auto}
.tnPortPicker button{flex:0 1 auto;max-width:100%;white-space:normal}
/* BAFT Toolbox — original Aurora-compatible cards; backend is the source of capability truth. */
.tlIntro{font-size:13px;color:var(--bcc-muted);line-height:1.7;margin-top:6px}
.tlCatalog{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:12px;margin-top:15px}
.tlTile{position:relative;display:flex;flex-direction:column;gap:9px;min-width:0;padding:15px;border-radius:14px;border:1px solid var(--bcc-line);background:linear-gradient(150deg,rgba(213,181,120,.075),var(--bcc-field) 42%);box-shadow:0 5px 20px #0001;transition:border-color .18s ease,transform .18s ease}
.tlTile:hover{border-color:var(--bcc-gold);transform:translateY(-2px)}
.tlTile h3{margin:0;font-size:16px}.tlTile p{color:var(--bcc-muted);font-size:12px;line-height:1.6;margin:0;flex:1}
.tlTileTop{display:flex;justify-content:space-between;align-items:flex-start;gap:8px;flex-wrap:wrap}
.tlMeta{font-size:11px;color:var(--bcc-muted);overflow-wrap:anywhere}.tlTile button{align-self:flex-start;max-width:100%}
.tlTile button:disabled{opacity:.65}.tlState{font-size:10px;border-radius:999px;padding:3px 7px;border:1px solid var(--bcc-line);font-weight:700}
.tlState.on{color:#9debc9;border-color:#316d57}.tlState.off{color:var(--bcc-muted)}
@media(max-width:620px){.tnWizardSteps{display:flex;overflow:auto;gap:7px;padding:0 0 8px}.tnWizardSteps span[data-tn-nav]{min-width:92px;flex:none}.tnFields{grid-template-columns:1fr}.tnWizardControls button{flex:1}.tlCatalog{grid-template-columns:repeat(auto-fit,minmax(170px,1fr))}}
@media(prefers-reduced-motion:reduce){.tlTile{transition:none}.tlTile:hover{transform:none}}

</style></head>
<body><div class="wrap">
<div class="top"><div><h1>BAFT Command Center</h1><div class="muted">Operational monitoring, finance & hardening · Step 5.4</div></div><button class="alt" onclick="setToken()">Admin Token</button></div>

<div class="card">
<div class="top"><b>Live Monitoring</b><span id="monitorTime" class="muted">—</span></div>
<div class="kpis" style="margin-top:12px"><div class="kpi">Nodes<b id="kNodes">—</b></div><div class="kpi">UP<b id="kUp">—</b></div><div class="kpi">DOWN<b id="kDown">—</b></div><div class="kpi">UNKNOWN<b id="kUnknown">—</b></div></div>
<div class="tablewrap"><table><thead><tr><th>Node</th><th>Status</th><th>Noise RTT</th><th>Health latency</th><th>Error rate</th><th>Last Seen</th><th>Sessions</th><th>Routes</th></tr></thead><tbody id="monitorRows"></tbody></table></div>
</div>

<div class="card"><div class="top"><b>7-Day Traffic & Uptime</b><select id="historyNode" onchange="loadHistory()"></select></div><canvas id="historyCanvas" width="1100" height="240"></canvas><div id="historyLegend" class="muted" style="margin-top:8px">Select a node.</div></div>

<div class="card"><b>Register / update node</b><div class="grid" style="margin-top:12px">
<input id="nid" placeholder="node id"><input id="alias" placeholder="alias"><input id="addr" placeholder="management host:port">
<input id="path4" placeholder="path IPv4 (optional)"><input id="path6" placeholder="path IPv6 (optional)">
<select id="role"><option value="foreign">foreign</option><option value="worker">worker</option><option value="master">master</option></select>
<input id="pub" placeholder="public key"><input id="agentEnv" placeholder="agent token env name"><button onclick="saveNode()">Save Node</button>
</div></div>

<div class="card"><b>Add a server over SSH</b>
<div class="muted" style="margin-top:6px">Enter the server's SSH details and press Connect and install. BCC connects, remembers the server's SSH key from this first connection and installs the BAFT agent. The SSH password or key is sent once over this connection and is never stored or logged.</div>
<div class="grid" style="margin-top:12px">
<input id="bsAlias" placeholder="server name (e.g. Germany 1)"><select id="bsRole"><option value="foreign">foreign (EX)</option><option value="worker">worker (IR)</option><option value="master">master (IR)</option></select>
<input id="bsHost" placeholder="host or IP"><input id="bsPort" placeholder="ssh port" value="22"><input id="bsUser" placeholder="ssh user" value="root"><input id="bsPass" type="password" autocomplete="off" placeholder="ssh password">
</div>
<details style="margin-top:10px"><summary class="muted">Use an SSH key or a custom node ID</summary>
<div class="grid" style="margin-top:10px;grid-template-columns:2fr 1fr 1fr">
<textarea id="bsKey" rows="3" autocomplete="off" spellcheck="false" placeholder="or paste an SSH private key (PEM)" style="background:#0b1019;border:1px solid #2a3650;color:#fff;border-radius:10px;padding:10px"></textarea><input id="bsPhrase" type="password" autocomplete="off" placeholder="key passphrase (if any)"><input id="bsId" placeholder="node id (optional)">
</div></details>
<div class="actions"><button id="bsGo" onclick="bsRun()">Connect and install</button><button id="bsNext" class="alt" onclick="bsTunnel()" hidden>Build a tunnel</button></div>
<div id="bsFp" class="muted" style="margin-top:10px;font-family:monospace;word-break:break-all"></div>
<pre id="bsOut" dir="auto" class="muted" style="white-space:pre-wrap;margin-top:10px;unicode-bidi:plaintext;text-align:start"></pre></div>

<div class="card"><div class="top"><b>Existing tunnels (discovery)</b><span><button class="alt" onclick="dcStart()">Discover all</button> <button class="alt" onclick="loadDiscovery()">Refresh</button></span></div>
<div class="muted">Report only. Discovery reads each node and never adopts, changes, restarts or deletes anything. States: MANAGED, DRIFTED, MISSING, DISCOVERED_UNMANAGED, OWNERSHIP_CONFLICT, UNKNOWN.</div>
<div class="tablewrap"><table><thead><tr><th>Node</th><th>Unit</th><th>State</th><th>Why</th></tr></thead><tbody id="dcRows"></tbody></table></div></div>
<div class="card"><div class="top"><b>Layered health</b><button class="alt" onclick="loadHealth()">Refresh</button></div>
<div class="muted">A state changes only after several consistent samples. NOT_ASSESSED means BCC has no signal for that layer; UNKNOWN means no evidence yet or for a while. Neither is DOWN.</div>
<div class="tablewrap"><table><thead><tr><th>Node</th><th>Overall</th><th>Layers</th><th>Recent transitions</th></tr></thead><tbody id="hlRows"></tbody></table></div></div>
<div class="card aurora-tunnel-toolbox" id="tunnel-toolbox" aria-labelledby="tlTitle">
 <div class="top"><div><b id="tlTitle"><span class="tn-en">Universal Tunnel Toolbox</span><span class="tn-fa">جعبه‌ابزار همه‌کارهٔ تونل</span></b><div class="tlIntro"><span class="tn-en">One control center, engine-aware workflows. Only verified backend drivers can start a real build; planned drivers stay locked until signed agents and two-node tests are ready.</span><span class="tn-fa">یک مرکز مدیریت برای موتورهای مختلف. فقط موتورهای تأییدشده می‌توانند ساخت واقعی را شروع کنند؛ موتورهای آینده تا آماده‌شدن عامل‌های امضاشده و آزمون دو سرور غیرفعال‌اند.</span></div></div><button class="alt" type="button" onclick="loadTunnelEngines()"><span class="tn-en">Refresh engines</span><span class="tn-fa">تازه‌سازی موتورها</span></button></div>
 <div id="tlSummary" class="muted" style="margin-top:10px" aria-live="polite"><span class="tn-en">Checking supported drivers…</span><span class="tn-fa">در حال بررسی موتورهای پشتیبانی‌شده…</span></div>
 <div class="tlCatalog" id="tlCatalog" aria-live="polite"></div>
</div>

<div class="card aurora-tunnel-studio"><div class="top"><div><b>Aurora · Tunnel Control Studio</b><div class="muted">Smart Tunnel Wizard / ساخت ساده تونل — Native BAFT, real reviewed deployment</div></div><button class="alt" onclick="loadTunnels()">Refresh / تازه‌سازی</button></div>
<div class="tnWizardSteps" id="tnWizardSteps" aria-label="Tunnel creation steps">
 <span data-tn-nav="0" class="selected">1 · <span class="tn-en">Type</span><span class="tn-fa">نوع</span></span>
 <span data-tn-nav="1">2 · <span class="tn-en">Servers</span><span class="tn-fa">سرورها</span></span>
 <span data-tn-nav="2">3 · <span class="tn-en">Ports &amp; destination</span><span class="tn-fa">پورت و مقصد</span></span>
 <span data-tn-nav="3">4 · <span class="tn-en">Safety</span><span class="tn-fa">ایمنی</span></span>
 <span data-tn-nav="4">5 · <span class="tn-en">Review &amp; deploy</span><span class="tn-fa">بررسی و ساخت</span></span>
</div>
<div class="tnWizardStage" data-tn-step="0">
 <h3><span class="tn-en">What do you want to connect?</span><span class="tn-fa">چه اتصالی می‌خواهید بسازید؟</span></h3>
 <label class="tnLabel" for="tnDirection"><span class="tn-en">How should the two servers connect?</span><span class="tn-fa">جهت اتصال دو سرور چگونه باشد؟</span></label>
 <select id="tnDirection" onchange="tnClearPlan()" aria-describedby="tnDirectionInfo">
  <option value="ir-to-ex">Direct · IR dials EX — Native BAFT / مستقیم · ایران به خارج وصل می‌شود</option>
  <option value="ex-to-ir" disabled>Reverse · EX dials IR — Planned / معکوس · خارج به ایران وصل می‌شود — در برنامه</option>
 </select>
 <div class="tnFlowPreview" id="tnDirectionInfo" role="note">
  <b><span class="tn-en">Native BAFT connection direction</span><span class="tn-fa">جهت اتصال موتور بومی BAFT</span></b>
  <div class="tnFlowTrack" dir="ltr"><span>🇮🇷 IR (dialer)</span><span aria-hidden="true">→</span><span>🌍 EX (listener)</span><span aria-hidden="true">→</span><span>EX service</span></div>
  <p><span class="tn-en">Direction describes the inter-server connection, separate from user ingress. Reverse engines stay unavailable until their real adapters and two-node tests are ready.</span><span class="tn-fa">جهت تونل مربوط به اتصال بین سرورهاست و از ورودی کاربران جداست. موتورهای معکوس تا آماده‌شدن پیاده‌سازی واقعی و آزمون دو سرور غیرفعال می‌مانند.</span></p>
 </div>
 <input type="hidden" id="tnEngine" value="baft">
 <div class="muted"><span class="tn-en">Engine: BAFT Secure — TLS + Noise. Other engines will be added once implemented and tested.</span><span class="tn-fa">موتور: BAFT Secure با TLS و Noise. موتورهای دیگر پس از پیاده‌سازی و آزمون اضافه خواهند شد.</span></div>
 <label class="tnLabel" for="tnMode"><span class="tn-en">Action</span><span class="tn-fa">عملیات</span></label>
 <select id="tnMode" onchange="tnModeChanged();tnJoinOwnerChanged()"><option value="independent">New independent tunnel / تونل مستقل جدید</option><option value="join">Join another IR to an active EX listener / اتصال IR دیگر به شنونده فعال EX</option><option value="replace">Edit selected active tunnel / ویرایش تونل انتخاب‌شده</option></select>
 <p class="muted"><span class="tn-fa">برای ویرایش، از فهرست تونل‌های فعال دکمه «ویرایش» را انتخاب کنید. ساخت مستقل نباید تونل قبلی را جایگزین کند.</span><span class="tn-en">To edit, choose Edit on an active tunnel below. A new independent tunnel must not replace an existing tunnel.</span></p>
</div>
<div class="tnWizardStage" data-tn-step="1" hidden>
 <h3><span class="tn-en">Choose the two servers</span><span class="tn-fa">دو سرور را انتخاب کنید</span></h3>
 <div class="tnFields"><label class="tnLabel"><span class="tn-en">Iran / IR node</span><span class="tn-fa">سرور ایران (IR)</span><select id="tnIr" onchange="tnWizardChanged()"></select></label>
 <label class="tnLabel" id="tnExField"><span class="tn-en">Abroad / EX node</span><span class="tn-fa">سرور خارج (EX)</span><select id="tnEx" onchange="tnWizardChanged()"></select></label>
 <label class="tnLabel" id="tnJoinOwnerField" hidden><span class="tn-en">Existing active EX listener</span><span class="tn-fa">شنوندهٔ فعال EX برای اشتراک</span><select id="tnJoinOwner" onchange="tnJoinOwnerChanged()"><option value="">Choose an active EX tunnel… / تونل فعال EX را انتخاب کنید…</option></select></label></div>
 <div id="tnJoinOwnerInfo" class="muted" role="status" aria-live="polite"></div>
 <div class="muted"><span class="tn-fa">مسیر BAFT از سرور ایران، ارتباط امن BAFT و سرور خارج به مقصد سرویس می‌رسد. پس از فعال‌شدن تونل، ورودی عمومی بررسی‌شده آماده می‌شود و فقط وقتی عامل ایران توانایی لازم را اعلام کند در صف می‌رود؛ موفقیت فقط با رسید احراز هویت‌شده و تأیید مستقل مرکز فرمان ثبت می‌شود.</span><span class="tn-en">BAFT path: Iran node → secure BAFT link → foreign node → service destination. When the tunnel becomes active BAFT stages the reviewed public IR ingress and queues it only once the IR agent reports operational-ingress-v1; only an authenticated Agent receipt plus the independent BCC check counts as success.</span></div>
</div>
<div class="tnWizardStage" data-tn-step="2" hidden>
 <h3><span class="tn-en">Ports and service destination</span><span class="tn-fa">پورت‌ها و مقصد سرویس</span></h3>
 <div class="tnFields">
  <label class="tnLabel"><span class="tn-en">BAFT inter-server port (EX)</span><span class="tn-fa">پورت ارتباط دو سرور BAFT (خارج)</span><input id="tnPort" type="number" inputmode="numeric" min="1" max="65535" value="8443" oninput="tnClearPlan()"></label>
  <label class="tnLabel"><span class="tn-en">Public client port on IR (defaults from service port)</span><span class="tn-fa">پورت عمومی کاربران در ایران (پیش‌فرض برابر پورت سرویس)</span><input id="tnClientPort" type="number" inputmode="numeric" min="1" max="65535" placeholder="auto from target" oninput="tnWizardIngressChanged()"></label>
 </div>
 <div class="tnFields"><button type="button" class="alt" onclick="tnAutoSelect()"><span class="tn-en">Pick EX port from path evidence</span><span class="tn-fa">انتخاب پورت خارج از شواهد مسیر</span></button><span id="tnAutoSelectNote" class="muted" aria-live="polite"></span></div>
 <div class="tnFields"><div class="tnLabel"><label for="tnTarget"><span class="tn-en">EX service destination (IP:port)</span><span class="tn-fa">نشانی و پورت مقصد سرویس در خارج</span></label><div class="tnPortPicker"><input id="tnTarget" dir="ltr" autocomplete="off" spellcheck="false" placeholder="127.0.0.1:2443" aria-describedby="tnTargetInfo tnServicePortHint" oninput="tnWizardTargetChanged()"><button type="button" class="alt" onclick="tnWizardGenerateServicePort()"><span class="tn-en">Suggest destination port</span><span class="tn-fa">پیشنهاد پورت مقصد</span></button></div></div></div>
 <p id="tnTargetInfo" class="muted"><span class="tn-fa">موتور بومی BAFT به IP ثابت مقصد و پورت نیاز دارد؛ برای IPv6 از قالب <bdi dir="ltr">[::1]:2443</bdi> استفاده کنید. نشانی مربوط به سرویسی است که سرور خارج باید به آن متصل شود.</span><span class="tn-en">Native BAFT requires a fixed destination IP and port; use <bdi dir="ltr">[::1]:2443</bdi> for IPv6. This is the service the foreign server connects to.</span></p>
 <p class="muted"><span class="tn-en">Port suggestions check known BCC inventory only. They do not scan or reserve OS sockets or configure the destination service.</span><span class="tn-fa">پیشنهاد پورت فقط تداخل‌های شناخته‌شده در BCC را بررسی می‌کند؛ پورت سیستم‌عامل را اسکن یا رزرو نمی‌کند و سرویس مقصد را تنظیم نمی‌کند.</span></p>
 <div id="tnServicePortHint" class="muted" role="status" aria-live="polite"></div>
 <div class="tnNotice" role="note"><strong><span class="tn-en">Critical port distinction</span><span class="tn-fa">تفاوت مهم پورت‌ها</span></strong><p><span class="tn-fa">BAFT در این نسخه فقط پورت تونل، شنونده داخلی ایران و مقصد سرویس خارج را تنظیم می‌کند. ورودی عمومی ایران و تنظیمات برنامهٔ مقصد جداگانه مدیریت می‌شوند. BAFT پس از فعال‌شدن تونل، عملیات امضاشده ورودی عمومی را برای همین پورت آماده می‌کند و فقط وقتی عامل ایران توانایی لازم را اعلام کند در صف می‌گذارد؛ آماده یا در صف به‌معنای موفقیت نیست و رسید احراز هویت‌شده عامل و تأیید مستقل لازم است.</span><span class="tn-en">This version configures the BAFT link, local IR route and foreign service target. After tunnel activation BAFT stages a signed public IR ingress operation for this same client port and queues it only when the IR agent reports operational-ingress-v1. Staged or queued is not success: the Agent must verify its private authority and return an authenticated receipt, and BCC verifies the port independently. Destination application and 3X-UI settings remain separately managed.</span></p></div>
 <label class="tnConfirm"><input type="checkbox" id="tnIngressConfirm" onchange="tnClearPlan()"> <span class="tn-en">I authorize BAFT to provision public IR ingress on this client port after the tunnel becomes active.</span><span class="tn-fa">اجازه می‌دهم BAFT پس از فعال‌شدن تونل، ورودی عمومی ایران را روی همین پورت کاربران ایجاد کند.</span></label>
 <button type="button" class="alt" onclick="tnWizardSuggest()"><span class="tn-en">Suggest free managed ports</span><span class="tn-fa">پیشنهاد پورت‌های بدون تداخل در BCC</span></button>
</div>
<div class="tnWizardStage" data-tn-step="3" hidden>
 <h3><span class="tn-en">Transport and safety</span><span class="tn-fa">نوع ارتباط و ایمنی</span></h3>
 <p class="muted" id="tnTransportModeNote"><span class="tn-fa">ارتباط دو سرور از BAFT Secure با TLS و Noise استفاده می‌کند. هر تونل مستقل شنونده اختصاصی خود را در سرور خارج دارد. پیوستن صریح، یک IR تازه را به شنونده فعال انتخاب‌شده اضافه می‌کند؛ این Multiplex مشترک TLS نیست. پورت ۴۴۳ فقط در صورت آزاد و سازگار بودن انتخاب می‌شود و به‌تنهایی مقاومت در برابر فیلترینگ را تضمین نمی‌کند. موتورهای دیگر تا پیاده‌سازی و آزمون واقعی غیرفعال‌اند.</span><span class="tn-en">Server-to-server transport uses BAFT Secure with TLS and Noise. Each independent tunnel gets a dedicated EX listener per tunnel. Explicit Join adds a new IR peer to the selected active listener; shared TLS multiplexing is not enabled. Port 443 alone does not guarantee censorship resistance. Other engines remain unavailable until implemented and tested.</span></p>
  <label class="tnConfirm" id="tnJoinConfirmField" hidden><input type="checkbox" id="tnJoinConfirm" onchange="tnClearPlan()"> <span class="tn-en">I authorize this logical tenant to share the selected EX listener. Keep its owner and existing tenants attached.</span><span class="tn-fa">اجازه می‌دهم این تونل منطقی از شنوندهٔ انتخاب‌شدهٔ EX استفاده کند؛ مالک و مستأجران فعلی حفظ شوند.</span></label>
 <div id="tnScopedFields" class="aurora-scope">
  <div class="tnNotice"><span class="tn-fa">هر تونل مستقل پیکربندی جداگانه دارد. مقادیر زیر پیشنهاد هستند؛ بررسی نهایی پورت‌ها هنگام Preview و استقرار انجام می‌شود.</span><span class="tn-en">Each independent tunnel has isolated config. Values below are suggestions; final conflicts are checked by the reviewed plan and deployment.</span></div>
  <label class="tnConfirm"><input type="checkbox" id="tnLegacyCoexist" onchange="tnClearPlan()"> <span class="tn-en">Keep active legacy tunnel on shared server (explicit consent)</span><span class="tn-fa">تونل قدیمی فعال روی سرور مشترک حفظ شود (تأیید صریح)</span></label>
  <details class="tnAdvanced"><summary><span class="tn-en">Advanced: instance and internal listeners</span><span class="tn-fa">پیشرفته: شناسه نمونه و پورت‌های داخلی</span></summary>
    <div class="tnFields">

      <label class="tnLabel" id="tnJoinInstanceField">Instance ID<input id="tnInstance" maxlength="40" autocomplete="off" placeholder="auto-created"></label>
      <label class="tnLabel">IR loopback route<input id="tnRouteListen" value="127.0.0.1:1444"></label>
      <label class="tnLabel" id="tnJoinExMetricField">EX metrics loopback<input id="tnExMetric" value="127.0.0.1:9202"></label>
      <label class="tnLabel" id="tnJoinIrMetricField">IR metrics loopback<input id="tnIrMetric" value="127.0.0.1:9302"></label>
    </div>
  </details>
 </div>
 <div id="tnEditNote" class="muted" style="margin-top:9px"></div>
</div>
<div class="tnWizardStage" data-tn-step="4" hidden>
 <h3><span class="tn-en">Review the real route before deployment</span><span class="tn-fa">قبل از ساخت، مسیر واقعی را بررسی کنید</span></h3>
 <div id="tnWizardSummary" class="tnReview" aria-live="polite"></div>
 <div class="tnNotice"><span class="tn-fa">تأیید طرح تونل، دسترسی عمومی کاربران را ثابت نمی‌کند. BAFT پس از فعال‌شدن تونل، ورودی را آماده می‌کند و فقط با اعلام توانایی عامل در صف می‌گذارد؛ عملیات تا تأیید رسید احراز هویت‌شده عامل و آزمون مستقل در انتظار می‌ماند. تداخل پورت یا سلامت نامعتبر عامل، استقرار را متوقف می‌کند.</span><span class="tn-en">A successful tunnel plan does not prove public reachability. BAFT stages ingress after activation and queues it only when the IR agent reports operational-ingress-v1; the operation remains pending until the authenticated Agent receipt and the independent BCC check succeed. Port conflicts or stale Agent health block deployment.</span></div>
 <div class="actions"><button type="button" onclick="tnPlan()"><span class="tn-en">Review real BAFT deployment plan</span><span class="tn-fa">بررسی طرح واقعی استقرار BAFT</span></button></div>
 <div id="tnPlan" style="margin-top:12px"></div>
</div>
<div class="tnWizardControls">
 <button type="button" class="alt" id="tnWizardBack" onclick="tnWizardStep(tnWizardIndex-1)" hidden><span class="tn-en">Previous</span><span class="tn-fa">قبلی</span></button>
 <button type="button" id="tnWizardNext" onclick="tnWizardStep(tnWizardIndex+1)"><span class="tn-en">Next step</span><span class="tn-fa">مرحله بعد</span></button>
 <button type="button" class="alt" onclick="tnNew()"><span class="tn-en">New tunnel / Reset</span><span class="tn-fa">تونل جدید / شروع دوباره</span></button>
</div>
<div class="tablewrap"><table><thead><tr><th>ID</th><th>EX</th><th>IR</th><th>Phase</th><th>Detail</th><th></th></tr></thead><tbody id="tnRows"></tbody></table></div><div class="tnNotice" role="note"><b>Public IR ingress check / بررسی ورودی عمومی ایران</b><p class="muted">For active tunnels only: request a signed read-only socket/firewall observation from the IR agent. It does not create a listener, reserve a port or prove internet reachability. / فقط بررسی وضعیت است؛ هیچ پورت یا قانون فایروالی ایجاد نمی‌شود و دسترسی کاربران از اینترنت تأیید نمی‌شود.</p><div id="tnIngressPreflightState" role="status" aria-live="polite">Select Check IR ingress on an active tunnel. / برای تونل فعال بررسی ورودی ایران را انتخاب کنید.</div><button type="button" class="alt" id="tnIngressPreflightRefresh" onclick="tnIngressPreflightRefresh()" hidden>Refresh read-only evidence / تازه‌سازی شواهد</button><div id="tnIngressReadinessState" role="status" aria-live="polite" class="muted"></div><pre id="tnIngressReadinessCard" class="muted" style="white-space:pre-wrap"></pre></div><div class="tnNotice" role="note"><b>Operational public IR ingress / ورودی عملیاتی عمومی ایران</b><p class="muted">Deploy stages the exact reviewed tunnel plan and queues it only when the enrolled IR agent reports operational-ingress-v1. Held or queued is not success, and an applied receipt is not Ready: the panel stays pending until Verified ingress readiness succeeds. Retirement is offered only after an applied receipt. / ساخت، طرح بررسی‌شده را آماده می‌کند و فقط وقتی عامل ایران توانایی لازم را اعلام کند در صف می‌گذارد. در انتظار یا در صف به‌معنای موفقیت نیست و رسید اعمال‌شده هم آماده بودن نیست؛ تا تأیید مستقل آمادگی، وضعیت در انتظار می‌ماند.</p><div id="tnIngressOperationState" role="status" aria-live="polite">Choose Deploy public ingress or check an existing operation. / ساخت ورودی عمومی را انتخاب کنید یا وضعیت عملیات را ببینید.</div><div class="actions"><button type="button" class="alt" id="tnIngressOperationRefresh" onclick="tnIngressOperationRefresh()" hidden>Refresh operation / تازه‌سازی عملیات</button><button type="button" id="tnIngressOperationRetire" onclick="tnIngressRetire()" hidden>Retire applied ingress / حذف ورودی اعمال‌شده</button></div></div></div>

<div class="card"><div class="top"><b>Change History / تاریخچه تغییرات</b><button class="alt" onclick="chLoad()">Refresh / بروزرسانی</button></div><div class="muted">Append-only, secret-free snapshots of confirmed BAFT-managed changes. / تاریخچه فقط‌افزودنی تغییرات تأییدشده بدون ذخیره اطلاعات محرمانه.</div><div class="grid" style="margin-top:8px"><input id="chNode" placeholder="node ID"><input id="chTunnel" placeholder="tunnel ID"></div><div id="chRows" style="margin-top:8px"></div></div>
<div class="card"><div class="top"><b>Safe SSH Port Migration / مهاجرت امن پورت SSH</b></div>
<div class="muted">Plan → dual-listen → authenticated verification → cutover. BAFT never changes cloud/firewall rules and rolls back if verification fails. / ابتدا پورت قدیمی حفظ می‌شود و فقط پس از احراز هویت موفق روی پورت جدید cutover انجام می‌شود.</div>
<div class="grid" style="margin-top:10px"><input id="smNode" placeholder="enrolled node ID"><input id="smOld" value="22" placeholder="old port"><input id="smNew" placeholder="new port"><input id="smUser" placeholder="SSH user"><input id="smFP" placeholder="SHA256 host fingerprint"></div>
<div style="margin-top:8px"><button class="alt" onclick="smPlan()">Plan / برنامه</button> <button class="alt" onclick="smStart()">Start dual-listen / شروع</button></div><div id="smState" class="muted" style="margin-top:8px"></div>
<div class="grid" style="margin-top:8px"><input id="smPassword" type="password" placeholder="password (not stored)"><textarea id="smKey" placeholder="private key (not stored)"></textarea></div><button class="alt" onclick="smVerify()">Verify authenticated access / تأیید دسترسی</button></div>
<div class="card"><div class="top"><b>Route Doctor / عیب‌یاب مسیر</b><button class="alt" onclick="rdRun()">Run diagnosis / اجرای تشخیص</button></div>
<div class="muted">Read-only diagnosis of an active BAFT tunnel. It reports the first causally blocking stage and never changes firewall, ports, certificates, routing or tunnel state. / تشخیص فقط‌خواندنی است و هیچ تنظیمی را تغییر نمی‌دهد.</div>
<div class="grid" style="margin-top:10px"><select id="rdTunnel"><option value="">active tunnel…</option></select></div>
<div id="rdResult" class="muted" style="margin-top:10px">No diagnosis selected. / هنوز تشخیصی اجرا نشده است.</div></div>
<div class="card"><div class="top"><b>Automatic Path Discovery / کشف خودکار مسیر</b><div><span class="badge up">AUTO</span> <button class="alt" onclick="pdRun()">Refresh graph now</button></div></div>
<div class="muted">BCC inventories every enrolled node and measures directed IPv4/IPv6 paths automatically. Only FULL_DATA edges can become routes; TCP/TLS-only and byte-ceiling paths are rejected. No country, provider or server name is hard-coded.</div>
<div id="pdState" class="muted" style="margin-top:8px">Waiting for discovery…</div>
<div style="margin-top:12px"><b>Connectivity Matrix / ماتریس اتصال</b><div class="muted">Rows are sources; columns are destinations. PASS requires fresh FULL_DATA evidence. Stale evidence is never green.</div><div id="pmMatrix" class="tablewrap"></div></div>
<div class="tablewrap"><table><thead><tr><th>From → To</th><th>Recommended path</th><th>Hops</th><th>RTT</th></tr></thead><tbody id="pgRows"></tbody></table></div>
<details style="margin-top:12px"><summary>Advanced manual probe / تست دستی پیشرفته</summary><div class="grid" style="margin-top:12px"><select id="ppSrc"></select><select id="ppDst"></select><select id="ppFamily"><option value="4">IPv4</option><option value="6">IPv6</option></select><input id="ppPorts" value="22,80,443,8443,9443,49152" placeholder="candidate ports"><button onclick="ppRun()">Run pair probe</button></div>
<div class="tablewrap"><table><thead><tr><th>Path</th><th>Family</th><th>Phase</th><th>Recommended</th><th>Evidence</th></tr></thead><tbody id="ppRows"></tbody></table></div></details></div>

<div class="card"><div class="top"><b>Cluster Nodes</b><span id="catalogHealth" class="badge unknown" role="status" aria-live="polite">Release catalog: unknown</span><span id="count" class="muted"></span></div>
<div id="nodeCards" class="grid" aria-label="Server cards"></div>
<div class="tablewrap"><table><thead><tr><th></th><th>Alias</th><th>ID</th><th>Address</th><th>Role</th><th>Health Check</th><th>Last Check</th><th>BAFT / Agent / BCC</th><th>Security</th></tr></thead><tbody id="rows"></tbody></table></div>
<div class="actions"><button id="deploySelected" onclick="deploy()" disabled>One-Click Deploy Selected</button><button class="alt" onclick="loadAll()">Refresh</button></div></div>

<div class="card"><div class="top"><b>BCC Backups / پشتیبان‌های BCC</b><button class="alt" onclick="loadBackups()">Refresh inventory</button></div><p class="muted">Encrypted BCC-wide backups. Restore preview only; no restore execution from this panel.</p><div class="actions"><button onclick="createBCCBackup()">Create encrypted backup / ایجاد پشتیبان</button></div><div id="backupStatus" class="muted" role="status" aria-live="polite"></div><div id="backupInventory" class="muted"></div></div>\n<div class="card"><b>Recent Jobs</b><div id="jobs" class="muted" style="margin-top:10px">Admin token required.</div></div>
<div class="card"><div class="top"><b>Immutable Audit Log</b><button class="alt" onclick="loadAudit()">Refresh</button></div>
<div class="muted" style="margin-top:8px">Read-only hash-chained admin events. No delete/edit action is exposed.</div>
<div class="tablewrap"><table><thead><tr><th>#</th><th>UTC</th><th>IP</th><th>Action</th><th>Target</th><th>Outcome</th><th>Hash</th></tr></thead><tbody id="auditRows"></tbody></table></div></div>
<div class="card"><div class="top"><b>Node Finance</b><button class="alt" onclick="setFinance()">Set Versioned Rate</button></div>
<div class="tablewrap"><table><thead><tr><th>Node</th><th>Traffic GiB</th><th>Cost</th><th>Revenue</th><th>Profit</th></tr></thead><tbody id="financeRows"></tbody></table></div></div>

<div class="card"><div class="top"><b>Finance Reports</b><span class="muted">Default timezone: Asia/Tehran</span></div>
<div class="grid" style="margin-top:12px">
<select id="reportPeriod"><option value="daily">Daily</option><option value="monthly">Monthly</option></select>
<input id="reportFrom" type="date"><input id="reportTo" type="date"><input id="reportTZ" value="Asia/Tehran" placeholder="Timezone">
<button onclick="loadFinanceReport()">Load Report</button><button class="alt" onclick="downloadFinanceCSV()">Download CSV</button>
</div>
<div class="tablewrap"><table><thead><tr><th>Period</th><th>Scope</th><th>Node</th><th>Ingress</th><th>Egress</th><th>Cost</th><th>Revenue</th><th>Profit</th><th>Currency</th></tr></thead><tbody id="reportRows"></tbody></table></div>
</div>
</div>
<script>
const q=s=>document.querySelector(s);let nodes=[],monitor=[];
function token(){return localStorage.getItem('bccToken')||''}
function setToken(){localStorage.setItem('bccToken',prompt('BCC admin token')||'');if(typeof tlCatalogRequestSeq!=='undefined')tlCatalogRequestSeq++;const catalog=q('#tlCatalog'),summary=q('#tlSummary');if(catalog)catalog.replaceChildren();if(summary)summary.textContent='';loadAll()}
function ah(){return {'Authorization':'Bearer '+token(),'Content-Type':'application/json'}}
function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function ago(v){if(!v||new Date(v).getFullYear()<2000)return '—';let d=(Date.now()-new Date(v).getTime())/1000;if(d<60)return Math.max(0,Math.round(d))+'s ago';if(d<3600)return Math.round(d/60)+'m ago';return Math.round(d/3600)+'h ago'}
function rate(v){return (Number(v||0)/1000).toFixed(2)+'/min'}
function routeHTML(rs){if(!rs||!rs.length)return '<span class=muted>—</span>';return rs.map(r=>'<div class=route><span class="badge '+esc(r.status)+'">'+esc(r.status)+'</span><span>'+esc(r.route_id)+'</span><span class=muted>'+((r.latency_ms??-1)>=0?esc(r.latency_ms)+' ms':'—')+' · '+esc(r.probe_kind||'')+'</span></div>').join('')}
async function loadBackups(){
 if(!token())return;
 let out=q('#backupInventory'),status=q('#backupStatus');if(!out||!status)return;
 try{
  let r=await fetch('/api/backups',{headers:ah()});
  if(!r.ok){status.textContent='Backup inventory unavailable (HTTP '+r.status+')';out.textContent='';return}
  let data=await r.json(),entries=Array.isArray(data.backups)?data.backups:[];
  out.innerHTML=entries.map(e=>'<div class="route"><span>'+esc(e.name)+' · '+esc(e.size)+' bytes</span> <button class="alt" data-name="'+esc(e.name)+'" onclick="previewBCCRestore(this.dataset.name)">Preview restore</button></div>').join('')||'No backups / بدون پشتیبان';
  status.textContent=entries.length+' backups';
 }catch(e){status.textContent='Backup inventory unavailable';out.textContent=''}
}
async function createBCCBackup(){
 if(!token()||!confirm('Create a new encrypted BCC backup? / ایجاد پشتیبان رمزگذاری‌شده؟'))return;
 let status=q('#backupStatus');status.textContent='Creating backup...';
 try{
  let r=await fetch('/api/backups/create',{method:'POST',headers:ah()});
  let data=await r.json().catch(()=>({}));
  if(!r.ok){status.textContent=data.reconciliation_required?'Outcome uncertain: reconcile backup '+String(data.name||''):'Backup failed (HTTP '+r.status+')';return}
  await loadBackups();status.textContent='Created: '+String(data.name||'')+'; '+status.textContent;
 }catch(e){status.textContent='Backup request failed; check audit before retry'}
}
async function previewBCCRestore(name){
 if(!token())return;
 let status=q('#backupStatus');status.textContent='Checking restore preview...';
 try{
  let r=await fetch('/api/backups/restore-preview',{method:'POST',headers:{...ah(),'Content-Type':'application/json'},body:JSON.stringify({filename:name})});
  if(!r.ok){status.textContent='Restore preview failed (HTTP '+r.status+')';return}
  let data=await r.json(),safe=data.verified===true&&!data.problem&&!data.would_refuse;
  let diff=['nodes','tunnels','jobs'].map(k=>{let d=data[k]||{};return k+': '+String(d.current??'?')+' → '+String(d.after_restore??'?')}).join(' | ');
  status.textContent='Restore preview '+(safe?'verified':'BLOCKED')+(data.problem?' — '+String(data.problem):'')+(data.would_refuse?' — REFUSED: '+String(data.would_refuse):'')+'; '+diff+'; warnings: '+(Array.isArray(data.warnings)?data.warnings.length:0)+'; no restore executed';
 }catch(e){status.textContent='Restore preview unavailable'}
}
function renderNodeCards(versionMap){
 let cards=q('#nodeCards');if(!cards)return;
 cards.innerHTML=(Array.isArray(nodes)?nodes:[]).map(n=>{
  let ts=(Array.isArray(tnWizardTunnels)?tnWizardTunnels:[]).filter(t=>t.ex_node===n.id||t.ir_node===n.id);
  let nodeActionsAllowed=!n.revoked;
  let tunnels=ts.map(t=>'<div class="route"><b>'+esc(t.id)+'</b> <span class="badge '+(t.phase==='active'?'up':t.phase==='rollback_failed'||t.phase==='decommission_failed'?'down':'unknown')+'">'+esc(t.phase||'unknown')+'</span> <span class="muted">'+esc(t.ex_node===n.id?'EX':'IR')+'</span>'+(t.phase==='active'&&nodeActionsAllowed?'<button class="alt" data-id="'+esc(t.id)+'" onclick="tnEdit(this.dataset.id)">Edit / ویرایش</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDrift(this.dataset.id)">Check drift</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressPreflight(this.dataset.id)">Check IR ingress (read-only)</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressReadiness(this.dataset.id)">Verified ingress readiness</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressDeploy(this.dataset.id)">Deploy public ingress</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressOperationCheck(this.dataset.id)">Ingress status</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnRotate(this.dataset.id)">Rotate cert</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Remove safely / حذف امن</button>':t.phase==='decommission_failed'&&nodeActionsAllowed?'<button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Retry decommission</button>':'')+'</div>').join('');
  return '<section class="card"><div class="top"><b>'+esc(n.alias||n.id)+'</b><span class="badge '+esc(n.health||'unknown')+'">'+esc(n.health||'unknown')+'</span></div><div class="muted">'+esc(n.id)+' · '+esc(n.role||'')+' · '+esc(n.address||'')+'</div><div style="margin-top:10px">'+componentVersionsHTML(versionMap[n.id])+'</div>'+(n.revoked?'<span class="badge down">REVOKED — actions disabled</span>':'')+'<details><summary>Tunnels / تونل‌ها ('+ts.length+')</summary>'+(tunnels||'<span class="muted">No tunnels / بدون تونل</span>')+'</details></section>'
 }).join('');
}
function componentVersionsHTML(item){let names={transport:'BAFT',agent:'Agent',bcc:'BCC'};let parts=item&&Array.isArray(item.components)?item.components:[];return ['transport','agent','bcc'].map(k=>{let c=parts.find(x=>x.component===k)||{};let state=['verified','mismatch','unknown'].includes(c.state)?c.state:'unknown';let label=state==='verified'&&c.version?c.version:state;return '<div class="route"><span class="muted">'+names[k]+'</span> <span class="badge '+(state==='verified'?'up':state==='mismatch'?'down':'unknown')+'">'+esc(label)+'</span></div>'}).join('')}
async function loadNodes(){if(!token())return;let r=await fetch('/api/nodes',{headers:ah()});if(!r.ok)return;nodes=await r.json();let versionMap=Object.create(null);window.nodeVersionMap=versionMap;let catalogStatus='unavailable',catalogVersion='',revocationSequence=0;try{let vr=await fetch('/api/nodes/versions',{headers:ah()});if(vr.ok){let vm=await vr.json();catalogStatus=['verified','unavailable','not_configured'].includes(vm.catalog_status)?vm.catalog_status:'unavailable';if(catalogStatus==='verified'){catalogVersion=typeof vm.catalog_version==='string'?vm.catalog_version:'';revocationSequence=Number.isSafeInteger(vm.revocation_sequence)?vm.revocation_sequence:0;}for(let item of vm.nodes||[]){if(item&&typeof item.node_id==='string'&&Array.isArray(item.components))versionMap[item.node_id]=item}}}catch(e){}let ch=q('#catalogHealth');ch.className='badge '+(catalogStatus==='verified'?'up':catalogStatus==='unavailable'?'down':'unknown');ch.textContent='Release catalog: '+catalogStatus+(catalogStatus==='verified'?' · '+catalogVersion+' · revocations #'+revocationSequence:'');q('#count').textContent=nodes.length+' nodes';renderNodeCards(versionMap);ppFill();q('#deploySelected').disabled=true;q('#rows').innerHTML=nodes.map(n=>'<tr><td><input type=checkbox class=pick value="'+esc(n.id)+'"></td><td>'+esc(n.alias)+'</td><td>'+esc(n.id)+'</td><td>'+esc(n.address)+'</td><td>'+esc(n.role)+'</td><td><span class="badge '+esc(n.health)+'">'+esc(n.health)+'</span> '+((n.latency_ms??-1)>=0?esc(n.latency_ms)+' ms':'')+'</td><td>'+esc(n.last_checked&&new Date(n.last_checked).getUTCFullYear()>1?n.last_checked:'—')+'</td><td>'+componentVersionsHTML(versionMap[n.id])+'</td><td>'+(n.revoked?'<span class="badge down">REVOKED</span>':'<button class="alt" data-id="'+esc(n.id)+'" onclick="rotateToken(this.dataset.id)">Rotate</button> <button data-id="'+esc(n.id)+'" onclick="revokeNode(this.dataset.id)">Kill</button>')+'</td></tr>').join('')}
async function loadMonitoring(){if(!token())return;let r=await fetch('/api/monitoring',{headers:ah()});if(!r.ok){q('#monitorTime').textContent=document.documentElement.lang==='fa'?'خطا در دریافت پایش':'Monitoring unavailable';for(let id of ['kNodes','kUp','kDown','kUnknown'])q('#'+id).textContent='—';q('#monitorRows').innerHTML='';return}monitor=await r.json();q('#monitorTime').textContent=new Date().toLocaleTimeString();q('#kNodes').textContent=monitor.length;q('#kUp').textContent=monitor.filter(x=>x.status==='up').length;q('#kDown').textContent=monitor.filter(x=>x.status==='down').length;q('#kUnknown').textContent=monitor.filter(x=>x.status==='unknown').length;
q('#monitorRows').innerHTML=monitor.map(n=>'<tr><td><b>'+esc(n.alias)+'</b><div class=muted>'+esc(n.node_id)+'</div></td><td><span class="badge '+esc(n.status)+'">'+esc(n.status)+'</span></td><td>'+((n.noise_latency_ms??-1)>=0?esc(n.noise_latency_ms)+' ms':'—')+'<div class=muted>Noise RTT</div></td><td>'+((n.latency_ms??-1)>=0?esc(n.latency_ms)+' ms':'—')+'<div class=muted>TCP health-check</div></td><td>'+rate(n.handshake_error_rate_milli_per_min)+'</td><td>'+ago(n.last_seen)+'</td><td>'+esc(n.active_sessions||0)+'</td><td>'+routeHTML(n.routes)+'</td></tr>').join('');
let sel=q('#historyNode'),cur=sel.value;sel.innerHTML=monitor.map(n=>'<option value="'+esc(n.node_id)+'">'+esc(n.alias)+'</option>').join('');if(cur&&monitor.some(n=>n.node_id===cur))sel.value=cur;if(!sel.dataset.loaded&&monitor.length){sel.dataset.loaded='1';loadHistory()}}
async function loadJobs(){if(!token())return;let r=await fetch('/api/jobs',{headers:ah()});if(!r.ok)return;let j=await r.json();q('#jobs').innerHTML=j.slice(0,20).map(x=>'<div>'+esc(x.id)+' · '+esc(x.type)+' · '+esc(x.node_id)+' · <b>'+esc(x.status)+'</b></div>').join('')||tnWizardText('No jobs','کاری ثبت نشده')}
async function loadAudit(){if(!token())return;let r=await fetch('/api/audit',{headers:ah()});if(!r.ok)return;let a=await r.json();q('#auditRows').innerHTML=a.slice().reverse().map(x=>'<tr><td>'+esc(x.sequence)+'</td><td>'+esc(x.timestamp)+'</td><td>'+esc(x.remote_ip)+'</td><td>'+esc(x.action)+'</td><td>'+esc(x.target||'—')+'</td><td>'+esc(x.outcome)+'</td><td class=muted>'+esc((x.hash||'').slice(0,16))+'…</td></tr>').join('')}
function micros(v){return (Number(v||0)/1000000).toFixed(4)}
async function loadFinance(){if(!token())return;let r=await fetch('/api/finance',{headers:ah()});if(!r.ok)return;let fs=await r.json();q('#financeRows').innerHTML=fs.map(x=>{let n=nodes.find(n=>n.id===x.node_id);let gib=(Number(x.ingress_bytes||0)+Number(x.egress_bytes||0))/1073741824;return '<tr><td>'+esc(n?.alias||x.node_id)+'</td><td>'+gib.toFixed(3)+'</td><td>'+micros(x.cost_micros)+'</td><td>'+micros(x.revenue_micros)+'</td><td>'+micros(x.profit_micros)+'</td></tr>'}).join('')}
async function loadHistory(){if(!token())return;let id=q('#historyNode').value;if(!id)return;let r=await fetch('/api/history?node_id='+encodeURIComponent(id),{headers:ah()});if(!r.ok)return;drawHistory(await r.json())}
function drawHistory(points){let c=q('#historyCanvas'),x=c.getContext('2d'),w=c.width,h=c.height;x.clearRect(0,0,w,h);x.strokeStyle='#263147';x.strokeRect(0,0,w,h);if(!points.length){q('#historyLegend').textContent='No history yet.';return}let vals=points.map(p=>Number(p.ingress_bytes||0)+Number(p.egress_bytes||0)),max=Math.max(1,...vals),step=w/Math.max(1,points.length-1);x.lineWidth=2;x.strokeStyle='#e8b54a';x.beginPath();points.forEach((p,i)=>{let y=h-20-(vals[i]/max)*(h-40);if(i===0)x.moveTo(0,y);else x.lineTo(i*step,y)});x.stroke();x.strokeStyle='#63d39a';x.beginPath();points.forEach((p,i)=>{let up=p.node_health==='up';let y=up?15:h-15;if(i===0)x.moveTo(0,y);else x.lineTo(i*step,y)});x.stroke();q('#historyLegend').textContent='Gold: cumulative traffic · Green: health-check uptime · '+points.length+' samples / last 7 days'}
async function saveNode(){let body={ID:q('#nid').value,Alias:q('#alias').value,Address:q('#addr').value,path_ipv4:q('#path4').value.trim(),path_ipv6:q('#path6').value.trim(),Role:q('#role').value,PublicKey:q('#pub').value,AgentTokenEnv:q('#agentEnv').value};let r=await fetch('/api/nodes',{method:'POST',headers:ah(),body:JSON.stringify(body)});if(!r.ok)alert(await r.text());else loadAll()}
q('#rows').addEventListener('change',e=>{if(e.target.matches('.pick'))q('#deploySelected').disabled=!document.querySelector('.pick:checked')});
async function deploy(){let ids=[...document.querySelectorAll('.pick:checked')].map(x=>x.value);if(!ids.length)return;let version=prompt('BAFT version');if(!version)return;let r=await fetch('/api/deploy',{method:'POST',headers:ah(),body:JSON.stringify({node_ids:ids,version})});alert(r.ok?'Deploy jobs queued':await r.text());loadJobs();loadAudit()}
async function revokeNode(node_id){let reason=prompt('Kill Switch reason','manual security revoke');if(reason===null)return;let r=await fetch('/api/nodes/revoke',{method:'POST',headers:ah(),body:JSON.stringify({node_id,reason})});alert(r.ok?'Node revoked immediately':await r.text());loadNodes();loadAudit()}
async function rotateToken(node_id){let agent_token_env=prompt('Environment variable containing the NEW agent token');if(!agent_token_env)return;let grace=prompt('Old-token grace seconds (0-900)','60');if(grace===null)return;let r=await fetch('/api/nodes/rotate-token',{method:'POST',headers:ah(),body:JSON.stringify({node_id,agent_token_env,grace_seconds:Number(grace)})});alert(r.ok?'Token rotation activated':await r.text());loadNodes();loadAudit()}
async function setFinance(){let node_id=prompt('Node ID');if(!node_id)return;let cost=prompt('Cost micros per GiB','0');let revenue=prompt('Revenue micros per GiB','0');let currency=prompt('Currency code','IRR')||'IRR';let effective_from=prompt('Effective from RFC3339 (blank = now)','')||'';if(cost===null||revenue===null)return;let r=await fetch('/api/finance',{method:'POST',headers:ah(),body:JSON.stringify({node_id,cost_micros_per_gib:Number(cost),revenue_micros_per_gib:Number(revenue),currency,effective_from})});alert(r.ok?'Versioned finance rate saved':await r.text());loadFinance();loadAudit()}
function reportURL(format){let p=q('#reportPeriod').value,f=q('#reportFrom').value,t=q('#reportTo').value,z=q('#reportTZ').value||'Asia/Tehran';return '/api/finance/report?period='+encodeURIComponent(p)+'&from='+encodeURIComponent(f)+'&to='+encodeURIComponent(t)+'&tz='+encodeURIComponent(z)+(format?'&format='+format:'')}
async function loadFinanceReport(){if(!token())return;let r=await fetch(reportURL(''),{headers:ah()});if(!r.ok){alert(await r.text());return}let data=await r.json();q('#reportRows').innerHTML=data.rows.map(x=>'<tr><td>'+esc(x.period)+'</td><td>'+esc(x.scope)+'</td><td>'+esc(x.node_id||'ALL')+'</td><td>'+esc(x.ingress_bytes)+'</td><td>'+esc(x.egress_bytes)+'</td><td>'+esc(x.cost_micros)+'</td><td>'+esc(x.revenue_micros)+'</td><td>'+esc(x.profit_micros)+'</td><td>'+esc(x.currency)+'</td></tr>').join('')}
async function downloadFinanceCSV(){if(!token())return;let r=await fetch(reportURL('csv'),{headers:ah()});if(!r.ok){alert(await r.text());return}let blob=await r.blob(),u=URL.createObjectURL(blob),a=document.createElement('a');a.href=u;a.download='baft-finance-'+q('#reportPeriod').value+'.csv';a.click();URL.revokeObjectURL(u)}
function initReportDates(){let d=new Date(),to=d.toISOString().slice(0,10),first=new Date(Date.UTC(d.getUTCFullYear(),d.getUTCMonth(),1)).toISOString().slice(0,10);q('#reportFrom').value=first;q('#reportTo').value=to}
let bsLast=null;
function bsT(en,fa){return document.documentElement.lang==='fa'?fa:en}
function bsAutoID(role,host){let s=host.toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,48);return (role==='foreign'?'ex-':'ir-')+(s||'server')}
async function bsRun(){let host=q('#bsHost').value.trim(),port=Number(q('#bsPort').value)||22,role=q('#bsRole').value;if(!host){q('#bsOut').textContent=bsT('Enter the server host or IP.','میزبان یا IP سرور را وارد کنید.');return}if(!q('#bsPass').value&&!q('#bsKey').value){q('#bsOut').textContent=bsT('Enter the SSH password, or paste an SSH key.','رمز SSH را وارد کنید یا کلید SSH را بچسبانید.');return}let id=q('#bsId').value.trim()||bsAutoID(role,host);q('#bsGo').disabled=true;q('#bsNext').hidden=true;bsLast=null;q('#bsFp').textContent='';q('#bsOut').textContent=bsT('Connecting to ','در حال اتصال به ')+host+'…';let r=await fetch('/api/bootstrap/hostkey',{method:'POST',headers:ah(),body:JSON.stringify({host,port})});let t=await r.text();if(!r.ok){q('#bsGo').disabled=false;q('#bsOut').textContent=bsT('FAILED','ناموفق')+'\n'+bsT('Cannot reach SSH on ','اتصال SSH برقرار نشد: ')+host+':'+port+'\n'+t;return}let d=JSON.parse(t);q('#bsFp').textContent=bsT('SSH key remembered: ','کلید SSH سرور ثبت شد: ')+d.key_type+'  '+d.fingerprint;q('#bsOut').textContent=bsT('Installing… this can take a few minutes.','در حال نصب… ممکن است چند دقیقه طول بکشد.');let body={node_id:id,alias:q('#bsAlias').value.trim()||host,role,host,port,user:q('#bsUser').value.trim(),host_key_fingerprint:d.fingerprint,password:q('#bsPass').value,private_key:q('#bsKey').value,passphrase:q('#bsPhrase').value};if(body.private_key)body.password='';r=await fetch('/api/bootstrap',{method:'POST',headers:ah(),body:JSON.stringify(body)});t=await r.text();body.password=body.private_key=body.passphrase='';q('#bsPass').value=q('#bsKey').value=q('#bsPhrase').value='';q('#bsGo').disabled=false;if(r.ok){bsLast={id,role};q('#bsNext').hidden=false;try{t=JSON.parse(t).output||bsT('Installed.','نصب شد.')}catch(e){}}q('#bsOut').textContent=(r.ok?bsT('OK','انجام شد'):bsT('FAILED','ناموفق'))+'\n'+t;loadAll()}
function bsTunnel(){if(!bsLast)return;tnFill();q(bsLast.role==='foreign'?'#tnEx':'#tnIr').value=bsLast.id;if(window.BAFT_UI)location.hash='#tunnels';else q('#tnEx').scrollIntoView({behavior:'smooth'})}
function tnJoinableOwners(){return (Array.isArray(tnWizardTunnels)?tnWizardTunnels:[]).filter(t=>t&&t.phase==='active'&&!t.join_owner_id&&t.id&&t.ex_node&&t.ir_node)}
function tnFill(){let opt=(role)=>nodes.filter(n=>role.includes(n.role)&&!n.revoked).map(n=>'<option value="'+esc(n.id)+'">'+esc(n.alias||n.id)+' ('+esc(n.id)+')</option>').join('');let ex=q('#tnEx'),ir=q('#tnIr'),a=ex.value,b=ir.value;ex.innerHTML='<option value="">EX node…</option>'+opt(['foreign']);ir.innerHTML='<option value="">IR node…</option>'+opt(['worker','master']);ex.value=a;ir.value=b;let owner=q('#tnJoinOwner'),selected=owner.value,owners=tnJoinableOwners();owner.innerHTML='<option value="">Choose an active EX tunnel… / تونل فعال EX را انتخاب کنید…</option>'+owners.map(t=>{let n=nodes.find(x=>x.id===t.ex_node);return '<option value="'+esc(t.id)+'">'+esc(t.id)+' · '+esc(n?.alias||t.ex_node)+':'+esc(t.port)+' · '+esc(t.ir_node)+'</option>'}).join('');if(owners.some(t=>t.id===selected))owner.value=selected}

// Universal Toolbox is backend-driven. This screen never installs unverified
// binaries, bypasses plan gates, or declares unsupported drivers operational.
const tlPersian={
 baft:'تونل امن بومی BAFT با طرح بررسی‌شده و مدیریت نمونه‌های مستقل',
 frp:'موتور ریورس‌پراکسی FRP؛ پس از پیاده‌سازی واقعی فعال می‌شود',
 rathole:'موتور معکوس Rathole؛ نیازمند بستهٔ امضاشده و آزمون دو سرور',
 backhaul:'موتور معکوس Backhaul؛ اتصال و بازیابی مستقل در برنامه است',
 wireguard:'تونل لایهٔ سه WireGuard؛ کنترل مسیر و فایروال هنوز آماده نیست',
 'ssh-socks':'خروجی SOCKS روی SSH؛ هنوز نصب و مدیریت خودکار ندارد',
 wss:'حامل WebSocket امن، نه موتور مستقل تونل',
 quic:'حامل QUIC بر بستر UDP، نه موتور مستقل تونل'
};
function tlPickEngine(id){
 const select=q('#tnEngine');
 if(id!=='baft'||!select)return;
 select.value=id;const direction=q('#tnDirection');if(direction)direction.value='ir-to-ex';tnClearPlan();tnWizardStep(0);
 const studio=document.querySelector('.aurora-tunnel-studio'),holder=studio?.closest?.('details');if(holder)holder.open=true;
 studio?.scrollIntoView({behavior:'smooth',block:'start'});
}
let tlCatalogRequestSeq=0;
async function loadTunnelEngines(){
 const requestSeq=++tlCatalogRequestSeq;
 const requestToken=token();
 const grid=q('#tlCatalog'),summary=q('#tlSummary');if(!grid||!summary)return;
 grid.replaceChildren(); // Fail closed while current capabilities are being refreshed.
 summary.textContent=tnWizardText('Checking supported drivers…','در حال بررسی موتورهای پشتیبانی‌شده…');
 if(!requestToken){grid.replaceChildren();summary.textContent=tnWizardText('Sign in to list trusted tunnel capabilities.','برای دیدن قابلیت‌های معتبر تونل وارد شوید.');return}
 try {
  const r=await fetch('/api/tunnel-engines',{method:'GET',headers:{Authorization:'Bearer '+requestToken,'Content-Type':'application/json'}});
  if(!r.ok)throw new Error('HTTP '+r.status);
  const caps=await r.json();
  // Only the newest request may mutate catalog UI, including after logout.
  if(requestSeq!==tlCatalogRequestSeq||token()!==requestToken)return;
  if(!Array.isArray(caps)||!caps.some(c=>c.name==='baft'&&c.deployable))throw new Error('Invalid capability catalog');
  const engines=caps.filter(c=>c.family!=='carrier');
  const deployable=engines.filter(c=>c.deployable&&c.name==='baft').length;
  const nextSummary=tnWizardText(deployable+' trusted engine ready for reviewed build • '+(engines.length-deployable)+' planned • '+(caps.length-engines.length)+' carriers not yet installed',deployable+' موتور معتبر برای ساخت واقعی • '+(engines.length-deployable)+' موتور در توسعه • '+(caps.length-engines.length)+' حامل هنوز نصب نشده');
  const nextCards=caps.map(c=>{
   const ready=c.deployable&&c.name==='baft',direction=c.direction==='ir_to_ex'?'IR → EX':c.direction==='ex_to_ir'?'EX → IR':tnWizardText('Variable','متغیر');
   const group=c.family==='carrier'?tnWizardText('Transport / carrier','حامل / ترنسپورت'):c.family==='reverse'?tnWizardText('Reverse engine','موتور معکوس'):tnWizardText('Direct engine','موتور مستقیم');
   const status=ready?tnWizardText('Plan available','قابل ساخت'):tnWizardText('Not installed / planned','در برنامه / نصب‌نشده');
   const action=ready?tnWizardText('Configure in Studio','ساخت در استودیو'):tnWizardText('Requires verified driver','نیازمند موتور واقعی');
   const req=Array.isArray(c.requirements)?c.requirements.map(x=>'<li>'+esc(x)+'</li>').join(''):'';
   return '<article class="tlTile"><div class="tlTileTop"><h3>'+esc(c.label||c.name)+'</h3><span class="tlState '+(ready?'on':'off')+'">'+esc(status)+'</span></div><div class="tlMeta">'+esc(group)+' · <span dir="ltr">'+esc(direction)+'</span></div><p>'+esc(document.documentElement.lang==='fa'?(tlPersian[c.name]||c.description):c.description)+'</p>'+(ready?'':'<details><summary class="tlMeta">'+esc(tnWizardText('Required before activation','پیش‌نیاز فعال‌سازی'))+'</summary><p>'+esc(c.reason||'')+'</p><ul>'+req+'</ul></details>')+'<button type="button" class="alt" '+(ready?'data-engine="'+esc(c.name)+'" onclick="tlPickEngine(this.dataset.engine)"':'disabled aria-disabled="true"')+'>'+esc(action)+'</button></article>';
  }).join('');
  if(requestSeq!==tlCatalogRequestSeq||token()!==requestToken)return;
  summary.textContent=nextSummary;
  grid.innerHTML=nextCards;
 }catch(e){if(requestSeq!==tlCatalogRequestSeq||token()!==requestToken)return;grid.replaceChildren();summary.textContent=tnWizardText('Engine status unavailable — no deployment is allowed from Toolbox.','وضعیت موتورهای تونل در دسترس نیست؛ ساخت از جعبه‌ابزار غیرفعال است.')}
}

// Native BAFT Smart Tunnel Wizard. It only submits fields supported by BCC's
// existing reviewed tunnel API; public client ingress is never silently made.
let tnWizardIndex=0,tnWizardTunnels=[];
function tnWizardText(en,fa){return document.documentElement.lang==='fa'?fa:en}
function tnWizardChanged(){tnIngressPreflightSeq++;tnIngressPreflightRefreshSeq++;tnIngressPreflightJobID='';tnIngressPreflightExpected=null;q('#tnIngressPreflightRefresh').hidden=true;tnIngressPreflightStatus(tnWizardText('Server selection changed. Repeat the read-only IR inspection on the selected server.','سرور انتخاب‌شده تغییر کرد؛ بررسی فقط‌خواندنی ورودی ایران را دوباره انجام دهید.'));if(q('#tnMode').value==='join'){q('#tnJoinConfirm').checked=false;tnWizardSuggest()}tnClearPlan()}
function tnTargetServicePort(value){const m=String(value||'').trim().match(/^(?:\[[^\]]+\]|[^:\s]+):([0-9]{1,5})$/);if(!m)return 0;const p=Number(m[1]);return Number.isInteger(p)&&p>=1&&p<=65535&&p!==22?p:0}
function tnWizardTargetChanged(){const hint=q('#tnServicePortHint');if(hint)hint.textContent='';if(!tnClientPortTouched){const port=tnTargetServicePort(q('#tnTarget').value),input=q('#tnClientPort'),next=port?String(port):'';if(input.value!==next){input.value=next;q('#tnIngressConfirm').checked=false;tnIngressPreflightSeq++;tnIngressPreflightRefreshSeq++;tnIngressPreflightJobID='';tnIngressPreflightExpected=null;q('#tnIngressPreflightRefresh').hidden=true;tnIngressPreflightStatus(tnWizardText('Destination changed; ingress preflight is invalidated.','مقصد تغییر کرد؛ بررسی قبلی ورودی دیگر معتبر نیست.'))}if(hint&&port)hint.textContent=tnWizardText('Client port defaulted to destination service port '+port+'. BAFT will stage ingress after tunnel activation (queued only when the IR agent reports operational-ingress-v1).','پورت کاربران به‌طور پیش‌فرض برابر پورت سرویس مقصد '+port+' شد؛ BAFT پس از فعال‌شدن تونل ورودی را آماده می‌کند (فقط با اعلام توانایی عامل در صف می‌رود).')}tnClearPlan()}
function tnWizardGenerateServicePort(){
 const input=q('#tnTarget'),hint=q('#tnServicePortHint'),ex=q('#tnEx').value;
 const tell=(en,fa)=>{if(hint)hint.textContent=tnWizardText(en,fa)};
 if(!ex){tell('Choose the foreign server first.','ابتدا سرور خارج را انتخاب کنید.');return}
 const raw=input.value.trim(),target=raw.match(/^(\[[0-9a-fA-F:]+\]|(?:\d{1,3}\.){3}\d{1,3}):(\d+)$/);
 if(raw&&!target){tell('Enter a fixed destination IP:port before suggesting a different port.','ابتدا مقصد را با IP ثابت و پورت وارد کنید تا پورت دیگری پیشنهاد شود.');return}
 const host=target?target[1]:'127.0.0.1',blocked=new Set([Number(q('#tnPort').value)]);
 const reserve=endpoint=>{const match=String(endpoint||'').match(/:(\d+)$/);if(match)blocked.add(Number(match[1]))};
 reserve(raw);reserve(q('#tnExMetric').value);
 for(const tunnel of tnWizardTunnels){
  if(!tnWizardActive(tunnel))continue;
  if(tunnel.ex_node===ex){blocked.add(Number(tunnel.port));reserve(tunnel.target);reserve(tunnel.ex_metrics_listen||'127.0.0.1:9191')}
  if(tunnel.ir_node===ex){reserve(tunnel.route_listen);reserve(tunnel.ir_metrics_listen||'127.0.0.1:9191')}
 }
 const bytes=new Uint32Array(1),span=40000,limit=0x100000000-0x100000000%span;
 for(let attempt=0;attempt<64;attempt++){
  try{crypto.getRandomValues(bytes)}catch(e){tell('Secure port suggestion unavailable; enter the destination manually.','پیشنهاد امن پورت در دسترس نیست؛ مقصد را دستی وارد کنید.');return}
  if(bytes[0]>=limit)continue;
  const candidate=20000+bytes[0]%span;
  if(blocked.has(candidate))continue;
  input.value=host+':'+candidate;tnWizardTargetChanged();
  tell('Suggested '+candidate+' — '+(tnClientPortTouched?'manual client port preserved; review any port mismatch.':'public client port defaulted to '+candidate+'.')+' Confirm the destination service listens at EX before testing traffic.','پورت پیشنهادی '+candidate+' — '+(tnClientPortTouched?'پورت دستی کاربران حفظ شد؛ اختلاف پورت‌ها را بررسی کنید.':'پورت عمومی کاربران نیز به '+candidate+' پیش‌فرض شد.')+' قبل از آزمون ترافیک، شنونده بودن سرویس مقصد در خارج را بررسی کنید.');
  return;
 }
 tell('Could not suggest a port; enter the destination manually.','پورت پیشنهادی پیدا نشد؛ مقصد را دستی وارد کنید.');
}
function tnWizardIngressChanged(){tnClientPortTouched=true;if(!q('#tnClientPort').value)q('#tnIngressConfirm').checked=false;tnIngressPreflightSeq++;tnIngressPreflightRefreshSeq++;tnIngressPreflightJobID='';tnIngressPreflightExpected=null;q('#tnIngressPreflightRefresh').hidden=true;tnIngressPreflightStatus(tnWizardText('Client port changed. Repeat read-only IR ingress inspection for the newly selected port.','پورت کانفیگ تغییر کرد؛ بررسی ورودی ایران را برای پورت جدید دوباره انجام دهید.'));tnClearPlan()}
function tnWizardActive(t){return !['superseded','decommissioned','rolled_back'].includes(t.phase)}
function tnWizardSharesLegacy(){const ex=q('#tnEx').value,ir=q('#tnIr').value;return tnWizardTunnels.some(t=>t.phase==='active'&&!t.instance_id&&(t.ex_node===ex||t.ir_node===ir))}
function tnWizardEnsureID(){if(q('#tnInstance').value.trim())return;const a=new Uint32Array(2);crypto.getRandomValues(a);q('#tnInstance').value='baft-'+[...a].map(v=>v.toString(36)).join('-').slice(0,29)}
function tnWizardSuggest(){
  if(tnEditBase){alert(tnWizardText('Editing keeps the existing instance and ports until you explicitly change them.','در حالت ویرایش، پورت‌ها و شناسه موجود تا زمان تغییر صریح حفظ می‌شوند.'));return}
  const joining=q('#tnMode').value==='join',owner=joining?tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value):null;
  if(joining&&!owner){alert(tnWizardText('Choose an active physical EX listener first.','ابتدا یک شنوندهٔ فیزیکی فعال EX انتخاب کنید.'));return}
  const ex=joining?owner.ex_node:q('#tnEx').value,ir=q('#tnIr').value,used=tnWizardTunnels.filter(t=>tnWizardActive(t));
  const next=(from,forbidden)=>{for(let p=from;p<65536;p++)if(!forbidden(p))return p;return null};
  if(joining){
   const route=next(1444,p=>used.some(t=>t.ir_node===ir&&t.route_listen==='127.0.0.1:'+p));
   if(route)q('#tnRouteListen').value='127.0.0.1:'+route;
   q('#tnPort').value=owner.port;q('#tnInstance').value=owner.instance_id||'';
   q('#tnExMetric').value=owner.ex_metrics_listen||'127.0.0.1:9191';
   if(owner.instance_id){const metric=next(9302,p=>used.some(t=>t.ir_node===ir&&(t.ir_metrics_listen||'127.0.0.1:9191')==='127.0.0.1:'+p));if(metric)q('#tnIrMetric').value='127.0.0.1:'+metric}
   tnClearPlan();return;
  }
  if(ex){let p=next(8443,p=>used.some(t=>t.ex_node===ex&&Number(t.port)===p));if(p)q('#tnPort').value=p}
  if(ir){let p=next(1444,p=>used.some(t=>t.ir_node===ir&&t.route_listen==='127.0.0.1:'+p));if(p)q('#tnRouteListen').value='127.0.0.1:'+p}
  if(ex){let p=next(9202,p=>used.some(t=>t.ex_node===ex&&(t.ex_metrics_listen||'127.0.0.1:9191')==='127.0.0.1:'+p));if(p)q('#tnExMetric').value='127.0.0.1:'+p}
  if(ir){let p=next(9302,p=>used.some(t=>t.ir_node===ir&&(t.ir_metrics_listen||'127.0.0.1:9191')==='127.0.0.1:'+p));if(p)q('#tnIrMetric').value='127.0.0.1:'+p}
  tnWizardEnsureID();tnClearPlan();
}
function tnWizardValidateStep(i){
  const mode=q('#tnMode').value,joining=mode==='join';
  if(i===0){
   if(q('#tnDirection').value!=='ir-to-ex'){alert(tnWizardText('Reverse topology is not yet supported by the native BAFT engine.','اتصال معکوس هنوز در موتور بومی BAFT پیاده نشده است.'));return false}
   if(mode==='replace'&&!tnEditBase){alert(tnWizardText('Choose Edit on an active tunnel in the list below.','برای ویرایش، ابتدا از فهرست پایین روی ویرایش تونل فعال بزنید.'));return false}
  }
  if(i===1){
   const owner=joining?tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value):null;
   // Re-derive the physical EX identity from the selected owner at validation
   // time; it must not depend on a possibly stale hidden EX select value.
   if(joining&&owner){q('#tnEx').value=owner.ex_node;q('#tnPort').value=String(owner.port);q('#tnInstance').value=owner.instance_id||'';q('#tnExMetric').value=owner.ex_metrics_listen||'127.0.0.1:9191'}
   if(!q('#tnEx').value||!q('#tnIr').value||q('#tnEx').value===q('#tnIr').value){alert(tnWizardText('Select different Iran and foreign servers.','سرور ایران و خارج را به‌درستی و جداگانه انتخاب کنید.'));return false}
   if(joining&&(!owner||owner.ex_node!==q('#tnEx').value)){alert(tnWizardText('Choose an active physical EX owner and a valid IR node. Same-IR joins use a separately scoped dialer.','مالک فیزیکی فعال EX و یک سرور IR معتبر انتخاب کنید؛ اتصال روی همان IR از نمونهٔ دیالر جدا استفاده می‌کند.'));return false}
  }
  if(i===2){
   const port=Number(q('#tnPort').value),owner=joining?tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value):null;
   if(!Number.isInteger(port)||port<1||port>65535){alert(tnWizardText('Enter a valid BAFT port (1–65535).','پورت معتبر BAFT را وارد کنید (۱ تا ۶۵۵۳۵).'));return false}
   if(joining&&(!owner||port!==Number(owner.port))){alert(tnWizardText('A Join must keep the selected EX listener port.','در Join باید پورت شنوندهٔ انتخاب‌شدهٔ EX حفظ شود.'));return false}
   const clientPortText=String(q('#tnClientPort').value||'').trim(),clientPort=Number(clientPortText);if(!/^[0-9]{1,5}$/.test(clientPortText)||!Number.isInteger(clientPort)||clientPort<1||clientPort>65535||clientPort===22){alert(tnWizardText('Select a valid client configuration port (1–65535 except reserved SSH port 22). The same port is the intended public IR ingress port.','یک پورت معتبر کانفیگ کاربران انتخاب کنید (۱ تا ۶۵۵۳۵ به‌جز پورت مدیریتی ۲۲). همین پورت باید ورودی عمومی ایران باشد.'));return false}
   if(!q('#tnIngressConfirm').checked){alert(tnWizardText('Confirm BAFT may provision public IR ingress on this port after tunnel activation; a verified Agent receipt is required.','تأیید کنید که BAFT پس از فعال‌شدن تونل ورودی عمومی را روی این پورت ایجاد می‌کند؛ رسید تأییدشده عامل لازم است.'));return false}
  }
  if(i===3){
   if(joining&&!q('#tnJoinConfirm').checked){alert(tnWizardText('Confirm that the new IR will share the selected EX listener.','تأیید کنید که IR جدید از شنوندهٔ انتخاب‌شدهٔ EX استفاده می‌کند.'));return false}
   if(mode==='independent'){
     tnWizardEnsureID();
     if(!/^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$/.test(q('#tnInstance').value.trim())){alert(tnWizardText('Enter a valid Instance ID in Advanced.','شناسه Instance در بخش پیشرفته معتبر نیست.'));return false}
     if(tnWizardSharesLegacy()&&!q('#tnLegacyCoexist').checked){alert(tnWizardText('Explicitly consent to keep the active legacy sibling tunnel.','حفظ تونل قدیمی فعال را صریحاً تأیید کنید.'));return false}
   }
  }
  return true
}
function tnWizardStep(i){
 if(i<0||i>4)return;
 if(i>tnWizardIndex){
  for(let j=tnWizardIndex;j<i;j++)if(!tnWizardValidateStep(j))return;
 }
 tnWizardIndex=i;
 for(const stage of document.querySelectorAll('.tnWizardStage'))stage.hidden=Number(stage.dataset.tnStep)!==i;
 for(const chip of q('#tnWizardSteps').children){chip.classList.toggle('selected',Number(chip.dataset.tnNav)===i)}
 q('#tnWizardBack').hidden=i===0;q('#tnWizardNext').hidden=i===4;
 if(i===4)tnWizardSummary();
 if(i===3){const adv=document.querySelector('.tnAdvanced');if(adv)adv.open=true}
}
function tnWizardSummary(){
  const r=tnReq(),c=q('#tnClientPort').value.trim(),joining=q('#tnMode').value==='join',legacy=!joining&&tnWizardSharesLegacy(),active=!!tnEditBase,destinationPort=tnTargetServicePort(r.target),portMismatch=!!c&&!!destinationPort&&Number(c)!==destinationPort,owner=joining?tnJoinableOwners().find(t=>t.id===r.join_existing_tunnel_id):null;
  const entry=c?tnWizardText('Client configuration port = IR public ingress port: '+esc(c)+' — BAFT stages ingress after activation, queued only when the IR agent reports operational-ingress-v1; authenticated receipt and independent check required','پورت کانفیگ کاربران = پورت ورودی عمومی ایران: '+esc(c)+' — پس از فعال‌شدن تونل آماده می‌شود و فقط با اعلام توانایی عامل در صف می‌رود؛ رسید احراز هویت‌شده و آزمون مستقل لازم است'):tnWizardText('Choose a destination with a valid port to default the IR client port','برای پیش‌فرض‌گذاری پورت کاربران ایران، مقصدی با پورت معتبر انتخاب کنید');
  q('#tnWizardSummary').innerHTML='<div><b>'+esc(tnWizardText('Driver','موتور'))+':</b> BAFT Secure (TLS + Noise) — '+esc(joining?tnWizardText('Join selected EX listener','اتصال به شنوندهٔ انتخاب‌شدهٔ EX'):active?tnWizardText('Edit selected instance','ویرایش نمونه انتخاب‌شده'):tnWizardText('New independent instance','نمونه مستقل جدید'))+'</div>'
   +(joining?'<div><b>'+esc(tnWizardText('Shared physical owner','مالک شنونده فیزیکی'))+':</b> '+esc(owner?.id||'')+' · '+esc(owner?.ex_node||r.ex_node)+'</div>':'')
   +'<div><b>'+esc(tnWizardText('Nodes','سرورها'))+':</b> '+esc(r.ir_node)+' → '+esc(r.ex_node)+'</div>'
   +'<div><b>'+esc(tnWizardText('Tunnel connection direction','جهت اتصال بین سرورها'))+':</b> <bdi dir="ltr">IR (dialer) → EX (listener)</bdi></div>'
   +'<div><b>'+esc(tnWizardText('BAFT outer EX listener','پورت ارتباط BAFT روی خارج'))+':</b> '+esc(r.port)+(joining?' · '+esc(tnWizardText('existing shared listener','شنوندهٔ مشترک فعلی')):'')+'</div>'
   +'<div><b>'+esc(tnWizardText('IR internal route','مسیر داخلی ایران'))+':</b> '+esc(r.route_listen||'(legacy default)')+'</div>'
   +'<div><b>'+esc(tnWizardText('EX service destination','مقصد سرویس روی خارج'))+':</b> <bdi dir="ltr">'+esc(r.target||tnWizardText('Not specified','تعیین نشده'))+'</bdi></div>'
   +'<div><b>'+esc(tnWizardText('User-facing IR entry','ورودی کاربران در ایران'))+':</b> '+entry+'</div>'
   +'<div><b>'+esc(tnWizardText('Existing EX tenants','مستأجران فعلی EX'))+':</b> '+esc(joining?tnWizardText('Owner and current tenants stay attached; only this IR is added','مالک و مستأجران فعلی حفظ می‌شوند؛ فقط این IR افزوده می‌شود'):legacy?tnWizardText('Present — explicit keep consent required','موجود — تأیید حفظ الزامی است'):tnWizardText('No active shared legacy detected in BCC','در BCC تونل قدیمی مشترک فعال شناسایی نشد'))+'</div>'
   +(portMismatch?'<div class="tnNotice" role="alert">'+esc(tnWizardText('Client port '+c+' differs from destination service port '+destinationPort+'. 3X-UI subscription links may need an external Custom Share Address/port rewrite; BAFT does not edit 3X-UI subscriptions.','پورت کاربران '+c+' با پورت سرویس مقصد '+destinationPort+' متفاوت است. ممکن است پیوند اشتراک 3X-UI به اصلاح بیرونی Custom Share Address/port نیاز داشته باشد؛ BAFT اشتراک‌های 3X-UI را ویرایش نمی‌کند.'))+'</div>':'')
   +'<div class="muted">'+esc(tnWizardText('Destination application and 3X-UI subscription settings remain separately managed.','تنظیمات برنامه مقصد و اشتراک‌های 3X-UI همچنان جداگانه مدیریت می‌شوند.'))+'</div>';
}
document.addEventListener('input',e=>{if(e.target.closest('.aurora-tunnel-studio')&&e.target.id!=='tnPlan')tnClearPlan()});

let tnClientPortTouched=false;
let tnPlanHash='',tnPlanReq=null,tnPlanReplaces='',tnPlanEpoch=0,tnDeployPending=false;
let tnAutoSelection=null;
let tnEditBase=null,tnEditSelectedID='';
function tnReq(){
 const ex=q('#tnEx').value,ir=q('#tnIr').value,port=Number(q('#tnPort').value)||8443,clientPort=Number(q('#tnClientPort').value)||0,target=q('#tnTarget').value.trim(),mode=q('#tnMode').value;
 if(mode==='join'){
  const owner=tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value);
  if(!owner)return {ex_node:ex,ir_node:ir,port,client_port:clientPort};
  const r={engine:'baft',join_existing_tunnel_id:owner.id,ex_node:owner.ex_node,ir_node:ir,public_address:owner.public_address||'',port:Number(owner.port),client_port:clientPort,target:target||owner.target||'',route_listen:q('#tnRouteListen').value.trim(),record_shaping:!!owner.record_shaping};
  if(owner.instance_id){r.instance_id=owner.instance_id;r.ex_metrics_listen=owner.ex_metrics_listen||'';r.ir_metrics_listen=q('#tnIrMetric').value.trim()}
  return r;
 }
 if(tnEditBase&&tnEditBase.ex_node===ex&&tnEditBase.ir_node===ir&&mode===(tnEditBase.instance_id?'independent':'replace')){let r=Object.assign({},tnEditBase,{port,client_port:clientPort,target:target||tnEditBase.target});if(r.instance_id){r.route_listen=q('#tnRouteListen').value.trim()||r.route_listen;r.ex_metrics_listen=q('#tnExMetric').value.trim()||r.ex_metrics_listen;r.ir_metrics_listen=q('#tnIrMetric').value.trim()||r.ir_metrics_listen;r.allow_legacy_coexistence=q('#tnLegacyCoexist').checked}return r}
 let r={ex_node:ex,ir_node:ir,port,client_port:clientPort};if(target)r.target=target;if(mode==='independent'){r.instance_id=q('#tnInstance').value.trim();r.route_listen=q('#tnRouteListen').value.trim();r.ex_metrics_listen=q('#tnExMetric').value.trim();r.ir_metrics_listen=q('#tnIrMetric').value.trim();r.allow_legacy_coexistence=q('#tnLegacyCoexist').checked}return r
}
function tnJoinOwnerChanged(){
 if(q('#tnMode').value!=='join')return;
 const owner=tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value),info=q('#tnJoinOwnerInfo');
 q('#tnJoinConfirm').checked=false;
 if(!owner){q('#tnEx').value='';q('#tnPort').value='8443';q('#tnInstance').value='';q('#tnExMetric').value='';q('#tnTarget').value='';info.textContent=tnWizardText('Choose an active physical owner tunnel.','تونل فعالِ مالک فیزیکی را انتخاب کنید.');tnModeChanged();return}
 q('#tnEx').value=owner.ex_node;q('#tnPort').value=owner.port;q('#tnInstance').value=owner.instance_id||'';q('#tnExMetric').value=owner.ex_metrics_listen||'127.0.0.1:9191';q('#tnTarget').value=owner.target||'';
 info.textContent=tnWizardText('Shared listener '+owner.id+' · '+owner.ex_node+':'+owner.port+' · current tenants remain attached.','شنونده مشترک '+owner.id+' · '+owner.ex_node+':'+owner.port+' · مستأجران فعلی حفظ می‌شوند.');
 tnModeChanged();tnWizardTargetChanged();tnWizardSuggest();
}
function tnModeChanged(){
 const mode=q('#tnMode').value,independent=mode==='independent',joining=mode==='join',owner=joining?tnJoinableOwners().find(t=>t.id===q('#tnJoinOwner').value):null;
 if(tnEditBase&&(joining||independent!==!!tnEditBase.instance_id)){tnEditBase=null;tnEditSelectedID=''}
 q('#tnScopedFields').hidden=!(independent||joining);
 q('#tnExField').hidden=joining;q('#tnJoinOwnerField').hidden=!joining;q('#tnJoinConfirmField').hidden=!joining;
 q('#tnPort').readOnly=joining;q('#tnInstance').readOnly=joining;q('#tnExMetric').readOnly=joining;
 q('#tnJoinInstanceField').hidden=joining&&!owner?.instance_id;q('#tnJoinExMetricField').hidden=joining&&!owner?.instance_id;q('#tnJoinIrMetricField').hidden=joining&&!owner?.instance_id;
 q('#tnLegacyCoexist').disabled=!independent;
 q('#tnEditNote').textContent=tnEditBase?'Editing an existing managed instance; always preview before deploy.':joining?(owner?'Only a new logical tenant will be added; the selected physical listener and current tenants remain.':'Choose an active physical EX owner, then select a new IR node.'):(independent?'Independent instance: existing tunnels stay online if resource checks pass.':'Use Edit on an active tunnel. Replacement will interrupt that instance.');
 q('#tnTransportModeNote').innerHTML=joining?'<span class="tn-fa">این عملیات IR جدید را به همان شنوندهٔ فعال EX و پورت آن اضافه می‌کند. مالک فیزیکی و مستأجران فعلی حفظ می‌شوند؛ فقط کلید Noise و route این تونل منطقی افزوده می‌شود. این قابلیت multiplex اشتراکی TLS نیست؛ پورت ۴۴۳ به‌تنهایی مقاومت در برابر فیلترینگ را تضمین نمی‌کند.</span><span class="tn-en">This adds the new IR to the selected active EX listener and its existing port. The physical owner and current tenants remain; only this tenant Noise key and route are added. This is not shared TLS multiplexing. Port 443 alone does not guarantee censorship resistance. Port 443 alone does not guarantee censorship resistance.</span>':'<span class="tn-fa">ارتباط دو سرور از BAFT Secure با TLS و Noise استفاده می‌کند. هر تونل مستقل شنونده اختصاصی خود را در سرور خارج دارد؛ پورت ۴۴۳ به‌تنهایی مقاومت در برابر فیلترینگ را تضمین نمی‌کند. پیوستن صریح، یک IR تازه را به شنونده فعال انتخاب‌شده اضافه می‌کند. موتورهای دیگر تا پیاده‌سازی و آزمون واقعی غیرفعال‌اند.</span><span class="tn-en">Server-to-server transport uses BAFT Secure with TLS and Noise. Each independent tunnel gets a dedicated EX listener per tunnel; shared TLS multiplexing is not enabled. Port 443 alone does not guarantee censorship resistance. Explicit Join adds a new IR peer to the selected active listener. Other engines remain unavailable until implemented and tested.</span>';
 q('#tnJoinConfirm').checked=false;tnClearPlan();
}
function tnNew(){tnClientPortTouched=false;tnEditBase=null;tnEditSelectedID='';q('#tnDirection').value='ir-to-ex';q('#tnEngine').value='baft';q('#tnMode').value='independent';q('#tnJoinOwner').value='';q('#tnJoinOwnerInfo').textContent='';q('#tnJoinConfirm').checked=false;q('#tnInstance').value='';q('#tnTarget').value='';q('#tnServicePortHint').textContent='';q('#tnClientPort').value='';q('#tnIngressConfirm').checked=false;q('#tnPort').value='8443';q('#tnRouteListen').value='127.0.0.1:1444';q('#tnExMetric').value='127.0.0.1:9202';q('#tnIrMetric').value='127.0.0.1:9302';q('#tnLegacyCoexist').checked=false;tnModeChanged();tnWizardStep(0)}
async function tnEdit(id){tnClearPlan();tnClientPortTouched=false;tnEditBase=null;tnEditSelectedID='';q('#tnEditNote').textContent='';const epoch=tnPlanEpoch;let r=await fetch('/api/tunnels?id='+encodeURIComponent(id),{headers:ah()});if(epoch!==tnPlanEpoch)return;if(!r.ok){alert(await r.text());return}let t=await r.json();if(epoch!==tnPlanEpoch)return;if(!t||t.id!==id||t.phase!=='active'){alert('Cannot edit: the selected tunnel is no longer active or details do not match. Refresh tunnels before retrying. / تونل انتخاب‌شده فعال نیست یا مشخصات آن مطابقت ندارد؛ فهرست را تازه‌سازی کنید.');return}tnEditSelectedID=id;tnEditBase={instance_id:t.instance_id||'',ex_node:t.ex_node,ir_node:t.ir_node,public_address:t.public_address,port:t.port,target:t.target,route_listen:t.route_listen,route_id:t.route_id,ex_metrics_listen:t.ex_metrics_listen||'',ir_metrics_listen:t.ir_metrics_listen||'',record_shaping:!!t.record_shaping};q('#tnDirection').value='ir-to-ex';q('#tnEngine').value='baft';q('#tnEx').value=t.ex_node;q('#tnIr').value=t.ir_node;q('#tnPort').value=t.port;q('#tnTarget').value=t.target;q('#tnClientPort').value=t.client_port||'';tnClientPortTouched=!!t.client_port;tnWizardTargetChanged();q('#tnIngressConfirm').checked=false;q('#tnMode').value=t.instance_id?'independent':'replace';q('#tnInstance').value=t.instance_id||'';q('#tnRouteListen').value=t.route_listen||'127.0.0.1:1444';q('#tnExMetric').value=t.ex_metrics_listen||'127.0.0.1:9202';q('#tnIrMetric').value=t.ir_metrics_listen||'127.0.0.1:9302';q('#tnLegacyCoexist').checked=false;tnModeChanged();q('#tnEditNote').innerHTML='Editing <b>'+esc(id)+'</b>: change the port or target, then preview the plan. '+esc(id)+' keeps running until the replacement is verified; if the replacement fails it rolls back and '+esc(id)+' stays active. <button class="alt" onclick="tnEditEnd()">Stop editing</button>';tnWizardStep(1);q('#tnIr').scrollIntoView({block:'center'})}
function tnEditEnd(){tnClearPlan();tnEditBase=null;tnEditSelectedID='';q('#tnEditNote').innerHTML='';q('#tnTarget').value=''}
function tnShowPlan(p){const unselectedReplacement=!!p.replaces&&(!tnEditBase||p.replaces!==tnEditSelectedID);const wrongSelectedEdit=!!p.ok&&!!tnEditBase&&p.replaces!==tnEditSelectedID;const canDeploy=!!p.ok&&!!p.hash&&!!p.request&&!unselectedReplacement&&!wrongSelectedEdit;tnPlanHash=canDeploy?p.hash:'';tnPlanReq=canDeploy?p.request:null;tnPlanReplaces=canDeploy?(p.replaces||''):'';let h='<div class="card" style="margin:0"><b>Deployment plan</b> <span class="badge '+(canDeploy?'up':'down')+'">'+(canDeploy?'ready':'blocked')+'</span><div class="muted" style="font-family:monospace;word-break:break-all;margin:6px 0">plan '+esc(p.hash)+'</div>';h+='<div class="tablewrap"><table><thead><tr><th>Gate</th><th>Status</th><th>Detail</th></tr></thead><tbody>'+p.gates.map(g=>'<tr><td>'+esc(g.name)+'</td><td><span class="badge '+(g.status==='PASS'?'up':g.status==='WARN'?'unknown':'down')+'">'+esc(g.status)+'</span></td><td>'+esc(g.detail)+'</td></tr>').join('')+'</tbody></table></div>';h+=(p.replaces?'<div style="margin-top:8px"><b>Replaces</b> '+esc(p.replaces)+' <span class="muted">(stays active until this plan is verified, then becomes superseded)</span></div>':'');h+=unselectedReplacement?'<div class="tnNotice" role="alert">This plan replaces an active tunnel that was not selected for editing. Choose Edit on that exact tunnel before deployment. / این طرح تونل فعالی را جایگزین می‌کند که برای ویرایش انتخاب نشده است. ابتدا همان تونل را ویرایش کنید.</div>':'';h+=wrongSelectedEdit?'<div class="tnNotice" role="alert">The selected tunnel is no longer the target of this plan. Refresh the list and choose Edit again. / تونل انتخاب‌شده دیگر مقصد جایگزینی این طرح نیست؛ فهرست را تازه‌سازی کرده و دوباره ویرایش را انتخاب کنید.</div>':'';h+=p.nodes.map(n=>'<div style="margin-top:10px"><b>'+esc(n.role.toUpperCase())+' '+esc(n.node_id)+'</b> <span class="muted">generation '+esc(n.generation_from)+' → '+esc(n.generation_to)+(n.generation_note?' ('+esc(n.generation_note)+')':'')+'</span><ul>'+n.changes.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div>').join('');h+='<div><b>Steps</b><ol>'+p.steps.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ol></div><div><b>Verified before it counts as deployed</b><ul>'+p.verify.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div><div><b>Rollback</b><ul>'+p.rollback.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div>';h+='<div class="actions"><button '+(canDeploy?'':'disabled ')+'onclick="tnDeploy()">Deploy this plan</button><button class="alt" onclick="tnClearPlan()">Discard</button></div></div>';q('#tnPlan').innerHTML=h}
function tnClearPlan(){tnPlanEpoch++;tnPlanHash='';tnPlanReq=null;tnPlanReplaces='';q('#tnPlan').innerHTML=''}
// Read-only: asks BCC to rank EX ports from fresh path probes and both agents'
// capability reports. Only fills the port field; the operator still reviews
// the plan. The native engine deploys HTTP/2, so only h2 is requested;
// IPv4 evidence is tried first, then IPv6.
async function tnAutoSelect(){const ex=q('#tnEx').value,ir=q('#tnIr').value,out=q('#tnAutoSelectNote');tnAutoSelection=null;if(!ex||!ir){out.textContent=tnWizardText('Choose both servers first.','ابتدا هر دو سرور را انتخاب کنید.');return}out.textContent=tnWizardText('Checking path evidence...','در حال بررسی شواهد مسیر...');let s=null;for(const family of ['4','6']){let r=await fetch('/api/tunnels/auto-select',{method:'POST',headers:ah(),body:JSON.stringify({ex_node:ex,ir_node:ir,family,carrier:'h2'})});if(!r.ok){out.textContent=await r.text();return}s=await r.json();if(s.selected)break}if(s.selected){const selected=s.selected;q('#tnPort').value=selected.port;tnClearPlan();tnAutoSelection=selected;out.textContent=tnWizardText('Selected port '+selected.port+' from fresh full-data path evidence ('+selected.connect_rtt_ms+' ms). The evidence will be revalidated during Review and Deploy.','پورت '+selected.port+' از شواهد تازه و کامل مسیر انتخاب شد ('+selected.connect_rtt_ms+' میلی‌ثانیه). شواهد هنگام بازبینی و اجرا دوباره اعتبارسنجی می‌شوند.');return}const why=(s.rejected||[]).slice(0,5).map(x=>x.port+' ('+x.carrier_id+'): '+x.reason).join('; ');out.textContent=tnWizardText('No port selected: ','پورتی انتخاب نشد: ')+(s.reason||'')+(why?' | '+why:'')}
async function tnPlan(){const autoSelection=tnAutoSelection;tnClearPlan();const epoch=tnPlanEpoch;for(let i=0;i<4;i++)if(!tnWizardValidateStep(i))return;let body=tnReq();if(!body.ex_node||!body.ir_node){alert('Choose both the EX and IR nodes');return}if(!tnEditBase&&q('#tnMode').value==='replace'){alert('To edit or replace an active tunnel, first click Edit on its row. / برای ویرایش، ابتدا روی ویرایش تونل فعال کلیک کنید.');return}if(q('#tnMode').value==='independent'&&!/^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$/.test(body.instance_id||'')){alert('Enter a unique valid Instance ID (letters, numbers, dot, underscore or hyphen). / شناسه نمونه مستقل الزامی است.');return}if(autoSelection&&autoSelection.source_node===body.ir_node&&autoSelection.destination_node===body.ex_node&&Number(autoSelection.port)===Number(body.port)){body.auto_select_evidence=autoSelection}let r=await fetch('/api/tunnels/plan',{method:'POST',headers:ah(),body:JSON.stringify(body)});let t=await r.text();if(epoch!==tnPlanEpoch)return;if(!r.ok){alert(t);return}tnShowPlan(JSON.parse(t));loadAudit()}
async function tnDeploy(){if(tnDeployPending||!tnPlanHash||!tnPlanReq)return;if(tnPlanReplaces&&!confirm('This plan replaces existing active tunnel '+tnPlanReplaces+'. Users may disconnect. Continue? / تونل فعال جایگزین می‌شود و ممکن است اتصال کاربران قطع شود. ادامه؟'))return;if(tnPlanReq.join_existing_tunnel_id&&!confirm('This adds IR '+tnPlanReq.ir_node+' to EX listener owner '+tnPlanReq.join_existing_tunnel_id+'. The existing listener and tenants stay attached. Continue? / این IR به شنونده EX انتخاب‌شده اضافه می‌شود و شنونده و مستأجران فعلی حفظ می‌شوند. ادامه؟'))return;if(tnPlanReq.allow_legacy_coexistence&&!confirm('Deploy independent BAFT instance alongside current legacy tunnel? Existing tunnel will not be replaced. Ensure ports are free. / تونل مستقل در کنار تونل فعلی ساخته شود؟'))return;tnDeployPending=true;const epoch=tnPlanEpoch;try{let body=Object.assign({},tnPlanReq,{plan_hash:tnPlanHash});let r=await fetch('/api/tunnels',{method:'POST',headers:ah(),body:JSON.stringify(body)});let t;try{t=await r.text()}catch(e){if(r.ok){if(epoch===tnPlanEpoch){tnClearPlan();tnEditEnd()}loadTunnels();loadAudit();return}throw e}if(epoch!==tnPlanEpoch){if(r.ok){loadTunnels();loadAudit()}return}if(r.status===409){try{let d=JSON.parse(t);alert(d.error);tnShowPlan(d.plan)}catch(e){alert(t)}return}if(!r.ok){alert(t);return}tnClearPlan();tnEditEnd();loadTunnels();loadAudit()}catch(e){if(epoch===tnPlanEpoch)tnClearPlan();loadTunnels();loadAudit();alert(tnWizardText('Deployment outcome unknown: refresh tunnel inventory before making a new reviewed plan. Do not retry the same request blindly.','نتیجه استقرار نامشخص است؛ قبل از ساخت طرح جدید فهرست تونل‌ها را بررسی کنید و درخواست را کورکورانه تکرار نکنید.'))}finally{tnDeployPending=false}}
let tnIngressPreflightJobID='',tnIngressPreflightSeq=0,tnIngressPreflightExpected=null,tnIngressPreflightSubmitPending=false,tnIngressPreflightRefreshSeq=0;
function tnIngressPreflightStatus(message){q('#tnIngressPreflightState').textContent=message}
async function tnIngressPreflight(id){
 if(!id)return;
 if(tnIngressPreflightSubmitPending){tnIngressPreflightStatus(tnWizardText('An IR inspection is already being submitted. Use Refresh after it is queued.','درخواست بررسی ایران در حال ارسال است. پس از ثبت، از تازه‌سازی استفاده کنید.'));return}
 const state=q('#tnIngressPreflightState');
 if(state&&state.scrollIntoView)state.scrollIntoView({block:'center'});
 const seq=++tnIngressPreflightSeq;
 ++tnIngressPreflightRefreshSeq;
 tnIngressPreflightJobID='';tnIngressPreflightExpected=null;
 q('#tnIngressPreflightRefresh').hidden=true;
 tnIngressPreflightStatus(tnWizardText('Loading active tunnel details…','در حال دریافت مشخصات تونل فعال…'));
 try{
  const r=await fetch('/api/tunnels?id='+encodeURIComponent(id),{headers:ah()});
  if(seq!==tnIngressPreflightSeq)return;
  if(!r.ok){tnIngressPreflightStatus(tnWizardText('Cannot inspect tunnel.','امکان بررسی تونل وجود ندارد.'));return}
  const tunnel=await r.json();
  if(seq!==tnIngressPreflightSeq)return;
  if(!tunnel||tunnel.id!==id||tunnel.phase!=='active'||!tunnel.ir_node||!tunnel.route_listen){
   tnIngressPreflightStatus(tnWizardText('Only an active tunnel with a known IR route may be inspected.','فقط تونل فعال با مسیر داخلی ایران قابل بررسی است.'));return
  }
  // The user-config port and IR public ingress port are one canonical value.
  // Never prompt for a second port; inspection cannot authorize provisioning.
  const input=String(q('#tnClientPort').value||'').trim();
  if(!input){tnIngressPreflightStatus(tnWizardText('Select a client configuration port in Wizard step 3 before inspecting IR ingress.','ابتدا پورت کانفیگ کاربران را در مرحله سوم ویزارد انتخاب کنید.'));return}
  const port=Number(input);
  if(!/^[0-9]{1,5}$/.test(input)||!Number.isInteger(port)||port<1||port>65535||port===22){
   tnIngressPreflightStatus(tnWizardText('Invalid port or reserved SSH port 22. Enter TCP 1–65535 except 22.','پورت نامعتبر یا پورت ۲۲ رزرو‌شده برای SSH است.'));return
  }
  const body={ir_node:tunnel.ir_node,tunnel_id:id,public_port:port,local_target:tunnel.route_listen};
  tnIngressPreflightStatus(tnWizardText('Submitting signed read-only inspection…','در حال ارسال بررسی امضاشده فقط‌خواندنی…'));
  tnIngressPreflightSubmitPending=true;
  const response=await fetch('/api/ingress/preflight',{method:'POST',headers:ah(),body:JSON.stringify(body)});
  const result=await response.text();
  if(seq!==tnIngressPreflightSeq)return;
  if(!response.ok){tnIngressPreflightStatus(tnWizardText('Preflight rejected: ','بررسی رد شد: ')+result);return}
  const accepted=JSON.parse(result);
  if(accepted.read_only!==true||accepted.ready_to_apply!==false||!accepted.job||!accepted.job.id||accepted.job.node_id!==body.ir_node){
   tnIngressPreflightStatus(tnWizardText('Untrusted response: check was not accepted as read-only.','پاسخ نامعتبر: فقط‌خواندنی بودن تأیید نشد.'));return
  }
  tnIngressPreflightJobID=accepted.job.id;tnIngressPreflightExpected=body;
  q('#tnIngressPreflightRefresh').hidden=false;
  tnIngressPreflightStatus(tnWizardText('Read-only inspection queued (','بررسی فقط‌خواندنی در صف (')+accepted.job.id+tnWizardText('). Refresh evidence after the IR agent responds; no port is configured.',' ). پس از پاسخ عامل ایران، شواهد را تازه‌سازی کنید؛ هیچ پورتی ایجاد نشده است.'));
 }catch(e){if(seq===tnIngressPreflightSeq)tnIngressPreflightStatus(tnWizardText('Preflight failed; no ingress has been configured.','بررسی ناموفق بود؛ ورودی عمومی تنظیم نشده است.'))}finally{tnIngressPreflightSubmitPending=false}
}
async function tnIngressPreflightRefresh(){
 const id=tnIngressPreflightJobID,seq=tnIngressPreflightSeq,refreshSeq=++tnIngressPreflightRefreshSeq,expected=tnIngressPreflightExpected;
 if(!id||!expected)return;
 tnIngressPreflightStatus(tnWizardText('Loading read-only receipt…','دریافت رسید فقط‌خواندنی…'));
 try{
  const r=await fetch('/api/ingress/preflight?id='+encodeURIComponent(id),{headers:ah()});
  const raw=await r.text();
  if(seq!==tnIngressPreflightSeq||refreshSeq!==tnIngressPreflightRefreshSeq||id!==tnIngressPreflightJobID)return;
  if(!r.ok){tnIngressPreflightStatus(tnWizardText('Read-only receipt unavailable.','رسید بررسی در دسترس نیست.'));return}
  const receipt=JSON.parse(raw);
  if(receipt.job_id!==id||receipt.node_id!==expected.ir_node||receipt.read_only!==true||receipt.ready_to_apply!==false){
   tnIngressPreflightStatus(tnWizardText('Untrusted ingress receipt; no permission to apply.','رسید ورودی نامعتبر است؛ مجوز اعمال وجود ندارد.'));return
  }
  if(receipt.status==='succeeded'){
   const o=receipt.evidence;
   if(!o||o.schema_version!==1||o.scope!=='agent_network_namespace_snapshot'||o.tunnel_id!==expected.tunnel_id||o.public_port!==expected.public_port||o.local_target!==expected.local_target||o.ready_to_apply!==false||o.ownership_verified!==false||o.firewall_policy_verified!==false||!Array.isArray(o.limitations)||!o.limitations.length){
    tnIngressPreflightStatus(tnWizardText('Incomplete ingress evidence; not authorized for apply.','شواهد ناقص است؛ اعمال مجاز نیست.'));return
   }
   tnIngressPreflightStatus(tnWizardText('Read-only observation: port ','مشاهده فقط‌خواندنی: پورت ')+String(o.public_port)+' '+String(o.public_port_state)+tnWizardText('; local route listener ','؛ شنونده مسیر داخلی ')+(o.target_listener_observed?'observed':'not observed')+tnWizardText('; limitations: ','؛ محدودیت‌ها: ')+o.limitations.join('; ')+tnWizardText('. NOT approved to apply or proof of public reachability.','. این نتیجه مجوز اعمال یا تأیید دسترسی عمومی نیست.'));
   return
  }
  if(receipt.status==='failed'){
   tnIngressPreflightStatus(tnWizardText('IR inspection failed. It did not provision public ingress.','بررسی عامل ایران شکست خورد و ورودی عمومی ساخته نشد.'));return
  }
  tnIngressPreflightStatus(tnWizardText('IR inspection ','بررسی ایران ')+String(receipt.status)+tnWizardText('; refresh after the agent responds; not ready to apply.','؛ پس از پاسخ عامل تازه‌سازی کنید؛ آماده اعمال نیست.'));
 }catch(e){if(seq===tnIngressPreflightSeq&&refreshSeq===tnIngressPreflightRefreshSeq&&id===tnIngressPreflightJobID)tnIngressPreflightStatus(tnWizardText('Could not read trusted ingress evidence.','دریافت شواهد معتبر ورودی ممکن نشد.'))}
}
let tnIngressOperationTunnelID='',tnIngressOperationPlanHash='',tnIngressOperationSeq=0,tnIngressOperationPending=false;
function tnIngressOperationStatus(message){q('#tnIngressOperationState').textContent=message}
function tnIngressOperationText(op){
 const st=String(op.state||'');
 if(st==='applied')return tnWizardText('Agent receipt recorded, but NOT Ready: use Verified ingress readiness for independent verification.','رسید عامل ثبت شد، اما آماده نیست: برای تأیید مستقل «آمادگی تأییدشده ورودی» را بزنید.');
 if(st==='held')return tnWizardText('Held: waiting for the IR agent to report operational-ingress-v1. Nothing was queued; not Ready.','در انتظار: منتظر اعلام توانایی عامل ایران هستیم. چیزی در صف نیست و آماده نیست.');
 if(st==='queued')return tnWizardText('Queued for the IR agent as ','در صف عامل ایران: ')+String(op.job_id||'')+tnWizardText('. Waiting for the authenticated receipt; not Ready.','؛ در انتظار رسید احراز هویت‌شده؛ آماده نیست.');
 if(st==='retiring')return tnWizardText('Retirement queued; waiting for the authenticated retirement receipt.','حذف در صف است؛ در انتظار رسید احراز هویت‌شده حذف.');
 if(st==='retired')return tnWizardText('Retire receipt recorded. Absence of the listener is not independently verified.','رسید حذف ثبت شد؛ نبودن شنونده به‌طور مستقل تأیید نشده است.');
 if(st==='blocked')return tnWizardText('Blocked: signed window passed; operator review required. No success is claimed.','مسدود: بازهٔ امضا گذشته؛ نیازمند بررسی اپراتور. موفقیتی اعلام نمی‌شود.');
 if(st==='failed')return tnWizardText('Agent reported failure or BLOCKED. No success is claimed; reconcile the node before retrying.','عامل شکست یا BLOCKED گزارش کرد. موفقیتی اعلام نمی‌شود؛ پیش از تلاش دوباره سرور را تطبیق دهید.');
 return tnWizardText('Operation ','عملیات ')+st+tnWizardText('; not Ready.','؛ آماده نیست.');
}
function tnIngressOperationValid(op,id,hash){return !!op&&typeof op==='object'&&op.tunnel_id===id&&op.plan_hash===hash&&op.ready_to_apply===false&&typeof op.state==='string'}
async function tnIngressOperationCheck(id){
 if(!id||tnIngressOperationPending)return;
 const seq=++tnIngressOperationSeq;tnIngressOperationTunnelID=id;q('#tnIngressOperationRetire').hidden=true;q('#tnIngressOperationRefresh').hidden=true;
 tnIngressOperationStatus(tnWizardText('Checking active tunnel and operation…','در حال بررسی تونل فعال و عملیات…'));
 try{
  const tr=await fetch('/api/tunnels?id='+encodeURIComponent(id),{headers:ah()});if(seq!==tnIngressOperationSeq)return;
  if(!tr.ok){tnIngressOperationStatus(tnWizardText('Tunnel details unavailable.','مشخصات تونل در دسترس نیست.'));return}
  const tunnel=await tr.json();if(seq!==tnIngressOperationSeq)return;
  if(!tunnel||tunnel.id!==id||tunnel.phase!=='active'||!tunnel.plan_hash||!Number.isInteger(tunnel.client_port)){tnIngressOperationStatus(tnWizardText('Only an active tunnel with a reviewed client port can operate ingress.','فقط تونل فعال با پورت بررسی‌شدهٔ کاربران می‌تواند ورودی را مدیریت کند.'));return}
  tnIngressOperationPlanHash=tunnel.plan_hash;
  const r=await fetch('/api/ingress/deploy?tunnel_id='+encodeURIComponent(id),{headers:ah()});const raw=await r.text();if(seq!==tnIngressOperationSeq)return;
  if(r.status===404){tnIngressOperationStatus(tnWizardText('No ingress operation is staged yet. Deploy will stage this exact active tunnel plan.','هنوز عملیات ورودی آماده نشده است. ساخت، همین طرح دقیق تونل فعال را آماده می‌کند.'));return}
  if(!r.ok){tnIngressOperationStatus(tnWizardText('Ingress operation status unavailable: ','وضعیت عملیات ورودی در دسترس نیست: ')+raw.slice(0,200));return}
  const op=JSON.parse(raw);
  if(!tnIngressOperationValid(op,id,tunnel.plan_hash)||op.public_port!==tunnel.client_port){tnIngressOperationStatus(tnWizardText('Operation evidence does not match the current tunnel plan; no readiness claimed.','شواهد عملیات با طرح فعلی تونل تطبیق ندارد؛ آمادگی تأیید نشده است.'));return}
  q('#tnIngressOperationRefresh').hidden=false;
  q('#tnIngressOperationRetire').hidden=!(op.state==='applied'&&op.receipt_verified===true&&op.receipt_phase==='applied'&&op.action==='public_ingress_apply');
  tnIngressOperationStatus(tnIngressOperationText(op));
 }catch(e){if(seq===tnIngressOperationSeq)tnIngressOperationStatus(tnWizardText('Could not verify ingress operation state.','تأیید وضعیت عملیات ورودی ممکن نشد.'))}
}
async function tnIngressDeploy(id){
 if(!id||tnIngressOperationPending)return;
 const seq=++tnIngressOperationSeq;tnIngressOperationTunnelID=id;q('#tnIngressOperationRetire').hidden=true;
 tnIngressOperationPending=true;
 try{
  const tr=await fetch('/api/tunnels?id='+encodeURIComponent(id),{headers:ah()});if(seq!==tnIngressOperationSeq)return;
  if(!tr.ok){tnIngressOperationStatus(tnWizardText('Tunnel details unavailable.','مشخصات تونل در دسترس نیست.'));return}
  const tunnel=await tr.json();if(seq!==tnIngressOperationSeq)return;
  if(!tunnel||tunnel.id!==id||tunnel.phase!=='active'||!tunnel.plan_hash||!Number.isInteger(tunnel.client_port)||tunnel.client_port<1||tunnel.client_port>65535||tunnel.client_port===22||!tunnel.route_listen){tnIngressOperationStatus(tnWizardText('Ingress deploy requires an active tunnel with its reviewed client port and IR route.','ساخت ورودی به تونل فعال، پورت بررسی‌شده و مسیر ایران نیاز دارد.'));return}
  if(!confirm(tnWizardText('Stage the exact reviewed plan for public IR ingress on port ','طرح بررسی‌شده برای ورودی عمومی ایران روی پورت ')+tunnel.client_port+tnWizardText('? It is queued only if the IR agent reports operational-ingress-v1, and is not successful until the authenticated receipt and independent verification.',' آماده شود؟ فقط در صورت اعلام توانایی عامل ایران در صف می‌رود و تا رسید احراز هویت‌شده و تأیید مستقل موفق نیست.')))return;
  tnIngressOperationPlanHash=tunnel.plan_hash;tnIngressOperationStatus(tnWizardText('Submitting reviewed public ingress operation…','در حال ارسال عملیات ورودی بررسی‌شده…'));
  const r=await fetch('/api/ingress/deploy',{method:'POST',headers:ah(),body:JSON.stringify({tunnel_id:id,plan_hash:tunnel.plan_hash})});const raw=await r.text();if(seq!==tnIngressOperationSeq)return;
  if(!r.ok){tnIngressOperationStatus(tnWizardText('Deploy refused: ','ساخت رد شد: ')+raw.slice(0,200));return}
  const result=JSON.parse(raw);if(!tnIngressOperationValid(result,id,tunnel.plan_hash)||!result.operation_id||!result.job_id){tnIngressOperationStatus(tnWizardText('Untrusted operation response; readiness not accepted.','پاسخ عملیات نامعتبر است؛ آمادگی پذیرفته نشد.'));return}
  q('#tnIngressOperationRefresh').hidden=false;tnIngressOperationStatus(tnIngressOperationText(result));
 }catch(e){if(seq===tnIngressOperationSeq)tnIngressOperationStatus(tnWizardText('Deploy outcome unknown; refresh status before retrying.','نتیجه ساخت نامشخص است؛ پیش از تلاش دوباره وضعیت را تازه‌سازی کنید.'))}finally{tnIngressOperationPending=false}
}
async function tnIngressOperationRefresh(){const id=tnIngressOperationTunnelID;if(!id)return;await tnIngressOperationCheck(id)}
async function tnIngressRetire(){
 const id=tnIngressOperationTunnelID,hash=tnIngressOperationPlanHash;if(!id||!hash||tnIngressOperationPending)return;
 if(!confirm(tnWizardText('Retire only the exact ingress operation with its applied receipt?','فقط عملیات ورودی دقیقی که رسید اعمال دارد حذف شود؟')))return;
 tnIngressOperationPending=true;const seq=++tnIngressOperationSeq;q('#tnIngressOperationRetire').hidden=true;
 try{const r=await fetch('/api/ingress/retire',{method:'POST',headers:ah(),body:JSON.stringify({tunnel_id:id,plan_hash:hash})});const raw=await r.text();if(seq!==tnIngressOperationSeq)return;if(!r.ok){tnIngressOperationStatus(tnWizardText('Retire refused: ','حذف رد شد: ')+raw.slice(0,200));return}const result=JSON.parse(raw);if(!tnIngressOperationValid(result,id,hash)||result.action!=='public_ingress_retire'){tnIngressOperationStatus(tnWizardText('Untrusted retirement response; readiness not accepted.','پاسخ حذف نامعتبر است؛ آمادگی پذیرفته نشد.'));return}q('#tnIngressOperationRefresh').hidden=false;tnIngressOperationStatus(tnIngressOperationText(result))}catch(e){if(seq===tnIngressOperationSeq)tnIngressOperationStatus(tnWizardText('Retire outcome unknown; refresh status before retrying.','نتیجه حذف نامشخص است؛ پیش از تلاش دوباره وضعیت را تازه‌سازی کنید.'))}finally{tnIngressOperationPending=false}
}
let tnIngressReadinessSeq=0;
function tnIngressReadinessStatus(message){q('#tnIngressReadinessState').textContent=message}
function tnIngressReadinessCardValid(c,id){
 const port=n=>Number.isInteger(n)&&n>=1&&n<=65535;
 return !!c&&typeof c==='object'&&c.tunnel_id===id&&typeof c.ir_node==='string'&&c.ir_node.length>0&&port(c.config_port)&&port(c.public_port)&&typeof c.plan_sha256==='string'&&/^[0-9a-f]{64}$/.test(c.plan_sha256)
}
async function tnIngressReadiness(id){
 const seq=++tnIngressReadinessSeq,box=q('#tnIngressReadinessCard');
 box.textContent='';
 tnIngressReadinessStatus(tnWizardText('Checking verified ingress readiness (BCC probes the public port itself)…','در حال بررسی آمادگی تأییدشده ورودی (مرکز فرمان خودش پورت عمومی را آزمایش می‌کند)…'));
 try{
  const r=await fetch('/api/ingress/readiness?tunnel_id='+encodeURIComponent(id),{headers:ah()});
  const raw=await r.text();
  if(seq!==tnIngressReadinessSeq)return;
  if(!r.ok){tnIngressReadinessStatus(tnWizardText('Readiness unavailable.','وضعیت آمادگی در دسترس نیست.'));return}
  const v=JSON.parse(raw);
  if(v&&v.ready===true&&v.verified===true&&tnIngressReadinessCardValid(v.card,id)){
   const c=v.card;
   tnIngressReadinessStatus(tnWizardText('Verified: BCC reached the public port and completed a payload round trip. Use this client configuration.','تأیید شد: مرکز فرمان به پورت عمومی رسید و تبادل داده را کامل کرد. این پیکربندی کاربران را به‌کار ببرید.'));
   box.textContent=[tnWizardText('Tunnel','تونل')+': '+c.tunnel_id,tnWizardText('IR server','سرور ایران')+': '+c.ir_node,tnWizardText('Client config port','پورت کانفیگ کاربران')+': '+c.config_port,tnWizardText('Public port','پورت عمومی')+': '+c.public_port,'plan sha256: '+c.plan_sha256].join('\n');
   return
  }
  const reason=v&&typeof v.reason==='string'?v.reason.slice(0,200):'';
  tnIngressReadinessStatus(tnWizardText('NOT ready: no client configuration is shown until independent verification succeeds.','آماده نیست: تا پیش از تأیید مستقل هیچ پیکربندی کاربری نمایش داده نمی‌شود.')+(reason?' ('+reason+')':''));
 }catch(e){if(seq===tnIngressReadinessSeq)tnIngressReadinessStatus(tnWizardText('Could not read ingress readiness.','خواندن وضعیت آمادگی ورودی ممکن نشد.'))}
}
async function tnRotate(id){if(!confirm('Rotate the TLS certificate of tunnel '+id+'? The IR trusts old + new until the new certificate is confirmed; any failure before that rolls back.'))return;let r=await fetch('/api/tunnels/rotate-cert',{method:'POST',headers:ah(),body:JSON.stringify({tunnel_id:id})});let t=await r.text();if(!r.ok){alert(t);return}try{alert('rotation '+JSON.parse(t).id+' started')}catch(e){}loadAudit()}
async function tnCancel(id){let reason=prompt('Cancel and roll back tunnel '+id+'. Reason (optional)','');if(reason===null)return;let r=await fetch('/api/tunnels/cancel',{method:'POST',headers:ah(),body:JSON.stringify({id,reason})});if(!r.ok)alert(await r.text());loadTunnels();loadAudit()}
async function tnDecommission(id){let pr=await fetch('/api/tunnels/decommission/plan',{method:'POST',headers:ah(),body:JSON.stringify({id})});let txt=await pr.text();if(!pr.ok){alert(txt);return}let p=JSON.parse(txt);if(!p.ok){alert('Decommission is blocked:\n'+p.gates.filter(g=>g.status==='FAIL').map(g=>g.name+': '+g.detail).join('\n'));loadAudit();return}let msg=p.warning+'\n\nOrder: IR route first, then EX listener.\nPlan: '+p.hash+'\n\nContinue?';if(!confirm(msg))return;let r=await fetch('/api/tunnels/decommission',{method:'POST',headers:ah(),body:JSON.stringify({id,plan_hash:p.hash})});let out=await r.text();if(r.status===409){try{let d=JSON.parse(out);alert(d.error+'\nReview a fresh decommission plan before retrying.')}catch(e){alert(out)}return}if(!r.ok){alert(out);return}loadTunnels();loadAudit()}
async function tnDrift(id){let r=await fetch('/api/tunnels/drift?id='+encodeURIComponent(id),{method:'POST',headers:ah()});if(!r.ok)alert(await r.text());loadTunnels();loadAudit()}
function ppFill(){let options='<option value="">node…</option>'+nodes.filter(n=>!n.revoked).map(n=>'<option value="'+esc(n.id)+'">'+esc(n.alias||n.id)+'</option>').join('');for(let id of ['ppSrc','ppDst']){let e=q('#'+id);if(!e)continue;let old=e.value;e.innerHTML=options;if(nodes.some(n=>n.id===old))e.value=old}}
function ppCls(c){return c==='FULL_DATA'?'up':(['BYTE_CEILING','CONNECT_ONLY','HANDSHAKE_ONLY','TIMEOUT_UNREACHABLE','LOCAL_CONFLICT'].includes(c)?'down':'unknown')}
async function ppRun(){let ports=q('#ppPorts').value.split(',').map(x=>Number(x.trim())).filter(x=>Number.isInteger(x)&&x>0&&x<65536);let body={source_node:q('#ppSrc').value,destination_node:q('#ppDst').value,family:q('#ppFamily').value,candidate_ports:[...new Set(ports)]};if(!body.source_node||!body.destination_node||body.source_node===body.destination_node){alert('Choose two different enrolled nodes');return}let r=await fetch('/api/path-probes',{method:'POST',headers:ah(),body:JSON.stringify(body)});let t=await r.text();if(!r.ok){alert(t);return}loadPathProbes();loadAudit()}
async function pdRun(){let r=await fetch('/api/path-discovery',{method:'POST',headers:ah(),body:JSON.stringify({candidate_ports:[]})});let t=await r.text();if(!r.ok&&!t.includes('already active'))alert(t);loadPathGraph();loadPathMatrix();loadPathProbes();loadAudit()}
async function loadPathGraph(){if(!token()||!q('#pgRows'))return;let [gr,dr]=await Promise.all([fetch('/api/path-graph',{headers:ah()}),fetch('/api/path-discovery',{headers:ah()})]);if(gr.ok){let g=await gr.json();q('#pgRows').innerHTML=(g.recommendations||[]).map(r=>{let hops=(r.hops||[]).map(h=>esc(h.source_node)+' → '+esc(h.destination_node)+' / IPv'+esc(h.family)+' :'+esc(h.port)).join(' · ');return '<tr><td>'+esc(r.source_node)+' → '+esc(r.destination_node)+'</td><td>'+hops+'</td><td>'+esc((r.hops||[]).length)+(r.direct?' · direct':' · multi-hop')+'</td><td>'+esc(r.total_rtt_ms||0)+' ms</td></tr>'}).join('')||'<tr><td colspan=4 class=muted>No FULL_DATA route evidence yet.</td></tr>'}if(dr.ok){let ds=await dr.json(),d=ds[0];q('#pdState').textContent=d?('Latest discovery: '+d.phase+' · '+(d.probe_ids||[]).length+' path candidates · '+ago(d.updated_at)):'Automatic discovery will start when at least two enrolled nodes are available.'}}
function pmCls(st){return st==='PASS'?'up':(st==='FAIL'||st==='DEGRADED'?'down':'unknown')}
function pmCell(c){if(!c)return '<span class="badge unknown">NOT_ASSESSED</span>';let ev=(c.candidates||[]).map(x=>'IPv'+esc(x.family)+' :'+esc(x.port)+' · '+esc(x.class)+(x.connect_rtt_ms?' · '+esc(x.connect_rtt_ms)+' ms':'')+(x.byte_ceiling?' · ceiling '+esc(x.byte_ceiling)+' B':'')+(x.detail?' · '+esc(x.detail):'')+' · '+esc(x.age_seconds)+'s old').join(' | ');return '<details><summary><span class="badge '+pmCls(c.state)+'">'+esc(c.state)+'</span></summary><div class=muted>'+esc(ev||'No completed evidence')+'</div></details>'}
async function loadPathMatrix(){if(!token()||!q('#pmMatrix'))return;let r=await fetch('/api/path-matrix',{headers:ah()});if(!r.ok)return;let m=await r.json(),by={};(m.cells||[]).forEach(c=>by[c.source_node+'\0'+c.destination_node]=c);let ns=m.nodes||[];let h='<table><thead><tr><th>From \ To</th>'+ns.map(n=>'<th>'+esc(n)+'</th>').join('')+'</tr></thead><tbody>';h+=ns.map(src=>'<tr><th>'+esc(src)+'</th>'+ns.map(dst=>src===dst?'<td class=muted>—</td>':'<td>'+pmCell(by[src+'\0'+dst])+'</td>').join('')+'</tr>').join('');q('#pmMatrix').innerHTML=h+'</tbody></table>'}
async function loadPathProbes(){if(!token()||!q('#ppRows'))return;let r=await fetch('/api/path-probes',{headers:ah()});if(!r.ok)return;let ps=await r.json();q('#ppRows').innerHTML=ps.map(p=>{let ev=(p.candidates||[]).map(c=>'<div><span class="badge '+ppCls(c.class)+'">'+esc(c.class||'PENDING')+'</span> '+esc(c.port)+(c.byte_ceiling?' · ceiling '+esc(c.byte_ceiling)+' B':'')+(c.connect_rtt_ms?' · '+esc(c.connect_rtt_ms)+' ms':'')+'<div class=muted>'+esc(c.detail||'')+'</div></div>').join('');return '<tr><td>'+esc(p.source_node)+' → '+esc(p.destination_node)+'</td><td>IPv'+esc(p.family)+'</td><td><span class="badge '+(p.phase==='complete'?'up':p.phase==='failed'?'down':'unknown')+'">'+esc(p.phase)+'</span></td><td>'+(p.recommended_port?'<b>'+esc(p.recommended_port)+'</b>':'—')+'</td><td>'+ev+'</td></tr>'}).join('')||'<tr><td colspan=5 class=muted>No path probes yet.</td></tr>'}
async function chLoad(){let qv=[];if(q('#chNode').value)qv.push('node_id='+encodeURIComponent(q('#chNode').value));if(q('#chTunnel').value)qv.push('tunnel_id='+encodeURIComponent(q('#chTunnel').value));let r=await fetch('/api/change-ledger?'+qv.join('&'),{headers:ah()});if(!r.ok)return;let xs=await r.json();q('#chRows').innerHTML=xs.slice().reverse().map(x=>'<details><summary><b>'+esc(x.id)+'</b> · '+esc(x.node_id)+' · '+esc(x.outcome)+' · gen '+esc(x.before.generation||0)+' → '+esc(x.after.generation||0)+'</summary><div class="muted">tunnel '+esc(x.tunnel_id||'—')+' · route '+esc(x.route_id||'—')+' · plan '+esc(x.plan_hash||'—')+' · digest '+esc(x.digest)+'</div><table><thead><tr><th>Field / فیلد</th><th>Before / قبل</th><th>After / بعد</th></tr></thead><tbody>'+(x.diff||[]).map(d=>'<tr><td>'+esc(d.field)+'</td><td>'+esc(d.before)+'</td><td>'+esc(d.after)+'</td></tr>').join('')+'</tbody></table></details>').join('')||'<div class="muted">No managed changes recorded / تغییری ثبت نشده</div>'}
let smPlanObj=null,smRun=null;
function smReq(){return {node_id:q('#smNode').value,old_port:+q('#smOld').value,new_port:+q('#smNew').value,user:q('#smUser').value,fingerprint:q('#smFP').value}}
async function smCall(body){try{let r=await fetch('/api/ssh-migration',{method:'POST',headers:ah(),body:JSON.stringify(body)});if(!r.ok){q('#smState').textContent=document.documentElement.lang==='fa'?'درخواست مهاجرت ناموفق بود؛ سرور و پورت‌ها را بررسی کنید.':'Migration request failed; check node and ports.';return null}return await r.json()}catch(e){q('#smState').textContent=document.documentElement.lang==='fa'?'درخواست مهاجرت ناموفق بود؛ دوباره تلاش کنید.':'Migration request failed; try again.';return null}}
async function smPlan(){smPlanObj=null;let plan=await smCall({action:'plan',request:smReq()});if(!plan)return;smPlanObj=plan;q('#smState').textContent='PLAN '+plan.plan_hash+' · '+plan.old_port+' → '+plan.new_port}
async function smStart(){if(!smPlanObj){await smPlan();if(!smPlanObj)return}const run=await smCall({action:'start',request:smReq(),plan_hash:smPlanObj.plan_hash});if(!run)return;smRun=run;q('#smState').textContent=run.phase+' · '+run.id}
async function smVerify(){if(!smRun){alert('Start migration first');return}const run=await smCall({action:'verify',id:smRun.id,password:q('#smPassword').value,private_key:q('#smKey').value});q('#smPassword').value='';q('#smKey').value='';if(!run)return;smRun=run;q('#smState').textContent=run.phase+' · '+run.id;loadAudit()}
function rdFill(ts){let e=q('#rdTunnel');if(!e)return;let old=e.value;e.innerHTML='<option value="">active tunnel…</option>'+ts.filter(t=>t.phase==='active').map(t=>'<option value="'+esc(t.id)+'">'+esc(t.id)+' · '+esc(t.ir_node)+' → '+esc(t.ex_node)+'</option>').join('');if(ts.some(t=>t.id===old&&t.phase==='active'))e.value=old}
function rdRender(r){let stages=(r.stages||[]).map(s=>'<div><span class="badge '+(s.state==='PASS'?'up':s.state==='FAIL'?'down':'unknown')+'">'+esc(s.state)+'</span> <b>'+esc(s.stage)+'</b> <span class=muted>'+esc(s.detail||'')+(s.reference?' · '+esc(s.reference):'')+'</span></div>').join('');q('#rdResult').innerHTML='<div><b>'+esc(r.summary_en||'')+'</b><div>'+esc(r.summary_fa||'')+'</div><div class=muted>Evidence '+esc(r.evidence_digest||'')+' · '+esc(r.id||'')+'</div></div>'+stages}
async function rdRun(){let id=q('#rdTunnel').value;if(!id){alert('Choose an active tunnel');return}let r=await fetch('/api/route-doctor',{method:'POST',headers:ah(),body:JSON.stringify({tunnel_id:id})});let t=await r.text();if(!r.ok){alert(t);return}rdRender(JSON.parse(t));loadAudit()}
function dcCls(st){return st==='MANAGED'?'up':(st==='DRIFTED'||st==='MISSING'||st==='OWNERSHIP_CONFLICT'?'down':'unknown')}
async function dcStart(){let r=await fetch('/api/discovery?all=1',{method:'POST',headers:ah()});if(!r.ok)alert(await r.text());loadDiscovery();loadAudit()}
async function loadDiscovery(){if(!token())return;let r=await fetch('/api/discovery',{headers:ah()});if(!r.ok)return;let d=await r.json();q('#dcRows').innerHTML=(d.nodes||[]).map(n=>{let head=n.pending?'<span class=muted>running…</span>':(n.problem?'<span class=muted>'+esc(n.problem)+'</span>':(n.at&&new Date(n.at).getUTCFullYear()>1?'<span class=muted>'+esc(n.at)+'</span>':'<span class=muted>not run</span>'));let ins=n.instances||[];if(!ins.length)return '<tr><td>'+esc(n.node_id)+'</td><td colspan=3>'+head+'</td></tr>';return ins.map((v,i)=>'<tr><td>'+(i===0?esc(n.node_id)+'<div>'+head+'</div>':'')+'</td><td>'+esc(v.unit)+(v.primary?' <span class=muted>(primary)</span>':'')+'</td><td><span class="badge '+dcCls(v.state)+'">'+esc(v.state)+'</span></td><td>'+esc((v.reasons||[]).join('; '))+'</td></tr>').join('')}).join('')}
function hlCls(st){return st==='UP'?'up':(st==='DOWN'?'down':'unknown')}
async function loadHealth(){if(!token())return;let r=await fetch('/api/health',{headers:ah()});if(!r.ok)return;let d=await r.json();q('#hlRows').innerHTML=(d.nodes||[]).map(n=>'<tr><td>'+esc(n.alias||n.node_id)+(n.conformity?'<div class=muted>config: '+esc(n.conformity)+'</div>':'')+'</td><td><span class="badge '+hlCls(n.overall)+'">'+esc(n.overall)+'</span>'+((n.why||[]).length?'<div class=muted>'+esc(n.why.join('; '))+'</div>':'')+'</td><td>'+(n.layers||[]).map(l=>'<div title="'+esc(l.evidence||l.note||'')+'"><span class="badge '+hlCls(l.state)+'">'+esc(l.layer)+' '+esc(l.state)+'</span> <span class=muted>'+esc(l.name)+(l.evidence?' · '+esc(l.evidence):'')+(l.note?' · '+esc(l.note):'')+'</span></div>').join('')+'</td><td>'+((n.transitions||[]).slice().reverse().map(t=>'<div class=muted>'+esc(t.at)+' · '+esc(t.layer)+' '+esc(t.from)+' → '+esc(t.to)+' · '+esc(t.reason)+' · '+esc(t.evidence)+'</div>').join('')||'<span class=muted>none</span>')+'</td></tr>').join('')}
function tnIngressStateText(i){if(!i)return '';const m={held:['held: waiting for the IR agent (not Ready)','در انتظار: منتظر عامل ایران (آماده نیست)'],queued:['queued for the IR agent (not Ready)','در صف عامل ایران (آماده نیست)'],applied:['applied, awaiting independent verification','اعمال شد؛ در انتظار راستی‌آزمایی مستقل'],blocked:['blocked: operator review required','مسدود: نیازمند بررسی اپراتور'],failed:['failed','ناموفق'],retiring:['retiring','در حال حذف'],retired:['retired','حذف شد']};let k=i.ready?['Ready','آماده']:(m[i.state]||[i.state,i.state]);return 'IR ingress: '+tnWizardText(k[0],k[1])+(i.reason&&!i.ready?' · '+i.reason:'')}
async function loadTunnels(){if(!token())return;let r=await fetch('/api/tunnels',{headers:ah()});if(!r.ok)return;let ts=await r.json();tnWizardTunnels=Array.isArray(ts)?ts:[];renderNodeCards(window.nodeVersionMap||Object.create(null));tnFill();rdFill(ts);q('#tnRows').innerHTML=ts.map(t=>{let final=['active','superseded','rolled_back','rollback_failed','decommissioned','decommission_failed'].includes(t.phase);let cls=t.phase==='active'?'up':(t.phase==='rolled_back'||t.phase==='rollback_failed'||t.phase==='decommission_failed'?'down':'unknown');let actions='';if(t.phase==='active')actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnEdit(this.dataset.id)">Edit</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDrift(this.dataset.id)">Check drift</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressPreflight(this.dataset.id)">Check IR ingress (read-only)</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressReadiness(this.dataset.id)">Verified ingress readiness</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressDeploy(this.dataset.id)">Deploy public ingress</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnIngressOperationCheck(this.dataset.id)">Ingress status</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnRotate(this.dataset.id)">Rotate cert</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Remove safely / حذف امن</button>'+(t.cert_epoch?'<div class=muted>cert epoch '+esc(t.cert_epoch)+'</div>':'');else if(t.phase==='decommission_failed')actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Retry decommission</button>';else if(!final)actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnCancel(this.dataset.id)">Cancel</button>';return '<tr><td>'+esc(t.id)+'</td><td>'+esc(t.ex_node)+'</td><td>'+esc(t.ir_node)+'</td><td><span class="badge '+cls+'">'+esc(t.phase)+'</span></td><td>'+esc(t.error||'')+(t.ingress?'<div class=muted data-ingress-state="'+esc(t.ingress.state)+'">'+esc(tnIngressStateText(t.ingress))+'</div>':'')+(t.drift?'<div class=muted>drift: <b>'+esc(t.drift.state)+'</b> · '+esc(Object.entries(t.drift.nodes||{}).filter(e=>e[1].state!=='IN_SYNC').map(e=>e[0]+': '+e[1].state+' ('+(e[1].problems||[]).join('; ')+')').join(' | '))+'</div>':'')+((t.evidence||[]).length?'<details><summary class=muted>evidence ('+t.evidence.length+')</summary>'+t.evidence.map(e=>'<div class=muted>'+esc(e.step)+' · '+esc(e.node)+' · '+(e.ok?'OK':'FAILED')+(e.problems?' · '+esc(e.problems.join('; ')):'')+'</div>').join('')+'</details>':'')+'</td><td>'+actions+'</td></tr>'}).join('')}
async function loadAll(){await loadNodes();await Promise.all([loadMonitoring(),loadJobs(),loadFinance(),loadAudit(),loadTunnels(),loadTunnelEngines(),loadHealth(),loadDiscovery(),loadPathProbes(),loadPathGraph(),loadPathMatrix(),loadBackups()])}
const fastPollMS=10000,slowPollMS=30000;let fastPollTimer=null,slowPollTimer=null;
async function loadFast(){if(document.hidden)return;await Promise.all([loadMonitoring(),loadTunnels(),loadHealth()])}
async function loadSlow(){if(document.hidden)return;await Promise.all([loadFinance(),loadDiscovery()])}
function stopPolling(){if(fastPollTimer!==null){clearInterval(fastPollTimer);fastPollTimer=null}if(slowPollTimer!==null){clearInterval(slowPollTimer);slowPollTimer=null}}
function startPolling(){stopPolling();if(document.hidden)return;fastPollTimer=setInterval(loadFast,fastPollMS);slowPollTimer=setInterval(loadSlow,slowPollMS)}
document.addEventListener('visibilitychange',()=>{if(document.hidden){stopPolling();return}loadFast();loadSlow();startPolling()});
initReportDates();loadAll();startPolling();
</script></body></html>`

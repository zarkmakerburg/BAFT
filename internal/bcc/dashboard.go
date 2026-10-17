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
.tnPortPicker{display:flex;gap:9px;align-items:stretch;min-width:0}
.tnPortPicker input{flex:1;min-width:0;width:auto}
.tnPortPicker button{flex:none;white-space:nowrap}
@media(max-width:430px){.tnPortPicker{flex-direction:column}}
@media(max-width:620px){.tnWizardSteps{display:flex;overflow:auto;gap:7px;padding:0 0 8px}.tnWizardSteps span[data-tn-nav]{min-width:92px;flex:none}.tnFields{grid-template-columns:1fr}.tnWizardControls button{flex:1}}

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
<div class="card aurora-tunnel-studio"><div class="top"><div><b>Aurora · Tunnel Control Studio</b><div class="muted">Smart Tunnel Wizard / ساخت ساده تونل — Native BAFT, real reviewed deployment</div></div><button class="alt" onclick="loadTunnels()">Refresh / تازه‌سازی</button></div>
<div class="tnWizardSteps" id="tnWizardSteps" aria-label="Tunnel creation steps">
 <span data-tn-nav="0" class="selected">1 · <span class="tn-en">Type</span><span class="tn-fa">نوع</span></span>
 <span data-tn-nav="1">2 · <span class="tn-en">Servers</span><span class="tn-fa">سرورها</span></span>
 <span data-tn-nav="2">3 · <span class="tn-en">Ports &amp; Xray</span><span class="tn-fa">پورت و Xray</span></span>
 <span data-tn-nav="3">4 · <span class="tn-en">Safety</span><span class="tn-fa">ایمنی</span></span>
 <span data-tn-nav="4">5 · <span class="tn-en">Review &amp; deploy</span><span class="tn-fa">بررسی و ساخت</span></span>
</div>
<div class="tnWizardStage" data-tn-step="0">
 <h3><span class="tn-en">What do you want to connect?</span><span class="tn-fa">چه اتصالی می‌خواهید بسازید؟</span></h3>
 <label class="tnLabel" for="tnDirection"><span class="tn-en">How should the two servers connect?</span><span class="tn-fa">جهت اتصال دو سرور چگونه باشد؟</span></label>
 <select id="tnDirection" onchange="tnClearPlan()" aria-describedby="tnDirectionInfo">
  <option value="ir-to-ex">Direct · IR dials EX — Native BAFT / مستقیم · ایران به خارج وصل می‌شود — فعال</option>
  <option value="ex-to-ir" disabled>Reverse · EX dials IR — External driver required / معکوس · خارج به ایران وصل می‌شود — در برنامه</option>
 </select>
 <div class="tnFlowPreview" id="tnDirectionInfo" role="note">
  <b><span class="tn-en">Available now — native BAFT Secure</span><span class="tn-fa">فعال در این نسخه — BAFT Secure بومی</span></b>
  <div class="tnFlowTrack" dir="ltr"><span>🇮🇷 IR (dialer)</span><span aria-hidden="true">→</span><span>🌍 EX (listener)</span><span aria-hidden="true">→</span><span>Xray EX</span></div>
  <p><span class="tn-en">Direction describes the inter-server connection, not the public client port. The Iran-side public listener is NOT created by this wizard. Reverse engines will appear only after successful two-node tests.</span><span class="tn-fa">جهت تونل مربوط به اتصال بین سرورهاست، نه پورت عمومی کاربران. این فرم ورودی عمومی ایران را نمی‌سازد. موتورهای معکوس فقط بعد از آزمون واقعی دو سرور فعال می‌شوند.</span></p>
 </div>
 <label class="tnLabel" for="tnEngine"><span class="tn-en">Choose an actually supported tunnel engine</span><span class="tn-fa">موتور تونل را از میان گزینه‌های واقعاً پشتیبانی‌شده انتخاب کنید</span></label>
 <select id="tnEngine" onchange="tnClearPlan()" aria-describedby="tnEngineInfo">
  <optgroup label="Direct — Iran initiates / مستقیم">
   <option value="baft">BAFT Secure — TLS + Noise (supported / فعال)</option>
  </optgroup>
  <optgroup label="Reverse — Abroad initiates / معکوس">
   <option value="frp" disabled>FRP — Planned / در برنامه</option>
   <option value="rathole" disabled>Rathole — Planned / در برنامه</option>
  </optgroup>
  <optgroup label="Other transports — not independent engines">
   <option value="wss" disabled>WSS / QUIC — Planned / در برنامه</option>
  </optgroup>
 </select>
 <div class="muted" id="tnEngineInfo"><span class="tn-fa">موتور BAFT ارتباط بین دو سرور را برقرار می‌کند؛ VLESS، VMess و Trojan پروتکل کاربران در Xray هستند، نه نوع تونل BAFT. از انتخاب نوع پشتیبانی‌نشده جلوگیری می‌شود.</span><span class="tn-en">BAFT links servers; VLESS, VMess and Trojan are user protocols configured in Xray, not BAFT tunnel drivers. Unsupported engines cannot be selected.</span></div>
 <label class="tnLabel" for="tnMode"><span class="tn-en">Action</span><span class="tn-fa">عملیات</span></label>
 <select id="tnMode" onchange="tnModeChanged()"><option value="independent">New independent tunnel / تونل مستقل جدید</option><option value="replace">Edit selected active tunnel / ویرایش تونل انتخاب‌شده</option></select>
 <p class="muted"><span class="tn-fa">برای ویرایش، از فهرست تونل‌های فعال دکمه «ویرایش» را انتخاب کنید. ساخت مستقل نباید تونل قبلی را جایگزین کند.</span><span class="tn-en">To edit, choose Edit on an active tunnel below. A new independent tunnel must not replace an existing tunnel.</span></p>
</div>
<div class="tnWizardStage" data-tn-step="1" hidden>
 <h3><span class="tn-en">Choose the two servers</span><span class="tn-fa">دو سرور را انتخاب کنید</span></h3>
 <div class="tnFields"><label class="tnLabel"><span class="tn-en">Iran / IR node</span><span class="tn-fa">سرور ایران (IR)</span><select id="tnIr" onchange="tnWizardChanged()"></select></label>
 <label class="tnLabel"><span class="tn-en">Abroad / EX node</span><span class="tn-fa">سرور خارج (EX)</span><select id="tnEx" onchange="tnWizardChanged()"></select></label></div>
 <div class="muted"><span class="tn-fa">مسیر BAFT: سرور ایران ← ارتباط امن BAFT ← سرور خارج ← سرویس Xray. جهت نهایی دریافت ترافیک کاربران به تنظیم ورودی ایران وابسته است.</span><span class="tn-en">BAFT path: Iran node → secure BAFT link → foreign node → Xray service. User ingress in Iran must be configured separately.</span></div>
</div>
<div class="tnWizardStage" data-tn-step="2" hidden>
 <h3><span class="tn-en">Ports and Xray destination</span><span class="tn-fa">پورت‌ها و مقصد Xray</span></h3>
 <div class="tnFields">
  <label class="tnLabel"><span class="tn-en">BAFT inter-server port (EX)</span><span class="tn-fa">پورت ارتباط دو سرور BAFT (خارج)</span><input id="tnPort" type="number" inputmode="numeric" min="1" max="65535" value="8443" oninput="tnClearPlan()"></label>
  <label class="tnLabel" for="tnXrayPort"><span class="tn-en">Xray inbound port on EX — choose or generate</span><span class="tn-fa">پورت اینباند Xray روی سرور خارج — دستی یا تصادفی</span><span class="tnPortPicker"><input id="tnXrayPort" type="number" inputmode="numeric" min="1" max="65535" placeholder="e.g. 2443" oninput="tnWizardTargetSync()"><button type="button" class="alt" id="tnGenerateXrayPort" onclick="tnWizardGenerateXrayPort()"><span class="tn-en">Generate random</span><span class="tn-fa">تولید تصادفی</span></button></span><span class="muted" id="tnXrayPortHint" role="status" aria-live="polite"><span class="tn-en">Enter the port you will later create in Sanaei / 3x-ui on the EX server.</span><span class="tn-fa">پورتی را وارد کنید که پس از ساخت تونل، در سنایی / 3x-ui سرور خارج اینباندش را می‌سازید.</span></span></label>
  <label class="tnLabel"><span class="tn-en">Xray user protocol (informational only)</span><span class="tn-fa">پروتکل کاربران در Xray (فقط جهت اطلاع)</span><select id="tnUserProtocol" onchange="tnClearPlan()"><option value="unknown">Unknown / در Xray تنظیم شده</option><option value="VLESS">VLESS (external)</option><option value="VMess">VMess (external)</option><option value="Trojan">Trojan (external)</option><option value="Shadowsocks">Shadowsocks (external)</option></select></label>
  <label class="tnLabel"><span class="tn-en">Public client port on IR (externally managed)</span><span class="tn-fa">پورت کاربران روی سرور ایران (مدیریت خارجی)</span><input id="tnClientPort" type="number" inputmode="numeric" min="1" max="65535" placeholder="443" oninput="tnWizardIngressChanged()"></label>
 </div>
 <div class="tnNotice" role="note"><strong><span class="tn-en">After the tunnel: configure Sanaei / 3x-ui yourself</span><span class="tn-fa">بعد از ساخت تونل: کانفیگ سنایی / 3x-ui را خودتان بسازید</span></strong><p><span class="tn-en">1. Choose an Xray port above, manually or with Generate. 2. Deploy the reviewed BAFT tunnel. 3. On the EX server, create an Xray inbound listening on exactly that port. 4. Configure your client subscription / Sanaei address with your own reachable Iran-side domain or IP and public client port, after wiring that ingress to the BAFT internal route. Test real user traffic. Random port generation only proposes a number: it neither checks OS availability nor creates any Xray inbound.</span><span class="tn-fa">۱. پورت Xray را دستی وارد کنید یا تولید تصادفی بزنید. ۲. تونل BAFT را با طرح تأییدشده بسازید. ۳. روی سرور خارج در سنایی / 3x-ui یک اینباند دقیقاً با همان پورت ایجاد کنید. ۴. بعد از اتصال ورودی عمومی ایران به مسیر داخلی BAFT، آدرس دامنه یا IP ایران و پورت عمومی انتخابی را خودتان در تنظیمات کانفیگ / اشتراک سنایی وارد و ترافیک واقعی را تست کنید. تولید تصادفی صرفاً پیشنهاد عدد است؛ آزادبودن پورت روی سیستم را تأیید نمی‌کند و اینباند نمی‌سازد.</span></p></div>
 <div class="tnNotice" role="note"><strong><span class="tn-en">Critical port distinction</span><span class="tn-fa">تفاوت مهم پورت‌ها</span></strong><p><span class="tn-fa">BAFT در این نسخه فقط پورت تونل، شنونده داخلی ایران و مقصد سرویس خارج را تنظیم می‌کند. این فرم پورت عمومی کاربران در ایران یا تنظیمات Xray/3x-ui را ایجاد نمی‌کند. اگر پورت عمومی را وارد می‌کنید، اتصال آن به مسیر داخلی BAFT باید جداگانه برقرار و آزمایش شود.</span><span class="tn-en">This version configures the BAFT link, local IR route and foreign service target. It does NOT create the public IR client listener or edit Xray/3x-ui. Entering a public port documents an externally configured ingress; it still requires independent wiring and end-to-end verification.</span></p></div>
 <label class="tnConfirm"><input type="checkbox" id="tnIngressConfirm" onchange="tnClearPlan()"> <span class="tn-en">I understand public IR ingress is not provisioned by this wizard.</span><span class="tn-fa">می‌دانم ورودی عمومی ایران توسط این راه‌انداز ساخته نمی‌شود.</span></label>
 <button type="button" class="alt" onclick="tnWizardSuggest()"><span class="tn-en">Suggest free managed ports</span><span class="tn-fa">پیشنهاد پورت‌های بدون تداخل در BCC</span></button>
</div>
<div class="tnWizardStage" data-tn-step="3" hidden>
 <h3><span class="tn-en">Transport and safety</span><span class="tn-fa">نوع ارتباط و ایمنی</span></h3>
 <p class="muted"><span class="tn-fa">پروتکل ارتباط سرورها: BAFT Secure با TLS و Noise. پروتکل واقعی کاربران (مثل VLESS) در Xray تنظیم می‌شود؛ این راه‌انداز آن را تغییر نمی‌دهد. روش‌های دیگر تا پیاده‌سازی واقعی غیرفعال‌اند.</span><span class="tn-en">Server-to-server transport: BAFT Secure with TLS and Noise. The actual user protocol (e.g. VLESS) is owned by Xray and is not modified here. Other drivers remain unavailable until implemented.</span></p>
 <div id="tnScopedFields" class="aurora-scope">
  <div class="tnNotice"><span class="tn-fa">هر تونل مستقل پیکربندی جداگانه دارد. مقادیر زیر پیشنهاد هستند؛ بررسی نهایی پورت‌ها هنگام Preview و استقرار انجام می‌شود.</span><span class="tn-en">Each independent tunnel has isolated config. Values below are suggestions; final conflicts are checked by the reviewed plan and deployment.</span></div>
  <label class="tnConfirm"><input type="checkbox" id="tnLegacyCoexist" onchange="tnClearPlan()"> <span class="tn-en">Keep active legacy tunnel on shared server (explicit consent)</span><span class="tn-fa">تونل قدیمی فعال روی سرور مشترک حفظ شود (تأیید صریح)</span></label>
  <details class="tnAdvanced"><summary><span class="tn-en">Advanced: instance and internal listeners</span><span class="tn-fa">پیشرفته: شناسه و پورت‌های داخلی</span></summary>
    <div class="tnFields">
      <label class="tnLabel">Instance ID<input id="tnInstance" maxlength="40" autocomplete="off" placeholder="auto-created"></label>
      <label class="tnLabel">IR loopback route<input id="tnRouteListen" value="127.0.0.1:1444"></label>
      <label class="tnLabel">EX metrics loopback<input id="tnExMetric" value="127.0.0.1:9202"></label>
      <label class="tnLabel">IR metrics loopback<input id="tnIrMetric" value="127.0.0.1:9302"></label>
      <label class="tnLabel">EX target address (advanced)<input id="tnTarget" value="127.0.0.1:2443" oninput="tnClearPlan()"></label>
    </div>
  </details>
 </div>
 <div id="tnEditNote" class="muted" style="margin-top:9px"></div>
</div>
<div class="tnWizardStage" data-tn-step="4" hidden>
 <h3><span class="tn-en">Review the real route before deployment</span><span class="tn-fa">قبل از ساخت، مسیر واقعی را بررسی کنید</span></h3>
 <div id="tnWizardSummary" class="tnReview" aria-live="polite"></div>
 <div class="tnNotice"><span class="tn-fa">تأیید طرح BAFT به معنای آزمایش ترافیک واقعی کاربران یا راه‌اندازی خودکار Xray/پورت عمومی ایران نیست. در صورت وجود خطای پورت یا نبود ارتباط Agent، ساخت متوقف می‌شود.</span><span class="tn-en">A successful BAFT plan does not verify user traffic or configure public IR ingress/Xray. Port conflicts or stale Agent health block deployment.</span></div>
 <div class="actions"><button type="button" onclick="tnPlan()"><span class="tn-en">Review real BAFT deployment plan</span><span class="tn-fa">بررسی طرح واقعی استقرار BAFT</span></button></div>
 <div id="tnPlan" style="margin-top:12px"></div>
</div>
<div class="tnWizardControls">
 <button type="button" class="alt" id="tnWizardBack" onclick="tnWizardStep(tnWizardIndex-1)" hidden><span class="tn-en">Previous</span><span class="tn-fa">قبلی</span></button>
 <button type="button" id="tnWizardNext" onclick="tnWizardStep(tnWizardIndex+1)"><span class="tn-en">Next step</span><span class="tn-fa">مرحله بعد</span></button>
 <button type="button" class="alt" onclick="tnNew()"><span class="tn-en">New tunnel / Reset</span><span class="tn-fa">تونل جدید / شروع دوباره</span></button>
</div>
<div class="tablewrap"><table><thead><tr><th>ID</th><th>EX</th><th>IR</th><th>Phase</th><th>Detail</th><th></th></tr></thead><tbody id="tnRows"></tbody></table></div></div>

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

<div class="card"><div class="top"><b>Cluster Nodes</b><span id="count" class="muted"></span></div>
<div class="tablewrap"><table><thead><tr><th></th><th>Alias</th><th>ID</th><th>Address</th><th>Role</th><th>Health Check</th><th>Last Check</th><th>Security</th></tr></thead><tbody id="rows"></tbody></table></div>
<div class="actions"><button id="deploySelected" onclick="deploy()" disabled>One-Click Deploy Selected</button><button class="alt" onclick="loadAll()">Refresh</button></div></div>

<div class="card"><b>Recent Jobs</b><div id="jobs" class="muted" style="margin-top:10px">Admin token required.</div></div>
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
function setToken(){localStorage.setItem('bccToken',prompt('BCC admin token')||'');loadAll()}
function ah(){return {'Authorization':'Bearer '+token(),'Content-Type':'application/json'}}
function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function ago(v){if(!v||new Date(v).getFullYear()<2000)return '—';let d=(Date.now()-new Date(v).getTime())/1000;if(d<60)return Math.max(0,Math.round(d))+'s ago';if(d<3600)return Math.round(d/60)+'m ago';return Math.round(d/3600)+'h ago'}
function rate(v){return (Number(v||0)/1000).toFixed(2)+'/min'}
function routeHTML(rs){if(!rs||!rs.length)return '<span class=muted>—</span>';return rs.map(r=>'<div class=route><span class="badge '+esc(r.status)+'">'+esc(r.status)+'</span><span>'+esc(r.route_id)+'</span><span class=muted>'+((r.latency_ms??-1)>=0?esc(r.latency_ms)+' ms':'—')+' · '+esc(r.probe_kind||'')+'</span></div>').join('')}
async function loadNodes(){if(!token())return;let r=await fetch('/api/nodes',{headers:ah()});if(!r.ok)return;nodes=await r.json();q('#count').textContent=nodes.length+' nodes';ppFill();q('#deploySelected').disabled=true;q('#rows').innerHTML=nodes.map(n=>'<tr><td><input type=checkbox class=pick value="'+esc(n.id)+'"></td><td>'+esc(n.alias)+'</td><td>'+esc(n.id)+'</td><td>'+esc(n.address)+'</td><td>'+esc(n.role)+'</td><td><span class="badge '+esc(n.health)+'">'+esc(n.health)+'</span> '+((n.latency_ms??-1)>=0?esc(n.latency_ms)+' ms':'')+'</td><td>'+esc(n.last_checked||'—')+'</td><td>'+(n.revoked?'<span class="badge down">REVOKED</span>':'<button class="alt" data-id="'+esc(n.id)+'" onclick="rotateToken(this.dataset.id)">Rotate</button> <button data-id="'+esc(n.id)+'" onclick="revokeNode(this.dataset.id)">Kill</button>')+'</td></tr>').join('')}
async function loadMonitoring(){if(!token())return;let r=await fetch('/api/monitoring',{headers:ah()});if(!r.ok){q('#monitorTime').textContent=document.documentElement.lang==='fa'?'خطا در دریافت پایش':'Monitoring unavailable';for(let id of ['kNodes','kUp','kDown','kUnknown'])q('#'+id).textContent='—';q('#monitorRows').innerHTML='';return}monitor=await r.json();q('#monitorTime').textContent=new Date().toLocaleTimeString();q('#kNodes').textContent=monitor.length;q('#kUp').textContent=monitor.filter(x=>x.status==='up').length;q('#kDown').textContent=monitor.filter(x=>x.status==='down').length;q('#kUnknown').textContent=monitor.filter(x=>x.status==='unknown').length;
q('#monitorRows').innerHTML=monitor.map(n=>'<tr><td><b>'+esc(n.alias)+'</b><div class=muted>'+esc(n.node_id)+'</div></td><td><span class="badge '+esc(n.status)+'">'+esc(n.status)+'</span></td><td>'+((n.noise_latency_ms??-1)>=0?esc(n.noise_latency_ms)+' ms':'—')+'<div class=muted>Noise RTT</div></td><td>'+((n.latency_ms??-1)>=0?esc(n.latency_ms)+' ms':'—')+'<div class=muted>TCP health-check</div></td><td>'+rate(n.handshake_error_rate_milli_per_min)+'</td><td>'+ago(n.last_seen)+'</td><td>'+esc(n.active_sessions||0)+'</td><td>'+routeHTML(n.routes)+'</td></tr>').join('');
let sel=q('#historyNode'),cur=sel.value;sel.innerHTML=monitor.map(n=>'<option value="'+esc(n.node_id)+'">'+esc(n.alias)+'</option>').join('');if(cur&&monitor.some(n=>n.node_id===cur))sel.value=cur;if(!sel.dataset.loaded&&monitor.length){sel.dataset.loaded='1';loadHistory()}}
async function loadJobs(){if(!token())return;let r=await fetch('/api/jobs',{headers:ah()});if(!r.ok)return;let j=await r.json();q('#jobs').innerHTML=j.slice(0,20).map(x=>'<div>'+esc(x.id)+' · '+esc(x.type)+' · '+esc(x.node_id)+' · <b>'+esc(x.status)+'</b></div>').join('')||'No jobs'}
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
function tnFill(){let opt=(role)=>nodes.filter(n=>role.includes(n.role)&&!n.revoked).map(n=>'<option value="'+esc(n.id)+'">'+esc(n.alias||n.id)+' ('+esc(n.id)+')</option>').join('');let ex=q('#tnEx'),ir=q('#tnIr'),a=ex.value,b=ir.value;ex.innerHTML='<option value="">EX node…</option>'+opt(['foreign']);ir.innerHTML='<option value="">IR node…</option>'+opt(['worker','master']);ex.value=a;ir.value=b}

// Native BAFT Smart Tunnel Wizard. It only submits fields supported by BCC's
// existing reviewed tunnel API; public client ingress is never silently made.
let tnWizardIndex=0,tnWizardTunnels=[];
function tnWizardText(en,fa){return document.documentElement.lang==='fa'?fa:en}
function tnWizardChanged(){tnClearPlan()}
function tnWizardTargetSync(){let n=Number(q('#tnXrayPort').value);if(Number.isInteger(n)&&n>=1&&n<=65535){q('#tnTarget').value='127.0.0.1:'+n}tnClearPlan()}
function tnWizardGenerateXrayPort(){
 // Best-effort suggestion from the high, non-privileged range. BCC inventory is
 // not a live OS/Xray socket scan; the user must verify before creating inbound.
 const ex=q('#tnEx').value,blocked=new Set([Number(q('#tnPort').value)]);
 for(const t of tnWizardTunnels){
  if(t.ex_node!==ex||!tnWizardActive(t))continue;
  blocked.add(Number(t.port));
  const target=String(t.target||'').match(/^127\.0\.0\.1:(\d+)$/);
  if(target)blocked.add(Number(target[1]));
  const metrics=String(t.ex_metrics_listen||'').match(/^127\.0\.0\.1:(\d+)$/);
  if(metrics)blocked.add(Number(metrics[1]));
 }
 const bytes=new Uint32Array(1);
 for(let attempt=0;attempt<64;attempt++){
  crypto.getRandomValues(bytes);
  const candidate=20000+bytes[0]%40000;
  if(blocked.has(candidate))continue;
  q('#tnXrayPort').value=String(candidate);
  tnWizardTargetSync();
  const hint=q('#tnXrayPortHint');
  if(hint)hint.textContent=tnWizardText('Suggested '+candidate+' — confirm it is free on EX, then create a matching inbound in Sanaei / 3x-ui after BAFT deployment.','پورت پیشنهادی '+candidate+' — آزادبودن آن را روی سرور خارج بررسی و پس از استقرار BAFT در سنایی / 3x-ui اینباند متناظر را ایجاد کنید.');
  return;
 }
 alert(tnWizardText('Could not suggest a port. Enter one manually.','پورت پیشنهادی پیدا نشد؛ پورت را دستی وارد کنید.'));
}
function tnWizardIngressChanged(){if(!q('#tnClientPort').value)q('#tnIngressConfirm').checked=false;tnClearPlan()}
function tnWizardActive(t){return !['superseded','decommissioned','rolled_back'].includes(t.phase)}
function tnWizardSharesLegacy(){const ex=q('#tnEx').value,ir=q('#tnIr').value;return tnWizardTunnels.some(t=>t.phase==='active'&&!t.instance_id&&(t.ex_node===ex||t.ir_node===ir))}
function tnWizardEnsureID(){if(q('#tnInstance').value.trim())return;const a=new Uint32Array(2);crypto.getRandomValues(a);q('#tnInstance').value='baft-'+[...a].map(v=>v.toString(36)).join('-').slice(0,29)}
function tnWizardSuggest(){
 if(tnEditBase){alert(tnWizardText('Editing keeps the existing instance and ports until you explicitly change them.','در حالت ویرایش، پورت‌ها و شناسه موجود تا زمان تغییر صریح حفظ می‌شوند.'));return}
 const ex=q('#tnEx').value,ir=q('#tnIr').value,used=tnWizardTunnels.filter(t=>tnWizardActive(t));
 const next=(from,forbidden)=>{for(let p=from;p<65536;p++)if(!forbidden(p))return p;return null};
 if(ex){let p=next(8443,p=>used.some(t=>t.ex_node===ex&&Number(t.port)===p));if(p)q('#tnPort').value=p}
 if(ir){let p=next(1444,p=>used.some(t=>t.ir_node===ir&&t.route_listen==='127.0.0.1:'+p));if(p)q('#tnRouteListen').value='127.0.0.1:'+p}
 if(ex){let p=next(9202,p=>used.some(t=>t.ex_node===ex&&(t.ex_metrics_listen||'127.0.0.1:9191')==='127.0.0.1:'+p));if(p)q('#tnExMetric').value='127.0.0.1:'+p}
 if(ir){let p=next(9302,p=>used.some(t=>t.ir_node===ir&&(t.ir_metrics_listen||'127.0.0.1:9191')==='127.0.0.1:'+p));if(p)q('#tnIrMetric').value='127.0.0.1:'+p}
 tnWizardEnsureID();tnClearPlan();
}
function tnWizardValidateStep(i){
 if(i===0){
  if(q('#tnDirection').value!=='ir-to-ex'){alert(tnWizardText('Reverse topology is not yet supported by the live BAFT engine.','اتصال معکوس هنوز در موتور فعال BAFT پیاده نشده است.'));return false}
  if(q('#tnEngine').value!=='baft'){alert(tnWizardText('This tunnel engine is not implemented.','این موتور تونل هنوز پیاده‌سازی نشده است.'));return false}
  if(q('#tnMode').value==='replace'&&!tnEditBase){alert(tnWizardText('Choose Edit on an active tunnel in the list below.','برای ویرایش، ابتدا از فهرست پایین روی ویرایش تونل فعال بزنید.'));return false}
 }
 if(i===1){
  if(!q('#tnEx').value||!q('#tnIr').value||q('#tnEx').value===q('#tnIr').value){alert(tnWizardText('Select different Iran and foreign servers.','سرور ایران و خارج را به‌درستی و جداگانه انتخاب کنید.'));return false}
 }
 if(i===2){
  for(const id of ['tnPort','tnXrayPort']){const raw=String(q('#'+id).value).trim(),n=Number(raw);if(!raw||!Number.isInteger(n)||n<1||n>65535){alert(tnWizardText('Enter valid BAFT and EX service ports (1–65535).','پورت معتبر BAFT و مقصد سرویس خارج را وارد کنید (۱ تا ۶۵۵۳۵).'));return false}}
  if(q('#tnClientPort').value){const n=Number(q('#tnClientPort').value);if(!Number.isInteger(n)||n<1||n>65535){alert(tnWizardText('Enter a valid public IR port.','پورت عمومی ایران معتبر نیست.'));return false}}
  if(!q('#tnIngressConfirm').checked){alert(tnWizardText('Confirm that BAFT does not create the public IR client ingress.','تأیید کنید که BAFT در این نسخه پورت عمومی کاربران ایران را ایجاد نمی‌کند.'));return false}
 }
 if(i===3){
  if(q('#tnMode').value==='independent'){
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
}
function tnWizardSummary(){
 const r=tnReq(),c=q('#tnClientPort').value.trim(),legacy=tnWizardSharesLegacy(),active=!!tnEditBase;
 const entry=c?tnWizardText('IR public client port '+esc(c)+' — EXTERNALLY MANAGED; not deployed by BAFT','پورت عمومی کاربران ایران '+esc(c)+' — مدیریت خارجی؛ BAFT آن را نمی‌سازد'):tnWizardText('IR public client port: not configured in BAFT','پورت عمومی کاربران ایران: در BAFT تنظیم نمی‌شود');
 q('#tnWizardSummary').innerHTML='<div><b>'+esc(tnWizardText('Driver','موتور'))+':</b> BAFT Secure (TLS + Noise) — '+esc(active?tnWizardText('Edit selected instance','ویرایش نمونه انتخاب‌شده'):tnWizardText('New independent instance','نمونه مستقل جدید'))+'</div>'
  +'<div><b>'+esc(tnWizardText('Nodes','سرورها'))+':</b> '+esc(r.ir_node)+' → '+esc(r.ex_node)+'</div>'
  +'<div><b>'+esc(tnWizardText('Tunnel connection direction','جهت اتصال بین سرورها'))+':</b> '+esc(tnWizardText('Direct: IR initiates → EX listens; separate from user ingress','مستقیم: ایران اتصال را آغاز می‌کند ← خارج شنونده است؛ مستقل از ورودی کاربران'))+'</div>'
  +'<div><b>'+esc(tnWizardText('BAFT outer EX listener','پورت ارتباط BAFT روی خارج'))+':</b> '+esc(r.port)+'</div>'
  +'<div><b>'+esc(tnWizardText('IR internal route','مسیر داخلی ایران'))+':</b> '+esc(r.route_listen||'(legacy default)')+'</div>'
  +'<div><b>'+esc(tnWizardText('EX service / Xray destination','مقصد Xray روی خارج'))+':</b> '+esc(r.target||'127.0.0.1:2443')+'</div>'
  +'<div class="tnNotice"><b>'+esc(tnWizardText('Next: configure your Sanaei / 3x-ui inbound yourself','مرحله بعد: ساخت دستی اینباند در سنایی / 3x-ui'))+'</b><p>'+esc(tnWizardText('After BAFT deploy, create an EX inbound at port '+q('#tnXrayPort').value+'. Once IR public ingress is actually wired to the BAFT local route, set your own IR IP/domain and public client port in the Sanaei client config. No Xray or IR public listener is created by BAFT.','بعد از استقرار BAFT، در سرور خارج اینباند سنایی با پورت '+q('#tnXrayPort').value+' بسازید. پس از اتصال واقعی ورودی عمومی ایران به مسیر داخلی BAFT، آدرس IP یا دامنه ایران و پورت عمومی را خودتان در کانفیگ سنایی وارد کنید. BAFT اینباند Xray یا ورودی عمومی ایران را نمی‌سازد.'))+'</p></div>'
  +'<div><b>'+esc(tnWizardText('User-facing IR entry','ورودی کاربران در ایران'))+':</b> '+entry+'</div>'
  +'<div><b>'+esc(tnWizardText('User protocol (external Xray)','پروتکل کاربران در Xray خارجی'))+':</b> '+esc(q('#tnUserProtocol').value)+' — '+esc(tnWizardText('not configured by BAFT','توسط BAFT تغییر نمی‌کند'))+'</div>'
  +'<div><b>'+esc(tnWizardText('Legacy sibling','تونل قدیمی هم‌سرور'))+':</b> '+(legacy?esc(tnWizardText('Present — explicit keep consent required','موجود — تأیید حفظ الزامی است')):esc(tnWizardText('No active shared legacy detected in BCC','در BCC تونل قدیمی مشترک فعال شناسایی نشد')))+'</div>'
  +'<div class="muted">'+esc(tnWizardText('Xray VLESS/VMess/Trojan settings are external and unchanged by this plan.','پروتکل کاربران در Xray مانند VLESS/VMess/Trojan در این طرح تغییر نمی‌کند.'))+'</div>';
}
document.addEventListener('input',e=>{if(e.target.closest('.aurora-tunnel-studio')&&e.target.id!=='tnPlan')tnClearPlan()});

let tnPlanHash='',tnPlanReq=null,tnPlanReplaces='';
let tnEditBase=null;
function tnReq(){const ex=q('#tnEx').value,ir=q('#tnIr').value,port=Number(q('#tnPort').value)||8443,target=q('#tnTarget').value.trim(),mode=q('#tnMode').value;if(tnEditBase&&tnEditBase.ex_node===ex&&tnEditBase.ir_node===ir&&mode===(tnEditBase.instance_id?'independent':'replace')){let r=Object.assign({},tnEditBase,{port,target:target||tnEditBase.target});if(r.instance_id){r.route_listen=q('#tnRouteListen').value.trim()||r.route_listen;r.ex_metrics_listen=q('#tnExMetric').value.trim()||r.ex_metrics_listen;r.ir_metrics_listen=q('#tnIrMetric').value.trim()||r.ir_metrics_listen;r.allow_legacy_coexistence=q('#tnLegacyCoexist').checked;}return r;}let r={ex_node:ex,ir_node:ir,port};if(target)r.target=target;if(mode==='independent'){r.instance_id=q('#tnInstance').value.trim();r.route_listen=q('#tnRouteListen').value.trim();r.ex_metrics_listen=q('#tnExMetric').value.trim();r.ir_metrics_listen=q('#tnIrMetric').value.trim();r.allow_legacy_coexistence=q('#tnLegacyCoexist').checked;}return r}
function tnModeChanged(){const independent=q('#tnMode').value==='independent';if(tnEditBase&&independent!==!!tnEditBase.instance_id)tnEditBase=null;q('#tnScopedFields').hidden=!independent;q('#tnEditNote').textContent=tnEditBase?'Editing an existing managed instance; always preview before deploy.':(independent?'Independent instance: existing tunnels stay online if resource checks pass.':'Use Edit on an active tunnel. Replacement will interrupt that instance.');tnClearPlan();}
function tnNew(){tnEditBase=null;q('#tnDirection').value='ir-to-ex';q('#tnEngine').value='baft';q('#tnMode').value='independent';q('#tnInstance').value='';q('#tnTarget').value='';q('#tnXrayPort').value='';q('#tnClientPort').value='';q('#tnUserProtocol').value='unknown';q('#tnIngressConfirm').checked=false;q('#tnPort').value='8443';q('#tnRouteListen').value='127.0.0.1:1444';q('#tnExMetric').value='127.0.0.1:9202';q('#tnIrMetric').value='127.0.0.1:9302';q('#tnLegacyCoexist').checked=false;tnModeChanged();tnWizardStep(0)}
async function tnEdit(id){let r=await fetch('/api/tunnels?id='+encodeURIComponent(id),{headers:ah()});if(!r.ok){alert(await r.text());return}let t=await r.json();tnEditBase={instance_id:t.instance_id||'',ex_node:t.ex_node,ir_node:t.ir_node,public_address:t.public_address,port:t.port,target:t.target,route_listen:t.route_listen,route_id:t.route_id,ex_metrics_listen:t.ex_metrics_listen||'',ir_metrics_listen:t.ir_metrics_listen||'',record_shaping:!!t.record_shaping};tnClearPlan();q('#tnEx').value=t.ex_node;q('#tnIr').value=t.ir_node;q('#tnPort').value=t.port;q('#tnTarget').value=t.target;q('#tnXrayPort').value=(t.target||'').startsWith('127.0.0.1:')?t.target.split(':')[1]:'';q('#tnClientPort').value='';q('#tnUserProtocol').value='unknown';q('#tnIngressConfirm').checked=false;q('#tnMode').value=t.instance_id?'independent':'replace';q('#tnInstance').value=t.instance_id||'';q('#tnRouteListen').value=t.route_listen||'127.0.0.1:1444';q('#tnExMetric').value=t.ex_metrics_listen||'127.0.0.1:9202';q('#tnIrMetric').value=t.ir_metrics_listen||'127.0.0.1:9302';q('#tnLegacyCoexist').checked=false;tnModeChanged();q('#tnEditNote').innerHTML='Editing <b>'+esc(id)+'</b>: change the port or target, then preview the plan. '+esc(id)+' keeps running until the replacement is verified; if the replacement fails it rolls back and '+esc(id)+' stays active. <button class="alt" onclick="tnEditEnd()">Stop editing</button>';tnWizardStep(1);q('#tnIr').scrollIntoView({block:'center'})}
function tnEditEnd(){tnEditBase=null;q('#tnEditNote').innerHTML='';q('#tnTarget').value=''}
function tnShowPlan(p){tnPlanHash=p.hash;tnPlanReq=p.request;tnPlanReplaces=p.replaces||'';let h='<div class="card" style="margin:0"><b>Deployment plan</b> <span class="badge '+(p.ok?'up':'down')+'">'+(p.ok?'ready':'blocked')+'</span><div class="muted" style="font-family:monospace;word-break:break-all;margin:6px 0">plan '+esc(p.hash)+'</div>';h+='<table><thead><tr><th>Gate</th><th>Status</th><th>Detail</th></tr></thead><tbody>'+p.gates.map(g=>'<tr><td>'+esc(g.name)+'</td><td><span class="badge '+(g.status==='PASS'?'up':g.status==='WARN'?'unknown':'down')+'">'+esc(g.status)+'</span></td><td>'+esc(g.detail)+'</td></tr>').join('')+'</tbody></table>';h+=(p.replaces?'<div style="margin-top:8px"><b>Replaces</b> '+esc(p.replaces)+' <span class="muted">(stays active until this plan is verified, then becomes superseded)</span></div>':'');h+=p.nodes.map(n=>'<div style="margin-top:10px"><b>'+esc(n.role.toUpperCase())+' '+esc(n.node_id)+'</b> <span class="muted">generation '+esc(n.generation_from)+' → '+esc(n.generation_to)+(n.generation_note?' ('+esc(n.generation_note)+')':'')+'</span><ul>'+n.changes.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div>').join('');h+='<div><b>Steps</b><ol>'+p.steps.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ol></div><div><b>Verified before it counts as deployed</b><ul>'+p.verify.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div><div><b>Rollback</b><ul>'+p.rollback.map(c=>'<li>'+esc(c)+'</li>').join('')+'</ul></div>';h+='<div class="actions"><button '+(p.ok?'':'disabled ')+'onclick="tnDeploy()">Deploy this plan</button><button class="alt" onclick="tnClearPlan()">Discard</button></div></div>';q('#tnPlan').innerHTML=h}
function tnClearPlan(){tnPlanHash='';tnPlanReq=null;tnPlanReplaces='';q('#tnPlan').innerHTML=''}
async function tnPlan(){for(let i=0;i<4;i++)if(!tnWizardValidateStep(i))return;let body=tnReq();if(!body.ex_node||!body.ir_node){alert('Choose the EX and IR nodes');return}if(!tnEditBase&&q('#tnMode').value==='replace'){alert('To edit or replace an active tunnel, first click Edit on its row. / برای ویرایش، ابتدا روی ویرایش تونل فعال کلیک کنید.');return}if(q('#tnMode').value==='independent'&&!/^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$/.test(body.instance_id||'')){alert('Enter a unique valid Instance ID (letters, numbers, dot, underscore or hyphen). / شناسه نمونه مستقل الزامی است.');return}let r=await fetch('/api/tunnels/plan',{method:'POST',headers:ah(),body:JSON.stringify(body)});let t=await r.text();if(!r.ok){alert(t);return}tnShowPlan(JSON.parse(t));loadAudit()}
async function tnDeploy(){if(!tnPlanHash||!tnPlanReq)return;if(tnPlanReplaces&&!confirm('This plan replaces existing active tunnel '+tnPlanReplaces+'. Users may disconnect. Continue? / تونل فعال جایگزین می‌شود و ممکن است اتصال کاربران قطع شود. ادامه؟'))return;if(tnPlanReq.allow_legacy_coexistence&&!confirm('Deploy independent BAFT instance alongside current legacy tunnel? Existing tunnel will not be replaced. Ensure ports are free. / تونل مستقل در کنار تونل فعلی ساخته شود؟'))return;let body=Object.assign({},tnPlanReq,{plan_hash:tnPlanHash});let r=await fetch('/api/tunnels',{method:'POST',headers:ah(),body:JSON.stringify(body)});let t=await r.text();if(r.status===409){try{let d=JSON.parse(t);alert(d.error);tnShowPlan(d.plan)}catch(e){alert(t)}return}if(!r.ok){alert(t);return}tnClearPlan();tnEditEnd();loadTunnels();loadAudit()}
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
async function loadDiscovery(){if(!token())return;let r=await fetch('/api/discovery',{headers:ah()});if(!r.ok)return;let d=await r.json();q('#dcRows').innerHTML=(d.nodes||[]).map(n=>{let head=n.pending?'<span class=muted>running…</span>':(n.problem?'<span class=muted>'+esc(n.problem)+'</span>':(n.at?'<span class=muted>'+esc(n.at)+'</span>':'<span class=muted>not run</span>'));let ins=n.instances||[];if(!ins.length)return '<tr><td>'+esc(n.node_id)+'</td><td colspan=3>'+head+'</td></tr>';return ins.map((v,i)=>'<tr><td>'+(i===0?esc(n.node_id)+'<div>'+head+'</div>':'')+'</td><td>'+esc(v.unit)+(v.primary?' <span class=muted>(primary)</span>':'')+'</td><td><span class="badge '+dcCls(v.state)+'">'+esc(v.state)+'</span></td><td>'+esc((v.reasons||[]).join('; '))+'</td></tr>').join('')}).join('')}
function hlCls(st){return st==='UP'?'up':(st==='DOWN'?'down':'unknown')}
async function loadHealth(){if(!token())return;let r=await fetch('/api/health',{headers:ah()});if(!r.ok)return;let d=await r.json();q('#hlRows').innerHTML=(d.nodes||[]).map(n=>'<tr><td>'+esc(n.alias||n.node_id)+(n.conformity?'<div class=muted>config: '+esc(n.conformity)+'</div>':'')+'</td><td><span class="badge '+hlCls(n.overall)+'">'+esc(n.overall)+'</span>'+((n.why||[]).length?'<div class=muted>'+esc(n.why.join('; '))+'</div>':'')+'</td><td>'+(n.layers||[]).map(l=>'<div title="'+esc(l.evidence||l.note||'')+'"><span class="badge '+hlCls(l.state)+'">'+esc(l.layer)+' '+esc(l.state)+'</span> <span class=muted>'+esc(l.name)+(l.evidence?' · '+esc(l.evidence):'')+(l.note?' · '+esc(l.note):'')+'</span></div>').join('')+'</td><td>'+((n.transitions||[]).slice().reverse().map(t=>'<div class=muted>'+esc(t.at)+' · '+esc(t.layer)+' '+esc(t.from)+' → '+esc(t.to)+' · '+esc(t.reason)+' · '+esc(t.evidence)+'</div>').join('')||'<span class=muted>none</span>')+'</td></tr>').join('')}
async function loadTunnels(){if(!token())return;let r=await fetch('/api/tunnels',{headers:ah()});if(!r.ok)return;let ts=await r.json();tnWizardTunnels=Array.isArray(ts)?ts:[];tnFill();rdFill(ts);q('#tnRows').innerHTML=ts.map(t=>{let final=['active','superseded','rolled_back','rollback_failed','decommissioned','decommission_failed'].includes(t.phase);let cls=t.phase==='active'?'up':(t.phase==='rolled_back'||t.phase==='rollback_failed'||t.phase==='decommission_failed'?'down':'unknown');let actions='';if(t.phase==='active')actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnEdit(this.dataset.id)">Edit</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDrift(this.dataset.id)">Check drift</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnRotate(this.dataset.id)">Rotate cert</button> <button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Remove safely / حذف امن</button>'+(t.cert_epoch?'<div class=muted>cert epoch '+esc(t.cert_epoch)+'</div>':'');else if(t.phase==='decommission_failed')actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnDecommission(this.dataset.id)">Retry decommission</button>';else if(!final)actions='<button class="alt" data-id="'+esc(t.id)+'" onclick="tnCancel(this.dataset.id)">Cancel</button>';return '<tr><td>'+esc(t.id)+'</td><td>'+esc(t.ex_node)+'</td><td>'+esc(t.ir_node)+'</td><td><span class="badge '+cls+'">'+esc(t.phase)+'</span></td><td>'+esc(t.error||'')+(t.drift?'<div class=muted>drift: <b>'+esc(t.drift.state)+'</b> · '+esc(Object.entries(t.drift.nodes||{}).filter(e=>e[1].state!=='IN_SYNC').map(e=>e[0]+': '+e[1].state+' ('+(e[1].problems||[]).join('; ')+')').join(' | '))+'</div>':'')+((t.evidence||[]).length?'<details><summary class=muted>evidence ('+t.evidence.length+')</summary>'+t.evidence.map(e=>'<div class=muted>'+esc(e.step)+' · '+esc(e.node)+' · '+(e.ok?'OK':'FAILED')+(e.problems?' · '+esc(e.problems.join('; ')):'')+'</div>').join('')+'</details>':'')+'</td><td>'+actions+'</td></tr>'}).join('')}
async function loadAll(){await loadNodes();await Promise.all([loadMonitoring(),loadJobs(),loadFinance(),loadAudit(),loadTunnels(),loadHealth(),loadDiscovery(),loadPathProbes(),loadPathGraph(),loadPathMatrix()])}
const fastPollMS=10000,slowPollMS=30000;let fastPollTimer=null,slowPollTimer=null;
async function loadFast(){if(document.hidden)return;await Promise.all([loadMonitoring(),loadTunnels(),loadHealth()])}
async function loadSlow(){if(document.hidden)return;await Promise.all([loadFinance(),loadDiscovery()])}
function stopPolling(){if(fastPollTimer!==null){clearInterval(fastPollTimer);fastPollTimer=null}if(slowPollTimer!==null){clearInterval(slowPollTimer);slowPollTimer=null}}
function startPolling(){stopPolling();if(document.hidden)return;fastPollTimer=setInterval(loadFast,fastPollMS);slowPollTimer=setInterval(loadSlow,slowPollMS)}
document.addEventListener('visibilitychange',()=>{if(document.hidden){stopPolling();return}loadFast();loadSlow();startPolling()});
initReportDates();loadAll();startPolling();
</script></body></html>`

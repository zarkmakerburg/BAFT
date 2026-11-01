/* BAFT Quick Tunnel — the guided, live way to build a tunnel (v0.5.0).
 *
 * One dialog, five steps: servers → service → connection test → review →
 * deploy & watch. It talks only to BCC's existing reviewed APIs:
 *   GET  /api/nodes, /api/tunnels           inventory
 *   POST /api/path-probes (+ GET ?id=)      signed agent path test
 *   POST /api/tunnels/auto-select           evidence-checked port choice
 *   POST /api/tunnels/plan                  reviewed plan + gates
 *   POST /api/tunnels {plan_hash}           create from the reviewed plan
 *   GET  /api/tunnels?id=                   truthful progress
 * Authority stays in BCC: the browser never signs, never skips review and
 * never reports success that BCC has not recorded (ADR-001). Pure logic is
 * exported for tests; the UI is wired only in a browser.
 */
(function (root) {
  'use strict';

  // ---------- pure logic (unit-tested in web/tests/quick-tunnel-wizard.test.cjs) ----------
  const AGENT_FRESH_MS = 5 * 60 * 1000;
  const TEST_PORTS = [443, 2053, 2083, 2087, 2096, 8443, 9443];
  const INACTIVE = ['superseded', 'decommissioned', 'rolled_back'];
  const TERMINAL_PROBE = ['complete', 'failed', 'cancelled'];
  const IR_ROLES = ['worker', 'master'];

  function nodeOnline(n, now) {
    const seen = Date.parse(n && n.agent_seen || '');
    return Boolean(n && !n.revoked && Number.isFinite(seen) && now - seen <= AGENT_FRESH_MS && now - seen >= -AGENT_FRESH_MS);
  }
  function splitNodes(nodes, now) {
    const usable = (nodes || []).filter(n => n && n.id && !n.revoked);
    const view = n => ({ id: n.id, name: n.alias || n.id, online: nodeOnline(n, now), role: n.role, autoIngress: autoIngressAvailable(n, now), ingressMode: ingressMode(n, now) });
    const byOnline = (a, b) => (b.online - a.online) || a.name.localeCompare(b.name);
    return {
      iran: usable.filter(n => IR_ROLES.includes(n.role)).map(view).sort(byOnline),
      foreign: usable.filter(n => n.role === 'foreign').map(view).sort(byOnline),
    };
  }
  // Automatic public port (operational ingress) is offered only when the Iran
  // server's agent advertised operational-ingress-v1 on a fresh poll. Agents
  // advertise it only where every Apply can actually run; elsewhere the wizard
  // must not promise a public port it can never open (v0.5.0, ADR-001).
  // operational-ingress-host-v1 is ADR-003 safe host mode on an ordinary
  // server: it also needs the operator's per-port acknowledgement, which BCC
  // binds into the reviewed plan (client_port_ack).
  const INGRESS_CAPABILITY = 'operational-ingress-v1';
  const INGRESS_HOST_CAPABILITY = 'operational-ingress-host-v1';
  function ingressMode(n, now) {
    if (!nodeOnline(n, now) || !Array.isArray(n.capabilities)) return '';
    if (n.capabilities.includes(INGRESS_CAPABILITY)) return 'lab';
    if (n.capabilities.includes(INGRESS_HOST_CAPABILITY)) return 'host';
    return '';
  }
  function autoIngressAvailable(n, now) { return ingressMode(n, now) !== ''; }
  function activeTunnels(tunnels) { return (tunnels || []).filter(t => t && !INACTIVE.includes(t.phase)); }
  function endpointPort(e) { const m = String(e || '').match(/:(\d+)$/); return m ? Number(m[1]) : 0; }
  function validPort(v) { const s = String(v ?? '').trim(); const p = Number(s); return /^[0-9]{1,5}$/.test(s) && Number.isInteger(p) && p >= 1 && p <= 65535; }

  // Client (public IR) port: 1–65535, never SSH 22, not already a client port
  // of another live tunnel on the same Iran server.
  function clientPortProblem(port, tunnels, ir) {
    if (!validPort(port)) return 'range';
    if (Number(port) === 22) return 'ssh';
    if (activeTunnels(tunnels).some(t => t.ir_node === ir && Number(t.client_port) === Number(port))) return 'taken';
    return '';
  }
  // Ports to test on the foreign server: common TLS ports plus a preferred
  // one, skipping ports live tunnels already listen on there.
  function candidatePorts(tunnels, ex, preferred) {
    const used = new Set(activeTunnels(tunnels).filter(t => t.ex_node === ex).map(t => Number(t.port)));
    const set = new Set(TEST_PORTS.filter(p => !used.has(p)));
    if (validPort(preferred) && !used.has(Number(preferred))) set.add(Number(preferred));
    return [...set].sort((a, b) => a - b).slice(0, 16);
  }
  // Loopback listeners for an independent instance, mirroring the classic
  // studio's allocation so plans never collide with live tunnels.
  function allocateInternal(tunnels, ex, ir) {
    const used = activeTunnels(tunnels);
    const next = (from, taken) => { for (let p = from; p < 65536; p++) if (!taken(p)) return p; return null; };
    const route = next(1444, p => used.some(t => t.ir_node === ir && t.route_listen === '127.0.0.1:' + p));
    const exMetric = next(9202, p => used.some(t => t.ex_node === ex && (t.ex_metrics_listen || '127.0.0.1:9191') === '127.0.0.1:' + p));
    const irMetric = next(9302, p => used.some(t => t.ir_node === ir && (t.ir_metrics_listen || '127.0.0.1:9191') === '127.0.0.1:' + p));
    if (!route || !exMetric || !irMetric) return null;
    return { route_listen: '127.0.0.1:' + route, ex_metrics_listen: '127.0.0.1:' + exMetric, ir_metrics_listen: '127.0.0.1:' + irMetric };
  }
  function instanceID(rand) {
    const a = rand(2);
    return ('baft-' + Array.from(a).map(v => (v >>> 0).toString(36)).join('-')).slice(0, 34);
  }
  function buildRequest(s) {
    const r = {
      ex_node: s.ex, ir_node: s.ir, port: Number(s.port),
      target: s.target, instance_id: s.instanceID,
      route_listen: s.internal.route_listen, ex_metrics_listen: s.internal.ex_metrics_listen, ir_metrics_listen: s.internal.ir_metrics_listen,
    };
    if (s.clientPort !== '' && s.clientPort !== undefined && s.clientPort !== null) {
      r.client_port = Number(s.clientPort);
      if (s.clientPortAck === true) r.client_port_ack = true;
    }
    const e = s.evidence;
    if (e && e.source_node === s.ir && e.destination_node === s.ex && Number(e.port) === Number(s.port)) r.auto_select_evidence = e;
    return r;
  }
  function targetFor(host, port) { return (String(host || '').includes(':') ? '[' + host + ']' : (host || '127.0.0.1')) + ':' + Number(port); }
  function validTargetHost(h) {
    const s = String(h || '').trim();
    if (/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.test(s)) return s.split('.').every(x => Number(x) <= 255);
    return /^[0-9A-Fa-f:]+$/.test(s) && s.includes(':') && s.length <= 45;
  }

  // Path-probe row state. Only FULL_DATA counts as "passes".
  function probeRow(c, phase) {
    const k = (c && c.class) || '';
    if (k === 'FULL_DATA') return 'ok';
    if (k === 'BYTE_CEILING' || k === 'HANDSHAKE_ONLY' || k === 'CONNECT_ONLY') return 'partial';
    if (k === 'TIMEOUT_UNREACHABLE') return 'blocked';
    if (k === 'LOCAL_CONFLICT') return 'busy';
    if (k) return 'unknown';
    return TERMINAL_PROBE.includes(phase) ? 'untested' : 'pending';
  }
  function probeMetrics(c) {
    const out = [];
    if (c.connect_rtt_ms) out.push(c.connect_rtt_ms + ' ms' + (c.connect_spread_ms ? ' ±' + c.connect_spread_ms : ''));
    if (c.attempts) out.push((c.ack_successes || 0) + '/' + c.attempts);
    if (c.throughput_kbps > 0) out.push('≈ ' + mbps(c.throughput_kbps) + ' Mbps');
    return out.join(' · ');
  }
  function mbps(kbps) { const v = Number(kbps) / 1000; return v >= 10 ? String(Math.round(v)) : v.toFixed(1); }

  // Tunnel progress. Steps are only marked done once BCC's phase is past them.
  const STEPS = [
    ['preparing_ex'], ['preparing_ir'], ['committing_ex', 'committing_ir'],
    ['health_ir', 'health_ex', 'observing_ir', 'observing_ex'], ['finalizing_ir', 'finalizing_ex'], ['active'],
  ];
  function progress(t) {
    const phase = t && t.phase || '';
    const ingress = t && t.ingress || null;
    let at = STEPS.findIndex(s => s.includes(phase));
    const failed = ['rolled_back', 'rollback_failed', 'superseded', 'decommissioned'].includes(phase);
    const rolling = phase === 'rolling_back';
    if (at < 0) at = failed || rolling ? -1 : 0;
    const steps = STEPS.map((_, i) => phase === 'active' ? 'done' : (at < 0 ? 'idle' : i < at ? 'done' : i === at ? 'now' : 'idle'));
    let ingressState = 'none';
    if (phase === 'active') {
      if (!t.client_port) ingressState = 'none';
      else if (ingress && ingress.ready) ingressState = 'verified';
      else if (ingress && ['failed', 'blocked'].includes(ingress.state)) ingressState = 'failed';
      else ingressState = 'pending';
    }
    const outcome = failed ? (phase === 'rollback_failed' ? 'attention' : 'failed') : rolling ? 'rolling' :
      phase === 'active' ? (ingressState === 'verified' || ingressState === 'none' ? 'ready' : ingressState === 'failed' ? 'partial' : 'active') : 'working';
    const done = ['ready', 'partial', 'failed', 'attention'].includes(outcome);
    return { steps, ingressState, outcome, done, error: t && t.error || '', reason: ingress && ingress.reason || '' };
  }
  function gateOrder(gates) { const rank = { FAIL: 0, WARN: 1, PASS: 2 }; return (gates || []).slice().sort((a, b) => (rank[a.status] ?? 3) - (rank[b.status] ?? 3)); }

  const core = Object.freeze({ AGENT_FRESH_MS, TEST_PORTS, INGRESS_CAPABILITY, INGRESS_HOST_CAPABILITY, ingressMode, autoIngressAvailable, nodeOnline, splitNodes, activeTunnels, endpointPort, validPort, clientPortProblem, candidatePorts, allocateInternal, instanceID, buildRequest, targetFor, validTargetHost, probeRow, probeMetrics, mbps, progress, gateOrder });
  if (typeof module === 'object' && module.exports) { module.exports = core; return; }

  // ---------- browser UI ----------
  const doc = root.document;
  const T = {
    en: {
      title: 'New tunnel', close: 'Close', back: 'Back', next: 'Next', steps: ['Servers', 'Service', 'Connection test', 'Review', 'Deploy'],
      signin: 'Sign in to BCC to create a tunnel. Every change is reviewed and signed by your command center.', signinBtn: 'Sign in',
      loading: 'Loading servers…', loadFail: 'Could not load servers from BCC: ',
      serversLead: 'Users connect to the Iran server; BAFT carries their traffic securely to the foreign server.',
      iran: 'Iran server (users connect here)', foreign: 'Foreign server (service runs here)', choose: 'Choose…',
      online: 'online', offline: 'agent offline', noIran: 'No Iran server is enrolled yet. Add one under Servers.', noForeign: 'No foreign server is enrolled yet. Add one under Servers.',
      offlineWarn: 'This server\'s agent has not reported in the last 5 minutes. Jobs will wait until it is back.', samePair: 'Choose two different servers.',
      serviceLead: 'Which service on the foreign server should users reach? For 3X-UI this is the inbound port.',
      servicePort: 'Service port on the foreign server', clientPort: 'Port users connect to on the Iran server', sameAsService: 'Same as the service port (recommended)',
      advanced: 'Advanced', serviceHost: 'Service address on the foreign server', hostHelp: 'Use 127.0.0.1 when the service runs on the foreign server itself.',
      consent: 'Open this port on the Iran server for users once the tunnel is active. BAFT opens only this port, only after an authenticated agent receipt, and verifies it independently.',
      portRange: 'Enter a port from 1 to 65535.', portSSH: 'Port 22 is reserved for SSH management.', portTaken: 'Another tunnel already uses this port on the selected Iran server.', badHost: 'Enter a fixed IPv4 or IPv6 address.',
      testLead: 'BAFT asks both servers, through signed jobs, to test real data on several ports. Only a port that carries full data is used.',
      family: 'Address family', runTest: 'Run connection test', retest: 'Test again', cancel: 'Cancel test', testing: 'Testing… ', starting: 'Starting test…',
      st: { ok: 'Data passes', partial: 'Connects but data is cut', blocked: 'Blocked or unreachable', busy: 'Port busy on foreign server', unknown: 'Not determined', untested: 'Not tested', pending: 'Waiting' },
      chosen: 'Selected port {p} from fresh test evidence. BCC re-checks it during review and deploy.', noneOk: 'No port carried full data. Try IPv6, check the foreign server firewall, or choose a port manually below.',
      testFail: 'Test failed: ', testCancel: 'Test cancelled.', selectFail: 'No port could be selected: ', manualPort: 'Or use this tunnel port without test evidence', useManual: 'Use this port', manualNote: 'Using port {p} without test evidence; review will show the gates.',
      needPort: 'Run the test or choose a tunnel port to continue.', review: 'Review plan', planning: 'Asking BCC for a reviewed plan…',
      planOk: 'BCC reviewed this plan. Nothing has changed on any server yet.', planBad: 'BCC refused this plan. Fix the items marked FAIL, then review again.',
      summary: 'Summary', route: 'Route', tunnelPort: 'Tunnel port (foreign)', service: 'Service', public: 'Public port (Iran)', gates: 'Safety checks', changes: 'What will change',
      deploy: 'Deploy', deploying: 'Deploying…', changed: 'The situation changed since review (for example a port was taken). Review again.', deployFail: 'BCC refused to deploy: ',
      prog: ['Prepare foreign server', 'Pair Iran server', 'Install on both servers', 'Health checks', 'Finalize', 'Tunnel active'], ingress: 'Public port on Iran verified',
      out: { working: 'Building the tunnel. You can close this window; progress continues in BCC.', active: 'Tunnel is active. Waiting for the public port to be opened and verified…', ready: 'Done. The tunnel is active and every required check passed.', partial: 'The tunnel is active, but the public port is not verified: ', failed: 'The tunnel was not built and BAFT rolled back its changes.', attention: 'Rollback did not complete. Open the tunnel list and review it before retrying.', rolling: 'A step failed. BAFT is rolling back its own changes…' },
      finish: 'Close', another: 'Create another',
      manualLead: 'Automatic public port is not available on this Iran server: its agent does not report it. To let BAFT open it, re-run the installer on the Iran server with --enable-host-ingress (safe host mode). The tunnel works either way — it listens on the Iran server itself, and you can point your panel at it.',
      hostAck: 'No other service on this Iran server is configured to use port {p}. BAFT itself refuses ports held by a running program, a systemd socket, another BAFT tunnel, an Xray/3X-UI inbound, an nginx listen, an HAProxy bind or a Docker published port, and never stops or edits another service; it cannot see programs started by hand or other services\' own settings.',
      hostLead: 'Safe host mode: BAFT opens only this port, with its own listener and its own firewall rule, and removes exactly those when the tunnel is removed.',
      manualHow: 'After deploy: in your Iran panel (for example a 3X-UI "dokodemo-door" inbound or a port forward), forward the public port users connect to → {p} on the Iran server.',
      reach: 'Tunnel entry on the Iran server', readyManual: 'Done. The tunnel is active. Point your public inbound on the Iran server to {p}.',
    },
    fa: {
      title: 'تونل جدید', close: 'بستن', back: 'قبلی', next: 'بعدی', steps: ['سرورها', 'سرویس', 'آزمون اتصال', 'بررسی', 'ساخت'],
      signin: 'برای ساخت تونل وارد مرکز فرمان شوید. هر تغییر را مرکز فرمان شما بررسی و امضا می‌کند.', signinBtn: 'ورود',
      loading: 'در حال دریافت فهرست سرورها…', loadFail: 'فهرست سرورها از مرکز فرمان دریافت نشد: ',
      serversLead: 'کاربران به سرور ایران وصل می‌شوند و BAFT ترافیکشان را امن به سرور خارج می‌رساند.',
      iran: 'سرور ایران (محل اتصال کاربران)', foreign: 'سرور خارج (محل اجرای سرویس)', choose: 'انتخاب کنید…',
      online: 'آنلاین', offline: 'عامل آفلاین', noIran: 'هنوز سرور ایرانی ثبت نشده است. از بخش سرورها اضافه کنید.', noForeign: 'هنوز سرور خارجی ثبت نشده است. از بخش سرورها اضافه کنید.',
      offlineWarn: 'عامل این سرور در ۵ دقیقهٔ گذشته گزارش نداده است. کارها تا بازگشت آن منتظر می‌مانند.', samePair: 'دو سرور متفاوت انتخاب کنید.',
      serviceLead: 'کاربران باید به کدام سرویس روی سرور خارج برسند؟ در 3X-UI همان پورت inbound است.',
      servicePort: 'پورت سرویس روی سرور خارج', clientPort: 'پورتی که کاربران روی سرور ایران به آن وصل می‌شوند', sameAsService: 'برابر پورت سرویس (پیشنهادی)',
      advanced: 'پیشرفته', serviceHost: 'نشانی سرویس روی سرور خارج', hostHelp: 'اگر سرویس روی خود سرور خارج اجرا می‌شود 127.0.0.1 را نگه دارید.',
      consent: 'پس از فعال‌شدن تونل، این پورت روی سرور ایران برای کاربران باز شود. BAFT فقط همین پورت را، فقط پس از رسید احراز هویت‌شدهٔ عامل، باز می‌کند و جداگانه تأیید می‌کند.',
      portRange: 'پورتی بین ۱ تا ۶۵۵۳۵ وارد کنید.', portSSH: 'پورت ۲۲ برای مدیریت SSH رزرو است.', portTaken: 'تونل دیگری روی سرور ایران انتخاب‌شده از این پورت استفاده می‌کند.', badHost: 'یک نشانی ثابت IPv4 یا IPv6 وارد کنید.',
      testLead: 'BAFT با کارهای امضاشده از هر دو سرور می‌خواهد دادهٔ واقعی را روی چند پورت آزمون کنند. فقط پورتی که دادهٔ کامل را عبور دهد استفاده می‌شود.',
      family: 'نوع نشانی', runTest: 'اجرای آزمون اتصال', retest: 'آزمون دوباره', cancel: 'لغو آزمون', testing: 'در حال آزمون… ', starting: 'در حال شروع آزمون…',
      st: { ok: 'داده عبور می‌کند', partial: 'وصل می‌شود ولی داده قطع می‌شود', blocked: 'مسدود یا غیرقابل دسترس', busy: 'پورت روی سرور خارج اشغال است', unknown: 'نامشخص', untested: 'آزمون نشد', pending: 'در انتظار' },
      chosen: 'پورت {p} از شواهد تازهٔ آزمون انتخاب شد. مرکز فرمان هنگام بررسی و ساخت دوباره آن را می‌سنجد.', noneOk: 'هیچ پورتی دادهٔ کامل را عبور نداد. IPv6 را امتحان کنید، فایروال سرور خارج را بررسی کنید یا پورت را دستی انتخاب کنید.',
      testFail: 'آزمون ناموفق بود: ', testCancel: 'آزمون لغو شد.', selectFail: 'پورتی انتخاب نشد: ', manualPort: 'یا از این پورت تونل بدون شواهد آزمون استفاده شود', useManual: 'استفاده از این پورت', manualNote: 'پورت {p} بدون شواهد آزمون استفاده می‌شود؛ نتیجهٔ بررسی‌ها در مرحلهٔ بعد نمایش داده می‌شود.',
      needPort: 'برای ادامه آزمون را اجرا کنید یا پورت تونل را انتخاب کنید.', review: 'بررسی طرح', planning: 'در حال دریافت طرح بررسی‌شده از مرکز فرمان…',
      planOk: 'مرکز فرمان این طرح را بررسی کرد. هنوز هیچ تغییری روی سرورها انجام نشده است.', planBad: 'مرکز فرمان این طرح را نپذیرفت. موارد FAIL را اصلاح و دوباره بررسی کنید.',
      summary: 'خلاصه', route: 'مسیر', tunnelPort: 'پورت تونل (خارج)', service: 'سرویس', public: 'پورت عمومی (ایران)', gates: 'بررسی‌های ایمنی', changes: 'تغییراتی که انجام می‌شود',
      deploy: 'ساخت تونل', deploying: 'در حال ساخت…', changed: 'وضعیت پس از بررسی تغییر کرده است (مثلاً پورتی اشغال شده). دوباره بررسی کنید.', deployFail: 'مرکز فرمان ساخت را نپذیرفت: ',
      prog: ['آماده‌سازی سرور خارج', 'جفت‌سازی سرور ایران', 'نصب روی هر دو سرور', 'بررسی سلامت', 'نهایی‌سازی', 'تونل فعال'], ingress: 'پورت عمومی ایران تأیید شد',
      out: { working: 'در حال ساخت تونل. می‌توانید این پنجره را ببندید؛ کار در مرکز فرمان ادامه دارد.', active: 'تونل فعال است. در انتظار باز و تأییدشدن پورت عمومی…', ready: 'انجام شد. تونل فعال است و همهٔ بررسی‌های لازم موفق بود.', partial: 'تونل فعال است ولی پورت عمومی تأیید نشده است: ', failed: 'تونل ساخته نشد و BAFT تغییرات خود را برگرداند.', attention: 'برگرداندن کامل نشد. پیش از تلاش دوباره، تونل را در فهرست بررسی کنید.', rolling: 'یکی از مراحل ناموفق بود؛ BAFT در حال برگرداندن تغییرات خود است…' },
      finish: 'بستن', another: 'ساخت تونل دیگر',
      manualLead: 'باز کردن خودکار پورت عمومی روی این سرور ایران در دسترس نیست: عامل آن چنین توانایی‌ای گزارش نمی‌کند. برای اینکه BAFT خودش پورت را باز کند، نصب‌کننده را روی سرور ایران با گزینهٔ --enable-host-ingress (حالت امن میزبان) دوباره اجرا کنید. تونل در هر حال کار می‌کند؛ روی خود سرور ایران گوش می‌دهد و می‌توانید پنل را به آن وصل کنید.',
      hostAck: 'هیچ سرویس دیگری روی این سرور ایران برای استفاده از پورت {p} تنظیم نشده است. BAFT خودش پورتی را که برنامهٔ در حال اجرا، سوکت systemd، تونل دیگر BAFT، inbound در Xray/3X-UI، listen در nginx، bind در HAProxy یا پورت منتشرشدهٔ Docker گرفته باشد رد می‌کند و هیچ سرویس دیگری را متوقف یا ویرایش نمی‌کند؛ برنامه‌هایی که دستی اجرا شده‌اند و تنظیمات اختصاصی سرویس‌های دیگر را نمی‌تواند ببیند.',
      hostLead: 'حالت امن میزبان: BAFT فقط همین پورت را با شنونده و قانون فایروال مخصوص خودش باز می‌کند و هنگام حذف تونل دقیقاً همان‌ها را برمی‌دارد.',
      manualHow: 'پس از ساخت: در پنل سرور ایران (مثلاً inbound از نوع dokodemo-door در 3X-UI یا یک port forward) پورت عمومی کاربران را به {p} روی همان سرور ایران هدایت کنید.',
      reach: 'ورودی تونل روی سرور ایران', readyManual: 'انجام شد. تونل فعال است. inbound عمومی سرور ایران را به {p} هدایت کنید.',
    },
  };
  const lang = () => (doc.documentElement.lang === 'fa' ? 'fa' : 'en');
  const t = k => T[lang()][k];
  const fmt = (s, p) => String(s).replace('{p}', p);
  const cfg = () => root.BAFT_UI || {};
  const live = () => cfg().mode === 'dashboard';

  async function api(path, opts = {}) {
    const c = cfg(), method = (opts.method || 'GET').toUpperCase(), headers = { Accept: 'application/json' };
    if (method !== 'GET') { headers['Content-Type'] = 'application/json'; if (c.csrfHeader) headers[c.csrfHeader] = c.csrf || ''; }
    const r = await root.fetch((c.base || '/') + 'api/' + path, { method, headers, body: opts.body === undefined ? undefined : JSON.stringify(opts.body), credentials: 'same-origin', redirect: 'error' });
    const text = await r.text();
    let data = null; try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
    return { ok: r.ok, status: r.status, data, text: text.trim() };
  }

  let box = null, returnFocus = null, prevOverflow = null, timer = null, seq = 0;
  let s = null;
  function fresh() {
    return { step: 0, nodes: null, tunnels: [], loadError: '', ex: '', ir: '', servicePort: '', clientPort: '', clientSame: true, host: '127.0.0.1', consent: false, ack: false,
      family: '4', probe: null, testNote: '', port: '', evidence: null, manual: '', plan: null, planReq: null, planNote: '', planBusy: false,
      instanceID: '', internal: null, tunnel: null, deployNote: '', deployBusy: false };
  }
  function el(tag, attrs, ...kids) {
    const e = doc.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') e.className = v; else if (k === 'text') e.textContent = v; else if (k.startsWith('on')) e.addEventListener(k.slice(2), v); else e.setAttribute(k, v === true ? '' : v);
    }
    for (const k of kids.flat()) if (k !== null && k !== undefined && k !== false) e.append(k.nodeType ? k : doc.createTextNode(String(k)));
    return e;
  }
  const stopTimer = () => { if (timer) { root.clearTimeout(timer); timer = null; } };

  function close() {
    if (!box) return; stopTimer(); seq++; box.remove(); box = null;
    doc.removeEventListener('keydown', onKey); doc.removeEventListener('focusin', onFocus);
    if (prevOverflow !== null) { doc.body.style.overflow = prevOverflow; prevOverflow = null; }
    if (returnFocus && returnFocus.isConnected) returnFocus.focus(); returnFocus = null;
    if (s && s.tunnel) root.dispatchEvent(new root.CustomEvent('baft:tunnels-changed'));
  }
  function focusables() { return Array.from(box.querySelectorAll('button,select,input,a[href],[tabindex]')).filter(x => !x.disabled && x.tabIndex >= 0 && x.getClientRects().length > 0); }
  function onFocus(e) { if (box && !box.contains(e.target)) { const f = focusables()[0]; if (f) f.focus(); } }
  function onKey(e) {
    if (!box) return;
    if (e.key === 'Escape') { e.preventDefault(); close(); return; }
    if (e.key !== 'Tab') return;
    const a = focusables(); if (!a.length) { e.preventDefault(); return; }
    const first = a[0], last = a[a.length - 1];
    if (e.shiftKey && doc.activeElement === first) { e.preventDefault(); last.focus(); } else if (!e.shiftKey && doc.activeElement === last) { e.preventDefault(); first.focus(); }
  }

  function render() {
    if (!box) return;
    const act = doc.activeElement && box.contains(doc.activeElement) ? doc.activeElement : null;
    const keep = act ? act.getAttribute('data-k') : null;
    let caret = null; try { if (act && typeof act.selectionStart === 'number') caret = [act.selectionStart, act.selectionEnd]; } catch (e) { caret = null; }
    const d = box.firstElementChild; d.replaceChildren(); d.setAttribute('dir', doc.documentElement.dir || 'auto');
    const head = el('h2', { class: 'baw-title', id: 'baw-heading', text: t('title') + ' — ' + t('steps')[s.step] });
    d.append(el('header', { class: 'baw-header' }, head, el('button', { type: 'button', class: 'baw-close', text: t('close'), onclick: close })));
    const prog = el('ol', { class: 'baw-progress qt-steps', 'aria-label': t('title') });
    t('steps').forEach((name, i) => prog.append(el('li', { 'aria-current': i === s.step ? 'step' : null, 'data-complete': i < s.step ? 'true' : null }, el('span', { class: 'qt-dot', 'aria-hidden': 'true' }), el('span', { class: 'qt-step-name', text: name }))));
    d.append(prog);
    const body = el('div', { class: 'baw-body', id: 'baw-content', 'aria-live': 'polite' });
    if (!live()) body.append(signinView());
    else if (!s.nodes && !s.loadError) body.append(el('p', { role: 'status', text: t('loading') }));
    else if (s.loadError) body.append(el('p', { role: 'alert', text: t('loadFail') + s.loadError }));
    else body.append([serversView, serviceView, testView, reviewView, deployView][s.step]());
    d.append(body);
    d.append(footer());
    const target = (keep && box.querySelector('[data-k="' + keep + '"]')) || null;
    if (target) { target.focus(); if (caret) { try { target.setSelectionRange(caret[0], caret[1]); } catch (e) { /* not a text field */ } } }
  }

  function signinView() {
    const c = cfg();
    return el('div', {}, el('p', { text: t('signin') }), el('p', {}, el('a', { class: 'qt-primary', href: (c.base || '/') + 'login', 'data-k': 'signin', text: t('signinBtn') })));
  }
  function nodeSelect(key, list, label, empty) {
    if (!list.length) return el('p', { role: 'alert', text: empty });
    const sel = el('select', { 'data-k': key, name: key, onchange: e => { s[key] = e.target.value; resetAfterServers(); render(); } },
      el('option', { value: '', text: t('choose') }),
      list.map(n => el('option', { value: n.id, selected: s[key] === n.id, text: n.name + (n.name !== n.id ? ' (' + n.id + ')' : '') + ' — ' + (n.online ? t('online') : t('offline')) })));
    const picked = list.find(n => n.id === s[key]);
    return el('label', {}, el('span', { text: label }), sel, picked && !picked.online ? el('small', { class: 'qt-txt-warn', text: t('offlineWarn') }) : null);
  }
  function resetAfterServers() { s.ack = false; s.probe = null; s.testNote = ''; s.port = ''; s.evidence = null; s.plan = null; s.planReq = null; s.planNote = ''; stopTimer(); seq++; }
  function serversView() {
    const g = splitNodes(s.nodes, Date.now());
    const same = s.ir && s.ex && s.ir === s.ex;
    return el('div', {}, el('p', { class: 'qt-lead', text: t('serversLead') }),
      el('div', { class: 'qt-route', 'aria-hidden': 'true', dir: 'ltr' }, el('span', { text: '👥' }), el('span', { text: '→' }), el('span', { text: '🇮🇷 IR' }), el('span', { class: 'qt-lock', text: '🔒 BAFT' }), el('span', { text: '🌍 EX' }), el('span', { text: '→' }), el('span', { text: '⚙︎' })),
      nodeSelect('ir', g.iran, t('iran'), t('noIran')), nodeSelect('ex', g.foreign, t('foreign'), t('noForeign')),
      same ? el('p', { role: 'alert', text: t('samePair') }) : null);
  }
  function serviceProblems() {
    const out = {};
    if (!validPort(s.servicePort)) out.service = t('portRange');
    if (!validTargetHost(s.host)) out.host = t('badHost');
    if (!irAuto()) return out;
    const cp = s.clientSame ? s.servicePort : s.clientPort;
    const p = clientPortProblem(cp, s.tunnels, s.ir);
    if (p) out.client = p === 'ssh' ? t('portSSH') : p === 'taken' ? t('portTaken') : t('portRange');
    return out;
  }
  function field(key, label, value, problem, attrs) {
    const input = el('input', Object.assign({ 'data-k': key, name: key, value, inputmode: 'numeric', autocomplete: 'off', maxlength: '5', 'aria-invalid': problem && value ? 'true' : 'false', oninput: e => { s[key] = e.target.value.trim(); if (key === 'servicePort' || key === 'clientPort') s.ack = false; s.plan = null; render(); } }, attrs || {}));
    return el('label', {}, el('span', { text: label }), input, problem && value !== '' ? el('small', { role: 'alert', class: 'qt-err', text: problem }) : null);
  }
  function serviceView() {
    const pr = serviceProblems();
    const host = el('input', { 'data-k': 'host', name: 'host', value: s.host, dir: 'ltr', autocomplete: 'off', spellcheck: 'false', maxlength: '45', 'aria-invalid': pr.host ? 'true' : 'false', oninput: e => { s.host = e.target.value.trim(); s.plan = null; render(); } });
    const adv = el('details', { class: 'qt-adv' }, el('summary', { text: t('advanced') }), el('label', {}, el('span', { text: t('serviceHost') }), host, el('small', { text: t('hostHelp') }), pr.host ? el('small', { role: 'alert', class: 'qt-err', text: pr.host }) : null));
    if (!irAuto()) {
      return el('div', {}, el('p', { class: 'qt-lead', text: t('serviceLead') }),
        field('servicePort', t('servicePort'), s.servicePort, pr.service, { placeholder: '2096' }), adv,
        el('p', { class: 'qt-note', role: 'note', 'data-k': 'manualLead', text: t('manualLead') }));
    }
    return el('div', {}, el('p', { class: 'qt-lead', text: t('serviceLead') }),
      field('servicePort', t('servicePort'), s.servicePort, pr.service, { placeholder: '2096' }),
      el('label', { class: 'qt-check' }, el('input', { type: 'checkbox', 'data-k': 'clientSame', checked: s.clientSame, onchange: e => { s.clientSame = e.target.checked; s.ack = false; s.plan = null; render(); } }), el('span', { text: t('sameAsService') })),
      s.clientSame ? (pr.client && s.servicePort ? el('small', { role: 'alert', class: 'qt-err', text: pr.client }) : null) : field('clientPort', t('clientPort'), s.clientPort, pr.client),
      el('details', { class: 'qt-adv' }, el('summary', { text: t('advanced') }), el('label', {}, el('span', { text: t('serviceHost') }), host, el('small', { text: t('hostHelp') }), pr.host ? el('small', { role: 'alert', class: 'qt-err', text: pr.host }) : null)),
      el('label', { class: 'qt-check qt-consent' }, el('input', { type: 'checkbox', 'data-k': 'consent', checked: s.consent, onchange: e => { s.consent = e.target.checked; s.plan = null; render(); } }), el('span', { text: t('consent') })),
      irMode() === 'host' ? el('p', { class: 'qt-note', role: 'note', 'data-k': 'hostLead', text: t('hostLead') }) : null,
      irMode() === 'host' ? el('label', { class: 'qt-check qt-consent' }, el('input', { type: 'checkbox', 'data-k': 'hostAck', checked: s.ack, onchange: e => { s.ack = e.target.checked; s.plan = null; render(); } }), el('span', { text: fmt(t('hostAck'), (s.clientSame ? s.servicePort : s.clientPort) || '…') })) : null);
  }

  // ----- connection test -----
  async function startTest() {
    stopTimer(); const my = ++seq;
    s.probe = null; s.port = ''; s.evidence = null; s.plan = null; s.testNote = t('starting'); render();
    const body = { source_node: s.ir, destination_node: s.ex, family: s.family, candidate_ports: candidatePorts(s.tunnels, s.ex, s.manual) };
    let r; try { r = await api('path-probes', { method: 'POST', body }); } catch (e) { r = { ok: false, text: e.message }; }
    if (my !== seq) return;
    if (!r.ok || !r.data) { s.testNote = t('testFail') + (r.text || r.status); render(); return; }
    s.probe = r.data; s.testNote = ''; render(); poll(my, 0);
  }
  function poll(my, n) {
    timer = root.setTimeout(async () => {
      timer = null; if (my !== seq || !s.probe) return;
      let r; try { r = await api('path-probes?id=' + encodeURIComponent(s.probe.id)); } catch (e) { r = { ok: false }; }
      if (my !== seq) return;
      if (r.ok && r.data) { s.probe = r.data; render(); }
      const phase = s.probe.phase;
      if (!TERMINAL_PROBE.includes(phase)) { if (n < 200) poll(my, n + 1); return; }
      if (phase === 'failed') { s.testNote = t('testFail') + (s.probe.error || ''); render(); return; }
      if (phase === 'cancelled') { s.testNote = t('testCancel'); render(); return; }
      if (!(s.probe.candidates || []).some(c => c.class === 'FULL_DATA')) { s.testNote = t('noneOk'); render(); return; }
      await choosePort(my);
    }, 3000);
  }
  async function choosePort(my) {
    let r; try { r = await api('tunnels/auto-select', { method: 'POST', body: { ex_node: s.ex, ir_node: s.ir, family: s.family, carrier: 'h2' } }); } catch (e) { r = { ok: false, text: e.message }; }
    if (my !== seq) return;
    const sel = r.ok && r.data && r.data.selected;
    if (sel) { s.port = String(sel.port); s.evidence = sel; s.testNote = fmt(t('chosen'), sel.port); }
    else s.testNote = t('selectFail') + ((r.data && r.data.reason) || r.text || '');
    render();
  }
  async function cancelTest() {
    if (!s.probe) return; const my = seq;
    const r = await api('path-probes/cancel', { method: 'POST', body: { id: s.probe.id } });
    if (my === seq && r.ok && r.data) { s.probe = r.data; render(); }
  }
  function testView() {
    const running = s.probe && !TERMINAL_PROBE.includes(s.probe.phase);
    const fam = el('select', { 'data-k': 'family', disabled: running, onchange: e => { s.family = e.target.value; } }, el('option', { value: '4', selected: s.family === '4', text: 'IPv4' }), el('option', { value: '6', selected: s.family === '6', text: 'IPv6' }));
    const rows = s.probe ? el('ul', { class: 'qt-board' }, (s.probe.candidates || []).map(c => {
      const k = probeRow(c, s.probe.phase);
      return el('li', { class: 'qt-row qt-' + k, 'data-state': k }, el('span', { class: 'qt-led', 'aria-hidden': 'true' }), el('b', { dir: 'ltr', text: String(c.port) }), el('span', { class: 'qt-label', text: t('st')[k] }), el('span', { class: 'qt-metric', dir: 'ltr', text: probeMetrics(c) }));
    })) : null;
    const done = s.probe ? (s.probe.candidates || []).filter(c => c.class).length + '/' + (s.probe.candidates || []).length : '';
    const manual = el('div', { class: 'qt-manual' }, el('label', {}, el('span', { text: t('manualPort') }), el('input', { 'data-k': 'manual', value: s.manual, inputmode: 'numeric', maxlength: '5', autocomplete: 'off', oninput: e => { s.manual = e.target.value.trim(); } })),
      el('button', { type: 'button', 'data-k': 'useManual', onclick: () => { if (validPort(s.manual)) { stopTimer(); seq++; s.port = String(Number(s.manual)); s.evidence = null; s.plan = null; s.testNote = fmt(t('manualNote'), s.port); render(); } }, text: t('useManual') }));
    return el('div', {}, el('p', { class: 'qt-lead', text: t('testLead') }),
      el('div', { class: 'qt-inline' }, el('label', {}, el('span', { text: t('family') }), fam),
        running ? el('button', { type: 'button', 'data-k': 'cancelTest', onclick: cancelTest, text: t('cancel') }) : el('button', { type: 'button', class: 'qt-primary', 'data-k': 'runTest', onclick: startTest, text: s.probe ? t('retest') : t('runTest') })),
      running ? el('p', { role: 'status', text: t('testing') + done }) : null,
      rows, s.testNote ? el('p', { role: 'status', class: s.port ? 'qt-good' : '', text: s.testNote }) : null,
      el('details', { class: 'qt-adv', open: Boolean(s.probe && !running && !s.port) || null }, el('summary', { text: t('advanced') }), manual));
  }

  // ----- review -----
  async function makePlan() {
    if (!s.instanceID) s.instanceID = instanceID(n => root.crypto.getRandomValues(new Uint32Array(n)));
    s.internal = allocateInternal(s.tunnels, s.ex, s.ir);
    s.target = targetFor(s.host, s.servicePort); s.clientPortFinal = irAuto() ? (s.clientSame ? s.servicePort : s.clientPort) : '';
    const req = buildRequest({ ex: s.ex, ir: s.ir, port: s.port, clientPort: s.clientPortFinal, clientPortAck: irMode() === 'host' && s.ack, target: s.target, instanceID: s.instanceID, internal: s.internal, evidence: s.evidence });
    s.plan = null; s.planReq = null; s.planBusy = true; s.planNote = t('planning'); render();
    const my = seq;
    let r; try { r = await api('tunnels/plan', { method: 'POST', body: req }); } catch (e) { r = { ok: false, text: e.message }; }
    if (my !== seq) return;
    s.planBusy = false;
    if (!r.ok || !r.data) { s.planNote = t('planBad') + ' ' + (r.text || r.status); render(); return; }
    s.plan = r.data; s.planReq = req; s.planNote = s.plan.ok ? t('planOk') : t('planBad'); render();
  }
  function irAuto() { return irMode() !== ''; }
  function irMode() { const n = splitNodes(s.nodes, Date.now()).iran.find(x => x.id === s.ir); return (n && n.ingressMode) || ''; }
  function nodeName(id) { const n = (s.nodes || []).find(x => x.id === id); return n && n.alias ? n.alias + ' (' + id + ')' : id; }
  function reviewView() {
    if (!s.plan && !s.planBusy && !s.planNote) { root.setTimeout(makePlan, 0); }
    const p = s.plan;
    const kv = (k, v) => el('div', { class: 'qt-kv' }, el('span', { text: k }), el('b', { dir: 'ltr', text: v }));
    const sum = el('div', { class: 'qt-summary' }, el('h3', { text: t('summary') }),
      kv(t('route'), nodeName(s.ir) + ' → ' + nodeName(s.ex)),
      s.clientPortFinal ? kv(t('public'), String(s.clientPortFinal)) : (s.internal ? kv(t('reach'), s.internal.route_listen) : null),
      kv(t('tunnelPort'), String(s.port)), kv(t('service'), targetFor(s.host, s.servicePort)));
    const how = !s.clientPortFinal && s.internal ? el('p', { class: 'qt-note', role: 'note', text: fmt(t('manualHow'), s.internal.route_listen) }) : null;
    const gates = p ? el('div', {}, el('h3', { text: t('gates') }), el('ul', { class: 'qt-gates' }, gateOrder(p.gates).map(g => el('li', { class: 'qt-gate qt-g-' + String(g.status).toLowerCase(), 'data-status': g.status }, el('span', { class: 'qt-badge', text: g.status }), el('span', {}, el('b', { text: g.name }), g.detail ? el('small', { text: g.detail }) : null))))) : null;
    const changes = p && (p.steps || []).length ? el('details', { class: 'qt-adv' }, el('summary', { text: t('changes') }), el('ol', {}, p.steps.map(x => el('li', { text: x })))) : null;
    return el('div', {}, sum, how, s.planNote ? el('p', { role: p && !p.ok ? 'alert' : 'status', class: p && p.ok ? 'qt-good' : '', text: s.planNote }) : null, gates, changes, s.deployNote ? el('p', { role: 'alert', text: s.deployNote }) : null);
  }

  // ----- deploy & watch -----
  async function deploy() {
    if (!s.plan || !s.plan.ok || s.deployBusy) return;
    s.deployBusy = true; s.deployNote = ''; render();
    const my = seq;
    let r; try { r = await api('tunnels', { method: 'POST', body: Object.assign({}, s.planReq, { plan_hash: s.plan.hash }) }); } catch (e) { r = { ok: false, text: e.message }; }
    if (my !== seq) return;
    s.deployBusy = false;
    if (r.status === 409) { s.plan = null; s.planNote = ''; s.deployNote = t('changed'); render(); return; }
    if (!r.ok || !r.data || !r.data.id) { s.deployNote = t('deployFail') + (r.text || r.status); render(); return; }
    s.tunnel = r.data; s.step = 4; render(); watch(my, 0);
  }
  function watch(my, n) {
    timer = root.setTimeout(async () => {
      timer = null; if (my !== seq || !s.tunnel) return;
      let r; try { r = await api('tunnels?id=' + encodeURIComponent(s.tunnel.id)); } catch (e) { r = { ok: false }; }
      if (my !== seq) return;
      if (r.ok && r.data) { s.tunnel = r.data; render(); }
      if (!progress(s.tunnel).done && n < 400) watch(my, n + 1);
    }, 3000);
  }
  function deployView() {
    const pr = progress(s.tunnel);
    const items = t('prog').map((name, i) => el('li', { class: 'qt-p qt-p-' + pr.steps[i], 'data-state': pr.steps[i] }, el('span', { class: 'qt-dot', 'aria-hidden': 'true' }), el('span', { text: name })));
    if (s.tunnel && s.tunnel.client_port) items.push(el('li', { class: 'qt-p qt-p-' + (pr.ingressState === 'verified' ? 'done' : pr.ingressState === 'pending' ? 'now' : pr.ingressState === 'failed' ? 'bad' : 'idle'), 'data-state': pr.ingressState }, el('span', { class: 'qt-dot', 'aria-hidden': 'true' }), el('span', { text: t('ingress') + ' (' + s.tunnel.client_port + ')' })));
    const entry = s.tunnel && !s.tunnel.client_port ? (s.tunnel.route_listen || (s.internal && s.internal.route_listen) || '') : '';
    const msg = pr.outcome === 'ready' && entry ? fmt(t('readyManual'), entry) : t('out')[pr.outcome] + (pr.outcome === 'partial' ? pr.reason : '');
    return el('div', {}, el('p', { class: 'qt-kv' }, el('span', { text: nodeName(s.ir) + ' → ' + nodeName(s.ex) }), el('b', { dir: 'ltr', text: (s.tunnel && s.tunnel.id) || '' })), el('ol', { class: 'qt-progress' }, items),
      el('p', { role: ['failed', 'attention', 'partial'].includes(pr.outcome) ? 'alert' : 'status', class: pr.outcome === 'ready' ? 'qt-good' : '', text: msg }),
      pr.error && pr.outcome !== 'ready' ? el('p', { class: 'qt-err', text: pr.error }) : null);
  }

  function canNext() {
    if (s.step === 0) return Boolean(s.ir && s.ex && s.ir !== s.ex);
    if (s.step === 1) return Object.keys(serviceProblems()).length === 0 && (!irAuto() || (s.consent && (irMode() !== 'host' || s.ack)));
    if (s.step === 2) return validPort(s.port);
    return false;
  }
  function footer() {
    const f = el('footer', { class: 'baw-actions' });
    if (!live() || !s.nodes) return f;
    if (s.step === 4) {
      const pr = progress(s.tunnel);
      f.append(el('button', { type: 'button', 'data-k': 'another', disabled: !pr.done || null, onclick: () => { stopTimer(); seq++; const keepN = s.nodes, keepT = s.tunnels; s = fresh(); s.nodes = keepN; s.tunnels = keepT; load(); render(); }, text: t('another') }),
        el('button', { type: 'button', class: 'qt-primary', 'data-k': 'finish', onclick: close, text: t('finish') }));
      return f;
    }
    f.append(el('button', { type: 'button', 'data-k': 'back', disabled: s.step === 0 || s.deployBusy || null, onclick: () => { stopTimer(); seq++; if (s.step === 3) { s.plan = null; s.planNote = ''; s.deployNote = ''; } s.step--; render(); }, text: t('back') }));
    if (s.step === 3) f.append(el('button', { type: 'button', class: 'qt-primary', 'data-k': 'deploy', disabled: !(s.plan && s.plan.ok) || s.deployBusy || null, onclick: deploy, text: s.deployBusy ? t('deploying') : t('deploy') }));
    else f.append(el('button', { type: 'button', class: 'qt-primary', 'data-k': 'next', 'data-baw-next': 'true', disabled: !canNext() || null, onclick: () => { if (!canNext()) return; if (s.step === 2) { s.plan = null; s.planNote = ''; s.deployNote = ''; } s.step++; render(); }, text: s.step === 2 ? t('review') : t('next') }));
    if (s.step === 2 && !validPort(s.port)) f.append(el('small', { class: 'qt-hint', text: t('needPort') }));
    return f;
  }

  async function load() {
    const my = seq;
    try {
      const [n, tl] = await Promise.all([api('nodes'), api('tunnels')]);
      if (my !== seq || !box) return;
      if (!n.ok) { s.loadError = n.text || String(n.status); render(); return; }
      s.nodes = Array.isArray(n.data) ? n.data : []; s.tunnels = tl.ok && Array.isArray(tl.data) ? tl.data : [];
    } catch (e) { if (my === seq) s.loadError = e.message; }
    render();
  }
  function open() {
    if (box) return;
    returnFocus = doc.activeElement; s = fresh(); seq++;
    prevOverflow = doc.body.style.overflow; doc.body.style.overflow = 'hidden';
    box = el('div', { class: 'baw-backdrop' });
    const dialog = el('section', { class: 'baw-dialog qt', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'baw-heading' });
    box.append(dialog); doc.body.append(box);
    box.addEventListener('click', e => { if (e.target === box) close(); });
    doc.addEventListener('keydown', onKey); doc.addEventListener('focusin', onFocus);
    render(); const f = focusables()[0]; if (f) f.focus();
    if (live()) load();
  }
  root.BAFTActionWizard = Object.freeze({ open, close, isOpen: () => Boolean(box), version: '2.0-live' });
  doc.addEventListener('click', e => { const b = e.target.closest && e.target.closest('[data-baw-open]'); if (b) { e.preventDefault(); open(); } });
  const launch = doc.getElementById('baw-open');
  if (launch) { launch.addEventListener('click', open); launch.setAttribute('aria-haspopup', 'dialog'); }
}(typeof window !== 'undefined' ? window : globalThis));

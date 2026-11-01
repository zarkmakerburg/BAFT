'use strict';
// Dependency-free browser walk-through of the Quick Tunnel wizard against a
// scripted mock BCC (no real BCC, agents or hosts). It proves the UI flow,
// request shapes, CSRF use and truthful states in a real Chromium; it is NOT
// evidence that a tunnel can be built. Node >= 22 and Chrome are required.
//   CHROME=/usr/bin/google-chrome OUT=/tmp/qt node web/qa/quick-tunnel-cdp.cjs
const fs = require('node:fs'), os = require('node:os'), path = require('node:path'), http = require('node:http');
const { spawn } = require('node:child_process');
const assert = require('node:assert/strict');
const web = path.resolve(__dirname, '..');
const out = process.env.OUT || path.join(os.tmpdir(), 'baft-quick-tunnel');
fs.mkdirSync(out, { recursive: true });
const sleep = ms => new Promise(r => setTimeout(r, ms));
const CSRF = 'qa-csrf-token';

function mockBCC(auto) {
  const now = () => new Date().toISOString();
  const log = [];
  let probePolls = 0, tunnelPolls = 0, created = null;
  const phases = ['preparing_ex', 'preparing_ir', 'committing_ex', 'committing_ir', 'health_ir', 'observing_ex', 'finalizing_ex', 'active', 'active', 'active'];
  const server = http.createServer((req, res) => {
    let body = '';
    req.on('data', c => { body += c; });
    req.on('end', () => {
      const u = new URL(req.url, 'http://x'), send = (code, data) => { res.writeHead(code, { 'Content-Type': data === undefined ? 'text/plain' : 'application/json' }); res.end(data === undefined ? '' : JSON.stringify(data)); };
      if (u.pathname === '/' || u.pathname === '/index.html') {
        const lang = u.searchParams.get('lang') === 'fa' ? 'fa' : 'en';
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end('<!doctype html><html lang="' + lang + '" dir="' + (lang === 'fa' ? 'rtl' : 'ltr') + '"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/bcc-action-wizard.css"><style>body{margin:0;min-height:100vh;background:#0b1220;color:#eef;font-family:system-ui,Vazirmatn,Tahoma,sans-serif}</style></head><body><main style="padding:24px"><button type="button" class="baw-launch" data-baw-open="true" id="launch">+ New tunnel</button></main><script>window.BAFT_UI={mode:"dashboard",base:"/",csrf:"' + CSRF + '",csrfHeader:"X-BAFT-CSRF"}</script><script src="/bcc-action-wizard.js"></script></body></html>');
        return;
      }
      if (u.pathname === '/bcc-action-wizard.js' || u.pathname === '/bcc-action-wizard.css') {
        res.writeHead(200, { 'Content-Type': u.pathname.endsWith('.js') ? 'text/javascript' : 'text/css' });
        res.end(fs.readFileSync(path.join(web, u.pathname.slice(1))));
        return;
      }
      if (!u.pathname.startsWith('/api/')) return send(404);
      const entry = { method: req.method, path: u.pathname + u.search, csrf: req.headers['x-baft-csrf'] || '', body: body ? JSON.parse(body) : undefined };
      log.push(entry);
      if (req.method !== 'GET' && entry.csrf !== CSRF) return send(403, { error: 'csrf' });
      const p = u.pathname.slice(5);
      if (p === 'nodes') return send(200, [
        { id: 'ir-1', alias: 'Tehran-1', role: 'worker', agent_seen: now(), capabilities: auto === 'host' ? ['carrier-h2', 'operational-ingress-host-v1'] : auto ? ['carrier-h2', 'operational-ingress-v1'] : ['carrier-h2'] },
        { id: 'ir-2', alias: 'Mashhad', role: 'worker', agent_seen: '2020-01-01T00:00:00Z' },
        { id: 'ex-1', alias: 'Frankfurt', role: 'foreign', agent_seen: now() },
        { id: 'old', role: 'foreign', revoked: true },
      ]);
      if (p === 'tunnels' && req.method === 'GET' && !u.searchParams.get('id')) return send(200, [{ id: 'live-1', phase: 'active', ex_node: 'ex-1', ir_node: 'ir-1', port: 8443, client_port: 2053, route_listen: '127.0.0.1:1444', ex_metrics_listen: '127.0.0.1:9202', ir_metrics_listen: '127.0.0.1:9302' }]);
      if (p === 'path-probes' && req.method === 'POST') return send(202, { id: 'pp-1', phase: 'queued', candidates: entry.body.candidate_ports.map(port => ({ port })) });
      if (p === 'path-probes' && req.method === 'GET') {
        probePolls++;
        const ports = log.find(x => x.path === '/api/path-probes' && x.method === 'POST').body.candidate_ports;
        const done = probePolls >= 2;
        return send(200, { id: 'pp-1', phase: done ? 'complete' : 'running', recommended_port: done ? 443 : 0, candidates: ports.map((port, i) => !done && i > 2 ? { port } :
          port === 443 ? { port, class: 'FULL_DATA', connect_rtt_ms: 41, connect_spread_ms: 3, attempts: 3, ack_successes: 3, throughput_kbps: 85400 } :
          port === 2053 ? { port, class: 'BYTE_CEILING', byte_ceiling: 16384, attempts: 3, ack_successes: 0 } :
          port === 2083 ? { port, class: 'TIMEOUT_UNREACHABLE', attempts: 3 } : { port, class: 'FULL_DATA', connect_rtt_ms: 58, attempts: 3, ack_successes: 3 }) });
      }
      if (p === 'tunnels/auto-select') return send(200, { selected: { port: 443, carrier_id: 'h2', connect_rtt_ms: 41, source_node: entry.body.ir_node, destination_node: entry.body.ex_node, family: entry.body.family }, rejected: [] });
      if (p === 'tunnels/plan') return send(200, { hash: 'plan-hash-1', ok: true, request: entry.body, gates: [{ name: 'agents fresh', status: 'PASS', detail: 'both agents reported recently' }, { name: 'port 443 free on EX', status: 'PASS', detail: '' }, { name: 'legacy tunnel on IR', status: 'WARN', detail: 'an older tunnel exists; it is kept' }], steps: ['EX: stage config generation 1', 'IR: pair with EX', 'Both: health check, then finalize'], nodes: [] });
      if (p === 'tunnels' && req.method === 'POST') { created = { id: 't-qa-1', phase: 'preparing_ex', ex_node: entry.body.ex_node, ir_node: entry.body.ir_node, port: entry.body.port, client_port: entry.body.client_port, route_listen: entry.body.route_listen }; return send(202, created); }
      if (p === 'tunnels' && req.method === 'GET') {
        tunnelPolls++;
        const phase = phases[Math.min(tunnelPolls, phases.length - 1)];
        const ingress = phase === 'active' && created.client_port ? (tunnelPolls >= phases.length - 1 ? { state: 'applied', ready: true } : { state: 'queued', ready: false }) : undefined;
        return send(200, Object.assign({}, created, { phase, ingress }));
      }
      return send(404, { error: 'unexpected ' + p });
    });
  });
  return { server, log };
}

async function connect(url) {
  const ws = new WebSocket(url), pending = new Map(); let serial = 0;
  await new Promise((r, j) => { ws.addEventListener('open', r, { once: true }); ws.addEventListener('error', () => j(Error('CDP connection failed')), { once: true }); });
  ws.addEventListener('message', ev => { const m = JSON.parse(ev.data); if (m.id && pending.has(m.id)) { const p = pending.get(m.id); pending.delete(m.id); m.error ? p.reject(Error(m.error.message)) : p.resolve(m.result); } });
  return { close: () => ws.close(), send: (method, params = {}) => new Promise((resolve, reject) => { const id = ++serial; pending.set(id, { resolve, reject }); ws.send(JSON.stringify({ id, method, params })); }) };
}

async function run(lang, width, height, scheme, auto = true) {
  const { server, log } = mockBCC(auto);
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const base = 'http://127.0.0.1:' + server.address().port + '/';
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'qt-chrome-'));
  const chrome = spawn(process.env.CHROME || 'google-chrome', ['--headless=new', '--no-sandbox', '--disable-gpu', '--remote-debugging-port=0', '--user-data-dir=' + dir, '--no-first-run', 'about:blank'], { stdio: 'ignore' });
  const name = lang + '-' + width + '-' + scheme + (auto === 'host' ? '-host' : auto ? '' : '-manual'), shots = [];
  try {
    const portFile = path.join(dir, 'DevToolsActivePort');
    for (let i = 0; i < 100 && !fs.existsSync(portFile); i++) await sleep(100);
    const port = fs.readFileSync(portFile, 'utf8').split('\n')[0];
    const target = await (await fetch('http://127.0.0.1:' + port + '/json/new?about:blank', { method: 'PUT' })).json();
    const cdp = await connect(target.webSocketDebuggerUrl);
    await cdp.send('Page.enable'); await cdp.send('Runtime.enable');
    const errors = [];
    await cdp.send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: width < 600 });
    await cdp.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: scheme }] });
    const ev = async expr => { const r = await cdp.send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }); if (r.exceptionDetails) throw Error('page exception: ' + JSON.stringify(r.exceptionDetails).slice(0, 400)); return r.result.value; };
    const until = async (expr, label, ms = 30000) => { for (let i = 0; i < ms / 100; i++) { if (await ev(expr)) return; await sleep(100); } throw Error('timeout waiting for ' + label); };
    const shot = async label => { const r = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); const f = path.join(out, name + '-' + label + '.png'); fs.writeFileSync(f, Buffer.from(r.data, 'base64')); shots.push(f); };
    const set = (k, v) => ev(`(()=>{const e=document.querySelector('[data-k="${k}"]');e.value=${JSON.stringify(v)};e.dispatchEvent(new Event(e.tagName==='SELECT'?'change':'input',{bubbles:true}));return true})()`);
    const click = k => ev(`(()=>{const e=document.querySelector('[data-k="${k}"]');if(!e||e.disabled)throw Error('not clickable: ${k}');e.click();return true})()`);
    const text = () => ev("document.querySelector('.baw-dialog').innerText");
    await cdp.send('Page.navigate', { url: base + '?lang=' + lang });
    await until("document.readyState==='complete'&&!!window.BAFTActionWizard", 'page');
    await ev("document.getElementById('launch').click()");
    await until("!!document.querySelector('[data-k=ir]')", 'servers');
    assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), true, 'next must wait for two servers');
    const opts = await ev("[...document.querySelectorAll('[data-k=ex] option')].map(o=>o.value)");
    assert.deepEqual(opts, ['', 'ex-1'], 'revoked foreign node must be hidden');
    await set('ir', 'ir-2'); assert.match(await text(), /offline|آفلاین/); assert.match(await text(), /5 minutes|۵ دقیقه/);
    await set('ir', 'ir-1'); await set('ex', 'ex-1'); await shot('1-servers');
    await click('next');
    await until("!!document.querySelector('[data-k=servicePort]')", 'service step');
    if (auto) {
      await set('servicePort', '22'); assert.match(await text(), /SSH/);
      await set('servicePort', '2053'); assert.match(await text(), /already uses|از این پورت استفاده/, 'client port collision with live tunnel on ir-1');
      await set('servicePort', '2096');
      assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), true, 'consent required');
      await click('consent');
      if (auto === 'host') {
        // Safe host mode: a separate per-port acknowledgement is required,
        // names the port and what BAFT cannot check, and resets on a change.
        assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), true, 'host mode needs the per-port acknowledgement');
        assert.match(await text(), /nginx/); assert.match(await text(), /2096/);
        await click('hostAck');
        assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), false);
        await set('servicePort', '2097');
        assert.equal(await ev("document.querySelector('[data-k=hostAck]').checked"), false, 'a different port voids the acknowledgement');
        await set('servicePort', '2096'); await click('hostAck');
      } else assert.equal(await ev("!!document.querySelector('[data-k=hostAck]')"), false, 'no host acknowledgement in the LAB');
    } else {
      await set('servicePort', '2096');
      assert.equal(await ev("!!document.querySelector('[data-k=consent]')"), false, 'no public-port consent without the IR capability');
      assert.equal(await ev("!!document.querySelector('[data-k=manualLead]')"), true, 'explains how the tunnel is reached instead');
      assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), false);
    }
    await shot('2-service');
    await click('next');
    await until("!!document.querySelector('[data-k=runTest]')", 'test step');
    assert.equal(await ev("document.querySelector('[data-k=next]').disabled"), true, 'no port without test or manual choice');
    await click('runTest');
    await until("document.querySelectorAll('.qt-row').length>0", 'probe board');
    await shot('3-testing');
    await until("!!document.querySelector('.qt-good')", 'port chosen', 20000);
    const rows = await ev("[...document.querySelectorAll('.qt-row')].map(r=>[r.querySelector('b').textContent,r.dataset.state])");
    assert.ok(rows.some(([p, s]) => p === '443' && s === 'ok'));
    assert.ok(rows.some(([p, s]) => p === '2053' && s === 'partial'));
    assert.ok(rows.some(([p, s]) => p === '2083' && s === 'blocked'));
    assert.ok(!rows.some(([p]) => p === '8443'), 'port of a live tunnel on ex-1 must not be probed');
    await shot('4-tested');
    await click('next');
    await until("document.querySelectorAll('.qt-gate').length===3", 'plan gates');
    assert.deepEqual(await ev("[...document.querySelectorAll('.qt-gate')].map(g=>g.dataset.status)"), ['WARN', 'PASS', 'PASS'], 'gates ordered FAIL/WARN/PASS');
    assert.match(await text(), /Tehran-1 \(ir-1\)/, 'route shows server names');
    if (!auto) assert.match(await text(), /127\.0\.0\.1:1445/, 'review shows the tunnel entry on the Iran server');
    await shot('5-review');
    await click('deploy');
    const steps = auto ? 7 : 6;
    await until("document.querySelectorAll('.qt-p').length===" + steps, 'progress');
    await shot('6-deploying');
    await until("document.querySelectorAll('.qt-p-done').length===" + steps, 'ready', 60000);
    assert.match(await text(), auto ? /every required check passed|همهٔ بررسی‌های لازم موفق/ : /127\.0\.0\.1:1445/);
    await shot('7-done');
    const post = p => log.filter(x => x.method === 'POST' && x.path === '/api/' + p);
    assert.ok(log.filter(x => x.method === 'POST').every(x => x.csrf === CSRF), 'every write carries CSRF');
    assert.deepEqual(post('path-probes')[0].body, { source_node: 'ir-1', destination_node: 'ex-1', family: '4', candidate_ports: [443, 2053, 2083, 2087, 2096, 9443] });
    const plan = post('tunnels/plan')[0].body;
    assert.equal(plan.port, 443); assert.equal(plan.target, '127.0.0.1:2096');
    if (auto) assert.equal(plan.client_port, 2096); else assert.equal('client_port' in plan, false, 'no public port requested without the IR capability');
    assert.equal(plan.client_port_ack === true, auto === 'host', 'the acknowledgement is sent exactly in host mode');
    assert.equal(plan.route_listen, '127.0.0.1:1445', 'route listener must skip the live tunnel');
    assert.equal(plan.ex_metrics_listen, '127.0.0.1:9203'); assert.equal(plan.ir_metrics_listen, '127.0.0.1:9303');
    assert.match(plan.instance_id, /^baft-[a-z0-9-]+$/); assert.equal(plan.auto_select_evidence.port, 443);
    assert.equal(post('tunnels')[0].body.plan_hash, 'plan-hash-1');
    await ev("document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'}))");
    assert.equal(await ev('window.BAFTActionWizard.isOpen()'), false);
    cdp.close();
    return { name, ok: true, shots, errors };
  } finally {
    chrome.kill('SIGKILL'); server.close();
  }
}

(async () => {
  const results = [];
  for (const [lang, w, h, scheme, auto] of [['en', 1280, 900, 'dark', true], ['fa', 390, 844, 'light', true], ['en', 1280, 900, 'dark', false], ['fa', 390, 844, 'light', false], ['en', 1280, 900, 'light', 'host'], ['fa', 390, 844, 'dark', 'host']]) results.push(await run(lang, w, h, scheme, auto));
  console.log(JSON.stringify(results, null, 1));
})().catch(e => { console.error(e); process.exit(1); });

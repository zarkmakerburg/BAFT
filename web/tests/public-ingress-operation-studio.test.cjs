'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const src = fs.readFileSync(path.resolve(__dirname, '../../internal/bcc/dashboard.go'), 'utf8');
const api = fs.readFileSync(path.resolve(__dirname, '../../internal/bcc/public_ingress_deploy_http.go'), 'utf8');
const js = src.slice(src.indexOf('<script>') + 8, src.indexOf('</script>'));

function controller() {
  const a = js.indexOf('let tnIngressOperationTunnelID='), b = js.indexOf('let tnIngressReadinessSeq=', a);
  assert.ok(a >= 0 && b > a, 'missing operation controller');
  return js.slice(a, b);
}
const SHA = 'a'.repeat(64);
const tunnel = { id: 'tun-1', phase: 'active', plan_hash: SHA, client_port: 54619, route_listen: '127.0.0.1:1443' };
const json = (status, body) => async () => ({ ok: status < 400, status, text: async () => JSON.stringify(body), json: async () => body });

function harness(routes, { lang = 'en', confirmAnswer = true } = {}) {
  const els = new Map(), requests = [];
  const q = key => { if (!els.has(key)) els.set(key, { textContent: '', innerHTML: '', hidden: true }); return els.get(key); };
  const c = {
    q, ah: () => ({ Authorization: 'Bearer t' }), tnWizardText: (en, fa) => (lang === 'fa' ? fa : en),
    confirm: () => confirmAnswer, console,
    fetch: (url, opts = {}) => {
      requests.push({ url, opts });
      const key = (opts.method || 'GET') + ' ' + url.split('?')[0];
      if (!routes[key]) throw Error('unexpected API call ' + key);
      return routes[key]();
    },
  };
  vm.createContext(c);
  vm.runInContext(controller(), c);
  return { c, q, requests };
}
const op = (over = {}) => ({
  tunnel_id: 'tun-1', plan_hash: SHA, public_port: 54619, operation_id: 'op-1', job_id: 'apply-1', action: 'public_ingress_apply',
  state: 'held', dispatch_phase: 'held', receipt_verified: false, ready_to_apply: false, reconciliation_pending: true, ...over,
});

test('active tunnel rows expose deploy and status, and the panel has retire/refresh controls', () => {
  assert.equal((src.match(/onclick="tnIngressDeploy\(this\.dataset\.id\)"/g) || []).length, 2);
  assert.equal((src.match(/onclick="tnIngressOperationCheck\(this\.dataset\.id\)"/g) || []).length, 2);
  for (const id of ['tnIngressOperationState', 'tnIngressOperationRefresh', 'tnIngressOperationRetire']) assert.match(src, new RegExp('id="' + id + '"'));
  assert.match(src, /Held or queued is not success, and an applied receipt is not Ready/);
});

test('deploy that stays held or queued never claims Ready or success', async () => {
  for (const state of ['held', 'queued']) {
    const h = harness({
      'GET /api/tunnels': json(200, tunnel),
      'POST /api/ingress/deploy': json(202, op({ state, dispatch_phase: state })),
    });
    await vm.runInContext('tnIngressDeploy("tun-1")', h.c);
    const post = h.requests.find(r => r.opts.method === 'POST');
    assert.deepEqual(JSON.parse(post.opts.body), { tunnel_id: 'tun-1', plan_hash: SHA });
    const text = h.q('#tnIngressOperationState').textContent;
    assert.match(text, /not Ready/);
    assert.doesNotMatch(text, /Verified:/);
    assert.equal(h.q('#tnIngressOperationRetire').hidden, true);
  }
});

test('deploy refused by the user, by a non-active tunnel or by a mismatching response makes no claim', async () => {
  const declined = harness({ 'GET /api/tunnels': json(200, tunnel) }, { confirmAnswer: false });
  await vm.runInContext('tnIngressDeploy("tun-1")', declined.c);
  assert.ok(!declined.requests.some(r => r.opts.method === 'POST'));

  const inactive = harness({ 'GET /api/tunnels': json(200, { ...tunnel, phase: 'pending' }) });
  await vm.runInContext('tnIngressDeploy("tun-1")', inactive.c);
  assert.ok(!inactive.requests.some(r => r.opts.method === 'POST'));
  assert.match(inactive.q('#tnIngressOperationState').textContent, /requires an active tunnel/);

  for (const bad of [op({ plan_hash: 'b'.repeat(64) }), op({ ready_to_apply: true }), op({ tunnel_id: 'other' }), op({ job_id: '' })]) {
    const h = harness({ 'GET /api/tunnels': json(200, tunnel), 'POST /api/ingress/deploy': json(202, bad) });
    await vm.runInContext('tnIngressDeploy("tun-1")', h.c);
    assert.match(h.q('#tnIngressOperationState').textContent, /Untrusted operation response/);
  }
});

test('status: retire is offered only for an applied, receipt-verified Apply that still is not Ready', async () => {
  const cases = [
    [op({ state: 'applied', receipt_verified: true, receipt_phase: 'applied' }), false],
    [op({ state: 'applied', receipt_verified: false, receipt_phase: 'applied' }), true],
    [op({ state: 'failed', receipt_verified: true, receipt_phase: 'blocked' }), true],
    [op({ state: 'queued' }), true],
    [op({ state: 'retiring', action: 'public_ingress_retire' }), true],
  ];
  for (const [body, hidden] of cases) {
    const h = harness({ 'GET /api/tunnels': json(200, tunnel), 'GET /api/ingress/deploy': json(200, body) });
    await vm.runInContext('tnIngressOperationCheck("tun-1")', h.c);
    assert.equal(h.q('#tnIngressOperationRetire').hidden, hidden, JSON.stringify(body));
    assert.doesNotMatch(h.q('#tnIngressOperationState').textContent, /^Verified/);
  }
  const applied = harness({ 'GET /api/tunnels': json(200, tunnel), 'GET /api/ingress/deploy': json(200, op({ state: 'applied', receipt_verified: true, receipt_phase: 'applied' })) });
  await vm.runInContext('tnIngressOperationCheck("tun-1")', applied.c);
  assert.match(applied.q('#tnIngressOperationState').textContent, /NOT Ready/);
});

test('status with 404, stale plan or wrong port claims nothing; fa strings are used under fa', async () => {
  const none = harness({ 'GET /api/tunnels': json(200, tunnel), 'GET /api/ingress/deploy': json(404, {}) });
  await vm.runInContext('tnIngressOperationCheck("tun-1")', none.c);
  assert.match(none.q('#tnIngressOperationState').textContent, /No ingress operation is staged/);
  for (const bad of [op({ plan_hash: 'c'.repeat(64) }), op({ public_port: 1234 })]) {
    const h = harness({ 'GET /api/tunnels': json(200, tunnel), 'GET /api/ingress/deploy': json(200, bad) });
    await vm.runInContext('tnIngressOperationCheck("tun-1")', h.c);
    assert.match(h.q('#tnIngressOperationState').textContent, /does not match the current tunnel plan/);
    assert.equal(h.q('#tnIngressOperationRetire').hidden, true);
  }
  const fa = harness({ 'GET /api/tunnels': json(200, tunnel), 'GET /api/ingress/deploy': json(200, op()) }, { lang: 'fa' });
  await vm.runInContext('tnIngressOperationCheck("tun-1")', fa.c);
  assert.match(fa.q('#tnIngressOperationState').textContent, /در انتظار/);
});

test('retire posts the exact plan hash and shows the queued retirement without success', async () => {
  const h = harness({
    'GET /api/tunnels': json(200, tunnel),
    'GET /api/ingress/deploy': json(200, op({ state: 'applied', receipt_verified: true, receipt_phase: 'applied' })),
    'POST /api/ingress/retire': json(202, op({ state: 'retiring', action: 'public_ingress_retire', job_id: 'retire-1' })),
  });
  await vm.runInContext('tnIngressOperationCheck("tun-1")', h.c);
  await vm.runInContext('tnIngressRetire()', h.c);
  const post = h.requests.find(r => r.opts.method === 'POST');
  assert.equal(post.url, '/api/ingress/retire');
  assert.deepEqual(JSON.parse(post.opts.body), { tunnel_id: 'tun-1', plan_hash: SHA });
  assert.match(h.q('#tnIngressOperationState').textContent, /Retirement queued/);
  assert.equal(h.q('#tnIngressOperationRetire').hidden, true);
});

test('server side never reports readiness and keeps Retire behind the exact applied receipt', () => {
  assert.match(api, /ReadyToApply: false/);
  assert.match(api, /obs is nil: this endpoint can never report Ready/);
  assert.match(api, /retirement requires the exact reviewed operation and authenticated applied receipt/);
});

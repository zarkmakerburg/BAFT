import test from 'node:test';
import assert from 'node:assert/strict';
import { setImmediate } from 'node:timers/promises';
import { createRealProvider, RESPONSE_MAX_BYTES } from '../generated/real-provider.js';
import { epoch, overview, tunnels, jsonResponse } from './fixtures.mjs';

const options = fetcher => ({ origin: 'https://bcc.example', basePath: '/private-bcc/', fetcher, now: () => epoch, onSessionExpired() {} });
const rejectsCode = code => error => error.code === code && error.message === `Aurora request: ${code}`;

test('explicit same-origin GET adapter preserves session and protected base path', async () => {
  const calls = [];
  const provider = createRealProvider(options(async (url, init) => {
    calls.push({ url, init }); return jsonResponse(url.endsWith('overview') ? overview() : tunnels());
  }));
  assert.equal(calls.length, 0);
  assert.equal(provider.mode, 'real');
  assert.equal('setTunnelPhase' in provider, false);
  assert.equal((await provider.getOverview()).mode, 'real');
  assert.equal((await provider.getTunnels())[0].id, 't-1');
  assert.deepEqual(calls.map(c => c.url), ['https://bcc.example/private-bcc/api/overview', 'https://bcc.example/private-bcc/api/tunnels']);
  for (const { init } of calls) {
    assert.equal(init.method, 'GET'); assert.equal(init.credentials, 'same-origin');
    assert.equal(init.cache, 'no-store'); assert.equal(init.redirect, 'error');
    assert.deepEqual(init.headers, { Accept: 'application/json' });
    assert.equal(init.body, undefined);
  }
});

test('cross-origin, credential-bearing, normalized traversal and malformed bases are rejected before fetch', () => {
  let calls = 0;
  const config = options(async () => { calls++; return jsonResponse(overview()); });
  for (const basePath of ['//evil.example/', '/a/../', '/%2e%2e/', '/a\\b/', '/a/?query', '/a/#hash', 'https://evil.example/', '/missing-slash']) {
    assert.throws(() => createRealProvider({ ...config, basePath }), rejectsCode('configuration'));
  }
  for (const origin of ['file:///tmp/', 'https://user:password@bcc.example', 'https://bcc.example/private/', 'https://bcc.example?query']) {
    assert.throws(() => createRealProvider({ ...config, origin }), rejectsCode('configuration'));
  }
  assert.equal(calls, 0);
});

test('session expiry invokes caller boundary once and never exposes response text', async () => {
  let expired = 0;
  const p = createRealProvider({ ...options(async () => jsonResponse({ secret: 'password-value' }, { status: 401 })), onSessionExpired() { expired++; } });
  await assert.rejects(p.getOverview(), rejectsCode('session_expired'));
  assert.equal(expired, 1);
});

test('HTTP, network, redirects, HTML, malformed JSON and schema errors stay sanitized', async () => {
  for (const [make, code] of [
    [() => jsonResponse({ error: 'private-token-and-url' }, { status: 403 }), 'http'],
    [() => { throw new Error('https://secret:credential@example/'); }, 'network'],
    [() => new Response('<html>private login</html>', { headers: { 'content-type': 'text/html' } }), 'invalid_data'],
    [() => new Response('{"broken":', { headers: { 'content-type': 'application/json' } }), 'invalid_data'],
    [() => jsonResponse({ ...overview(), nodes: 'private-malformed-value' }), 'invalid_data'],
    [() => { const r = jsonResponse(overview()); Object.defineProperty(r, 'redirected', { value: true }); return r; }, 'http'],
    [() => { const r = jsonResponse(overview()); Object.defineProperty(r, 'url', { value: 'https://other.example/api/overview' }); return r; }, 'http']
  ]) {
    await assert.rejects(createRealProvider(options(async () => make())).getOverview(), rejectsCode(code));
  }
});

test('response budget covers declared size and actual streamed bytes', async () => {
  const declared = jsonResponse(overview()); declared.headers.set('content-length', String(RESPONSE_MAX_BYTES + 1));
  await assert.rejects(createRealProvider(options(async () => declared)).getOverview(), rejectsCode('too_large'));
  const streamed = new Response(new ReadableStream({ start(c) {
    c.enqueue(new Uint8Array(600_000)); c.enqueue(new Uint8Array(600_000)); c.close();
  } }), { headers: { 'content-type': 'application/json' } });
  await assert.rejects(createRealProvider(options(async () => streamed)).getOverview(), rejectsCode('too_large'));
});

test('deadline covers an uncooperative fetch and a stalled body', async () => {
  await assert.rejects(createRealProvider({ ...options(() => new Promise(() => {})), timeoutMs: 10 }).getOverview(), rejectsCode('timeout'));
  let cancelled = false;
  const body = new Response(new ReadableStream({ cancel() { cancelled = true; } }), { headers: { 'content-type': 'application/json' } });
  await assert.rejects(createRealProvider({ ...options(async () => body), timeoutMs: 10 }).getOverview(), rejectsCode('timeout'));
  await setImmediate();
  assert.equal(cancelled, true);
});

test('caller cancellation and already-aborted signals do not start or revive requests', async () => {
  let calls = 0, expired = 0, resolveFetch;
  const pending = new Promise(resolve => { resolveFetch = resolve; });
  const p = createRealProvider({ ...options(() => { calls++; return pending; }), onSessionExpired() { expired++; } });
  const pre = new AbortController(); pre.abort();
  await assert.rejects(p.getOverview(pre.signal), rejectsCode('aborted')); assert.equal(calls, 0);
  const abort = new AbortController(); const task = p.getOverview(abort.signal); abort.abort();
  await assert.rejects(task, rejectsCode('aborted'));
  resolveFetch(jsonResponse({ secret: 'late-response' }, { status: 401 }));
  await setImmediate();
  assert.equal(expired, 0, 'aborted late response must not expire a session');
});

test('rejected response bodies are cancelled without reading arbitrary HTTP error data', async () => {
  let cancelled = false;
  const response = new Response(new ReadableStream({ cancel() { cancelled = true; } }), { status: 503 });
  await assert.rejects(createRealProvider(options(async () => response)).getOverview(), rejectsCode('http'));
  await setImmediate();
  assert.equal(cancelled, true);
});

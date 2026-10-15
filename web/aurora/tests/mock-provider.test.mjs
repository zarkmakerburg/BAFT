import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createMockProvider } from '../generated/mock-provider.js';

test('DEMO provider is deterministic and in-memory changes stay isolated', async () => {
  const a = createMockProvider(), b = createMockProvider();
  assert.equal(a.mode, 'demo'); assert.equal(a.label, 'DEMO');
  assert.deepEqual(await a.getOverview(), await b.getOverview());
  const first = await a.getOverview(); first.nodes[0].alias = 'caller mutation';
  assert.notEqual((await a.getOverview()).nodes[0].alias, 'caller mutation');
  await a.setTunnelPhase('demo-tunnel-1', 'rolled_back');
  assert.equal((await a.getTunnels())[0].desiredPhase, 'rolled_back');
  assert.equal((await b.getTunnels())[0].desiredPhase, 'active');
  assert.equal((await a.getOverview()).links.length, 0);
  await assert.rejects(a.setTunnelPhase('a-real-node', 'active'), { code: 'invalid_data' });
  await assert.rejects(a.setTunnelPhase('demo-tunnel-1', 'restart'), { code: 'invalid_data' });
});

test('DEMO import and every operation work when fetch is forbidden', async () => {
  const saved = globalThis.fetch;
  globalThis.fetch = () => { throw new Error('network forbidden'); };
  try {
    const demo = createMockProvider();
    await demo.getOverview(); await demo.getTunnels();
    await demo.setTunnelPhase('demo-tunnel-1', 'rolled_back');
    const abort = new AbortController(); abort.abort();
    await assert.rejects(demo.getOverview(abort.signal), { code: 'aborted' });
  } finally { globalThis.fetch = saved; }
});

test('fresh-process mock imports do not touch a network or install polling', () => {
  const url = new URL('../generated/mock-provider.js', import.meta.url).href;
  execFileSync(process.execPath, ['--input-type=module', '-e', `
    globalThis.fetch = () => { throw Error('forbidden network'); };
    globalThis.setInterval = () => { throw Error('forbidden polling'); };
    const { createMockProvider } = await import(${JSON.stringify(url)});
    const demo = createMockProvider();
    await demo.getOverview(); await demo.getTunnels();
    await demo.setTunnelPhase('demo-tunnel-1', 'rolled_back');
  `], { timeout: 5000, stdio: 'pipe' });
});

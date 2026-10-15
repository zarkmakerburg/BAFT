import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRealProvider } from '../generated/real-provider.js';
import { reassessOverview, EVIDENCE_TTL_MS } from '../generated/domain.js';

const fixture = name => JSON.parse(readFileSync(new URL(`./wire/${name}.json`, import.meta.url), 'utf8'));
const at = Date.parse('2026-01-01T12:00:00.123456789Z');
function reader(empty = false) {
  const calls = [];
  const provider = createRealProvider({ origin: 'https://example.invalid', basePath: '/synthetic-contract/', now: () => at,
    onSessionExpired() { assert.fail('unexpected expiry'); }, async fetcher(url, options) {
      calls.push({ url, options });
      const endpoint = url.endsWith('/overview') ? 'overview' : 'tunnels';
      return new Response(JSON.stringify(fixture((empty ? 'empty-' : '') + endpoint)), { headers: { 'content-type': 'application/json' } });
    }
  });
  return { provider, calls };
}

test('Go-handler shared fixtures decode through the real adapter without a live request', async () => {
  const { provider, calls } = reader();
  const overview = await provider.getOverview();
  assert.equal(overview.generatedAt, '2026-01-01T12:00:00.123456789Z');
  assert.equal(overview.nodes[0].latencyMs.value, 0);
  assert.equal(overview.nodes[0].inBps.value, 0);
  assert.equal(overview.nodes[0].inBps.freshness, 'unknown');
  assert.equal(overview.nodes[0].inBps.observedAt, null);
  assert.deepEqual(overview.nodes[0].location, { lat: 35.69, lon: 51.39, source: 'country-anchor' });
  assert.equal(overview.nodes[1].effectiveStatus, 'unknown');
  assert.equal(overview.nodes[1].revoked, true);
  assert.equal(overview.nodes[1].latencyMs.value, null);
  assert.ok(overview.links.every(link => link.desiredPhase === 'active' && link.deliveryHealth === 'unknown'));
  const tunnels = await provider.getTunnels();
  assert.equal(tunnels[0].instanceId, 'demo-slot-1');
  assert.equal(tunnels[1].instanceId, null);
  assert.ok(tunnels.every(tunnel => tunnel.deliveryHealth === 'unknown'));
  assert.deepEqual(calls.map(call => call.url), ['https://example.invalid/synthetic-contract/api/overview', 'https://example.invalid/synthetic-contract/api/tunnels']);
  assert.ok(calls.every(call => call.options.method === 'GET' && call.options.credentials === 'same-origin' && !call.options.body));
});

test('actual Go empty-list shapes remain empty, never zero-filled resources', async () => {
  const { provider } = reader(true);
  const overview = await provider.getOverview();
  assert.deepEqual(overview.nodes, []); assert.deepEqual(overview.links, []);
  assert.deepEqual(await provider.getTunnels(), []);
});

test('wire timestamps do not renew cached node evidence or certify payload delivery', async () => {
  const { provider } = reader();
  const original = await provider.getOverview();
  const stale = reassessOverview(original, at + EVIDENCE_TTL_MS);
  assert.equal(stale.generatedAt, original.generatedAt);
  assert.equal(stale.fetchedAt, original.fetchedAt);
  assert.ok(stale.nodes.every(node => node.effectiveStatus === 'unknown'));
  assert.ok(stale.links.every(link => link.deliveryHealth === 'unknown'));
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { exactCounter, freshness, reassessOverview } from '../generated/domain.js';
import { ContractError, decodeOverview, decodeTunnels, timestamp } from '../generated/validation.js';
import { epoch, overview, stamp, tunnels } from './fixtures.mjs';

test('actual overview concepts preserve unknown, zero, country anchors and scope', () => {
  const value = decodeOverview(overview(), 'real', epoch);
  assert.equal(value.mode, 'real');
  assert.equal(value.nodes[0].effectiveStatus, 'up');
  assert.equal(value.nodes[0].inBps.value, 0);
  assert.equal(value.nodes[0].outBps.value, null);
  assert.equal(value.nodes[0].inBps.freshness, 'unknown');
  assert.equal(value.nodes[0].inBps.observedAt, null);
  assert.equal(value.nodes[0].location, null);
  assert.equal(value.nodes[1].location.source, 'country-anchor');
  assert.equal(value.nodes[1].effectiveStatus, 'unknown');
  assert.equal(value.nodes[1].agentFreshness, 'unknown');
  assert.equal(value.links[0].desiredPhase, 'active');
  assert.equal(value.links[0].deliveryHealth, 'unknown');
});

test('cached reassessment expires evidence at the boundary without renewing fetch time', () => {
  const initial = decodeOverview(overview(), 'real', epoch);
  assert.equal(reassessOverview(initial, epoch + 179_999).nodes[0].effectiveStatus, 'up');
  const expired = reassessOverview(initial, epoch + 180_000);
  assert.equal(expired.freshness, 'stale');
  assert.equal(expired.nodes[0].effectiveStatus, 'unknown');
  assert.equal(expired.nodes[0].agentFreshness, 'unknown');
  assert.equal(expired.fetchedAt, initial.fetchedAt);
  assert.equal(expired.generatedAt, initial.generatedAt);
  assert.equal(initial.nodes[0].effectiveStatus, 'up');
  assert.equal(freshness(stamp, epoch - 1), 'future');
  assert.equal(reassessOverview(initial, epoch - 1).nodes[0].effectiveStatus, 'unknown');
});

test('future, absent and revoked node reports cannot imply healthy state', () => {
  for (const mutate of [n => { delete n.last_seen; }, n => { n.last_seen = '2026-01-01T12:00:01Z'; }, n => { n.revoked = true; }, n => { n.status = 'a-new-backend-state'; }]) {
    const wire = overview(); mutate(wire.nodes[0]);
    assert.equal(decodeOverview(wire, 'real', epoch).nodes[0].effectiveStatus, 'unknown');
  }
});

test('malformed types, out-of-range measurements, duplicate and wrong-scope resources reject', () => {
  const mutations = [
    x => { x.generated_at = '2026-02-31T12:00:00Z'; },
    x => { x.nodes = null; }, x => { x.links = {}; },
    x => { x.nodes[0].latency_ms = -1; }, x => { x.nodes[0].in_bps = Infinity; },
    x => { x.nodes[0].out_bps = '10'; }, x => { x.nodes[0].country = 'de'; },
    x => { x.nodes[0].revoked = 'false'; }, x => { x.nodes[1].lat = 91; },
    x => { x.nodes.push(x.nodes[0]); }, x => { x.links.push(x.links[0]); },
    x => { x.links[0].ir_node = 'some-other-instance'; },
    x => { x.links[0].ex_node = x.links[0].ir_node; },
    x => { x.links[0].port = 65536; }, x => { x.links[0].port = 1.5; },
    x => { x.nodes[0].id = '../escape'; }, x => { x.nodes[0].alias = 'line\nbreak'; }
  ];
  for (const mutate of mutations) {
    const wire = overview(); mutate(wire);
    assert.throws(() => decodeOverview(wire, 'real', epoch), ContractError);
  }
  assert.throws(() => decodeOverview(overview(), 'real', Infinity), ContractError);
});

test('only selected presentation fields leave the adapter', () => {
  const wire = overview();
  wire.nodes[0].agent_token_hash = 'never-render-this';
  wire.nodes[0].address = 'private-operator-address';
  const decoded = decodeOverview(wire, 'real', epoch);
  assert.ok(!JSON.stringify(decoded).includes('never-render-this'));
  assert.ok(!JSON.stringify(decoded).includes('private-operator-address'));
});

test('uint64 counters retain exact decimals and reject rounded JSON numbers', () => {
  assert.equal(exactCounter('18446744073709551615'), '18446744073709551615');
  assert.equal(exactCounter('18446744073709551616'), null);
  assert.equal(exactCounter(JSON.parse('9007199254740993')), null);
  for (const invalid of [null, -1, 0.5, NaN, Infinity, '01', '-1', '1e3', {}, true]) assert.equal(exactCounter(invalid), null);
  assert.equal(exactCounter(0), '0');
  assert.equal(exactCounter(Number.MAX_SAFE_INTEGER), '9007199254740991');
});

test('RFC3339 accepts Go nanoseconds and timezone offsets without accepting invalid dates', () => {
  for (const value of ['2024-02-29T12:00:00.123456789Z', '2026-01-01T15:30:00+03:30']) assert.equal(timestamp(value), value);
  for (const value of ['2025-02-29T12:00:00Z', '2026-01-01', '2026-01-01T25:00:00Z', '2026-01-01T12:00:00+24:00']) assert.throws(() => timestamp(value), ContractError);
});

test('tunnel desired phase and lifecycle timestamp never become delivery health', () => {
  const value = decodeTunnels(tunnels(), epoch + 180_000)[0];
  assert.equal(value.freshness, 'stale');
  assert.equal(value.deliveryHealth, 'unknown');
  assert.equal(value.instanceId, 'slot-1');
  const duplicate = tunnels(); duplicate.push(duplicate[0]);
  assert.throws(() => decodeTunnels(duplicate, epoch), ContractError);
});

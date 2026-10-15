import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

// Source-level compatibility guard. This does NOT substitute for rendered UI,
// runtime auth/route tests, or owner-reviewed visual acceptance.
const root = new URL('../../../', import.meta.url);
const read = (path) => readFileSync(new URL(path, root), 'utf8');
const legacy = read('internal/bcc/dashboard.go');
const aurora = read('web/aurora/preview/index.html');
const adr = read('docs/adr/0008-aurora-incremental-frontend.md');

test('legacy BCC critical operational surfaces remain present', () => {
  const ids = {
    kNodes: 'node status KPI',
    kUp: 'UP KPI',
    kDown: 'DOWN KPI',
    kUnknown: 'UNKNOWN KPI',
    historyCanvas: 'seven-day traffic/uptime chart',
    tnPlan: 'tunnel review plan',
    chRows: 'change history',
    rdResult: 'route diagnosis',
    pmMatrix: 'connectivity matrix',
    jobs: 'recent jobs',
  };
  for (const [id, label] of Object.entries(ids)) {
    assert.ok(legacy.includes(`id="${id}"`), `missing legacy ${label} (#${id})`);
  }
});

test('classic dashboard retains basic mobile and reduced-motion CSS', () => {
  assert.match(legacy, /@media\(max-width:820px\)/);
  assert.match(legacy, /@media\(max-width:520px\)/);
  assert.match(legacy, /prefers-reduced-motion:reduce/);
});

test('Aurora remains a separate opt-in preview, not a silently removed fallback', () => {
  assert.match(aurora, /<html\b/i);
  assert.match(adr, /Preserve the existing dashboard and classic fallback/i);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { en, fa, catalog } from '../generated/catalog.js';
import { parsePreferences, readPreferences, savePreferences, viewFromHash, PREFERENCE_KEY } from '../generated/preferences.js';
import { mountAurora } from '../generated/shell.js';

const source = name => readFileSync(new URL(name, import.meta.url), 'utf8');
test('catalogs have exact parity, nonempty copy, and explicit DEMO/unknown boundaries', () => {
  assert.deepEqual(Object.keys(en).sort(), Object.keys(fa).sort());
  for (const dictionary of [en, fa]) {
    for (const value of Object.values(dictionary)) assert.ok(value.trim().length > 0);
    assert.match(dictionary.demo, /DEMO/);
    assert.match(dictionary.demoClock, /UNKNOWN/);
  }
  assert.equal(catalog('fa'), fa); assert.equal(catalog('en'), en);
});
test('preferences validate values and safely tolerate blocked or corrupted storage', () => {
  assert.deepEqual(parsePreferences(null, false), { language: 'fa', theme: 'light' });
  for (const value of ['{', 'null', '[]', 'true']) assert.deepEqual(parsePreferences(value), { language: 'fa', theme: 'dark' });
  assert.deepEqual(parsePreferences('{"language":"en","theme":"light","credential":"not-copied"}'), { language: 'en', theme: 'light' });
  assert.deepEqual(parsePreferences('{"language":"xx","theme":"css-injection"}'), { language: 'fa', theme: 'dark' });
  assert.deepEqual(readPreferences({ getItem() { throw Error('disabled'); } }), { language: 'fa', theme: 'dark' });
  assert.equal(savePreferences({ setItem() { throw Error('quota'); } }, { language: 'en', theme: 'light' }), false);
  let entry;
  assert.equal(savePreferences({ setItem(key, value) { entry = [key, value]; } }, { language: 'en', theme: 'light' }), true);
  assert.deepEqual(entry, [PREFERENCE_KEY, '{"language":"en","theme":"light"}']);
});
test('only built views route; arbitrary hashes fall back without executing anything', () => {
  assert.equal(viewFromHash('#nodes'), 'nodes'); assert.equal(viewFromHash('#tunnels'), 'tunnels');
  for (const hash of ['', '#studio', '#javascript:alert(1)', '#incident']) assert.equal(viewFromHash(hash), 'overview');
});
test('runtime refuses a real provider before touching DOM', () => {
  assert.throws(() => mountAurora({}, { mode: 'real' }), /requires DEMO/);
});
test('preview policy refuses operational network, forms and inline execution', () => {
  const html = source('../preview/index.html');
  assert.match(html, /connect-src 'none'/); assert.match(html, /form-action 'none'/);
  assert.match(html, /object-src 'none'/); assert.match(html, /type="module" src="\.\.\/generated\/preview-entry\.js"/);
  assert.doesNotMatch(html, /https?:|unsafe-inline|<form/);
});
test('shell owns safe text rendering, cancellation and explicit cleanup without real reads or polling', () => {
  const shell = source('../src/shell.ts');
  assert.doesNotMatch(shell, /innerHTML|outerHTML|insertAdjacentHTML|createRealProvider|setTunnelPhase|setInterval|setTimeout|\bfetch\(/);
  assert.match(shell, /textContent/); assert.match(shell, /abort\.abort\(\)/);
  assert.match(shell, /mounted\.get\(root\)\?\.\(\)/); assert.match(shell, /sequence !== request/);
  assert.match(shell, /reassessOverview\(data, Date\.now\(\)\)/);
  assert.match(shell, /result.mode !== 'demo'/);
  assert.match(shell, /role', 'status/); assert.match(shell, /aria-current/);
  const entry = source('../src/preview-entry.ts');
  assert.match(entry, /pagehide/); assert.match(entry, /pageshow/); assert.match(entry, /event.persisted/);
});
test('responsive/accessibility source hooks exist (not a visual or keyboard execution)', () => {
  const css = source('../preview/shell.css');
  assert.match(css, /min-block-size:44px/); assert.match(css, /max-width:600px/);
  assert.match(css, /minmax\(0,1fr\)/); assert.match(css, /prefers-reduced-motion/);
  assert.match(css, /focus-visible/); assert.match(css, /forced-colors/);
  assert.match(css, /border-inline-end/); assert.doesNotMatch(css, /\b(?:margin|padding|border)-(?:left|right)\s*:/);
});
test('theme token text, accent, focus and control borders meet calculated contrast minima', () => {
  const css = source('../preview/shell.css');
  function luminance(hex) {
    const channels = hex.match(/[0-9a-f]{2}/g).map(v => parseInt(v, 16) / 255).map(v => v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  }
  function contrast(a, b) { const x = luminance(a), y = luminance(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); }
  for (const block of css.match(/--surface:[^}]+/g)) {
    const tokens = Object.fromEntries([...block.matchAll(/--([a-z-]+):#([0-9a-f]{6})/g)].map(([, key, value]) => [key, value]));
    for (const background of ['surface', 'base', 'raised']) {
      for (const foreground of ['text', 'muted', 'accent']) assert.ok(contrast(tokens[foreground], tokens[background]) >= 4.5, `${foreground}/${background}`);
      for (const foreground of ['line', 'focus']) assert.ok(contrast(tokens[foreground], tokens[background]) >= 3, `${foreground}/${background}`);
    }
    assert.ok(contrast(tokens['accent-ink'], tokens.accent) >= 4.5);
  }
});

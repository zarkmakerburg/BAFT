import test from 'node:test';
import assert from 'node:assert/strict';
import { realm, settle, deferred } from './harness.mjs';

test('mounted DOM loads historical DEMO evidence without reporting payload health', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview();
  ui.mount({ mode: 'demo', getOverview: () => pending.promise });
  assert.match(ui.root.querySelector('[role=status]').textContent, /بارگذاری/);
  assert.equal(ui.root.querySelector('[data-action=reload]').disabled, true);
  pending.resolve(snapshot); await settle();
  assert.equal(ui.root.querySelectorAll('main').length, 1);
  assert.match(ui.root.textContent, /DEMO/);
  assert.ok(ui.root.querySelectorAll('article').length >= 3);
  assert.doesNotMatch(ui.root.textContent, /گزارش روشن/);
  assert.equal(ui.errors.length, 0);
});

test('all four language/theme combinations mount, switch, persist and restore focus', async t => {
  for (const language of ['fa', 'en']) for (const theme of ['dark', 'light']) {
    const ui = await realm({ language, theme }); t.after(() => ui.close());
    ui.mount(); await settle();
    assert.equal(ui.document.documentElement.lang, language);
    assert.equal(ui.document.documentElement.dir, language === 'fa' ? 'rtl' : 'ltr');
    assert.equal(ui.root.dataset.theme, theme);
    ui.click('language');
    assert.equal(ui.document.documentElement.lang, language === 'fa' ? 'en' : 'fa');
    assert.equal(ui.document.activeElement.dataset.action, 'language');
    ui.click('theme');
    assert.equal(ui.root.dataset.theme, theme === 'dark' ? 'light' : 'dark');
    assert.equal(ui.document.activeElement.dataset.action, 'theme');
    const stored = JSON.parse(ui.window.localStorage.getItem('baft.aurora.demo.preferences.v1'));
    assert.equal(stored.language, ui.document.documentElement.lang);
    assert.equal(stored.theme, ui.root.dataset.theme);
    assert.equal(ui.errors.length, 0);
  }
});

test('completion preserves a control focused while data was pending', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview();
  ui.mount({ mode: 'demo', getOverview: () => pending.promise });
  ui.click('language');
  assert.equal(ui.document.activeElement.dataset.action, 'language');
  pending.resolve(snapshot); await settle();
  assert.equal(ui.document.activeElement.dataset.action, 'language');
});

test('navigation while pending keeps main focused after completion', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview();
  ui.mount({ mode: 'demo', getOverview: () => pending.promise });
  await ui.navigate('nodes');
  assert.equal(ui.document.activeElement.tagName, 'MAIN');
  pending.resolve(snapshot); await settle();
  assert.equal(ui.document.activeElement.tagName, 'MAIN');
  assert.equal(ui.root.querySelector('[aria-current=page]').getAttribute('href'), '#nodes');
});

test('completion never steals focus from a target outside the shell', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview();
  const outside = ui.document.createElement('button'); outside.textContent = 'Outside'; ui.document.body.append(outside);
  ui.mount({ mode: 'demo', getOverview: () => pending.promise });
  outside.focus(); pending.resolve(snapshot); await settle();
  assert.equal(ui.document.activeElement, outside);
});

test('errors are safe and explicit retry recovers without duplicate requests', async t => {
  const ui = await realm({ language: 'en', theme: 'light' }); t.after(() => ui.close());
  const sample = ui.demo(); let calls = 0;
  ui.mount({ mode: 'demo', async getOverview(signal) {
    calls++; if (calls === 1) throw Error('synthetic-private-detail');
    return sample.getOverview(signal);
  } });
  await settle();
  assert.match(ui.root.querySelector('[role=status]').textContent, /could not/);
  assert.doesNotMatch(ui.root.textContent, /synthetic-private-detail/);
  ui.click('reload'); ui.click('reload'); await settle();
  assert.equal(calls, 2); assert.equal(ui.root.querySelectorAll('article').length, 3);
});

test('empty and mislabeled real data never become DEMO resources', async t => {
  const ui = await realm({ language: 'en', theme: 'dark' }); t.after(() => ui.close());
  const snapshot = await ui.demo().getOverview();
  ui.mount({ mode: 'demo', async getOverview() { return { ...snapshot, nodes: [], links: [] }; } }); await settle();
  assert.equal(ui.root.querySelectorAll('article').length, 0); assert.match(ui.root.textContent, /No resources/);
  ui.mount({ mode: 'demo', async getOverview() { return { ...snapshot, mode: 'real' }; } }); await settle();
  assert.equal(ui.root.querySelectorAll('article').length, 0); assert.match(ui.root.textContent, /could not/);
});

test('destroy aborts pending data, removes listeners and refuses late repaint', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview(); let signal;
  const destroy = ui.mount({ mode: 'demo', getOverview(value) { signal = value; return pending.promise; } });
  destroy();
  assert.equal(signal.aborted, true); assert.equal(ui.root.childNodes.length, 0);
  assert.equal(ui.document.documentElement.lang, 'en'); assert.equal(ui.document.documentElement.dir, 'ltr');
  pending.resolve(snapshot); await settle(); await ui.navigate('nodes');
  assert.equal(ui.root.childNodes.length, 0);
  assert.equal(ui.errors.length, 0);
});

test('remount has one listener owner and stale destroy cannot remove new UI', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const old = ui.mount(); await settle();
  ui.mount(); await settle(); old();
  ui.click('language');
  assert.equal(ui.document.documentElement.lang, 'en'); // Double listener would toggle twice.
  assert.equal(ui.root.querySelectorAll('main').length, 1);
  assert.equal(ui.errors.length, 0);
});

test('resource text is inert even when synthetic aliases resemble HTML', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const snapshot = await ui.demo().getOverview();
  const alias = '<img src=x onerror="throw 1">';
  ui.mount({ mode: 'demo', async getOverview() { return { ...snapshot, nodes: snapshot.nodes.map(n => ({ ...n, alias })) }; } }); await settle();
  assert.match(ui.root.textContent, /<img src=x/); assert.equal(ui.root.querySelectorAll('img,script').length, 0);
  assert.equal(ui.errors.length, 0);
});

test('blocked storage keeps preference controls functional and reports a safe notice', async t => {
  const ui = await realm(); t.after(() => ui.close());
  Object.defineProperty(ui.window, 'localStorage', { get() { throw Error('private storage reason'); } });
  ui.mount(); await settle(); ui.click('language');
  assert.equal(ui.document.documentElement.lang, 'en');
  assert.match(ui.root.querySelector('[role=status]').textContent, /could not be saved/);
  assert.doesNotMatch(ui.root.textContent, /private storage reason/);
  assert.equal(ui.errors.length, 0);
});

test('entry page lifecycle events tear down and remount on a persisted pageshow', async t => {
  const ui = await realm(); t.after(() => ui.close());
  ui.root.id = 'aurora-preview';
  await ui.load('preview-entry.js'); await settle();
  assert.equal(ui.root.querySelectorAll('main').length, 1);
  ui.window.dispatchEvent(new ui.window.PageTransitionEvent('pagehide', { persisted: true }));
  assert.equal(ui.root.childNodes.length, 0);
  ui.window.dispatchEvent(new ui.window.PageTransitionEvent('pageshow', { persisted: true })); await settle();
  assert.equal(ui.root.querySelectorAll('main').length, 1);
  ui.click('language'); assert.equal(ui.document.documentElement.lang, 'en');
  assert.equal(ui.errors.length, 0);
});

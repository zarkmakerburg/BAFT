import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';
import { realm, settle, deferred } from './harness.mjs';

test('invalid host mode and real providers are refused before touching the root', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const { mountAurora } = await ui.load('shell.js');
  assert.throws(() => mountAurora({}, { mode: 'demo' }, { host: 'automatic' }), /host mode/);
  assert.throws(() => mountAurora({}, { mode: 'real' }, { host: 'embedded' }), /requires DEMO/);
});

test('embedded preferences stay inside the root for every language and theme', async t => {
  for (const language of ['fa', 'en']) for (const theme of ['dark', 'light']) {
    const ui = await realm({ language, theme }); t.after(() => ui.close());
    const outside = ui.document.createElement('button'); outside.textContent = 'BCC control';
    ui.document.body.prepend(outside); outside.focus();
    ui.window.history.replaceState(null, '', '#servers');
    ui.mount(undefined, { host: 'embedded' }); await settle();
    assert.equal(ui.document.documentElement.lang, 'en');
    assert.equal(ui.document.documentElement.dir, 'ltr');
    assert.equal(ui.window.location.hash, '#servers');
    assert.equal(ui.document.activeElement, outside);
    assert.equal(ui.root.lang, language);
    assert.equal(ui.root.dir, language === 'fa' ? 'rtl' : 'ltr');
    assert.equal(ui.root.dataset.theme, theme);
    assert.equal(ui.root.querySelectorAll('main').length, 0, 'do not nest a main landmark inside BCC');
    assert.equal(ui.root.querySelector('[aria-current=page]').getAttribute('href'), '#overview');
    ui.click('language'); ui.click('theme');
    assert.equal(ui.root.lang, language === 'fa' ? 'en' : 'fa');
    assert.equal(ui.root.dataset.theme, theme === 'dark' ? 'light' : 'dark');
    assert.equal(ui.document.documentElement.lang, 'en');
    assert.equal(ui.document.documentElement.dir, 'ltr');
    assert.equal(ui.window.location.hash, '#servers');
    assert.equal(ui.document.activeElement.dataset.action, 'theme');
    assert.equal(ui.errors.length, 0);
  }
});

test('embedded navigation and skip use local focus without changing or consuming host routes', async t => {
  const ui = await realm(); t.after(() => ui.close());
  ui.window.history.replaceState(null, '', '#servers');
  ui.mount(undefined, { host: 'embedded' }); await settle();
  for (const href of ['#nodes', '#tunnels', '#overview', '#aurora-content']) {
    const click = new ui.window.MouseEvent('click', { bubbles: true, cancelable: true });
    const link = ui.root.querySelector(`a[href="${href}"]`);
    assert.equal(link.dispatchEvent(click), false, 'owned link must prevent hash navigation');
    assert.equal(ui.window.location.hash, '#servers');
    assert.equal(ui.document.activeElement.id, 'aurora-content');
    if (href !== '#aurora-content') assert.equal(ui.root.querySelector('[aria-current=page]').getAttribute('href'), href);
  }
  const content = ui.root.querySelector('#aurora-content');
  const outside = ui.document.createElement('button'); ui.document.body.append(outside); outside.focus();
  await ui.navigate('backups');
  assert.equal(ui.root.querySelector('#aurora-content'), content, 'host routing must not trigger an Aurora repaint');
  assert.equal(ui.document.activeElement, outside);
  assert.equal(ui.errors.length, 0);
});

test('embedded pending completion preserves local focus and later host appearance', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const pending = deferred(); const snapshot = await ui.demo().getOverview();
  const destroy = ui.mount({ mode: 'demo', getOverview: () => pending.promise }, { host: 'embedded' });
  ui.root.querySelector('a[href="#nodes"]').click();
  ui.document.documentElement.lang = 'fa'; ui.document.documentElement.dir = 'rtl';
  pending.resolve(snapshot); await settle();
  assert.equal(ui.document.activeElement.id, 'aurora-content');
  assert.equal(ui.root.querySelector('[aria-current=page]').getAttribute('href'), '#nodes');
  destroy();
  assert.equal(ui.document.documentElement.lang, 'fa');
  assert.equal(ui.document.documentElement.dir, 'rtl');
});

test('embedded teardown restores owned root attributes and rejects late data', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const original = { lang: 'en', dir: 'ltr', 'data-theme': 'host', 'data-aurora-root': 'owned', 'data-aurora-host': 'previous' };
  for (const [key, value] of Object.entries(original)) ui.root.setAttribute(key, value);
  const pending = deferred(); const snapshot = await ui.demo().getOverview(); let signal;
  const destroy = ui.mount({ mode: 'demo', getOverview(value) { signal = value; return pending.promise; } }, { host: 'embedded' });
  destroy();
  assert.equal(signal.aborted, true);
  for (const [key, value] of Object.entries(original)) assert.equal(ui.root.getAttribute(key), value);
  pending.resolve(snapshot); await settle(); await ui.navigate('tunnels');
  assert.equal(ui.root.childNodes.length, 0);
  assert.equal(ui.document.documentElement.lang, 'en');
  assert.equal(ui.document.documentElement.dir, 'ltr');
  assert.equal(ui.errors.length, 0);
});

test('embedded remount keeps one owner and stale teardown cannot remove a new shell', async t => {
  const ui = await realm(); t.after(() => ui.close());
  const old = ui.mount(undefined, { host: 'embedded' }); await settle();
  ui.mount(undefined, { host: 'embedded' }); await settle(); old(); ui.click('language');
  assert.equal(ui.root.lang, 'en');
  assert.equal(ui.root.querySelectorAll('#aurora-content').length, 1);
  assert.equal(ui.document.documentElement.lang, 'en');
});

function checkScopedCSS(css) {
  const dom = new JSDOM('<!doctype html><html><head></head><body></body></html>');
  try {
    const style = dom.window.document.createElement('style'); style.textContent = css; dom.window.document.head.append(style);
    assert.ok(style.sheet, 'CSS must parse');
    let styles = 0;
    function inspect(rules) {
      for (const rule of rules) {
        if (rule.selectorText) {
          // Commas inside :where() are not selector-list boundaries.
          const selectors = rule.selectorText.split(/,(?![^()]*\))/);
          for (const selector of selectors) assert.match(selector.trim(), /^\[data-aurora-root\](?=$|[\s[:])/);
          styles++;
        } else if (rule.cssRules) inspect(rule.cssRules);
        else if (rule.type === dom.window.CSSRule.FONT_FACE_RULE) assert.equal(rule.style.getPropertyValue('font-family'), 'AuroraVazirmatn');
        else assert.fail('Unreviewed global CSS rule');
      }
    }
    inspect(style.sheet.cssRules); assert.ok(styles > 30, 'inspect nested responsive/motion rules too');
  } finally { dom.window.close(); }
}

test('shared shell CSS scopes every selector including responsive and reduced-motion rules', () => {
  const css = readFileSync(new URL('../../preview/shell.css', import.meta.url), 'utf8');
  // CSSOM in this test realm does not retain every browser declaration.
  // Keep font loading and vendor interaction declarations in the source too.
  assert.match(css, /@font-face\{font-family:AuroraVazirmatn;src:url\('\.\.\/\.\.\/assets\/vazirmatn\.woff2'\) format\('woff2'\);font-display:swap\}/);
  assert.match(css, /-webkit-tap-highlight-color:transparent/);
  checkScopedCSS(css);
  assert.throws(() => checkScopedCSS(css + '\nbody{margin:0}'), /data-aurora-root/);
  assert.throws(() => checkScopedCSS(css + '\n@media(max-width:600px){button{font:inherit}}'), /data-aurora-root/);
});

test('loading shared CSS leaves host controls, headings, body and landmarks unchanged', () => {
  const dom = new JSDOM('<!doctype html><html><head></head><body><button>Host</button><a href="#servers">Host</a><main class="content"><h1>Host</h1><p>Host</p><div class="layout">Host</div></main><div data-aurora-root></div></body></html>');
  try {
    const { document } = dom.window;
    const nodes = [...document.querySelectorAll('body,button,a,main,h1,p,.layout')];
    const properties = ['display', 'margin', 'padding', 'font', 'box-sizing', 'outline', 'min-block-size'];
    const snapshot = () => nodes.map(node => properties.map(key => dom.window.getComputedStyle(node).getPropertyValue(key)));
    const before = snapshot(); const style = document.createElement('style');
    style.textContent = readFileSync(new URL('../../preview/shell.css', import.meta.url), 'utf8'); document.head.append(style);
    assert.deepEqual(snapshot(), before);
  } finally { dom.window.close(); }
});

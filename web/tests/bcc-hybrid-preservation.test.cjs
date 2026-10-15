'use strict';
// Source retention and isolated bootstrap behavior; never rendered parity.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.join(__dirname, '..', '..');
const scripts = ['assets/world-land.js', 'bcc-earth-v3.js', 'bcc-welcome-globe-v3.js',
  'bcc-map.js', 'bcc-app.js', 'bcc-operations-v3.js'];
const sourcePaths = ['web/bcc-app.html', 'web/bcc-app.css', 'web/assets.go',
  ...scripts.map(name => 'web/' + name)];
const sources = Object.fromEntries(sourcePaths.map(name => [name, fs.readFileSync(path.join(root, name), 'utf8')]));
const shellIDs = ['welcomeGlobe', 'nav', 'menuToggle', 'search', 'classicLink', 'main'];
const dashboardIDs = ['mapCanvas', 'kpis', 'serverList', 'range', 'trafficChart', 'events', 'health', 'costs', 'quick'];

function inspect(snapshot) {
  const errors = [];
  const html = snapshot['web/bcc-app.html'] || '';
  const app = snapshot['web/bcc-app.js'] || '';
  const css = snapshot['web/bcc-app.css'] || '';
  const embedded = (snapshot['web/assets.go'] || '').match(/^\/\/go:embed (.+)$/m)?.[1].split(/\s+/) || [];
  const loaded = Array.from(html.matchAll(/<script\s+src="\.\/([^"]+)"/g), match => match[1]);
  for (const name of [...scripts, 'bcc-app.css']) {
    if (!snapshot['web/' + name]?.trim()) errors.push('missing asset: ' + name);
    if (!embedded.includes(name.startsWith('assets/') ? 'assets/*' : name)) errors.push('not embedded: ' + name);
  }
  if (!embedded.includes('bcc-app.html')) errors.push('page not embedded');
  if (!html.includes('href="./bcc-app.css"')) errors.push('stylesheet not loaded');
  if (JSON.stringify(loaded) !== JSON.stringify(scripts)) errors.push('visual/operations script dependency order changed');
  for (const id of shellIDs) if (!html.includes(`id="${id}"`)) errors.push('missing shell control: ' + id);
  for (const id of dashboardIDs) if (!app.includes(`id="${id}"`)) errors.push('missing dashboard control: ' + id);
  for (const route of ['dashboard', 'servers', 'tunnels', 'monitoring', 'security', 'ssl', 'backups', 'updates', 'finance', 'settings']) {
    const routes = app.match(/const ROUTES=\[([^\]]+)\]/)?.[1] || '';
    if (!routes.includes(`'${route}'`)) errors.push('missing navigation: ' + route);
  }
  if (!app.includes("$('classicLink').href=cfg.base+'classic/'")) errors.push('secret-path classic fallback missing');
  if (!app.includes('window.BAFT_MAP.create') || !snapshot['web/bcc-map.js']?.includes('window.BAFT_MAP=')) errors.push('map bridge missing');
  if (!snapshot['web/bcc-earth-v3.js']?.includes('window.BAFT_EARTH=')) errors.push('earth renderer missing');
  if (!app.includes('window.BAFT_OPS') || !snapshot['web/bcc-operations-v3.js']?.includes('window.BAFT_OPS=')) errors.push('operations bridge missing');
  if (!css.includes(':root[data-theme=light]')) errors.push('light theme missing');
  if (!/prefers-reduced-motion\s*:\s*reduce/.test(css)) errors.push('reduced motion missing');
  return errors;
}

function boot(html, preferences, systemLight = false, blocked = false) {
  const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  assert.ok(script, 'appearance bootstrap missing');
  const documentElement = { dataset: {} };
  vm.runInNewContext(script, {
    document: { documentElement },
    localStorage: { getItem(key) { if (blocked) throw new Error('storage denied'); return preferences[key] ?? null; } },
    matchMedia: () => ({ matches: systemLight }),
  }, { timeout: 1000 });
  return documentElement;
}

test('Hybrid retains modern BCC navigation, controls, visual assets and classic fallback', () => {
  assert.deepEqual(inspect(sources), []);
});

test('deleting any modern dashboard control is rejected independently of classic source', () => {
  for (const id of dashboardIDs) {
    const mutated = { ...sources, 'web/bcc-app.js': sources['web/bcc-app.js'].replace(`id="${id}"`, `id="removed-${id}"`) };
    assert.ok(inspect(mutated).includes('missing dashboard control: ' + id), id);
  }
});

test('deleting or unembedding each visual dependency is rejected', () => {
  for (const name of [...scripts, 'bcc-app.css']) {
    assert.ok(inspect({ ...sources, ['web/' + name]: '' }).includes('missing asset: ' + name), name);
    const token = name.startsWith('assets/') ? 'assets/*' : name;
    const changed = sources['web/assets.go'].replace(/^(\/\/go:embed .+)$/m, line => line.replace(token, 'removed'));
    assert.ok(inspect({ ...sources, 'web/assets.go': changed }).includes('not embedded: ' + name), name);
  }
});

test('removing fallback or reduced-motion support is rejected', () => {
  const fallback = sources['web/bcc-app.js'].replace("$('classicLink').href=cfg.base+'classic/'", 'removed');
  assert.ok(inspect({ ...sources, 'web/bcc-app.js': fallback }).includes('secret-path classic fallback missing'));
  const motion = sources['web/bcc-app.css'].replaceAll('prefers-reduced-motion', 'removed-motion');
  assert.ok(inspect({ ...sources, 'web/bcc-app.css': motion }).includes('reduced motion missing'));
});

test('reordering script dependencies is rejected', () => {
  const html = sources['web/bcc-app.html'].replace('src="./bcc-map.js"', 'src="./bcc-app.js"');
  assert.ok(inspect({ ...sources, 'web/bcc-app.html': html }).includes('visual/operations script dependency order changed'));
});

for (const language of ['fa', 'en']) for (const theme of ['dark', 'light']) {
  test(`actual appearance bootstrap preserves ${language}/${theme} before app initialization`, () => {
    const result = boot(sources['web/bcc-app.html'], { 'baft.lang': language, 'baft.theme': theme });
    assert.equal(result.lang, language);
    assert.equal(result.dir, language === 'fa' ? 'rtl' : 'ltr');
    assert.equal(result.dataset.theme, theme);
  });
}

test('invalid or unavailable storage safely uses Farsi direction and system appearance', () => {
  for (const blocked of [false, true]) for (const light of [false, true]) {
    const result = boot(sources['web/bcc-app.html'], { 'baft.lang': '<bad>', 'baft.theme': 'unknown' }, light, blocked);
    assert.equal(result.lang, 'fa');
    assert.equal(result.dir, 'rtl');
    assert.equal(result.dataset.theme, light ? 'light' : 'dark');
  }
});

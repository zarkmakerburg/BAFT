import { JSDOM, VirtualConsole } from 'jsdom';
import { SourceTextModule } from 'node:vm';
import { readFileSync } from 'node:fs';

/** Executes trusted compiled modules inside their own DOM realm, without a browser. */
export async function realm(preferences = { language: 'fa', theme: 'dark' }) {
  const errors = [];
  const console = new VirtualConsole();
  console.on('jsdomError', error => errors.push(error));
  const dom = new JSDOM('<!doctype html><html lang="en" dir="ltr"><body><div id="root"></div></body></html>', {
    url: 'https://example.invalid/aurora/', runScripts: 'outside-only', virtualConsole: console
    // Default resource loading stays disabled. No HTML scripts/CSS/network are loaded.
  });
  const { window } = dom;
  window.matchMedia = () => ({ matches: true }); // Theme choice stub, not media/layout proof.
  window.localStorage.setItem('baft.aurora.demo.preferences.v1', JSON.stringify(preferences));
  const forbidden = () => { throw Error('Network forbidden in DOM units'); };
  window.fetch = forbidden;
  window.XMLHttpRequest = forbidden;
  window.WebSocket = forbidden;
  const context = dom.getInternalVMContext();
  const modules = new Map();
  const allowed = new Set(['shell.js', 'mock-provider.js', 'real-provider.js', 'domain.js', 'validation.js', 'catalog.js', 'preferences.js', 'preview-entry.js']);
  function module(name) {
    if (!allowed.has(name)) throw Error(`Unreviewed module: ${name}`);
    if (!modules.has(name)) modules.set(name, new SourceTextModule(
      readFileSync(new URL(`../../generated/${name}`, import.meta.url), 'utf8'), { context, identifier: name }
    ));
    return modules.get(name);
  }
  async function load(name) {
    const result = module(name);
    if (result.status === 'unlinked') await result.link(specifier => {
      if (!specifier.startsWith('./')) throw Error('Only reviewed local modules allowed');
      return module(specifier.slice(2));
    });
    await result.evaluate();
    return result.namespace;
  }
  const { mountAurora } = await load('shell.js');
  const { createMockProvider } = await load('mock-provider.js');
  const root = window.document.getElementById('root');
  let destroy = () => {};
  return {
    window, document: window.document, root, errors, load,
    demo: createMockProvider,
    mount(provider = createMockProvider(), options = {}) { destroy = mountAurora(root, provider, options); return destroy; },
    click(action) { root.querySelector(`button[data-action="${action}"]`).click(); },
    async navigate(view) {
      const changed = new Promise(resolve => window.addEventListener('hashchange', resolve, { once: true }));
      window.location.hash = view; await changed;
    },
    close() { destroy(); dom.window.close(); }
  };
}

export const settle = async () => { await new Promise(resolve => setImmediate(resolve)); };
export function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

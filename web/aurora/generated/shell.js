import { createMockProvider } from './mock-provider.js';
import { reassessOverview } from './domain.js';
import { catalog } from './catalog.js';
import { readPreferences, savePreferences, viewFromHash } from './preferences.js';
const mounted = new WeakMap();
/** Owns one opt-in DEMO root. No polling, real provider, live actions or global fetch patch. */
export function mountAurora(root, provider = createMockProvider(), options = {}) {
    if (provider.mode !== 'demo')
        throw new Error('Aurora preview requires DEMO data');
    const host = options.host ?? 'standalone';
    if (host !== 'standalone' && host !== 'embedded')
        throw new Error('Invalid Aurora host mode');
    const embedded = host === 'embedded';
    mounted.get(root)?.();
    const doc = root.ownerDocument;
    const win = doc.defaultView;
    if (!win)
        throw new Error('A browser document is required');
    const HTMLElementClass = win.HTMLElement;
    const HTMLAnchorElementClass = win.HTMLAnchorElement;
    const abort = new AbortController();
    const previous = { language: doc.documentElement.lang, direction: doc.documentElement.dir };
    const previousRoot = new Map(['lang', 'dir', 'data-theme', 'data-aurora-root', 'data-aurora-host'].map(name => [name, root.getAttribute(name)]));
    root.dataset['auroraRoot'] = '';
    root.dataset['auroraHost'] = host;
    let preferences;
    try {
        preferences = readPreferences(win.localStorage, win.matchMedia('(prefers-color-scheme: dark)').matches);
    }
    catch {
        preferences = { language: 'fa', theme: 'dark' };
    }
    let view = embedded ? 'overview' : viewFromHash(win.location.hash);
    let data = null;
    let state = 'loading';
    let saved = true;
    let disposed = false;
    let request = 0;
    function element(tag, css = '', text = '') {
        const node = doc.createElement(tag);
        if (css)
            node.className = css;
        if (text)
            node.textContent = text;
        return node;
    }
    function control(action, text) {
        const button = element('button', 'button', text);
        button.type = 'button';
        button.dataset['action'] = action;
        return button;
    }
    function badge(text) { return element('span', 'badge', text); }
    function field(list, label, value, isolated = false) {
        const row = element('div', 'field');
        row.append(element('dt', '', label));
        const description = element('dd');
        if (isolated) {
            const bdi = element('bdi', '', value);
            bdi.dir = 'ltr';
            description.append(bdi);
        }
        else
            description.textContent = value;
        row.append(description);
        list.append(row);
    }
    function time(value, t) {
        if (value === null)
            return t.missing;
        const date = new Date(value);
        if (!Number.isFinite(date.getTime()))
            return t.unknown;
        return new Intl.DateTimeFormat(preferences.language === 'fa' ? 'fa-IR' : 'en-GB', {
            dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC'
        }).format(date) + ' UTC';
    }
    function section(title, note) {
        const result = element('section', 'panel');
        result.append(element('h2', '', title), element('p', 'muted', note));
        return result;
    }
    function render(focusAction) {
        if (disposed)
            return;
        // Replacing the root removes its focused descendants. Preserve only a
        // known shell target; never steal focus from elsewhere in the document.
        const active = doc.activeElement;
        const ownedFocus = active !== null && root.contains(active);
        const previousAction = ownedFocus && active instanceof HTMLElementClass ? active.dataset['action'] : undefined;
        const action = focusAction ?? (previousAction === 'language' || previousAction === 'theme' || previousAction === 'reload' ? previousAction : undefined);
        const mainFocused = ownedFocus && active.id === 'aurora-content';
        const linkHref = ['#overview', '#nodes', '#tunnels', '#aurora-content'].find(href => ownedFocus && active instanceof HTMLAnchorElementClass && active.getAttribute('href') === href);
        const t = catalog(preferences.language);
        root.lang = preferences.language;
        root.dir = preferences.language === 'fa' ? 'rtl' : 'ltr';
        if (!embedded) {
            doc.documentElement.lang = preferences.language;
            doc.documentElement.dir = root.dir;
        }
        root.dataset['theme'] = preferences.theme;
        const fragment = doc.createDocumentFragment();
        const skip = element('a', 'skip', t.skip);
        skip.href = '#aurora-content';
        fragment.append(skip);
        const header = element('header', 'topbar');
        const brand = element('div', 'brand');
        const mark = element('span', 'brand-mark', 'B');
        mark.setAttribute('aria-hidden', 'true');
        const brandText = element('div');
        brandText.append(element('strong', '', 'BAFT'), element('span', 'muted', t.brandNote));
        brand.append(mark, brandText);
        header.append(brand);
        const appearance = element('div', 'controls');
        appearance.setAttribute('role', 'group');
        appearance.setAttribute('aria-label', t.controls);
        const language = control('language', preferences.language === 'fa' ? 'EN' : 'فارسی');
        language.setAttribute('aria-label', t.language);
        const theme = control('theme', preferences.theme === 'dark' ? '☀' : '◐');
        theme.setAttribute('aria-label', t.theme);
        theme.setAttribute('aria-pressed', String(preferences.theme === 'light'));
        appearance.append(language, theme);
        header.append(appearance);
        fragment.append(header);
        const layout = element('div', 'layout');
        const sidebar = element('aside', 'sidebar');
        const nav = element('nav');
        nav.setAttribute('aria-label', t.nav);
        for (const name of ['overview', 'nodes', 'tunnels']) {
            const link = element('a', 'nav-link', t[name]);
            link.href = `#${name}`;
            if (view === name)
                link.setAttribute('aria-current', 'page');
            nav.append(link);
        }
        sidebar.append(element('p', 'eyebrow', t.eyebrow), nav, element('p', 'sidebar-note muted', t.localOnly));
        const main = element(embedded ? 'section' : 'main', 'content');
        main.id = 'aurora-content';
        main.tabIndex = -1;
        if (embedded)
            main.setAttribute('aria-label', t[view]);
        const intro = element('div', 'intro');
        const heading = element('div');
        heading.append(element('p', 'eyebrow', 'BAFT / AURORA'), element('h1', '', view === 'overview' ? t.title : t[view]));
        intro.append(heading, badge(t.demo));
        main.append(intro);
        const notice = element('div', 'notice');
        notice.append(element('strong', '', t.notice), element('p', '', t.demoClock));
        main.append(notice);
        const status = element('div', 'load-status');
        status.setAttribute('role', 'status');
        status.setAttribute('aria-live', 'polite');
        status.textContent = state === 'loading' ? t.loading : state === 'error' ? t.error : '';
        if (!saved)
            status.append(element('p', '', t.storageFailed));
        main.append(status);
        const reload = control('reload', state === 'error' ? t.retry : t.refresh);
        reload.disabled = state === 'loading';
        main.append(reload);
        if (data !== null && state === 'ready') {
            const snapshot = reassessOverview(data, Date.now());
            if (view === 'overview') {
                const statistics = element('section', 'statistics');
                statistics.setAttribute('aria-label', t.overview);
                const numbers = new Intl.NumberFormat(preferences.language === 'fa' ? 'fa-IR' : 'en-GB');
                for (const [label, value, note] of [[t.nodeCount, numbers.format(snapshot.nodes.length), t.demo],
                    [t.tunnelCount, numbers.format(snapshot.links.length), t.phase], [t.delivery, t.unknown, t.deliveryNote]]) {
                    const card = element('div', 'stat');
                    card.append(element('p', 'muted', label), element('strong', 'stat-value', value), element('p', 'muted', note));
                    statistics.append(card);
                }
                main.append(statistics);
            }
            if (view !== 'tunnels') {
                const nodes = section(t.nodeTitle, t.nodeNote);
                const cards = element('div', 'resource-grid');
                for (const node of snapshot.nodes) {
                    const card = element('article', 'resource');
                    const title = element('h3');
                    const name = element('bdi', '', node.alias);
                    title.append(name);
                    card.append(title, badge(t[node.effectiveStatus]));
                    const fields = element('dl');
                    field(fields, t.role, node.role === 'worker' ? t.worker : node.role === 'foreign' ? t.foreign : t.unknown);
                    field(fields, t.country, node.country ?? t.unknown, node.country !== null);
                    field(fields, t.lastSeen, time(node.lastSeen, t));
                    field(fields, t.status, t[node.reportFreshness]);
                    card.append(fields);
                    cards.append(card);
                }
                if (snapshot.nodes.length === 0)
                    cards.append(element('p', '', t.empty));
                nodes.append(cards);
                main.append(nodes);
            }
            if (view !== 'nodes') {
                const tunnels = section(t.tunnelTitle, t.tunnelNote);
                for (const tunnel of snapshot.links) {
                    const card = element('article', 'resource');
                    const name = element('h3');
                    const id = element('bdi', '', tunnel.id);
                    id.dir = 'ltr';
                    name.append(id);
                    card.append(name);
                    const fields = element('dl');
                    field(fields, t.phase, tunnel.desiredPhase === 'active' ? t.active : tunnel.desiredPhase === 'rolled_back' ? t.rolled_back : t.unknown);
                    field(fields, t.path, `${tunnel.irNode} → ${tunnel.exNode}`, true);
                    field(fields, t.delivery, t.unknown);
                    card.append(fields);
                    tunnels.append(card);
                }
                if (snapshot.links.length === 0)
                    tunnels.append(element('p', '', t.empty));
                main.append(tunnels);
            }
            const evidence = section(t.dataBoundary, t.boundaryNote);
            const source = element('p', 'muted', `${t.generated}: ${time(snapshot.generatedAt, t)} · ${t[snapshot.freshness]}`);
            evidence.append(source);
            main.append(evidence);
        }
        const next = section(t.next, t.nextNote);
        next.classList.add('qualification');
        main.append(next, element('footer', 'muted', t.footer));
        layout.append(sidebar, main);
        fragment.append(layout);
        root.replaceChildren(fragment);
        if (action)
            root.querySelector(`button[data-action="${action}"]`)?.focus();
        else if (linkHref)
            root.querySelector(`a[href="${linkHref}"]`)?.focus();
        else if (mainFocused)
            root.querySelector('#aurora-content')?.focus();
    }
    async function reload(returnFocus = false) {
        const sequence = ++request;
        state = 'loading';
        render();
        try {
            const result = await provider.getOverview(abort.signal);
            if (disposed || sequence !== request)
                return;
            if (result.mode !== 'demo')
                throw new Error('Invalid demo source');
            const restore = returnFocus && doc.activeElement === doc.body;
            data = result;
            state = 'ready';
            render(restore ? 'reload' : undefined);
        }
        catch {
            if (disposed || sequence !== request)
                return;
            const restore = returnFocus && doc.activeElement === doc.body;
            data = null;
            state = 'error';
            render(restore ? 'reload' : undefined);
        }
    }
    root.addEventListener('click', event => {
        const target = event.target;
        if (!(target instanceof win.Element))
            return;
        // Embedded islands do not own the BCC hash router or document language.
        const link = target.closest('a');
        const href = link?.getAttribute('href');
        if (embedded && link && root.contains(link) &&
            (href === '#overview' || href === '#nodes' || href === '#tunnels' || href === '#aurora-content')) {
            event.preventDefault();
            if (href !== '#aurora-content') {
                view = viewFromHash(href);
                render();
            }
            root.querySelector('#aurora-content')?.focus();
            return;
        }
        const action = target.closest('button[data-action]')?.dataset['action'];
        if (action === 'reload') {
            if (state !== 'loading')
                void reload(true);
            return;
        }
        if (action !== 'theme' && action !== 'language')
            return;
        preferences = action === 'theme' ? { ...preferences, theme: preferences.theme === 'dark' ? 'light' : 'dark' }
            : { ...preferences, language: preferences.language === 'fa' ? 'en' : 'fa' };
        try {
            saved = savePreferences(win.localStorage, preferences);
        }
        catch {
            saved = false;
        }
        render(action);
    }, { signal: abort.signal });
    if (!embedded)
        win.addEventListener('hashchange', () => {
            if (win.location.hash === '#aurora-content') {
                root.querySelector('#aurora-content')?.focus();
                return;
            }
            view = viewFromHash(win.location.hash);
            render();
            root.querySelector('#aurora-content')?.focus();
        }, { signal: abort.signal });
    const destroy = () => {
        if (disposed)
            return;
        disposed = true;
        request++;
        abort.abort();
        root.replaceChildren();
        for (const [name, value] of previousRoot) {
            if (value === null)
                root.removeAttribute(name);
            else
                root.setAttribute(name, value);
        }
        if (!embedded) {
            doc.documentElement.lang = previous.language;
            doc.documentElement.dir = previous.direction;
        }
        mounted.delete(root);
    };
    mounted.set(root, destroy);
    void reload();
    return destroy;
}

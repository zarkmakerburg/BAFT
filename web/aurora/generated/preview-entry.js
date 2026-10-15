import { mountAurora } from './shell.js';
const root = document.getElementById('aurora-preview');
if (root) {
    let destroy = mountAurora(root);
    window.addEventListener('pagehide', () => { destroy(); });
    // A bfcache restore does not re-execute modules after pagehide teardown.
    window.addEventListener('pageshow', event => {
        if (event.persisted)
            destroy = mountAurora(root);
    });
}

# Aurora Hybrid Imperial Console — P0 inventory and preservation contract

Date: 2026-10-17 · Tracking: #67, #243 · Status: source-audit only, no visual acceptance claimed.

## Existing assets and integration points (verified on default branch)
- `web/assets.go`: Go `embed.FS` serving `bcc-app.html`, `bcc-app.css`, `bcc-app.js`, `bcc-map.js`, `bcc-operations-v3.js`, `bcc-earth-v3.js`, `bcc-welcome-globe-v3.js` and bundled `assets/*`. UI embedded in BCC; do not assume standalone React/Next.js.
- `web/bcc-app.html`: `lang="fa" dir="rtl" data-theme="dark"` bootstrap; persisted local theme and language; landing `canvas#welcomeGlobe`.
- `web/bcc-earth-v3.js`: WebGL sphere shader using day/night/ocean-mask/relief textures; requires WebGL graceful fallback.
- `web/bcc-welcome-globe-v3.js`: animated canvas globe with draggable rotation and illustrative Tehran-origin arcs. **Landing arcs are illustrative**, not live link measurements. Keep label/semantics honest.
- `web/bcc-map.js`: 2D canvas world map with reported IR→EX links and geographic nodes; loads bundled Earth images, checks reduced-motion, and derives country/location points from UI data. Treat coordinates with known precision.
- `web/bcc-app.css`: existing theme/UI including Aurora Tunnel Studio integration.
- `web/tests/aurora-tunnel-studio.test.cjs`: existing tests asserting integration with classic tunnel controls and reviewed decommission flow.
- `docs/bcc-command-center-v3.md`: frozen original visual spec; dictates WGS84, real arcs, no fabricated metrics, FA/EN, reduced motion and no live changes.
- `docs/qa/bcc-v3-design-qa.md`: visual after-captures and final browser recheck still blocked at latest cited state; **no browser visual pass** established by this inventory.
- `web/assets/earth-textures-NOTICE.txt`: asset attribution/usage notice; preserve on redistribution.

## Architecture decision
No migration to a foreign dashboard framework by default: this project embeds vanilla HTML/CSS/JS through Go. Adopt design tokens and compatible standalone components while keeping bundle/API/security intact. A future framework migration requires separate technical and bundle/performance decision.

## Must-preserve behavioral baseline
1. Landing draggable rotating 3D globe, globe texture day/night and fallback; interactive primary BCC earth.
2. Network map: real positioned nodes and IR→EX arcs, hover/select actions and details, no unknown=>healthy conversion.
3. Dark/light, FA RTL / EN LTR, responsive/mobile and reduced-motion, accessible interactions.
4. All existing BCC cards, navigation, tunnel management, confirmations, decommission plan/hash, authentication and authorization.
5. Live vs illustrative contract: never re-label welcome demo arcs or missing metrics as live.
6. Verify network map and globe separately; screenshot actual current authenticated and empty state, mock DEMO separately.

## Proposed work after inventory
A. Capture current baseline in Chrome at 1440×1000 and 390 px, FA/EN, dark/light, populated demo and real empty state; inventory interaction and performance. **Not yet complete.**
B. Add CSS token namespace `--baft-imperial-*` behind opt-in feature flag; do not alter production defaults; test compatibility.
C. Upgrade globe/map visuals and selection synchronization in small reviewed commits with screenshot, reduced-motion and performance evidence.
D. Integrate charts/navigation and test API parity; run existing `web/tests/*` and BCC CI checks.
E. Post SHA/CI/screenshots to #67. No merge/release/deploy without explicit HQ authorization.

## Blockers and open checks
- Need fresh browser screenshots and direct observation on this exact commit before calling visual acceptance.
- Need a complete audit of actual map and globe JS event APIs, available telemetry fields and link status mapping.
- License gate applies to any third-party source code, art, font or template; inspiration alone is not an import authorization.

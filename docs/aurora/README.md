# Aurora implementation baseline and execution ledger

Baseline: `1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7` (2026-10-15).
This is a source inventory and implementation plan, not UI or operational acceptance.

Current incremental source adds [A1 data foundation](../../web/aurora/README.md)
and [A2 opt-in DEMO shell](a2-shell.md). A2 source is implemented but browser,
mobile/visual/accessibility/performance qualification remains OPEN; the historical
bootstrap inventory below must not be mistaken for current runtime integration.

Authority: [Master Engineering](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6178389947),
[Aurora](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6187169497),
[combined execution contract](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6187258582),
and roadmap [#199](https://github.com/zarkmakerburg/BAFT/issues/199).

## Existing architecture and reusable parts

| Surface | Actual source | Reuse / boundary |
|---|---|---|
| Page and bootstrap | `web/bcc-app.html`, `web/assets.go` | Go embeds public assets; `Page` injects request-specific `BAFT_UI` JSON and secret-path base. Never put request secrets in generated files. |
| Shell, navigation and pages | `web/bcc-app.js` | Ten hash routes; native session, theme/language preferences, dashboard and tool mounting. Preserve routes during incremental migration. |
| Design tokens and layout | `web/bcc-app.css` | Dark/light tokens, bundled Vazirmatn font, logical direction rules and focus styles already exist; reuse before replacing. |
| Operations bridge | `web/bcc-operations-v3.js`, `internal/bcc/dashboard.go` | Existing operations are mounted from server-provided HTML/JS with scoped IDs. It patches API fetch for same-origin sessions and CSRF. This global bridge is a coupling risk, not a foundation to copy into new modules. |
| Map and globe | `web/bcc-map.js`, `web/bcc-earth-v3.js`, `web/bcc-welcome-globe-v3.js` | Existing canvas/rendering and local assets; retain as fallback while evaluating an accessible topology surface. Country anchors are approximate locations. |
| Access and HTTP APIs | `internal/bcc/access.go`, `internal/bcc/server.go` | Secret-path session routing, protected assets and CSRF checks remain server-owned. Preserve classic fallback and API semantics. |
| Operational domain | `internal/bcc/overview.go`, `tunnel.go`, `route_doctor.go` | Desired tunnel phase is not data-plane health. Doctor assesses stored evidence; it does not create a fresh network measurement. |
| Automated UI checks | `web/tests/*.test.cjs`, `.github/workflows/bcc-v3-ui-tests.yml` | Node syntax and contract tests. Passing these is not browser/accessibility acceptance. |
| Browser harness | `web/qa/bcc-app-chrome.cjs` | Playwright against an explicitly configured BCC login. Existing matrix has all four desktop combinations but only FA/dark and EN/light at mobile 390px. |

No repository `AGENTS.md`, `package.json` or `tsconfig.json` was found in the
materialized baseline. There is no established React/Next.js build to assume.
See [ADR 0008](../adr/0008-aurora-incremental-frontend.md) before adding tooling.

## Routes and backend contracts

| Route | Current surface / dependency | Remaining boundary |
|---|---|---|
| dashboard | overview, alerts, audit, map and traffic | Validate response schemas and preserve unknown/stale values. |
| servers | inventory, SSH bootstrap, manual enrollment and migration tools | Credential cleanup, host-key scope and explicit country override must survive migration. |
| tunnels | plan/create/edit, discovery, Doctor, graph/matrix and change ledger | Keep plan-hash confirmation and rollback; no mock action may enter live APIs. |
| monitoring | monitoring, node history and layered health | Do not turn aggregate node metrics into per-tunnel failure evidence. |
| security | immutable audit and session information | Read-only history; do not invent multiple RBAC roles. |
| ssl | certificate rotations | A pairing certificate without an expiry report remains unknown. |
| backups | inventory, create and restore preview | HTTP restore preview is not a restore operation. Actual restore is offline CLI. |
| updates | deployment and recent jobs | Existing deploy/job contracts, not an assumed generic updates API. |
| finance | versioned rates, ledger and reports | Integer micros and byte counters retain exact semantics. |
| settings | theme/language/map preferences and version | Browser preferences are not server configuration or a generic settings API. |

The app's `api` helper sends same-origin credentials, disables caching, refuses
redirects, includes CSRF for writes and dispatches session expiry on 401.
New adapters must validate unknown JSON before rendering; TypeScript types
alone do not validate network responses. Error bodies are not uniformly JSON.
Use safe operator-facing messages and do not display arbitrary backend text.

Polling currently runs every 15 seconds while visible; UI evidence freshness
is repainted separately. Preserve the hidden-tab pause and avoid overlapping
requests or increasing traffic until a measured need exists.

## Readiness and prioritized workstreams

All rows are development work. None is production-verified Aurora.

| ID | Existing source foundation | Next bounded output | Acceptance needed |
|---|---|---|---|
| A | Themes, FA/EN catalogs, shared CSS and session shell | Typed domain/provider boundary, semantic tokens and accessible primitives | Strict typecheck, schema tests, catalog parity, theme/direction checks |
| B | Responsive CSS; limited mobile QA matrix | 320/390/768px layouts, touch controls and table alternatives | All four language/theme combinations, focus and overflow checks |
| C | Geographic canvas and selection panel | Accessible topology view and inspector with measured/unknown overlays | Keyboard selection, fit/zoom, no invented coordinates or healthy edges |
| D | Existing operational plan/edit tools | Guided/expert Studio with effective-config/diff/confirmation | Backend capability contract and safety tests before new live writes |
| E | Overview, history and layered monitoring | Deterministic mock provider and validated read-only real adapters | Per-layer scope/timestamps; missing and stale metrics stay unknown |
| F | Separate classifier/SQLite PRs #202/#203 | Evidence-driven incident display after read API review | Scoped trusted producer, stale/replay behavior, persistence and sibling isolation |
| G | Existing search/navigation | Command palette and focus-safe inspector | Permissions-aware actions, Ctrl/Cmd+K, Escape and focus return |
| H | Change ledger and tunnel lifecycle | Journey and before/after history/restore preview | No implied traffic delivery or implemented online restore |
| I | Canvas motion and reduced-motion support | Purposeful optional effects after functional gates | Reduced effects, lifecycle cleanup and measured rendering cost |

Quiet/Balanced/Boost remain demo or explanatory controls until supported
tunables and measured effects exist. They do not authorize transport, shaping,
encryption or exit selection changes. Traffic Identity shows known protocol
and encryption properties; unmeasured fingerprint similarity and blockability
remain unknown. No claims of browser/YouTube impersonation.

## Risk register

| Priority | Risk | Required mitigation |
|---|---|---|
| P0 | New shell bypasses secret-path/session/CSRF or leaks bootstrap values | Retain server auth, same-origin adapters and session-expiry tests; no credentials in fixtures/artifacts. |
| P0 | Studio bypasses candidate/plan-hash/rollback gates | Keep authoritative backend validation; new actions demo-only until reviewed integration. |
| P1 | Desired state, cached PASS or node counters appear as proven tunnel health | Separate control/data state, schema/scope checks and observation-time TTLs. |
| P1 | Global operations bridge and new components both own handlers/IDs/timers | One owner per mounted surface; explicit teardown and rollback to existing page. |
| P1 | Incident UI races unmerged persistence/classifier changes | Keep #202/#203 independent; implement reviewed authenticated read contract before wiring. |
| P1 | React/graph/chart dependencies create unnecessary rewrite or supply-chain cost | Record license, maintenance, security, compatibility and compressed-size evidence before adoption. |
| P2 | Attractive desktop preview conceals mobile/RTL/accessibility failures | Complete matrix, keyboard and reduced-motion tests before visual acceptance. |

Initial uncompressed source sizes: app JS 79,163 bytes, CSS 31,363,
map JS 14,747, operations JS 18,109 (143,382 combined). These exclude other
assets, compression, parse time, fonts and textures; they are not a performance
benchmark. Capture baseline transfer/render/memory measurements before setting
regression budgets. Do not estimate supported user counts from these sizes.

## Validation and milestone ledger

2026-10-15 bootstrap:

- Fresh isolated checkout from the exact baseline; no existing user work changed.
- Source/API/embedding/QA inspection completed for the surfaces above.
- Baseline `node --test web/tests/*.test.cjs`: **14 PASS**, zero failure/skip.
- Runtime visual/accessibility/performance matrix: **NOT RUN** this slice.
- Go build/test: **NOT RUN** in this new environment (Go toolchain absent).
- TypeScript compiler: absent; no dependency added or typecheck claimed.
- Playwright module is present; a configured local/staging BCC and browser
  executable still require verification before native authenticated QA.
- No Aurora UI, new API adapter, screenshot or nine-workstream completion is
  claimed by these planning documents. No production action occurred.

Next: implement A1, the strict typed domain and mock/real provider boundary,
with a pinned reviewed compiler and deterministic contract tests. Then build
the bilingual dual-theme shell and expand the mobile QA matrix. Keep exact
SHA evidence and failed attempts in #67/#199; claim scope before edits.

Native two-node/candidate pre-cutover tests remain owner-deferred and OPEN.
Local unit/integration/fuzz/failure injection work can continue independently.

## A1 — typed provider foundation (2026-10-15)

Implemented in `web/aurora`: strict domain models, unknown-input validation,
read-only same-origin overview/tunnel adapter, deterministic DEMO provider and
isolated in-memory demo edits. No page or default route is activated. Generated
ES2022 modules are checked into source and compared with a temporary rebuild;
Go embedding remains unchanged. No Node service/runtime dependency is required.

Health/freshness stays scoped: active deployment is not payload health, absent
is not zero, missing/future/stale evidence cannot imply UP, metric timestamps
are not borrowed from unrelated observations, and country anchors are approximate.
Cached evidence can be reassessed without renewing timestamps. Exact counter
helpers reject unsafe rounded JSON numbers; finance integration is not claimed.

The real provider only reads existing overview/tunnel endpoints, preserving
same-origin cookies, redirect refusal, safe session expiry and cancellation.
It has a streamed byte ceiling/deadline and no write interface or mock fallback.
The mock provider is synthetic, explicitly DEMO, and has no fetch/polling effects.

Development checks: strict typecheck and negative capability tests PASS; exact
generated rebuild PASS; 19 behavioral tests PASS; existing UI contracts 14/14
PASS; all current/new JS scripts syntax PASS; workflow supply-chain policy PASS.
Initial provider run failed two new regressions (late cancelled 401 callback,
unclosed error stream); corrected implementation passes both. Failed output is
retained in evidence and not counted as a pass. Registry audit reports zero
known advisories for the one pinned build dependency; see
[admission record](toolchain-review.md) and [package contract](../../web/aurora/README.md).

Exact published SHA and current-head CI are reported in #67/#199 after publication;
prior bootstrap CI is not acceptance of this source. Go tests are not run locally
in this toolchain-limited environment. No Go source or embedded asset list changed.
Browser, responsive/RTL/theme visuals, accessibility/performance measurements and
native two-node acceptance remain NOT RUN/OPEN. Next: A2/B shared opt-in shell,
FA/EN + dark/light tokens and accessible primitives, then mobile-first layouts.

# A2 shell — implemented source, visual qualification OPEN

2026-10-15. Parent source: `b5548ebb1a4751cb868cee8587afa7605ff2ab03`.
Board claim: [6189304572](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6189304572).

## Bounded implementation

`web/aurora/preview/index.html` is an opt-in **static DEMO** entry. It is not
embedded or routed by BCC, and does not replace the dashboard or classic page.
Open it only through an appropriate local static preview serving the repository
`web/` tree; ES module files need HTTP, not `file:`. No production URL is claimed.
Current managed environment has no supported control-browser capability, so no
preview server or alternative browser installation was started here.

The shell implements FA/EN catalogs, RTL/LTR document direction, dark/light
semantic tokens, safe persisted preview-only preferences, overview/node/tunnel
read views, accessible landmarks/skip link/focus hooks, loading/empty/error/unknown
states, and mobile CSS at 600/900px with min-content-safe layouts and 44px controls.
The 320px acceptance matrix below remains OPEN: source rules are not measurements.
No map, Studio, incident workflow, live telemetry or operational action is implied.

The sole provider is A1's explicit DEMO provider. Runtime and negative type checks
reject live providers. The preview CSP also refuses network connections, forms,
inline scripts and objects. No credentials, live IPs or backend HTML are included.
Node aliases, IDs, time and statuses render as text; link desired phase is never
payload health. The historical fixture reassesses UNKNOWN at each render using
the real current clock, without renewing evidence time. There is no polling or
timer. The existing backend, session/CSRF and real-provider contracts are unchanged.

One mount owns its DOM root/listeners/abort signal; remount destroys the previous
owner. Destroy cancels pending loads and rejects late render/session effects.
Theme/language changes return focus to their replacement controls; navigation
focuses main. User-triggered reload returns focus only when the old control was
removed and the user has not focused another element. Browser behavior of these
hooks is **not yet executed**. Entry teardown includes bfcache re-mount handling.

## Measured cost and budgets

No new dependency or asset was admitted. Reuses bundled Vazirmatn and its existing
OFL notice; no external font/CDN. Compiler remains exact-pinned TypeScript6.0.3.
Foundation bytes remain unchanged: 15,508 raw / 5,106 summed gzip bytes. A2 adds
four ES2022 modules; before the tiny bfcache entry amendment, they measured 18,948
raw / 6,317 summed gzip. Final command output on the published SHA is authoritative.
CSS initially measured 5,828 raw / 1,931 gzip. These exclude shared font/HTML,
browser parse/render/memory costs and are not runtime performance results.

The original A1 32KiB raw/12KiB gzip guard is preserved. A2 modules have a separate
32KiB/10KiB guard; preview CSS 12KiB/4KiB. This separation reviews the scope addition
without weakening A1's original allowance. Current BCC embedding/transfer delta
is still zero; serving the opt-in preview naturally transfers its own files.

## Actual checks and all attempts

- Fresh live PR200/204, board and previous A1 release/CI read; no active overlapping
  claim when this unique scoped claim was acquired or at the second pre-edit check.
- Initial review API calls had the wrong repo argument and returned argument errors;
  corrected calls succeeded with no submitted reviews/threads. No mutation retried.
- Existing clean checkout b5548eb was reused only as worktree origin; a new isolated
  A2 worktree/branch was created. Historical dirty poster and unpublished A1 candidate
  were not changed. AGENTS absence search returned1; guessed mock filename read
  returned2, then actual package inventory/read succeeded. Missing browser/tool paths
  returned2; no permission bypass, network workaround or browser install attempted.
- Locked install with lifecycle scripts disabled succeeded. Initial strict build and
  positive/negative type checks passed. Initial size check **FAILED** because combined
  A1+A2 exceeded A1's foundation-only guard; split scope budgets above then passed.
- Strict/negative checks, exact isolated rebuild (8 modules), 27 Node tests (19 A1 +
  8 A2), and existing UI14/14 PASS. Eight A2 tests are pure preference/catalog/security
  and source-contract/token-contrast checks, NOT an executed DOM/browser suite.
- The calculated WCAG contrast check covers theme foreground/muted/accent >=4.5:1
  and borders/focus >=3:1 against declared surfaces; this is not a complete rendered
  accessibility audit. No runtime performance or visual score is invented.
- One shell chain returned1 after all14 UI tests passed because an optional scripts
  filename search had no matches; not reported as a test failure. npm's environment
  http-proxy deprecation warning remains; environment config was not edited.
- A later multi-file patch failed context verification before applying; the exact
  current lines were inspected, then the corrected patch and full build/tests ran.
- Exact-source test commands, CI results and any later attempts belong in #67/#199.

## Outstanding acceptance matrix

| Evidence | Required cases | Status |
| --- | --- | --- |
| Screenshots/overflow | FA/EN × dark/light × desktop/mobile, plus320/390/768/1440px | NOT RUN — supported browser control unavailable; installed Playwright's Chromium binary absent |
| Keyboard and focus | Skip, navigation, theme/language, reload/error, remount/bfcache | NOT RUN — source hooks only |
| Accessibility | Rendered contrast, screen reader/name/order, zoom200%, forced colors | NOT RUN — token arithmetic only |
| Motion/performance | Reduced motion, low-effects, transfer/parse/render/memory baseline | NOT RUN — source rules and bytes only |
| Live integration | Session expiry and auth/classic fallback in actual BCC | NOT RUN — isolated DEMO only |
| Native nodes | IR↔EX and candidate-before-cutover | Owner-DEFERRED/OPEN |

Do not mark A2/B accepted or advance a production cutover from source/CI alone.
Next: qualify this exact source in the supported browser environment, fix observed
mobile/keyboard/visual issues, then continue shared primitives and B/C work. No new
production action, main merge/tag/release, transport/shaping, pool/exit switching,
restart-resume or standby resumption is authorized by this frontend slice.

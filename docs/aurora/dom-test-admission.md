# A2 DOM behavior regression qualification — 2026-10-15

This is a Node DOM unit test, not browser installation, browser control, served
preview, rendering, screenshot or visual/mobile acceptance. Those A2/B gates stay
OPEN. It exercises the actual compiled shell in an isolated DOM JavaScript realm,
so lifecycle assertions improve on source-string checks without claiming layout.

## Test-tool admission

Development-only `jsdom@30.1.3` is exact pinned with SHA512 lock integrity
`sha512-hQMUPvRgN7KxsfIytDQji7bimuJeRQa6Qo5+iuiZQk5CafeRxLIA10/cG5YCBFNpD9RO3oqIiN4CO1Y44kxqXg==`.
The installed `LICENSE.txt` and upstream license were read: MIT, copyright Elijah
Insua. Preserve notices when redistributing test tooling. The resolved development
graph has37 packages (including the existing compiler), no install hooks or optional
packages; lock metadata licenses are MIT, MIT-0, BSD-2/3-Clause, BlueOak-1.0.0,
CC0-1.0, ISC and Apache-2.0. These licenses remain in their installed packages.
This inventory is not a legal opinion or undisclosed-vulnerability guarantee.

Primary review sources:

- <https://github.com/jsdom/jsdom> — maintained DOM/HTML test implementation;
  scripts/subresource loading are disabled by default; no layout/rendering engine.
- <https://github.com/jsdom/jsdom/blob/main/LICENSE.txt> — upstream MIT terms.
- <https://registry.npmjs.org/jsdom/30.1.3> — exact metadata, Node compatibility,
  integrity and package size. Registry publish2026-10-15T09:24:38.912Z; release index
  still listed30.1.2 during review. Exact30.1.3 GitHub release page could not be
  retrieved, so no independent GitHub release-date verification is claimed.
- <https://github.com/jsdom/jsdom/releases> — active30.x maintenance, including
  recent focus/DOM correctness fixes. Registry node engines:
  `^22.22.2 || ^24.15.0 || >=26.0.0`; local24.19.0 satisfies. Existing CI selects
  current Node22; older Node22 patch versions no longer qualify for this test tool.

Alternatives: LinkeDOM0.18.13 (ISC) metadata reviewed but NOT installed; it explicitly
does not aim to emulate the browser behavior needed for focus/event lifecycle.
Source-only checks did not catch pending-load focus loss. A full browser is still
required for layout/accessibility; no browser/server workaround is introduced here.

Lock-only resolution and npm advisory audit ran BEFORE installing the new graph:
zero known registry advisories. `npm ci --ignore-scripts --no-audit --no-fund` then
installed37 packages with no lifecycle script. Exact jsdom unpacked metadata is
5,540,685 bytes; measured entire node_modules45,823,423 bytes including compiler.
No jsdom import reaches runtime source/generated modules. Production/browser bundle
delta from the tool is zero; only the small focus fix changes a generated shell.
No canvas package or browser binary is downloaded.

## Harness boundary

Tests load whitelisted local committed ES2022 modules through Node's
`SourceTextModule` in `getInternalVMContext()`, with `runScripts: outside-only`.
No host/window global mixing, HTML script execution, external module imports or
subresource loading. Fetch/XHR/WebSocket are fail-closed test stubs. Only synthetic
fixtures and trusted repository modules execute. The VM module flag is experimental
and its warning is retained; CI must execute it rather than treating a missing
capability as a pass. `matchMedia` supplies only a theme-choice stub, not viewport
simulation. Every realm is torn down with `window.close()`.

## Reproduced regression and fix

Parent source `2abb0f4` with the new behavioral tests: first run **7 PASS /2 FAIL**.
After language change or navigation while a provider promise was pending, the
response replaced the focused control/main with a new DOM tree and left focus on
BODY. This is a concrete DOM lifecycle defect; no claim about rendered appearance.

`render` now captures only a known focused shell control or main before replacing
the tree, then restores its corresponding target. Existing explicit action focus
retains precedence. Focus outside the shell is preserved. No DOM timer, network,
provider, source evidence, auth or backend behavior changed. Regression tests retain
the original two failures and add an outside-target non-interference assertion.

Other executed cases cover all four language/theme combinations, safe persistence,
loading/empty/error/retry, duplicate reload prevention, wrong provider mode,
historical UNKNOWN, inert text, remount single ownership, abort/late-response and
listener cleanup. This does NOT establish screen-reader, key traversal, bfcache in
a browser, media queries, contrast/layout,320px usability or render performance.
Final exact-SHA results/all attempts are recorded in #67/#199.

Review attempts retained: oversized registry time-history output was truncated and
not used as an archive; focused exact metadata remained intact. Upstream raw license
and exact release requests failed; ordinary upstream LICENSE page succeeded. No
blocked access was bypassed. Initial Node DOM run exposed2 failures before the fix.
Environment npm proxy and VM-module experimental warnings remain explicit.
The first fix build hit TypeScript TS18047 on nullable-window narrowing in a
nested function; capturing the checked realm constructor before the closure fixed
the type error, without a cast or relaxed compiler flag. Earlier UI CI log shows
Node22.23.3, satisfying the new package/test-tool engine requirement.

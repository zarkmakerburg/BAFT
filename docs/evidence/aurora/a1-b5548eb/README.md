# Aurora A1 all-attempt evidence — 2026-10-15

Source: `b5548ebb1a4751cb868cee8587afa7605ff2ab03`.
Tree: `210da43edebd2ca3364c52ca19b861ecc819639c`; parent `3d5511c`.
Branch `codex/aurora-foundation`, draft PR204 stacked on PR200. #199 stays open.
This evidence branch only adds documentation, not runtime files.

## Exact fetched source qualification

Clean detached checkout fetched from GitHub; source tree matches the staged
and API-created tree exactly. Locked `npm ci --ignore-scripts --no-audit --no-fund`
PASS; `npm test` PASS: strict and negative type tests, isolated generated-byte
comparison, size guard and19 behavioral tests, ZERO SKIP. Existing UI14/14 PASS;
all current/new JS script syntax checks PASS;13-workflow supply-chain policy PASS.
Local Node24.19.0/npm11.9.0, compiler6.0.3. No local Go toolchain/check claimed.

Four generated ES2022 modules:15,508 raw bytes /5,106 summed per-file gzip(level9).
No runtime dependency and zero current BCC embedding/transfer delta. This is a
module-byte guard, not browser/render/network performance acceptance.

Exact-head UI workflow38713217696/job116349503966 SUCCESS. Complete decoded log
is retained in `aurora-a1-ui-ci.log`, including actual Node22 compilation,
19 new tests and14 existing UI tests. Static38713217513 SUCCESS. Main CI38713217511
was IN_PROGRESS at this evidence snapshot; no all-green claim. Source-associated
workflow/job statuses are distinct from local tests. No CI retry requested.

## Retained development attempts

- Source/bootstrap/ADR and both owner directives read; no AGENTS found. Fresh
  clone materialized an isolated checkout; no user poster/other candidate edited.
- Initial compiler5.9.3 installed with scripts disabled, metadata/license reviewed;
  initial strict build PASS. Early domain/mock tests10/10 PASS.
- First full new provider test run18 tests:16 PASS,2 FAIL. Late cancelled401
  invoked session-expiry callback; early HTTP error left body uncancelled.
  Both failures were reproduced and corrected before publication. Fixed18/18;
  fresh-process mock isolation then added; final19/19 PASS. Logs retain failures.
- Precommit/dependency/workflow/syntax checks PASS. Initial5.9.3 audit0 advisories.
- One compiler-edit script was invoked from the nested package with root-relative
  paths, raising FileNotFoundError for web/aurora/package.json before source edits.
  A following unchanged npm install completed, so that shell's zero exit was NOT
  accepted as a successful compiler update. Corrected from repository root.
- Exact compiler6.0.3 admitted after official release2026-04-16, registry metadata,
  Apache-2.0 LICENSE.txt/ThirdPartyNoticeText.txt and integrity review. Build/tests,
  locked reinstall and retest PASS; final audit0 known advisories. This is a
  point-in-time advisory database result, not a universal security guarantee.
- npm emitted host http-proxy deprecation warnings; no proxy/security config was changed.
- Concurrent claim6188600769 preceded this run's6188603626 but was not present in
  the first returned board snapshot. Both used the same textual ID. Fresh read
  exposed the collision; the other execution paused/released only its claim in
  report6188697537 without publishing. Own claim6188603626 remains the publisher.
  The other candidate10files/343lines was read as status, preserved and not merged.
- Published24 paths with exact expected_sha3d5511c and force=false. Remote tree
  equality and clean exact fetched checkout verified. No history rewrite.

Bootstrap3d5511c original main mesh timeout remains unresolved history, with ONE
successful bounded retry as reported6188555208; A1 does not claim to fix it.

## Scope / handoff

Strict typed node/tunnel models, unknown-input validation, conservative evidence
freshness and exact counters, independent synthetic DEMO state, bounded same-origin
read-only providers, pinned compiler/reproducible generated assets and UI CI gates.
GET overview/tunnels only; no credentials or write interface, silent DEMO fallback,
network-at-import or polling. All tests stub transport; no operational API call.
See source docs/aurora/toolchain-review.md and web/aurora/README.md for contracts.

No new page or backend/embedding/auth/CSRF/default-route change. Browser,
FA/EN/theme/mobile screenshots, accessibility/render metrics, native IR/EX and
candidate payload acceptance NOT RUN/OPEN. Next A2/B: opt-in bilingual dual-theme
shell/primitives/mobile, explicit DOM/timer/request ownership and cached evidence
reassessment. No production/VPS/DNS/secret, merge/tag/release, transport/shaping,
pool/exit switching, restart-resume or standby role action. Claim held for final CI.

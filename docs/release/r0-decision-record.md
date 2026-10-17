# BAFT R0 release decision record (draft)

Status: **NOT APPROVED / NOT BUILT / NOT SIGNED / NOT RELEASED**.

## Immutable baseline
- Base branch: `main`, commit `b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`.
- Preparation branch: `hq/rc0-release-readiness-stage209`.
- Last documented release: `v0.3.1` (2026-10-12); independently confirm remote tag/release before selecting next version.
- This document does not authorize merging, tagging, signing, publishing, or deploying.

## Scope and deduplication
- Confirmed on the base source: principal SQLite commit reconciliation/fail-closed code and tests from PRs #175/#177/#179; exclusive atomic backup temp-file protection from #169; installer rollback reporting and fault test from #172; recovery COMMIT fencing and partial-write test from #168/#181.
- These are **source-presence checks only**, not fresh CI or proof of complete patch equivalence. Do not cherry-pick old draft PRs blindly.
- PR #197 installer multi-tunnel and PR #200 broad tunnel-builder work remain independently reviewed and unqualified for R0. Aurora embedded work #214/#215 remains optional and default-off; preserve classic BCC.

## Release gates (all required before HQ decision)
- [ ] Verify release tag and compare exact candidate commit/tree against `v0.3.1`.
- [ ] Define a minimal exact-SHA candidate scope and obtain green **candidate-head** full CI, including Go tests, `-race`, vet, security/supply-chain and integration.
- [ ] Run installer fault-injection, upgrade/rollback, configuration/state preservation and migration compatibility tests on disposable fixtures.
- [ ] Build Linux `amd64`/`arm64` candidate binaries from exact SHA with `scripts/release/build.sh` and verify checksums/reproducibility.
- [ ] Validate third-party notices, license and release trust material.
- [ ] Require independent/offline release signing and signature verification; the GitHub tag workflow produces **unsigned** candidates only.
- [ ] Prepare change notes, compatibility matrix, rollout and rollback plan, evidence links and remaining risks.
- [ ] Obtain explicit HQ approval for any main merge, tag, release publication or deployment. Native Iran/Germany testing remains deferred and must be identified as residual risk.

## Decision
**HOLD.** No exact candidate SHA, complete candidate CI, disposable upgrade/rollback acceptance or independently signed artifact has been established. Advance non-overlapping release engineering and attach evidence to issue #67.

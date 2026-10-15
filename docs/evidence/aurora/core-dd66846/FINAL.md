# Stage182 P0 evidence slice — FINAL (2026-10-15)

PR212 remains OPEN / DRAFT / UNMERGED on codex/aurora-core-evidence-stage182.
Source: dd66846f2f5053dbfe8572e215ffe93505c318c9.
Tree: 345c171988158f9c4b2347dfa19a96a5750068e4.
Append-only history: PR2114019717f → cd633e5 → dd66846f. All source refs were checked again; PR20886cb4317, PR210e8b0d805 and PR2114019717f remain unchanged. Publication and evidence updates used exact expected_sha / force:false.

## Actual results

All six source-associated workflows SUCCESS:

- Full CI https://github.com/zarkmakerburg/BAFT/actions/runs/38774305763 — 15/15 jobs SUCCESS; test116557838117 Race detector, Vet and Protocol fuzz smoke each SUCCESS. Full decoded log retained verbatim in main-go.log (3,181,982 characters).
- Actual combined merge/source suite https://github.com/zarkmakerburg/BAFT/actions/runs/38774305949 — SUCCESS; seven Python Git regressions, strict/negative TypeScript, generated modules/size checks, 33 Aurora +15 DOM +14 classic Node tests PASS; actual Go BCC256.992s and incidentclass0.398s PASS. Full94,412-character log in merged-workflow.log.
- BCC UI38774305973, Core reconciliation38774306017, static-analysis38774306015, r3.138774305871 — SUCCESS.

No rerun was requested. The first source cd633e5 had invalid job.env runner context, so its absent combined workflow was never accepted. It is preserved and corrected by dd66846f. The first local recovery fixture failed because clone does not import custom refs; explicit ref fetch corrected the fixture. All failed/unsupported read attempts are recorded in README.md and HQ Stage182–187. Mandatory suites were not bypassed. Historical initial-status.json remains unchanged; this FINAL supersedes its pending statuses.

## Recoverable exact combined Git objects

Artifact https://github.com/zarkmakerburg/BAFT/actions/runs/38774305949/artifacts/11992808888 contains the self-contained bundle, manifest, checksums and logs. Exact downloaded ZIP digest67e19e352ed49ebf0454018332608093b124b8edfe34203d15abc73f334bc407; bundle digest5bfdee33b257cce81e0d63c9d5fe64a8f7be7cc26c8a7e5938173b15cbc9d006. CRC, expected inventory, fresh empty-repository bundle verify/fetch, exact hashes and full fsck PASS. Retained manifest.json and recovery.json match the downloaded artifact.

Evidence commit fe9d241d54568dd93530b4d1ceb0c352c35bd4c5; combined tree47a1b6bd435180e8a5957d48292dc989865bf029; ordered parents dd66846f2f5053dbfe8572e215ffe93505c318c9 and86cb4317fbab16e71208f701851b70140e380e84. These Git objects are in the bundle, not a published repository integration ref. Actual artifact expiry2027-01-13T17:04:42Z; archive before expiry. This evidence branch retains textual proof/logs, not the41MiB bundle binary. No CI write permission was added.

The current-branch comprehensive CI and the combined-workspace three-suite gate are separate proofs. Do not claim a full-repository race run on the combined tree; later persisted integration and feature changes need their own full exact-head checks.

## Previous failure preserved

prior-race-port-bind.log retains all3,375,175 characters from original failing job116539433338 / run38768955651. Its sole test failure is TestRuntimePersistentTelemetrySurvivesOutageAndRestart, address127.0.0.1:39399 already in use; no WARNING: DATA RACE occurs. The Stage180 unique-address fix is inherited unchanged. PR211 current automated gates and this source now passed; this does not prove all possible external port TOCTOU collisions are eliminated.

## Remaining gates and next owned slice

This closes only the bounded P0 workflow/recovery evidence task. No release, production, mainline merge, route mounting or visual acceptance occurred. Reviews/threads were empty, not approvals. Local Go is unavailable; Go results above are actual CI execution. Narrow key/token scans were zero-hit and are not a universal security audit.

Proceed from live HEADs and a fresh claim. P1 must preserve authenticated default Command Center V3 AND secret-path/classic/ fallback, all current bcc-app/map/operations/earth/welcome-globe effects and operational controls. Existing classic-only source parity is insufficient. First extend the preservation inventory/guard, then establish a reviewed isolated integration candidate and reversible feature-flagged authenticated route integration. Recheck pins, full combined security/race/rollback contracts and actual rendered FA/EN × dark/light ×320/390/768/1440, keyboard, reduced motion, accessibility and performance. No DEMO/live mixture or surprise writes. Native Arno↔Goldapp Main tests remain owner-DEFERRED. Merge/release/deploy and forbidden transport/shaping/pool/restart/standby scopes require HQ authority.

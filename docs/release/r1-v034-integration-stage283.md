# R1 v0.3.4 integration candidate — Stage283

Status: **review candidate only; release HOLD**. This record implements R1-01 and prepares R1-02 from the [Stage282 control index](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6230931719). It does not mark the remaining R1 work or any future engine complete.

## Immutable inputs and assembly

- Current main at audit: `04e2cc71aa616049f488e72561982f7a2edeb86f`.
- Last public release baseline: v0.3.3, source `90b77bb87a6778e7346f6f06820bc79a2680d5d5`. Main contains subsequent reviewed tunnel/UI fixes.
- Integration branch: `hq/r1-stage283-v034-integration`.
- Preserved spine: PR#241 head `c8807293be8668f6e8530b329665694bfe4bf2fd`, whose ancestors include #237 → #239 → #240. Main is an ancestor of this spine. Existing source branches are not rewritten.
- Independent installer input: PR#235 head `cc8e162379df7358190350c648850e5949b724ca`; apply only its delta in `install.sh` and `tests/e2e/uninstall.sh` from the actual current-main merge-base.
- Independent release-policy input: PR#220 head `211c929d41924fbe17ec3bb19615f7d0e8429490`; apply only its remaining three-path delta (`scripts/release/accept.sh`, `tests/release/accept_test.go`, `docs/release/ci-admission.md`). The older declared PR base includes inherited R0 work already represented in main; it is not replayed.
- Telemetry input: PR#217 head `efb55e1fd11b9b77fa3842259a16c39b0da694a8` supplies the future-clock reproducer. Its wall-clock-only cutoff is replaced by the reconciliation below. The broad source branch is not imported.
- New candidate paths beyond those inputs: this record, the bounded retention correction and delayed-event regression, plus R3 workflow deadline/diagnostic alignment. R3 is mandatory under the selected admission policy; its same full-repository race suite still used Go's default 10-minute package deadline. Align it to the spine's explicit 15-minute package / 30-minute job budget, preserving every test, correctness/vet/fuzz gate, trigger and permission. The prior #240 timeout is recorded, not hidden by this change.
- Claim: [R1-01/02](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6231013766); [release-policy/clock finding](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6231064491).

Assembly uses a dedicated worktree based on the preserved spine and explicit bounded patches. It performs no PR merge, main update, tag, release, production action, or real signing-key operation. The final commit/tree, draft PR and candidate-head CI evidence are recorded in #67; commit IDs are not self-embedded in this document.

## Resulting behavior and boundaries

1. Keep Classic BCC and its real operational cards. Aurora routes to the same embedded Tunnel Studio and reviewed native plan/deploy/decommission APIs. The existing R1 implementation remains JS/Go; selecting a future typed UI architecture is UI-01, not an implicit rewrite.
2. Combine the backend catalog, refresh/auth response fencing, explicit generic destination, IPv4/IPv6 endpoint construction, secure bounded port suggestions, and separate engine/direction selection. Selecting a destination suggestion invalidates an older reviewed plan; no suggestion claims a port reservation.
3. Only Native BAFT is admitted. FRP, Rathole and other engine cards remain disabled by backend admission and cannot create a plan or job. The versioned evidence library is a contract foundation, not a live node-probe, durable inventory service or deployed adapter.
4. Use `--no-create-home` for service accounts; explicit state-directory creation retains owner/mode 0700 without copying arbitrary `/etc/skel` content. The inherited systemd uninstall test checks ordinary and hidden skel entries and mode 700.
5. Release acceptance requires the latest intended exact-source main workflow to be completed/success with all 15 named jobs, plus exact-source static/gosec and R3. Failed/unfinished/missing evidence rejects. This is one executable gate, not the entire Stage282 freeze matrix: browser E2E, Core/soak, provenance, signing, RC/final artifact acceptance and authorization remain separate.
6. Cap telemetry history's pruning reference at the earlier of the report event timestamp and BCC's ingestion wall clock. Delayed reports retain R0's preceding event-time window; future node clocks cannot advance pruning beyond BCC time and erase recent history. Do not rewrite the report timestamp, ledger/cursor semantics, replay protections, seven-day read filter, count bound, freshness rules or SQLite transaction behavior.

The tunnel payload stays application-neutral and opaque. No application account, subscription or application config is managed. Future L3/L2/UDP/multihop support is still governed by the Stage282 capability matrix and real lifecycle admission.

## Open-PR audit and decisions

All 40 open PR heads were fetched and inspected as immutable Git objects. None of their heads is an ancestor of current main, so ancestry alone cannot show whether patches are present. For each head, the audit compares its merge-base with current main, exact changed-path blobs, and a reverse `git apply --check` on a clean worktree of current main. The table's path count is the comparison to **current main's merge-base**, not a claim of uniquely owned feature files. Stacked historical branches often include 100+ inherited Core paths; those are not safe wholesale integration units.

A successful reverse check establishes textual patch presence. A failed reverse check may be context drift; it does not establish a missing fix. “SOURCE PRESENT” records targeted source/test presence, not complete semantic equivalence, independent review or fresh field acceptance. “HOLD” keeps the work open and outside this bounded candidate. It does not declare the work obsolete, safe to close, or exempt from release-blocker review at R1-07.

| PR | Source head | Paths vs main merge-base | Disposition |
| --- | --- | ---: | --- |
| [#241](https://github.com/zarkmakerburg/BAFT/pull/241) | `c8807293be8668f6e8530b329665694bfe4bf2fd` | 23 | INCLUDE: spine; evidence contract + bounded race CI budget. |
| [#240](https://github.com/zarkmakerburg/BAFT/pull/240) | `890517d46d1dcc93dbe1f5830bb08f6d4fff0f82` | 19 | INCLUDE through spine: reconciles #231/#234 with catalog. |
| [#239](https://github.com/zarkmakerburg/BAFT/pull/239) | `59b50da2097bbb06db4b2e256eebc990a38ac3c0` | 12 | INCLUDE through spine: authenticated/stale catalog fencing. |
| [#237](https://github.com/zarkmakerburg/BAFT/pull/237) | `ced4b68ad9a3c2a49bdaf456149caacb66392595` | 9 | INCLUDE through spine: real backend catalog, external adapters locked. |
| [#235](https://github.com/zarkmakerburg/BAFT/pull/235) | `cc8e162379df7358190350c648850e5949b724ca` | 2 | INCLUDE: exact bounded two-path installer delta. |
| [#234](https://github.com/zarkmakerburg/BAFT/pull/234) | `9863d009ff4bcd34ae8d98e38800aa2a6587188f` | 7 | RECONCILED in #240; no wholesale import of older Studio. |
| [#231](https://github.com/zarkmakerburg/BAFT/pull/231) | `b9f84c05177b1c855beb0241e1c313ad3d82b8dd` | 5 | RECONCILED in #240; explicit generic destination and safe suggestion. |
| [#222](https://github.com/zarkmakerburg/BAFT/pull/222) | `15304fda1f13e9dc4acbb850f2c1a92ee5d40f2b` | 4 | HISTORICAL reference; Stage282 in #67 is the current roadmap. |
| [#221](https://github.com/zarkmakerburg/BAFT/pull/221) | `14cfc6a42186ece49393f5401b69ffa8046771c3` | 6 | PRESENT: all six changed path blobs match main; reverse patch check passes. |
| [#220](https://github.com/zarkmakerburg/BAFT/pull/220) | `211c929d41924fbe17ec3bb19615f7d0e8429490` | 3 | INCLUDE: only three remaining paths against current main; inherited R0 paths omitted. |
| [#217](https://github.com/zarkmakerburg/BAFT/pull/217) | `efb55e1fd11b9b77fa3842259a16c39b0da694a8` | 3 | RECONCILE: retain source future-clock test; cap pruning at BCC receipt time while preserving R0 event-time policy. |
| [#215](https://github.com/zarkmakerburg/BAFT/pull/215) | `8206c9496ebec0c52c5e1e191ce6b8b246363037` | 141 | HOLD: old Core/Aurora integration diagnostics; not a replacement baseline. |
| [#214](https://github.com/zarkmakerburg/BAFT/pull/214) | `6c68887a23c4e7476cd3a80042eb2824e1a36fa1` | 170 | HOLD: standalone embedded-preview isolation; assess in UI-01/parity work. |
| [#213](https://github.com/zarkmakerburg/BAFT/pull/213) | `a55b6faca2182ab2c1495a4ad40e0b09bbcb9b66` | 141 | HOLD: historical Hybrid preservation tests with inherited Core. |
| [#212](https://github.com/zarkmakerburg/BAFT/pull/212) | `dd66846f2f5053dbfe8572e215ffe93505c318c9` | 138 | HOLD: historical diagnostic evidence and inherited Core. |
| [#211](https://github.com/zarkmakerburg/BAFT/pull/211) | `4019717f5030bb3b17bacb118378a6d8bf47c0b4` | 135 | HOLD: isolated three-way probe, not a qualified product integration. |
| [#210](https://github.com/zarkmakerburg/BAFT/pull/210) | `e8b0d8050b647bd441f093e03e31a1a51f7fd2ea` | 133 | HOLD: merge-tree diagnostics, not a qualified product integration. |
| [#209](https://github.com/zarkmakerburg/BAFT/pull/209) | `e3e0dca74c38802c67fdcc43cb73ed790fe23622` | 118 | HOLD: Core/incident reconciliation diagnostics, not a release baseline. |
| [#208](https://github.com/zarkmakerburg/BAFT/pull/208) | `86cb4317fbab16e71208f701851b70140e380e84` | 167 | HOLD: standalone Hybrid parity tests; assess in R1-04/UI-01. |
| [#207](https://github.com/zarkmakerburg/BAFT/pull/207) | `e0c8d1606841bed93fc328e52887fe879878f7bf` | 165 | HOLD: standalone read-wire/navigation integration; assess in UI-01. |
| [#206](https://github.com/zarkmakerburg/BAFT/pull/206) | `00876e4fc71e5c7a9876e2f29d77cc50c48aba44` | 157 | HOLD: standalone navigation/focus/wrapping; assess in UI-01. |
| [#205](https://github.com/zarkmakerburg/BAFT/pull/205) | `efc2f047f294f4bc8211aa97bd29ffe12573e10e` | 164 | HOLD: standalone provider wire fixtures; assess in UI-01/D-05. |
| [#204](https://github.com/zarkmakerburg/BAFT/pull/204) | `de446b5df8f045c69ca388b3c500515143f7812a` | 156 | HOLD: standalone typed Aurora foundation; architecture decision belongs to UI-01. |
| [#203](https://github.com/zarkmakerburg/BAFT/pull/203) | `39424cf6e4f5a79ec1282b7c2bc516ac306ea6aa` | 17 | HOLD: separate SQLite v13 incident migration; current schema retained. |
| [#202](https://github.com/zarkmakerburg/BAFT/pull/202) | `4741a231ca74fb0c3afa3b62d423f74f74b19a09` | 11 | HOLD: incident classifier foundation; separate incident qualification. |
| [#200](https://github.com/zarkmakerburg/BAFT/pull/200) | `1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7` | 116 | HOLD: broad candidate lifecycle/verification Core; separate #228 qualification before full lifecycle/adapters. |
| [#197](https://github.com/zarkmakerburg/BAFT/pull/197) | `fb1950b295442d891aa6a09481d030e49c507b24` | 6 | HOLD: broad installer multi-instance/pairing/discovery; separate install/lifecycle qualification. |
| [#193](https://github.com/zarkmakerburg/BAFT/pull/193) | `001762ea8b80aaa2e229fed19b5aad371ec263a0` | 10 | HOLD: GeoIP location enhancement; current manual/node location behavior retained. |
| [#181](https://github.com/zarkmakerburg/BAFT/pull/181) | `5fbe7197442435cf26f6f9ea4477e565cba5b3f3` | 3 | PRESENT: reverse patch check passes; two of three complete path blobs match. |
| [#179](https://github.com/zarkmakerburg/BAFT/pull/179) | `6d25766c7f068395280202dbe579b68db8298576` | 2 | PRESENT: reverse patch check passes; migration reconciliation test retained. |
| [#177](https://github.com/zarkmakerburg/BAFT/pull/177) | `2739bf40a9b8b2be89aa5900664f1f02142af9ce` | 5 | SOURCE PRESENT: uncertainty guard/reconciliation and tests retained; evolved contexts require regression. |
| [#175](https://github.com/zarkmakerburg/BAFT/pull/175) | `3b2fa3f6ff93ea17d46249bedff7d4b8b1fee597` | 3 | SOURCE PRESENT: uncertain-commit guard/reconciliation retained; evolved store context. |
| [#172](https://github.com/zarkmakerburg/BAFT/pull/172) | `b4821067e9b2718da7a2bf2e6d015acb5b9e66b1` | 2 | SOURCE PRESENT: rollback reporting and fault assertions retained; warn replaces log in evolved installer. |
| [#171](https://github.com/zarkmakerburg/BAFT/pull/171) | `a7b944736c93f8247b5abaed0ac762d28733c5ec` | 5 | SOURCE PRESENT: four runtime/test blobs exact; API-read documentation has evolved. |
| [#170](https://github.com/zarkmakerburg/BAFT/pull/170) | `41338419ad66e7a75191dd6c2a56e19e5ddcc14e` | 2 | PRESENT: both path blobs exact and reverse patch check passes. |
| [#169](https://github.com/zarkmakerburg/BAFT/pull/169) | `4637e1f3b0c34ea821af939b453c5e832a3a2b97` | 2 | PRESENT: both path blobs exact and reverse patch check passes. |
| [#168](https://github.com/zarkmakerburg/BAFT/pull/168) | `3a1cf632fedd7391a691c83f0e253cc06cf139fe` | 2 | SOURCE PRESENT: MarkCommitSent precedes attempted wire write; current writer uses fault-injectable hook. |
| [#167](https://github.com/zarkmakerburg/BAFT/pull/167) | `db3f3b17f1b78e1035c44106d1cca6a33adc7d93` | 16 | SOURCE PRESENT: thirteen test/runtime blobs exact; server/store/tunnel contexts evolved. |
| [#166](https://github.com/zarkmakerburg/BAFT/pull/166) | `c7f186a9975f5081092b173a6a96d4b2cf9290af` | 3 | SOURCE PRESENT: both restore runtime/test blobs exact; API-read documentation has evolved. |
| [#136](https://github.com/zarkmakerburg/BAFT/pull/136) | `c0b52467253e8e0a893ad5f010dec2526835cd56` | 2 | HOLD: WS single-write optimization/test is absent; separate carrier/performance qualification. |

### Specific overlap checks

- #231/#234 are reconciled through #240 rather than restored over the catalog-aware dashboard. Behavior coverage includes explicit destination preservation, released/failed-decommission resource suggestions, secure-random failure/exhaustion, plan invalidation, IPv6, FA/EN review, unsupported direction/engine rejection, and reset behavior. This is not browser rendering evidence.
- #221 has all six changed path blobs exactly equal to main and a successful reverse patch check. Stale/future monitoring health continues to project unknown rather than invented reachability.
- #175/#177 retain the added guard/reconciliation code and tests; whitespace-normalized added code is present despite surrounding store/server evolution.
- #172 retains rollback failure tracking, restart verification, and fault assertions; its old logging calls became `warn`. #166/#171 runtime/test blobs match main, while the API contract document has changed since their historical snapshot.
- #168 retains the fence before any COMMIT send and uncertain-send handling. The source's direct encode is now a fault-injectable writer; importing its older code would lose later hooks. #181's full reverse patch check succeeds.
- #167 retains thirteen exact runtime/test blobs, with subsequent server/store/tunnel evolution. Current mutation durability/audit behavior must be qualified by R1-03/06 and candidate CI, not inferred from file names.
- #136's carrier optimization is absent. No WS throughput, field-carrier or same-flow continuity improvement is claimed by this candidate.
- #197's multi-instance CLI/pairing/discovery changes and #200's staged two-node candidates are broad, independently qualified work. Neither their unmerged code nor inherited copies from #204–#215 are silently installed here. Their remaining lifecycle and discovery gaps feed the planned native inventory/full-lifecycle work; R1-07 must explicitly classify any release-impacting defects.
- #202/#203 incident/schema-v13 work is not pulled through a UI branch. No incident migration or standalone Aurora provider API is enabled.

## Observed validation

Negative controls were performed before the final correction:

- Original event-time-only code plus the unchanged #217 reproducer: **FAIL**, `future-dated telemetry erased a valid recent history point`.
- PR#217's proposed wall-clock-only alternative plus the new persisted delayed-window test: **FAIL**, `delayed report lost its event-time window or retained expired history`.
- Final bounded correction: both regressions pass. The delayed test reopens SQLite, verifies the preceding event window survives and expired history is pruned. Existing current-time filtering and failed-save replay/accounting tests also pass under race instrumentation (4 tests, 1.797s).

Current local candidate checks:

| Check | Result |
| --- | --- |
| Node embedded UI/catalog behavior | PASS, 69 tests, 0 failures/skips; Node v24.19.0. VM/contract checks, not browser E2E. |
| Release acceptance fixtures | PASS, `go test -race ./tests/release -count=1` (6.362s), seven top-level tests and negative subcases; fake GitHub CLI/disposable Git remotes. |
| Installer plan/verification fixtures | PASS, `go test ./tests/installer -count=1` (4.295s). |
| Documentation contracts | PASS, `go test ./tests/docs -count=1` (0.019s). |
| Targeted retention/monitoring/failed-save regressions | PASS, 4 tests with race instrumentation. |
| Complete BCC and engine race suites | PASS, `go test -race -count=1 -timeout=15m ./internal/bcc ./internal/tunnelengine`; ok  	github.com/zarkmakerburg/baft/internal/bcc	325.245s; ok  	github.com/zarkmakerburg/baft/internal/tunnelengine	1.027s. No race warning or failing package. |
| Shell/workflow syntax | PASS for changed installer, uninstall E2E and release acceptance scripts; parsed main/R3 YAML and Bash syntax of every embedded run step. Read-only workflow permissions retained. |
| Supply-chain policy | PASS, 13 workflows, no production release secrets. |
| Vet | PASS for BCC, tunnelengine and release fixture packages. |
| Main-CI named-job equality | PASS, all 15 workflow jobs exactly match the admission list. An initial ad-hoc parser mistakenly counted trigger keys; restricting it to the `jobs` block corrected the audit, with no product-code change. |
| Whitespace and added Go-file formatting | PASS before commit. |

Compiler: Go1.27.1 on Linux. The R3 workflow alignment changes only deadlines/diagnostics; no race tests are filtered or removed. Source PR#241 has full CI39010014009/static39010013841 green; #235 has CI39005842152/R339005842107/static39005842327 green; #220 has CI38943151691/R338943151519/static38943151722 green. These are source-input observations and **do not qualify this new candidate SHA**. #240's historical whole-CI39008650275 timed out; the spine includes #241's explicit 15-minute package/30-minute job budget and race-failure annotations without deleting/filtering tests. Review-thread reads for #235/#239/#240/#241/#220 returned zero threads at the audit snapshot; that is not an approval.

## Remaining R1 queue and release status

- R1-01 audit is complete for selecting this bounded candidate. R1-02 requires its published draft/immutable tree and candidate-head CI; no source PR is closed or marked merged by this record.
- Next: **R1-03**, API behavior regression for create/edit/remove, sibling isolation, IPv4/IPv6, suggestions and PlanHash. Then R1-04 Classic/Aurora action/API parity inventory.
- R1-05 is still open: actual browser FA/EN × light/dark × mobile/desktop, auth/stale races and visual QA. Local Node VM checks do not satisfy it; no screenshots or browser pass are fabricated.
- R1-06 still needs the full disposable systemd installer/update/backup/restore/security evidence on the candidate source, including the skel regression. Local shell/Go fixtures do not replace those E2E jobs.
- R1-07 still needs release-blocker classification, authorized integration/freeze on main, all exact-source gates, artifacts/provenance, independent/offline signing, RC acceptance and final artifact acceptance. Native Iran–Germany/two-node field acceptance remains deferred; no external engine or continuity certification follows from this report.
- R1-08 remains subject to explicit current HQ authorization for publication and rollout. No automatic background scheduling, tag, merge, publish, production restart or rollout is started.
- v0.3.4 target is still 2026-10-23 18:00 Asia/Tehran, contingent on Stage282 gates and active work sessions. An unmet gate moves the target; it does not turn into a pass.

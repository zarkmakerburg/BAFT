# Candidate preparation jobs — exact source evidence, 2026-10-14

Final reviewed source: `4d9f357abcc2145f0e463664e7bbea9b366ea2a9`, tree
`2e98dca8df1143736dfc15b8fda9605e066b0bdc`, parent
`fa6ac1bb0ca87b2c3d39dedb8233a85384744af1`. The final commit only strengthens
the native systemd fixture to include/validate both candidate artifact reports;
no runtime code changed. Local fixture commit `5bae41c1e1f3f4615f3b6477882cfa81431d32fe`
has the same final tree. Publication used exact expected_sha=fa6ac1b, force=false.
PR automatically synchronized and stayed OPEN/DRAFT. No second close/reopen.

Core runtime source: `fa6ac1bb0ca87b2c3d39dedb8233a85384744af1`, tree
`97bf43d02dfdffdf87c7d34de370c4b0481eba4a`, parent
`42585baa9e5967abac2977c028cd2406409ccf31`. Source branch
`codex/smart-tunnel-p0`, draft PR #200. Local commit
`231da8054c0a9c5649d0a75be7f37bd037525219` has the identical tree and remains
preserved. Publication checked live HEAD/draft and used `expected_sha=42585ba`
with `force=false`. No reset, rebase, force-push or existing commit removal.

Fetched core synthetic merge `9f6f9132bbd51d685709c4a7b99ea58d4ca829f5` has the core
source tree. Final fetched synthetic merge `bb9b7115000a15058c1c53dad1f40ec12fb63bb4`
has the identical FINAL tree. Evidence is maintained on a separate branch based
on final 4d9f357; its diff from the
source contains only this evidence directory. Claim: #67 comment 6167734014,
`codex-20261014-candidate-jobs`; release is recorded in the final #67/#199 report.

New EX config generation/reply consumption is a separate signed staging job.
IR candidate validation completes before either commit is queued. BCC requires
matching fresh scoped artifact/resource reports from BOTH nodes; activation
binds config digest and generation, rechecks private dependencies/PKI/resources,
and never regenerates configuration at cutover. Completed staging permits an
explicit same-reply retry. Older persisted in-flight receipts retain their
protocol, while old agents refuse the explicit new prepare flag before cutover.
The reply is wiped after staging; reports contain no private key/config bytes.

Actual pre-cutover candidate TLS/Noise/handshake/application payload remains OPEN.
These artifact reports are not that proof. Real process payload still occurs
after commits, before destructive finalization. No zero-downtime, field,
fingerprint/blockability, host-wide locking/reservation, service-credential or
mixed/shared-binary acceptance. No production/server change, main merge,
tag/release, transport/shaping change, exit switching, restart-resume or standby
resumption. Installer #197 was fetched/read at fb1950b2, not executed or changed;
its same-name unit collision is modeled and refused, not full coexistence proven.

## All retained attempts

Development snapshots preceded the final immutable source commit; only attempts
15–21 are exact core fa6ac1b checks, while 23 onward qualify the exact final
4d9f357. Every log is under [attempts](attempts).
Code fixes happened between development attempts; no failed run is relabeled.

| Attempt | Result and reason |
|---|---|
| 01 baseline ordering | FAIL on 42585ba plus the new regression: BCC queued EX commit immediately after IR reply rather than separate staging. |
| 02 initial build | PASS of the ordering test; other packages built, no matching tests there. |
| 03 full affected | FAIL: old BCC sequencing/evidence expectations, omitted BCC job-signing allowlist (HTTP 500), nil-map bug in the new test fixture. Production staging guards remained enforced. |
| 04 updated BCC | FAIL only on expected evidence count (new preparation adds two entries). |
| 05 candidate/signed flow | Candidate node tests PASS; Agent FAIL because implicit singleton metrics came from its environment, not hard-coded 9191. Scoped metrics still must exactly match reviewed endpoints. |
| 06 ordering/report/Agent | FAIL on old job/evidence counts after the new 16-job protocol. |
| 07 full affected | BCC/tunnelnode/agentjob PASS; Agent old compiled 14-job expectation FAIL. Corrected to 16. |
| 08 scoped candidate contract | PASS, including signed IR refusal before either service cutover and older receipt compatibility. |
| 09 focused three runs | PASS, three runs of candidate/node/Agent/BCC/signing checks. |
| 10 development full all | PASS full unit/integration; later plan text and installer-contract test were checked separately and in final exact-source checks. |
| 11 development broad race | FAIL aggregate: BCC global ten-minute deadline, while executing unchanged alert/distribution/SQLite code. Agent/tunnelnode/agentjob PASS (257.278s / 122.198s / 1.082s). No race finding reported before timeout. |
| 12 installer/native local | Installer same-name unit refusal PASS 3/3. Native two-live-instance fixture SKIP 3/3: controlled child process namespace/FD access unavailable. Not a native pass. |
| 13 plan/new protocol | PASS revised plan explanation, candidate protocol and compatibility tests. |
| 14 tool/publication attempts | Installer path/read corrections, failed patch context, refused truncated JSON export, verified API tree/lease, delayed PR synchronization and broad-race timeout are all retained. |
| 15 exact full all | PASS full unit/integration on fetched clean fa6ac1b; BCC 212.950s, Agent 114.466s, tunnelnode 122.411s. |
| 16 exact broad race | FAIL: BCC aggregate timeout at 600.155s during unchanged unreachable-incident test; Agent rotation /activating retry evidence failed (250.618s). agentjob and tunnelnode PASS (2.146s / 171.517s). Preserved, not relabeled. |
| 17 exact focused/vet/UI | PASS affected vet, supply-chain policy (13 workflows), 12 UI tests and THREE focused node/BCC/Agent/signing/installer checks. Native process fixture explicitly SKIP 3/3. |
| 18 exact scoped BCC race | PASS 80.870s, including candidate protocol, secrets, plan, health/observation, rollback/cancel/replacement, delivery and decommission compatibility. This is a scoped check, not the entire BCC package. |
| 19 exact environment | Actual occupied socket/inventory and safety tests PASS; controlled-child namespace/FD, two Unix socket cases and PKI uid/gid mutation explicitly SKIP. Environment limitations remain separate from native CI. |
| 20 exact timeout isolation | PASS 14.910s of the unchanged alert test active at development broad-race timeout; does not qualify the entire BCC suite. |
| 21 exact Agent rotation isolation | PASS THREE race runs of /activating scenario, 35.806s. Does not erase the full Agent run failure. |
| 22 native fa6 active tunnel | Actual failed-systemd-job JSON: tunnel ACTIVE, both candidate reports valid, 2×2 MiB payload round-tripped and previous proofs passed. Failure was old fixture expected set omitting candidate_artifacts; final fixture requires them. |
| 23 final exact full all | PASS fetched clean 4d9f357: BCC 154.607s, Agent 96.336s, tunnelnode 110.936s. |
| 24 final exact assertion/vet/UI/BCC race | PASS revised assertion against actual failed-job active evidence (no local systemd execution), affected vet, supply-chain policy, 12 UI tests, task-scoped BCC race 25.032s. |
| 25 final exact Agent/node race | Full Agent/tunnelnode/agentjob race PASS: 218.213s / 95.626s / 1.314s. Earlier broad fa6 failures remain retained. |
| 26 final environment | Two Unix socket cases explicitly SKIP (EPERM). Anchored initial selection did not match full process/uid-gid names; corrected separately in attempt 27. |
| 27 final process/uid-gid environment | Corrected selection actually ran: controlled child process namespace/FD and PKI uid/gid mutation explicitly SKIP. Never counted from an unmatched test. |

Go: `go1.27.1 linux/amd64`. Native process fixtures use actual current-built
BAFT/baft-pair and H2/TLS/Noise, with a systemd adapter; the Agent signing/lifecycle
fixture uses a simulated service and simulated delivery evidence. It is not a
real transport or physical-host acceptance. Local environment lacks selected
process namespace/FD inspection, AF_UNIX creation and arbitrary uid/gid changes.

## CI and PR synchronization

Source Git ref advanced and was fetched at fa6ac1b, but PR REST/Git refs remained
at 42585ba and no exact-source workflow runs existed. Confirmed by independent
raw REST and git ls-remote reads. One bounded close/reopen refreshed PR #200;
both calls succeeded, preserving draft, restoring OPEN, and updating its HEAD
to fa6ac1b. No ready/merge transition. Four current-source workflows started.

Initial/current snapshots are retained in ci-initial.json. Current CI run
38563198183 test job 115849193494 reports the required native resource step
SUCCESS after full unit/integration SUCCESS. The exact source workflow runs
native cases with `BAFT_REQUIRE_RESOURCE_EVIDENCE=1` and `-count=3`: resource
environment skips become failures. This includes the updated real two-live-
instance candidate staging/IR validation/commit fixture, old-route continuity,
sibling PID/generation/artifact stability, and installer-unit refusal.

At this initial snapshot UI 38563198875 and static 38563197944 SUCCESS; R3
38563198177 and aggregate CI still incomplete. Attempting main-job log download
before job completion returned transient BlobNotFound/404; preserved in snapshot.
Do not infer final all-green CI or individual native log contents from pending
snapshots. Completion snapshots and any retrieved native output are appended
below before the final report. Core fa6 main job completed SUCCESS including
full native race/vet/fuzz; its aggregate workflow FAIL was the systemd fixture
expected-set assertion. Native core output retrieved in native-fa6.log: THREE
two-live-instance and THREE installer-contract passes, NO resource skips. Final
4d9 native/systemd/CI evidence remains separately recorded; historical core
passes never replace final-source qualification.

Final-source qualification snapshot: ci-qualification.json. All local final-SHA
unit/integration PASS; full Agent/tunnelnode/agentjob race PASS; task-scoped BCC
race PASS; affected vet/policy/12 UI tests PASS. Local entire BCC race is NOT
claimed successful: earlier broad runs timed out. Final real-systemd launch job
115854292744 SUCCESS with actual native log excerpt in systemd-4d9.log: tunnel
build/traffic, completed/cancelled certificate rotation and scoped sibling
decommission/retry/continued payload PASS. This is a throwaway runner, not field
acceptance. Native final-source mandatory resource step SUCCESS; its full main
job log remains unavailable until completion.

At this qualification snapshot final UI 38564701042, static 38564701055 and R3
38564700891 SUCCESS; 14 of 15 main CI jobs SUCCESS. Test job 115854292544 remains
in its full race step. No final all-green inference. A later completion snapshot
is appended separately if available; pending evidence is retained unchanged.

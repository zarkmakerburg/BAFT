# Shared read-wire contract FINAL qualification

PR205 is OPEN/DRAFT/unmerged. Current source efc2f047f294f4bc8211aa97bd29ffe12573e10e,
tree3a39ac38a6ea0535e4eccff05a4d3cfe4ebddcdf; parent e646fac, rooted at current PR204 de446b5.
Original failure and corrected commits are preserved; no reset/rebase/force push or retry.

## Exact source and execution

Local fetched head: strict positive/negative TypeScript, byte-identical8-module rebuild,
size guards,30 Aurora +12 DOM +14 classic Node tests PASS/0 FAIL/0 SKIP. Corrected
logger uses errexit and pipefail; full fixed-exact-local.log retained. Runtime files,
dependency inputs, BCC embedding/API/auth/session/CSRF unchanged (path-exclusion diff0).
Local Go/gofmt were NOT RUN because the tools are absent.

All THREE current-source workflows SUCCESS:
- UI38739243814 / job116436330027, full decoded log retained.
- Static38739243083, workflow status read.
- Main38739243073, ALL15 jobs SUCCESS; complete test116436327090 decoded log retained.

GitHub PR workflows checked out synthetic merge585d0f9429f9032816090d5e44321e35cee79663.
Its fetched tree exactly equals current source3a39ac38, parentsde446b5 andefc2f04.
This is source-tree equivalence, not an actual main merge. Local tests ran the head.

Actual Go1.27.1 main job: full go test ./... passed, including internal/bcc82.783s;
full go test -race ./... passed, including internal/bcc313.752s; vet and protocol fuzz
steps SUCCESS, alongside existing targeted stress checks. The new non-skipped
TestAuroraReadWireContract is part of both default package suites; it was not separately
invoked with -run/-v and no invented named-pass log is claimed. Complete decoded main
test job (3,182,032 characters /27,471 split lines) is
[main-job116436327090.log](main-job116436327090.log). Git blob2c6a9b0dfc10344a59127d2e6570775b4030086f.
It may require GitHub Raw/Download because of its size. Other14 main-job outcomes are
status qualification, not independent full-log review or native field acceptance.

## Failed attempt is still visible

e646fac main38738495678/test116433805473 FAILED: test setup inserted into lazily nil
Tunnels map at line96. It compiled but did not complete producer acceptance. Other14
original main jobs and original UI/static succeeded. Complete first-go-job log and
first-go-failure.md were retained before the one-line harness-only correction.
Earlier pending snapshots, first/fixed local/UI logs and attempts are unmodified.
No production handler defect was established by that harness failure.

Fetched evidence6c0bc288 and all11 pre-final text/JSON/log files were verified exactly
equal to submitted content. Final append uses a separate evidence ref and preserves
source. Limited private-key/GitHub-token pattern scan0 matches; not universal secret
assurance. npm/VM/action deprecation warnings remain in full logs.

## Scope and open gates

This qualifies selected synthetic fields produced by actual in-process read handlers
and consumed through the same fixtures by the TypeScript adapter. The fixtures are
subsets, not full captures or a new API version. No network/live-node/UI activation.
Legacy test-admin auth does not qualify browser cookie/CSRF or persistence integration.
Desired active intent remains UNKNOWN payload delivery; missing per-metric times do
not become fresh, revoked/missing/expired data do not become healthy.

No submitted PR205 review or inline thread observed at final source check.
Claim6192411548 can be released after this qualification is reported in #67/#199.
The full #199 roadmap and A2/B visual gate stay OPEN. Browser/served preview/screenshots/
mobile320/key traversal/screen-reader/rendered accessibility/performance NOT RUN;
native IR↔EX/candidate owner-DEFERRED/OPEN. No production/VPS/DNS/secrets, main merge,
tag/release, transport/shaping/pool/exit switching/restart-resume/standby change.

# Independent verification and iMonitor requirements — 2026-10-14

Published source under review: `59cc9dfe719a535914b58e358d2760938fad2f37`.
Code tree: `66210d9867612d13c424624e8cb97fdfccd711ad`.
PR #200 remains draft; #199 remains open. This evidence commit only adds docs.

## Actual published-source results

- `go test ./... -count=1 -timeout=15m`: PASS, exit 0.
- `go test -race ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout=15m`: PASS, exit 0; 38.949s / 46.922s / 1.044s.
- `go test ./internal/tunnelnode -run TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle -count=3 -timeout=5m -v`: PASS 3/3, exit 0.
- `go vet ./internal/tunnelnode ./internal/agent ./internal/agentjob`: PASS, exit 0.
- Ownership mutation acceptance is reported verbatim in `baft-published-owner.log`; an environment skip is not a pass of the ownership change.

Tests use real BAFT/pair binaries and H2/TLS/Noise echo traffic, simulated systemd;
no production node or field test. Go 1.27.1 installed in scratch. Published-source
checkout stayed detached and clean for tests. No transport/shaping changes.

All four published-head workflows were SUCCESS at final read: CI 38522602557,
R3 38522602492, static 38522602495, BCC UI 38522602498.

## Concurrent publication and preserved development attempts

Initial read: `d8bb3b69decfba2ba57430bcfe61eab64a6ba3cc`, 524 #67 comments;
no matching active claim observed. Later fresh read revealed claim 6162291939,
which appeared after that initial read and was released by report 6162443280.
The remote head advanced to 59cc9df before this run's publication check.
It implements the same staged-PKI objective with additional same-host/finalization
checks. No local runtime patch was pushed over it, merged, reset or rebased.

The superseded local draft is retained as an ARCHIVE ONLY patch: two local commits
171dd4934840cbdaa846e5c48acf514be6a84594 and
a523b6a47e8b06a045414535451e03c8290eb50c, based on d8bb3b6.
Do not apply it over 59cc9df; it was not submitted to PR #200.

Development logs preserve the original Prepare-live-PKI regression failure,
first-install absence handling failures, the outdated generator failure fixture,
read-only service query assertion failures and corrected passes. The first full
suite at 171dd49 failed the Agent fake non-PEM certificate fixture; tunnelnode race
and sibling 3/3 passed. The Agent fixture was corrected independently and the full
suite, affected-package race and sibling 3/3 passed at a523b6a. These are distinct
from the independent final tests of the ACTUAL published source above.

Initial non-escalated git read failed because browser-proxy was unavailable;
authorized escalated git read/clone worked. An incorrect source filename read and
connector argument-binding attempts did not change repository state. No failures
were replaced or counted as successful by rerun.

## iMonitor application

Owner attachment read and upstream clone reviewed at
`iranmonitor/imonitor@5f39fe2c9da144dc83e5c5bb666665836fbcc4e6`.
[Supplemental requirements](imonitor-requirements.md) map useful ideas to existing
#199/#145/#146/#147/#74 work. No upstream source or speed-test protocol is copied.

The proposed 10s/direction, 64 MiB TOTAL, one concurrent test budget is a design
proposal for isolated lab measurement, not an implemented runtime default or
claimed capacity. Actual next P0 remains pre-cutover candidate configuration/
handshake/payload, physical-host inventory/reservations and installer #197/shared
binary coexistence. No production change, main merge, tag/release, new transport,
shaping, EX pool, exit substitution, restart-resume or standby role activation.

# Doctor and data-gated finalization: final evidence

Reviewed code HEAD: `d8bb3b69decfba2ba57430bcfe61eab64a6ba3cc`.
Code tree: `189e4a189bbd4449608149f59aa9525e97b3acf8`.
Baseline implementation: `1de4e132efcf5d9d937bd940fdbb8bf13cab7776`.
Main baseline: `b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`.

Evidence is a separate append-only branch. It does not alter the reviewed code.
Source publications `094dafa`, `28dfdec`, `c8ac288` and `d8bb3b6` remain in history.

## Exact-head local checks

A clean detached checkout uses Go 1.27.1, with its bin directory on PATH.

| Check | Result | Log |
| --- | --- | --- |
| Full unit/integration: `go test ./... -count=1 -timeout=10m` | PASS | [unit/integration](199-d8-unit-integration.log) |
| Full affected-package race, 20-minute budget | PASS (BCC 759.259s) | [race](199-d8-race.log) |
| Two real instances, sibling lifecycle, three repeats | PASS 3/3 | [real process](199-d8-live.log) |
| Launch acceptance shell syntax | PASS | [empty successful output](199-d8-launch-syntax.log) |
| Dashboard waiting/unknown/retry behavior | PASS | [behavior](199-d8-dashboard-contract.log), [harness](199-d8-dashboard-contract.js) |
| Actual uid/gid mutation | Explicit SKIP (only uid/gid 0 mapped; chown 65534 returns EINVAL) | [ownership](199-d8-ownership.log) |

Race command:
`go test -race ./internal/carrier/h2 ./internal/bcc ./internal/agent ./internal/tunnelnode ./internal/node ./internal/session ./internal/agentjob -count=1 -timeout=20m`.

Real-process command:
`go test ./internal/tunnelnode -run TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle -count=3 -v -timeout=5m`.

The real-process test runs actual BAFT binaries, H2/TLS/Noise and echo payload.
It rejects idle data evidence, checks both fresh runtime proofs and seals, and
keeps the same Germany TCP flow/IR PID/generation/artifacts through Bulgaria
create, changed address/port/CA, rollback, injected failure and retirement.
Local service control is simulated; this is not production/two-VPS acceptance.
BCC/Agent adapter tests model signing/lifecycle with synthetic runtime counters;
those tests are not claimed as real traffic.

## Regression and CI history, including all failed attempts

All [development logs](development/) are retained; partial/skipped/compile-only
runs are never relabelled PASS.

- Doctor setup attempts first failed from test fixture errors (undefined type,
  uninitialized map); `199-doctor-before.log` then reproduced the actual false
  PASS/FAIL, stale/wrong-instance and raw TCP inference defects. The focused
  post-fix log passes.
- Data-gate early attempts failed from stale capability fixtures, a missing
  signed-action allowlist entry, and ten-vs-fourteen job fixture expectations.
  Those were corrected. The simulated coordinator now stamps its own jobs.
  Focused attempt2 caught that clock defect; focused attempt3 passes.
- Full development unit attempt1 failed four stale topology fixtures; attempt2
  passed. These source-in-progress runs are not exact-final acceptance.
- Development race attempt1 retains those fixture failures and a 600-second
  suite timeout while SQLite-heavy BCC tests were still progressing. It is FAIL,
  even though no race warning was observed. No tests/repetitions were removed;
  the final budget is 20 minutes.
- Parent `094dafa` full unit and 3/3 live runs passed. Its race was interrupted
  (exit130) when the atomic-plan fixture clock was corrected; partial output
  is not a PASS.
- Parent `28dfdec` full unit and full affected-package race passed (BCC 612.990s),
  with 3/3 live runs. [Race log](199-28d-race.log). Parent `c8ac288` changed only
  the launch shell fixture; its local unit, focused race and 3/3 live runs also
  passed. These predecessor results are labelled by their own SHA. Go subtree
  hashes and the shell-only comparison are retained separately.
- c8 GitHub R3 failed the complete correctness gate from ephemeral port reuse:
  [full failure log](development/github-c8-r3-failed.log), workflow 38493450584,
  job 115623755978. The adapter now holds EX/IR/target sockets from allocation
  to teardown; rotation uses the held EX listener. Production code was unchanged
  by this fixture fix. Five repeats of rotation-disconnect and signed BCC flow
  passed under race: [log](199-agent-listener-reserve-race.log).
- c8 GitHub systemd launch acceptance job 115623756295 succeeded. Its updated
  fixture rejects idle finalization, then sends actual echo traffic before
  waiting for active for the singleton and both scoped siblings. The earlier
  active-before-traffic ordering could not satisfy the new gate.

Final-head systemd launch acceptance succeeded (job `115625408789`):
[retained excerpt](github-d8-launch-excerpt.log). It exercised real signed-release
BCC/Agent/systemd services, pre-finalization payload, certificate rotation,
rollback, secret cleanup and scoped sibling decommission on a throwaway GitHub
runner. This is stronger than local fake service control, but not a two-VPS
field rollout.

CI checked out synthetic PR merge `edb9ccc8bc0542e4673c4b674acd92d0e305eb8a`,
whose code tree is exactly `189e4a189bbd4449608149f59aa9525e97b3acf8`, equal to
reviewed HEAD `d8bb3b6`. The GitHub snapshot records both identities.
GitHub workflow status is recorded in [snapshot](github-workflows.json), with
its observation timestamp. R3 full unit/race/static/fuzz succeeded; its retained
[excerpt](github-d8-r3-excerpt.log) includes BCC race 445.847s. Any IN_PROGRESS
status is historical at snapshot creation, not silently promoted to SUCCESS. PR
workflows may checkout a synthetic merge ref; the checked-out SHA/tree is
recorded separately from the associated PR HEAD rather than conflating them.

## Scope and limits

Doctor v2 binds its digest to instance, generations, time/TTL and stage evidence.
Missing/stale/malformed/conflicting/wrong-instance evidence is NOT_ASSESSED;
aggregate node health, plan hashes and plaintext raw probes do not prove TLS,
Noise, authorization or application traffic. Both nodes need fresh scoped
observations and complete consistency evidence. Old evidence is not refreshed
merely by running Doctor again.

New BCC deployments require fresh authenticated data-proof capabilities from
both agents and their installed binaries. The loopback GET-only proof endpoint
exports hashes, boot identity, counters and timestamps, never payload, keys,
endpoint addresses or peer labels. Complete successful application-socket DATA
writes are counted; admission and duplicates are not. Unknown/idle evidence
retains backups with bounded retries and an overall verification deadline.
Delayed receipt or sealing cannot renew old data. Both nodes seal before any
backup release. Incomplete cleanup retains ownership and is held for explicit
re-verification/retry, never declared active or rollbackable.

This is bounded recent bidirectional socket delivery through one configured
Noise-authenticated route/peer after commit and before destructive backup
release. It does not prove L7 semantics, a challenge flow, uninterrupted health,
zero downtime, browser fingerprints, filtering or resistance to blocking.

Open gates: fully staged pre-cutover payload/PKI verification, physical host
bind/filesystem/unit inventory, broad capabilities, older in-flight/shared-binary
upgrade compatibility, #197 coexistence, broader concurrency/fault coverage,
continuous Doctor refresh and authorized live field acceptance. Clock freshness
is strict: future agent timestamps remain unknown; both hosts need synchronized
clocks for deployment proof. #199 remains open.
No production/server mutation, main merge, tag/release, transport/shaping change
or automatic process-restart recovery was performed.

No credential/pairing/private-key marker was found in the retained local and failed R3 logs.
SHA-256 checksums for every evidence file are in [manifest](sha256.txt).

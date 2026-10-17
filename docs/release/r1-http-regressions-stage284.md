# R1-03 HTTP regressions and native tunnel safety — Stage284

This is the next bounded change to the single R1 review candidate, draft [PR#242](https://github.com/zarkmakerburg/BAFT/pull/242), under [Stage282](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6230931719). The [worker claim](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6231306438) and [HQ Stage285 checkpoint](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6231726995) define this scope. No competing source branch or UI implementation was opened.

## Qualified input

Parent `9caae8359351eb2be63acf8f7cd21fc1ea675eff`, tree `9af8d1f31057f537a64c3c06d4ef37dd8e2e12e1`, remains an ancestor of this change. Its exact-source workflows completed successfully:

| Parent gate | Evidence | Result |
| --- | --- | --- |
| Main CI | [39013208774](https://github.com/zarkmakerburg/BAFT/actions/runs/39013208774) | SUCCESS, all 15 jobs |
| BCC UI / Node22 | [39013208768](https://github.com/zarkmakerburg/BAFT/actions/runs/39013208768) | SUCCESS |
| R3 | [39013208778](https://github.com/zarkmakerburg/BAFT/actions/runs/39013208778) | SUCCESS |
| Static analysis | [39013208776](https://github.com/zarkmakerburg/BAFT/actions/runs/39013208776) | SUCCESS |

These results complete the previous R1-02 candidate check; they do not qualify the changed head. The new immutable commit/tree, current-head CI results and final local full-BCC race result are recorded in #67. No unresolved review threads were returned for #242 at the initial check; that is not review approval.

## Two defects reproduced and corrected

### Invalid native endpoints accepted by the API

On the parent, authenticated `POST /api/tunnels/plan` returned HTTP200 with `ok:true` for destination `192.0.2.40:0` and `[2001:db8::40]:65536`. The native job regular expressions constrained the shape without validating IP octets or the numeric port range. The same gap affected native listener, route and optional metrics endpoints.

`agentjob.Job.Validate` now validates parsed IP endpoints and ports 1..65535 for native EX/IR preparation, while retaining the existing strict parameter allowlist and scope patterns. Route and metrics listeners must parse as loopback addresses. Native EX listener ports have the same numeric bounds. Valid IPv4 and bracketed IPv6 remain accepted. BCC's existing validation calls this same boundary before a plan/job is created; signing and agent verification also use it. No new DNS target resolution, action, dependency, schema version or state migration is introduced.

### Scoped rollback dropped instance identity

After an EX observation reported a destination different from the reviewed replacement, BCC correctly started rollback, but `startRollbackLocked` created rollback jobs containing only `tunnel_id`. The agent selects its manager from the signed `instance_id`; omission selects its legacy/default scope. This is a misdirected or failed restoration risk, not proof that a live sibling was modified.

Every newly generated rollback job now carries the tunnel's nonempty `instance_id`. Legacy singleton jobs keep the existing shape. The existing dispatch/signature rules already allow and preserve this field, so no new action or permission is added. This applies to rollback triggered by an error or cancellation through the shared rollback generator. The HTTP regression checks the signed payload as well as the returned job record, completes IR/EX rollback, and proves that both the original active tunnel and a separate active sibling retain their state and receive no new work.

## Meaningful HTTP coverage

`internal/bcc/tunnel_http_workflow_test.go` uses a real loopback HTTP server with the production router, Bearer authentication, default security guard, disposable SQLite store, signed jobs and HTTP agent acknowledgments. The temporary Ed25519 job key exists only inside the test. Agents return observation/retirement fixtures; no actual host process, carrier or external node is exercised.

| Contract | Evidence |
| --- | --- |
| Plan → create → activation → edit → decommission | Full HTTP flow for IPv4 and IPv6; preview creates no tunnel/job; deployment binds the reviewed hash; original stays active until verified replacement activation; only that original becomes superseded. |
| Shared IR / independent EX | Two separate active instances share an IR with independent route/metrics resources; sibling record stays byte-equivalent and receives no queued/dispatched work during create, edit and removal. |
| Signed agent dispatch | Verify each job signature, node, action, ID and every parameter before acknowledging it, including instance identity. |
| Ordered destructive removal | Reviewed decommission binds the instance; stale removal hash refuses before state/job mutation; EX gets no retirement job until the IR acknowledgment; final state is durable after reopening SQLite. |
| Unauthorized requests | Missing/wrong admin tokens and an agent token cannot plan, create or decommission. Check both RAM and persisted tunnel/job state. |
| Non-native / unknown engine | Authenticated HTTP plan/create reject FRP, Rathole, Backhaul, WireGuard, SSH SOCKS, WSS, QUIC, unknown and invalid-case engine values; no fallback to BAFT and no job/state creation. |
| Destination validation | Reject hostname, missing port, unbracketed IPv6, zero/out-of-range ports and malformed IPv4/IPv6 through both plan and create endpoints. |
| Stale PlanHash | A changed destination and a changed applied generation return409 plus a fresh plan, before state/job mutation. Failed changed-target deploy does not consume or corrupt the previously reviewed request. |
| Suggestions / review are advisory | Two valid previews for the same EX listener create no reservation. Once one deployment is created, the conflicting deployment is rejected and cannot alter the first. This proves BCC resource admission, not an OS port reservation. |
| Wrong observed destination | A replacement observation with the wrong target triggers rollback, cannot finalize/supersede the original, and restores the correct instance while preserving a distinct active sibling. |
| Port suggestion failure | Existing Node behavior tests retain destination/plan on secure-random failure or retry exhaustion, preserve explicit input, avoid inventory conflicts and invalidate an old plan after a successful change. These are VM/contract checks. |

The native V1 API still has its existing fixed topology; this change does not introduce a generic direction/carrier/crypto request schema. Future independent axes belong to D-01 and driver contracts. Browser-side unsupported-direction guards are covered by existing Node tests; no unsupported protocol is newly admitted.

## Validation and negative controls

Final focused command:

```sh
go test -race ./internal/agentjob ./internal/bcc -run '^(TestTunnelHTTP|TestTunnelEndpoints)' -count=1 -v
```

PASS: 6 top-level tests and 44 subcases (50 test nodes), zero failures; BCC14.658s and agentjob1.060s. The signed endpoint test includes valid boundaries 1/65535 and malformed endpoints, and verifies rejection even when a malformed job has a valid signature produced outside the normal Sign function.

Negative proof uses the final regression files with a Go overlay that restores only the two runtime files from parent `9caae8359351eb2be63acf8f7cd21fc1ea675eff`. It leaves the worktree unchanged. Expected failures include invalid endpoint signing/acceptance, HTTP200 for invalid destinations, and rollback jobs missing the instance. The initial pre-fix run and overlay run are both retained in the working evidence log; 21 failing test nodes occurred in the final parent overlay. The implementation was fixed; assertions were not weakened to accept these outcomes.

Additional completed checks:

- All69 Node UI/catalog tests pass (0 failures/skips); Node24.19.0 locally. Candidate CI independently uses Node22.
- Complete race suites: agentjob1.086s, agent51.157s, tunnelnode38.749s, all PASS.
- Vet passes for BCC, agentjob, agent and tunnelnode; gofmt and diff whitespace checks pass.
- Supply-chain policy passes for all13 workflows; no dependency, workflow permission or real release secret change.

The full local `go test -race -count=1 -timeout=15m ./internal/bcc` and new-head GitHub workflows are final qualification evidence in #67. This document does not borrow the parent's green result for the changed source, and contains no self-referential commit ID.

## Compatibility, rollout and remaining gates

Only native preparation validation and newly generated rollback-job scope change. Existing valid legacy/scoped requests and signed jobs keep schema1. Older malformed preparation jobs can now be refused before signing/execution; previously queued or already signed/dispatched records are not silently rewritten or retargeted. R1-06 upgrade acceptance must cover pending operations and failed rollback records before rollout. Returning to the parent source would reintroduce these defects; it is not an accepted production rollback strategy. No running installation was changed by this source/test work.

R1-03 implementation/automated regression results are separate from review acceptance. After new-head CI and any review fixes, continue R1-04 Classic/Aurora action/API parity, R1-05 real-browser eight-mode QA, and R1-06 installer/update/backup/restore/security. Final freeze, two-node field acceptance, signed artifacts, RC/stable acceptance and current publication/rollout authorization remain required. External engines remain disabled. No main merge, tag, release, production restart/access or real signing key operation is performed here.

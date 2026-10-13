# Smart Tunnel Builder: P0 review and first implementation slice

Scope: #199, reusing #101/#102, #145/#146/#147 and #74.
Reviewed baseline: `main@b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`.
Installer dependency reviewed separately: draft #197 at
`fb1950b295442d891aa6a09481d030e49c507b24`.

This is development and laboratory evidence. The epic remains open. Production,
main merge and release/tag gates from #67 have not been exercised.

## Existing foundation and dependencies

- #102 is already merged: BCC and Agent carry instance identity; config, state,
  unit, ownership markers, generations and metrics have instance scope. Sibling
  activation does not supersede an unrelated scoped instance.
- #197 is still a draft. It changes installer second-instance selection and
  pairing; this slice does not duplicate or edit that installer work.
- #145 and #147 are closed, but their acceptance still needs rechecking for
  tunnel-scoped identity, freshness and truthful unknown states. #146 remains
  open. An issue closure alone is not a new candidate's acceptance evidence.
- #194 added offline country detection and one-step SSH enrollment. #193 is
  an overlapping older draft with conflicts; its remaining UX changes need
  selective integration into the current offline implementation.

## Confirmed and corrected in this slice

### Bind-resource preflight

`tunnelResourceConflict` only compared fields with the same purpose and compared
endpoint strings verbatim. It missed a metrics listener colliding with a route
or EX listener, both inside one instance and between sibling instances. It also
missed equivalent decimal port spellings.

Preflight now compares every declared TCP bind on each node. Wildcard listener
ports are reserved across address families because Go's `tcp` listener may
become dual-stack. Explicit different loopback addresses remain independent.
Independent hosts can use the same numeric ports.

Numeric port bounds and actual IP parsing are checked for listener, route,
target and metrics before any tunnel/job is stored. Legacy singleton requests
cannot declare metrics endpoints that the signed job would silently omit.
Known same-slot replacements remain supported; scoped/legacy mixing remains
explicitly refused.

The first unpatched regression attempt failed seven cross-purpose collision
cases and six invalid-endpoint cases. Additional final cases cover IPv6 metrics
against a dual-stack wildcard and ignored legacy metrics fields. Both BuildPlan
and creation refuse collisions without changing tunnel/job inventory.

### Truthful rollback completion

Rollback used to ignore daemon-reload, stop, disable, completion-receipt and
active-receipt errors. A corrupt transaction also returned "nothing to roll
back". Regressions reproduced false success for the three service operations,
corrupt transaction and failed completion persistence.

Rollback now returns incomplete/error, retains the active change until a
durable completion exists, and permits retry after repair. A missing transaction
is refused when an active receipt or installed BAFT marker still owns the change.
Unreadable live ownership artifacts cause refusal before mutation. Temporary
secret cleanup errors are also reported rather than silently acknowledged.

### Restore previous running intent and enablement

The real-process fixture twice failed after a Bulgaria port-edit rollback:
committing EX closed the old IR carrier, putting IR into the simulated
systemd `activating` state. The old code saved only `state == active`, then
explicitly stopped the previous service during rollback. Treating `activating`
and `reloading` as running intent restores it.

A separate regression showed that a previously active but boot-disabled service
was left enabled by rollback. Running state and boot enablement are now restored
independently.

### Service-health evidence

Restart-count query failures, empty output and invalid counters must not be
accepted as stable health evidence. All four focused fault cases failed before
the fix and pass afterward. Query errors now return unavailable evidence, and
only a valid unsigned counter can participate in the stability comparison.

## Two-live-instance laboratory regression

`TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle` uses real built
`baft`/`baft-pair` binaries, H2/TLS/Noise transport and integrity-checked echo
payload. Two IR instances share one filesystem/service namespace; each EX is a
separate host fixture. Service control is simulated; no machine's actual systemd
or production config is touched.

One Germany TCP connection remains open through Bulgaria prepare, activation,
port edit, rollback, injected restart failure, recovery and retirement. Every
checkpoint verifies the echoed bytes, the same Germany IR PID and generation,
and unchanged Germany config, unit, ownership marker and Noise-key files.
Bulgaria's previous working route is also tested after rollback.

Attempt 1 and attempt 2 failed at Bulgaria rollback. Attempt 3 passed after the
running-intent fix. These attempts are retained; they are not relabelled as
three successful runs. The final exact-commit validation is recorded in the PR
and #67, including repeated/race attempts.

## Remaining P0 gates and review findings

1. **Actual deployment verification:** `Manager.Health` proves service stability
   and TCP acceptance. It does not prove the tunnel's Noise handshake, route
   authorization, upstream availability or end-to-end payload. The laboratory
   test's echo check does not add that verification to every deployment. Reuse
   the existing probe/doctor machinery with exact tunnel identity before deleting
   rollback backups; do not equate lifecycle `active` with verified traffic.
2. **Unknown states:** `RunRouteDoctor` currently converts absent listener/target
   evidence to FAIL and treats some missing consistency evidence as PASS.
   Aggregate node health and a path-matrix candidate also need exact tunnel/port
   binding. This is a source-review finding; dedicated regressions and a
   structured freshness-aware evidence contract are still required.
3. **Physical host preflight:** this slice protects BCC-declared binds. Fresh
   local listener inventory, filesystem/unit namespace ownership and node
   capabilities still need checking on both agents. Unknown external services
   cannot be declared free solely from BCC's database.
4. **Installer coexistence:** compare #197's standalone instance namespaces and
   unit naming with the BCC-scoped namespaces, plus shared-binary upgrades,
   locking and rollback ownership. Keep that work coordinated with #197.
5. **Legacy migration:** the basic form still uses the singleton by default.
   Planning a second endpoint can be a same-slot replacement. A guided explicit
   migration/add-instance flow must distinguish "add" from "edit/replace" and
   preserve the user's explicit EX identity.
6. **Persistence/concurrency:** same-process manager scoping and host-wide resource
   reservation require further concurrency review. BCC store locking protects
   its metadata; it is not proof of a cross-process host reservation.
7. **Rollback/finalization fault matrix:** audit every preparation, backup,
   generation and filesystem failure. Finalization cleanup and install-time
   error aggregation still need broader injected-fault coverage.
8. **Live acceptance:** the container fixture is not a two-VPS systemd/agent/BCC
   rollout. That gate needs a separately authorized isolated live laboratory.
9. **PKI rollback ordering:** the current rollback restores a replaced PKI tree
   after restarting the old service, and skips a missing `.before-ID` tree.
   This source finding needs fault/changed-host regressions, verified backup
   identity and restoration before restart; the current two-instance fixture
   deliberately keeps the endpoint certificate name unchanged.

## P1 and P2 implementation map

| Area | Current evidence | Next requirement |
| --- | --- | --- |
| Capabilities | Config validation accepts H2 and WS; uTLS requires WS; H3 is explicitly rejected | Versioned discovery on BOTH nodes; support/experimental/unavailable states |
| Builder | Reviewed plan hash, explicit deploy, edit-by-replacement exist | Guided/expert forms with instance identity, effective settings and impact |
| Security | Signed allowlisted jobs and instance parameters are retained by this slice | Persian inner/outer encryption/authentication guidance, no weaker fallback, credential-redaction review |
| Traffic Identity | Protocol implementation is inspectable; comparative field evidence is absent | Measured fingerprints from #74; show unknown where measurement is missing |
| Network Doctor | Existing path-probe/matrix/doctor code can be reused | Exact identity, timestamp, confidence, actionable unknown/error states |
| Enrollment/map UX | One-step enrollment and offline IP country lookup exist | Catch/finally for rejected SSH requests; restore-auto location option; label country anchors approximate |
| Profiles | Tunables and resource cost require engine-specific validation | Quiet/Balanced/Boost ceilings, isolated before/after measurement, safe fallback |
| Capacity | No verified user-count/capacity acceptance in this slice | Load tests, CPU per throughput, memory, latency/loss and stability evidence |
| Operations | Ownership, generation, audit and rollback foundations exist | Topology, version/diff history, templates, alerting and Lab/Production safeguards |

No new transport, shaping change, EX failover, restart-resume feature, browser
impersonation claim or numerical blocking probability is introduced here.

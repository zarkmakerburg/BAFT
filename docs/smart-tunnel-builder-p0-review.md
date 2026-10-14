# Smart Tunnel Builder: P0 review and implementation slices

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

### Preserve the first failing transport stage

Final validation exposed an existing H2 diagnostic race: after TLS failed, the
Noise pipe writer's secondary error could overwrite `phase=tls_handshake_error`.
The existing test failed in 100 repeated attempts on unchanged baseline H2 code.
The trace now retains an unrecovered TCP/TLS failure when reporting a transport
error, clears failed attempts after a successful/reused connection, and ignores
late losing dials. Error identity and endpoint sanitization remain intact.
Deterministic pipe/dial-order regressions and 100 repetitions with race passed.

### Verified PKI restoration before restarting the previous service

The next slice starts from `2737902`. Changed-host regressions reproduced the
old service restarting with the new certificate set, plus false rollback success
in six missing/changed/symlink/extra-file scenarios. Those failed attempts are
retained in `docs/evidence/smart-tunnel-p0/pki-before.log`.

Before replacing an existing EX PKI tree, preparation now records its complete
identity durably. The old and installed identities include all directories,
regular-file bytes, modes and uid/gid; symlinks/special files are refused. Rollback
checks both trees before changing config, unit, generation or service. It restores the
old tree before config validation and restart, using the existing atomic directory
exchange on Linux instead of deleting live material first.

An already restored live tree is recognized by its recorded old identity, not by
the absence of a backup. Faults after exchange/restart remain claimed and explicit
retry through a fresh Manager works; the new set is retained until rollback
finishes. Unknown backup/live edits and older replaced-PKI transactions without
identity evidence fail closed. They require explicit repair rather than guessing
ownership. Failed preparation captures partial generated material and reports an
incomplete abort if restoration cannot be proved.

Live identity is checked again after service operations before discarding the
exchanged-out tree. A regression also reproduced false completion when a
certificate was changed during restart; this now leaves rollback incomplete
and retains the backup and active claim.

The focused tests cover restart-time certificate name, missing/changed backups,
changed live material/modes/ownership, added files, symlinks, missing identity,
interrupted exchange, restart failure and absent/partial/generated PKI preparation failures.
The two-instance fixture now changes Bulgaria's public address as well as its
port, forcing a new CA/leaf; after rollback its original PKI and real echo payload
are checked while the original Germany flow remains open.

The uid/gid mutation subtest requires a privileged environment with another
mapped uid/gid. It skips in this container (chown returns EINVAL) and on
unprivileged runners. That case is not counted as live ownership acceptance;
the failed environment attempt is retained alongside the guarded retry.

This does not add automatic recovery after process restart, a general deployment
payload gate, a host-wide lock, or a staged certificate cutover. The existing EX
prepare path still replaces disk material before commit; fully staging that
material and proving coordinated cutover remains a separate P0 design task.

## Fresh instance evidence and data-gated finalization follow-up

Route Doctor schema v2 binds its digest to tunnel/instance, both generations,
assessment time, TTL and stages. A stage cannot pass from aggregate node health,
a plan hash or a plaintext BAFTP1 candidate probe. Missing, stale, malformed,
wrong-instance and conflicting evidence stays NOT_ASSESSED. It neither opens a
probe listener on an active tunnel port nor infers filtering from untyped errors.
Fresh artifact-scoped observations and complete two-node drift evidence are
required independently. Re-running assessment does not refresh observations.

New BCC deployments require a fresh authenticated capability report from BOTH
agents. Each agent checks the installed `baft capabilities` command. Older or
downgraded clients revoke the claim; host/role/key/token changes clear it. The
capability is narrowly `tunnel-verify-v1`, not general host resource discovery.
Existing active tunnels are not migrated by this change. An already in-flight
older transaction or shared-binary upgrade still needs a coordinated upgrade
review; this slice does not add automatic recovery after process restart.

The session delivery hook counts only complete received DATA segments written
to the application socket, not admission into a buffer or duplicate frames. A
GET-only, non-cacheable endpoint on the existing literal-loopback metrics
listener exports config digest, boot identity, counters, timestamps and current
application authority, with no payload, keys, addresses or peer/route labels.
The proof is supported only for one Noise-authenticated peer and one route,
TLS 1.3, H2/WS, and no fallback. Multi-route/multi-peer and reused Runtime objects
remain unsupported rather than attributing aggregate counters to a tunnel.

Agents bind that runtime report to current config/unit/marker digests, generation,
instance and transaction, require stable service/current carrier authority and
fresh actual delivery in both directions, and check the EX target accepts TCP.
BCC accepts only correctly scoped signed-job results. Idle is NOT_ASSESSED:
users must create test traffic on the selected tunnel. Retries are bounded at
10 seconds; a 20-minute overall verification deadline retains rollback material
until rollback is attempted. Endpoint reads have strict schema, size, clock,
loopback, timeout and redirect bounds. A delayed acknowledgement cannot renew
old DATA. Three minutes is an evidence window, not a continuity guarantee.

Both nodes must seal a fresh PASS while retaining their backups before ANY
backup release is queued. Node cleanup first persists a finalizing phase;
filesystem/receipt failures retain the active claim and are reported. BCC holds
`finalize_incomplete`, refuses rollback after release begins, and exposes an
explicit retry which rechecks both nodes before attempting cleanup again. The
panel shows Persian waiting guidance and unknown evidence states.

This proves bounded recent bidirectional DATA delivery through the sole
configured/authenticated route. It does not prove application semantics, one
specific challenge flow, future availability, zero downtime, a measured browser
fingerprint, or field resistance to blocking. Verification happens AFTER commit
but BEFORE destructive backup release. Fully staged pre-cutover payload/PKI
verification remains open. Direct/manual legacy Manager transactions keep their
older contract unless explicitly opted in; BCC-created transactions opt in.

Unit fixtures model BCC signing/lifecycle and Agent adapters; their synthetic
proof counters are not field evidence. The separate two-real-process fixture
checks actual echo bytes, idle rejection and fresh proofs before both seals.
Local service control remains simulated. Final exact-head evidence is linked
from PR #200 and HQ #67, including failed development attempts.

## Two-live-instance laboratory regression

`TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle` uses real built
`baft`/`baft-pair` binaries, H2/TLS/Noise transport and integrity-checked echo
payload. Two IR instances share one filesystem/service namespace; each EX is a
separate host fixture. Service control is simulated; no machine's actual systemd
or production config is touched.

One Germany TCP connection remains open through Bulgaria prepare, activation,
address/port edit with PKI replacement, rollback, injected restart failure,
recovery and retirement. Every
checkpoint verifies the echoed bytes, the same Germany IR PID and generation,
and unchanged Germany config, unit, ownership marker and Noise-key files.
Bulgaria's previous working route is also tested after rollback.

Attempt 1 and attempt 2 failed at Bulgaria rollback. Attempt 3 passed after the
running-intent fix. These attempts are retained; they are not relabelled as
three successful runs. Candidate 678d001 later passed three repeats but failed
the GitHub R3 startup check: a 50ms fixture settle window could precede actual
listener readiness. The follow-up waits for handshake and real echo payload
before health sampling; five local repeats passed. Final exact-commit validation
is recorded in the PR and #67, including failed/repeated/race attempts.

## Remaining P0 gates and review findings

1. **Pre-cutover verification:** the new capability/data/seal contract gates
   destructive finalization. Commit still precedes actual payload evidence.
   Fully staged coordinated cutover, per-flow challenge/application semantics,
   and older in-flight transaction compatibility remain open.
2. **Continuous Doctor evidence:** the false PASS/FAIL and scope/freshness defects
   have dedicated regressions and a schema-v2 contract. Stored observations
   expire; continuous instance-scoped refresh and broader measured diagnostics
   are still required. Old raw candidate probes never substitute for these.
3. **Physical host preflight:** this slice protects BCC-declared binds. Fresh
   local listener inventory, filesystem/unit namespace ownership still need checking on both agents. This slice
   discovers only the narrow data-proof capability, not every host capability. Unknown external services
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
9. **PKI cutover:** rollback ordering and backup/live identity now have
   changed-host/fault regressions in the follow-up slice. The prepare path still
   replaces existing live disk PKI before activation; fully staged, coordinated
   cutover and served-certificate/payload evidence are still required. Identity
   checks do not prove an atomic host-wide reservation against concurrent writers.

## P1 and P2 implementation map

| Area | Current evidence | Next requirement |
| --- | --- | --- |
| Capabilities | Config validation accepts H2 and WS; uTLS requires WS; H3 is explicitly rejected | Narrow data-proof discovery now exists; broader support/experimental/unavailable discovery remains |
| Builder | Reviewed plan hash, explicit deploy, edit-by-replacement exist | Guided/expert forms with instance identity, effective settings and impact |
| Security | Signed allowlisted jobs and instance parameters are retained by this slice | Persian inner/outer encryption/authentication guidance, no weaker fallback, credential-redaction review |
| Traffic Identity | Protocol implementation is inspectable; comparative field evidence is absent | Measured fingerprints from #74; show unknown where measurement is missing |
| Network Doctor | Existing path-probe/matrix/doctor code can be reused | Fresh scoped Doctor v2 now exists; broader network measurements and continuous refresh remain |
| Enrollment/map UX | One-step enrollment and offline IP country lookup exist | Catch/finally for rejected SSH requests; restore-auto location option; label country anchors approximate |
| Profiles | Tunables and resource cost require engine-specific validation | Quiet/Balanced/Boost ceilings, isolated before/after measurement, safe fallback |
| Capacity | No verified user-count/capacity acceptance in this slice | Load tests, CPU per throughput, memory, latency/loss and stability evidence |
| Operations | Ownership, generation, audit and rollback foundations exist | Topology, version/diff history, templates, alerting and Lab/Production safeguards |

No new transport, shaping change, EX failover, restart-resume feature, browser
impersonation claim or numerical blocking probability is introduced here.

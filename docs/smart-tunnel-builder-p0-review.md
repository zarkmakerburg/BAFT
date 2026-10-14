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

That rollback slice did not add automatic recovery after process restart, a
general deployment payload gate, a host-wide lock, or staged certificate cutover.
The later sections describe the data gate and staged PKI follow-ups; coordinated
pre-cutover payload verification remains open.

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
but BEFORE destructive backup release. Pre-cutover payload verification remains
open; PKI artifact staging is described below. Direct/manual legacy Manager transactions keep their
older contract unless explicitly opted in; BCC-created transactions opt in.

Unit fixtures model BCC signing/lifecycle and Agent adapters; their synthetic
proof counters are not field evidence. The separate two-real-process fixture
checks actual echo bytes, idle rejection and fresh proofs before both seals.
Local service control remains simulated. Final exact-head evidence is linked
from PR #200 and HQ #67, including failed development attempts.

## Staged PKI preparation follow-up

Baseline: PR #200 at `d8bb3b69decfba2ba57430bcfe61eab64a6ba3cc`.
Two focused regressions failed on that baseline: changed-host preparation
replaced the live certificate tree, and initial preparation installed live
material before commit.

EX preparation now generates a candidate in the same instance's `pki.next-ID`
directory. It leaves live certificates, config, service, generation, unit and
marker unchanged. The pairing code uses the candidate CA. Preparation verifies
the CA, hostname, validity window and both certificate/private-key pairs, applies
the existing protected modes/ownership and records the complete tree identity.
Unchanged-host reuse records and rechecks the live identity without rewriting it.

Commit rechecks both previous and candidate identities before accepting the
reply and before changing live artifacts. Its final staged config already uses
the permanent PKI paths; config syntax and certificate material are checked
separately. Existing PKI is exchanged atomically with the candidate on Linux,
then retained at `pki.before-ID`. First installation renames the candidate into
the previously absent live path. Explicit rollback recognizes either location
of the previous tree if interruption occurred between exchange and rename.
Cancellation never restarts the service and removes only recorded generated
material. Unknown manual edits, missing backups and invalid certificates are
refused while retaining the active claim. Finalization checks the installed PKI
identity before deleting rollback material.

Tests cover initial/changed-host prepare and cancellation, candidate/live/reuse
drift, invalid generated hostname/key, interrupted first activation and both
exchange windows, rollback after restart failure, and finalization after PKI
drift. The real-process sibling fixture also sends an echo payload through the
previous Bulgaria route while its replacement is still prepared. This checks
the old route's continued usability; it is not a payload test of the candidate.

The change is PKI artifact staging, not full coordinated pre-cutover handshake
and application verification. Separate candidate jobs described below now
complete EX configuration before commit; payload evidence still comes after commit. This
does not promise zero downtime, introduce a host-wide reservation or enable
automatic process-restart recovery. Older in-flight transactions retain their
previous PKI rollback contract when the new preparation fields are absent.

## iMonitor ideas integrated into the existing roadmap

The current `imonitor-for-baft-analysis-20261014.md` report was read. It reviewed
iMonitor at `5f39fe2c9da144dc83e5c5bb666665836fbcc4e6` and distinguishes design
ideas from runtime acceptance. No iMonitor source is copied into this slice.

| Useful idea | BAFT execution path and evidence limit |
| --- | --- |
| Separate connectivity from delivered data | Doctor v2 and the scoped two-node delivery gate above; unknown remains unassessed |
| Real interface/address/route/socket inventory | Next P0: read-only host preflight on BOTH agents, ownership and IPv4/IPv6 conflict checks; inventory alone is not reservation |
| Bounded authenticated upload/download | Reuse path-probe and selected-instance evidence; volume, duration, concurrency, cancellation and receiver-confirmed bytes must be bounded |
| Persian step-by-step guidance | Extend the current builder and SSH enrollment rather than introducing a second panel |
| Explain encryption and profiles | Describe verified inner/outer encryption and supported tunables; no unmeasured browser or blocking claim |
| Layered resources/history and ownership | Keep host, agent, service and tunnel observations separate, preserve gaps and sibling resources; monitoring-only edits must not restart a tunnel |

The report's current-license conflict, insecure SSH host-key examples,
overbroad blocked-region classification and unverified notification delivery are
not adopted. Existing #145/#146/#147, #74 and #197 remain the integration points.

## Node-local resource preflight follow-up

Baseline: `59cc9dfe719a535914b58e358d2760938fad2f37`. An actual listening
socket reproduced a false-safe prepare on that baseline: it returned a pairing
code without noticing the occupied port.

New BCC prepare jobs explicitly require `verify_resources=true` on BOTH agents.
The signed allowlist validates this field; older agents reject the unknown
parameter rather than silently omitting the safety check. The transaction records
the requirement for commit. Older/direct transactions keep their existing opt-in
contract; this does not retrofit all in-flight work or add broad capability UI.

Each node checks the actual local TCP4/TCP6 listener tables, with bounded reads
and parsing. A missing/malformed/truncated table is unavailable evidence, never
"free". Wildcard ports are conservatively treated as potentially dual-stack.
The inspection does not open a candidate listener, scan external hosts, stop a
service, alter firewall rules or disclose process arguments. It is repeated
before accepting the EX reply and immediately before installing either node's
staged config. A newly occupied port leaves the preparation claimed and intact
for explicit retry/rollback; the EX pairing secret is not consumed by that refusal.

Existing config/unit/marker/generation/instance identities must match; partial,
unmanaged, symlinked, special-file or corrupt active ownership is refused. A
colliding existing socket can be reused only if it was declared by that verified
configuration and its inode belongs to the current systemd MainPID. The process
start identity and network namespace are checked around bounded FD reads, and
MainPID is rechecked afterward. The admin Unix socket also needs exact path,
inode and service ownership; unrelated files/sockets are preserved.
Unix accepted clients can retain the listener pathname. The parser selects
only the listening stream socket; an active management connection does not
make that owner appear ambiguous. Pure parser and real client-held Unix socket
regressions cover this distinction.

This is fresh node-local inspection, not an atomic reservation against another
process. The remaining window between check and bind still relies on service
startup failure/rollback and the existing data/finalization gate. Other network
namespaces, absent procfs or unavailable FD ownership cannot be declared safe.
General interfaces/routes, disk capacity, all shared installer resources and
cross-process host locking remain open. Adapter fixtures explicitly model socket
inventory; they do not count as real host acceptance.
OS permission/UID/GID hardening beyond the existing PKI checks still requires
review; matching artifact bytes/markers is not a general permission audit.

Local regression tests cover the real occupied bind, a new bind between prepare
and commit with safe retry, unknown/malformed inventory, wildcard/address-family
collisions, proof-bound reuse and changed/unmanaged namespace artifacts. This
runtime cannot inspect another process's namespace and refuses AF_UNIX creation;
the full two-process resource fixture and live Unix-socket subtest are explicitly
SKIP, not PASS. CI's required `Node-local tunnel resource acceptance` runs these
tests three times with `BAFT_REQUIRE_RESOURCE_EVIDENCE=1`, converting either
environment skip into failure. Current-head Linux/real-systemd qualification must
finish before acceptance; predecessor green checks do not satisfy this gate.

## Prepared configuration identity and validation follow-up

Baseline: PR #200 at `0da45e07d71c5f7a84f29bc07c9f98d644745f4b`.
Regressions reproduced acceptance of valid edits to a prepared IR config, CA,
Noise key, symlink and permission changes. An invalid prepared IR config was
refused only after rollback had reloaded/restarted the previous service. A
changed EX target was also accepted and consumed the pairing material. A
separate injected change after the binary's successful config validation
reproduced active PKI/config replacement with unchecked candidate bytes.

New preparations durably bind IR config bytes and config/input modes and uid/gid
to the transaction before returning its reply. EX preparation similarly binds
the parameters, pending state, one-time PSK and Noise key before returning its
code; drift is refused before reply acceptance/consumption. Once the EX reply
completes its candidate, that configuration is validated and bound before any
live PKI activation. Reads are bounded to 1 MiB, reject symlink/special files
and directory aliases, and sample metadata around a single open descriptor.
Neither private keys, config contents nor pairing codes are added to job output.

Commit verifies the recorded config and dependencies with the typed loader and
the installed binary before backup/live mutation, repeats the identity check
before activation and installs the captured validated bytes. Role, Noise-key,
candidate CA and management paths must belong to the selected instance.
An invalid/changed candidate retains its prepared claim and leaves live
configuration, unit, marker, generation, certificates and service untouched.
Restoring the recorded candidate permits an explicit retry. Older in-flight
receipts without preparation identities keep their prior identity contract;
they still receive bounded syntax/path validation before activation. Failed IR
preparation now also reports any incomplete rollback instead of hiding it.

This closes candidate drift/validation ordering gaps, not the entire staging
roadmap. New EX candidates are now generated by the separate StageEX job;
BCC does not yet coordinate a separate candidate process handshake or
payload BEFORE either active service changes. Root/operator writes after the
last check and service-credential readability need further host locking and
access review. Shared-binary upgrades and installer #197 coexistence remain
separate gates; no installer source, transport/shaping or recovery behavior is
changed. Exact final SHA/CI evidence and all attempts are linked from PR #200
and #67/#199 rather than counting predecessor checks as acceptance.

### Separate signed candidate preparation before either commit

New BCC deployments explicitly request `stage_config=true` on BOTH prepares.
EX reply consumption and final config generation move to `tunnel_stage_ex`;
IR then runs a read-only `tunnel_validate_candidate`. Both return only scoped
artifact reports: tunnel/instance/role, candidate config SHA, current generation,
effective route/listener/peer/target/shaping/metrics, passive resource-check flag
and observation time. They contain no key, config bytes or pairing reply.

BCC rejects generic success, mismatched scope/parameters/generation, incomplete
digests, missing resource evidence and reports older than five minutes (future
clock allowance one minute). Only after BOTH reports pass does it queue the EX
activation. The signed activation contains the config SHA and generation; the
Agent cannot regenerate configuration at that boundary. IR activation is queued
only while both reports remain fresh. Local dependency/PKI/resource checks repeat
before installation, and final observation must match the reviewed candidate SHA.
Singleton metrics preserve the Agent's configured loopback endpoint because the
legacy request cannot declare it; scoped metrics must match the explicit plan.

Completed EX staging can be explicitly retried with the same reply; another reply
or altered artifacts are refused. Failure after one-time pairing consumption but
before durable sealing is not claimed resumable: explicit rollback is required.
Old persisted in-flight BCC/node receipts keep the prior protocol, while an older
Agent rejects the new prepare parameter before cutover. No restart-resume feature
is introduced. These are artifact reports, not running-candidate TLS/Noise or
payload proof. Pre-cutover authenticated traffic remains a separate P0 gate.

Installer #197's actual `install.sh` was fetched at the recorded SHA. It places
standalone instances under `/etc/baft-NAME` and `/var/lib/baft-NAME`, but uses the
same `baft-NAME.service` suffix as BCC's nested instance paths. Binaries, release
record and helpers remain shared. The existing node-local unit/ownership gate
must refuse that same-name external unit; source inspection alone does not prove
mixed-version/shared-binary compatibility. The installer branch is unchanged.

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
3. **Physical host preflight:** new explicit prepare jobs inspect local listener
   and instance filesystem/unit ownership on both nodes and repeat the check at
   commit. Native Linux/process/Unix acceptance is still pending as described
   above. Interface/route inventory, general capabilities, shared installer
   resources and atomic cross-process reservation remain open.
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
9. **PKI cutover:** preparation now stages and validates candidate PKI without
   changing live material; explicit interruption/cancellation regressions and
   previous-route payload evidence are recorded above. Coordinated candidate
   handshake/payload before cutover and broader install/cleanup faults remain
   open. Identity checks do not prove a host-wide reservation against concurrent writers.

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

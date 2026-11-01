# v0.4.1 initial audit and release-tail recovery fix

Audit date: 2026-10-30. This is a bounded engineering checkpoint, not v0.4.1
acceptance or authorization to merge, tag, publish or deploy. Issue #269 remains
open. No production resources were changed.

## Verified starting point

- Live `main`: `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`.
- Main tree: `e065727d66ce97a917b9fc64e4e7ec7db9d73e3c`.
- Annotated v0.4.0 tag object: `69e77c655caa0fd0d18ca87226f20c23a2af2b35`,
  pointing to that same commit. The tag itself is unsigned; that fact alone
  says nothing about the separate release-asset signatures.
- The published v0.4.0 release lists 18 assets, including architecture-specific
  binaries, the offline bundle, checksums, manifest and release certificate.
  Asset bytes/signatures/provenance were not independently verified here.
- The Actions API returned 21 completed successful runs for the exact main
  SHA at inspection time, including shared operational ingress and operational
  recovery. This is observed hosted workflow status, not a new local LAB run
  or an independent audit of every job's acceptance evidence.
  References: [CI](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208691),
  [shared ingress](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208454),
  [recovery](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208504),
  [UI](https://github.com/zarkmakerburg/BAFT/actions/runs/41008433301),
  [static analysis](https://github.com/zarkmakerburg/BAFT/actions/runs/41044986750),
  [vulnerability scan](https://github.com/zarkmakerburg/BAFT/actions/runs/41008259495).
- #269 and HQ #67 are open. Open draft PRs #287, #288, #289 and #290 have
  heads already reachable from main (checked with `git merge-base
  --is-ancestor`). Re-merging them is unnecessary. The clone has 358 remote
  refs, including the origin HEAD alias; existing branches were left intact.

## Current implementation versus the mission

| Area | Verified implementation/evidence | Remaining work |
| --- | --- | --- |
| Public ingress | Signed operations, held authority, durable journal, exact nft ownership and remove-only restart reconciliation exist. | BCC retains the desired reservation after failed Apply; its existing Retire admission requires applied ownership evidence. Add an explicit authenticated recovery path, not bookkeeping deletion. |
| Release metadata | The verified-release writer commits the tombstone before removing lifecycle and pending data. | This checkpoint fixes two crash-boundary and identity-matching defects described below; full signed Agent acceptance is still required. |
| Agent/reboot recovery | Separate process-death recovery tests and a hosted operational recovery workflow exist. | Remove-only cleanup is not automatic listener adoption/rebinding or uninterrupted service after Agent death. Systemd restart/boot availability acceptance remains separate. |
| Shared transport | Live tenant management on one authenticated Noise listener exists, including BCC combined ingress acceptance. | Tenants still use separate authenticated sessions. `multiplex-v1` is absent from `protocol.KnownCapabilities`; multiple authenticated tenants inside one session remain unimplemented. |
| Wizard/auto-select | Existing code and regression tests bind transport evidence to the reviewed plan and revalidate it. | Run final-SHA UI/browser acceptance and adversarial stale-evidence cases; source inspection alone is not full UX acceptance. |
| Health/rotation/SNI | These improvements were not independently validated in this checkpoint. | Audit and execute the mission's >64 KB bidirectional, silent-cut, make-before-break rotation and reviewed SNI acceptance. |

Relevant source contracts: `docs/design/shared-multiplexing.md`,
`docs/release/v040-shared-ingress-acceptance.md`,
`internal/agent/ingress_restart.go`,
`internal/ingressownership/restart_reconcile.go` and
`internal/protocol/capabilities.go`.

## Reproduced release-tail defects

The initial regression commit is
`ad9a11ae3f7db55cf2cdc5ac3e476e002a5eeabb`. Its production code is identical
to the verified main baseline. The initial tests actually failed before the
fix, with exit 1:

1. A tombstone plus surviving lifecycle/pending data allowed `ReservePending`
   to acknowledge the released binding through its idempotency shortcut.
2. A crash after the tombstone but before lifecycle removal prevented a fresh
   generation from completing the already-committed release.
3. A tombstone with different tunnel/instance identity could clear a pending
   claim because those identity fields were not compared.

The fix commit is `e86405b6851c4cc82012f3af037d1ec7e5b65785` on
`codex/v041-release-crash-recovery-20261030`:

- Apply the durable release/generation fence before idempotent readback or
  cleanup writes.
- Require every tombstone resource identity to match the old pending claim.
- Complete lifecycle removal only when its full binding matches and every
  current/superseded job is retained in the tombstone's spent-job set.
- Preserve lifecycle-before-pending removal order, file durability and the
  existing OS lock. No new listener/firewall mutation or receipt is introduced.

Code changes relative to canonical main: 269 added / 22 deleted lines across
`internal/ingressauthority/reservation_unix.go`, `release_unix.go` and
`release_crash_unix_test.go`. Audit/evidence documentation is counted separately.

New regression cases cover old-binding replay, both metadata boundaries,
unaccounted current/superseded jobs and foreign tunnel/instance tombstones.
The subprocess test kills a separate writer after a durable marker, then
reopens the store. Its prerequisite host-retirement evidence is seeded:
this is real store-writer process death, not Agent, kernel/nft, systemd or
reboot acceptance. Repeating these tests against the baseline production code
also failed, including the subprocess tombstone boundary.

## Validation and limitations

Go toolchain: `go1.27.3 linux/amd64`, matching `go.mod`.

- Initial targeted regressions: RED, exit 1, before production changes.
- Targeted regressions after the fix: PASS, exit 0.
- Full authority package with race detector: exit 0, 42 top-level PASS,
  14 explicitly skipped LAB-only cases. This does not satisfy zero-SKIP LAB
  acceptance.
- Scoped vet over authority/ownership/Agent/BCC/Agent CLI: exit 0.
- A broad precommit `go test ./... -count=1 -timeout=10m` failed in five
  `ingressinventory` tests. The same failures were reproduced against the
  unchanged baseline production code: restricted UID mapping rejects chown,
  `/proc/1/ns/net` access is denied and process-generation inventory is
  incomplete. Tests were neither changed nor weakened to hide this failure.
- `unshare --mount --net --fork true` is rejected with `Operation not
  permitted`; nft is unavailable and PID 1 is supervisord. This environment
  cannot supply the required isolated kernel/systemd acceptance.
- Exact-commit broader checks are recorded separately in the evidence logs;
  no earlier green result may be used as a final release gate.

Postcommit checks on `e86405b6851c4cc82012f3af037d1ec7e5b65785`:
`go vet ./...` exited 0. `go test ./... -count=1 -timeout=10m` exited 1:
the same five inventory cases failed with the baseline's restricted-environment
errors; every other package, including BCC, completed successfully. See
[`evidence/v041-release-tail`](evidence/v041-release-tail/README.md) for the
original RED, baseline comparison and candidate logs with exit codes.

The broader five-package race command exited 1: authority, ownership, Agent and
Agent CLI completed successfully, but the BCC package exceeded the local
10-minute suite timeout while `TestUnreachableIncidentIsDeterministicAndRestartSafe`
was running. Its retained stack is in SQLite state persistence beneath health
evaluation; no `WARNING: DATA RACE` was emitted. This is an unresolved race-suite
validation failure, not a passing BCC race result or a proven environmental
failure. BCC production/test files were unchanged by this branch. Investigate
or complete that gate in suitable infrastructure before release acceptance.

The pinned 28-case manifest and existing acceptance runners are unchanged.
Local logs retain the original failures and skip declarations. Hosted
candidate CI, full-race/security/UI acceptance, artifact provenance and trusted
offline signing remain required before any release decision.

## Next engineering order

1. Validate this narrow release-tail change on an exact candidate SHA in the
   existing hosted LAB gates; review the metadata trust boundary independently.
2. Design and implement explicit signed failed-Apply retirement/reconciliation
   across BCC and Agent, with replay, expiry, revoked enrollment, unknown owner
   and ambiguous host-state refusal tests. Do not manufacture an applied or
   retired receipt.
3. Complete Agent/systemd cold-start availability and recovery acceptance.
4. Write an ADR for authenticated multi-tenant session framing and capability
   negotiation, prove v0.4.0 compatibility, then implement real `multiplex-v1`.
5. Execute reviewed Wizard/auto-select, health, rotation and SNI acceptance,
   followed by the full exact-SHA release gates and offline signing.

Disposition: v0.4.1 **NO-GO**; the overall mission and #269 are incomplete.

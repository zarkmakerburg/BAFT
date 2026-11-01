# Fresh signed Retire after receipt-delivery failure

This is one bounded P0 recovery change, stacked in PR #293 on PR #292.
It does not close #269 or establish complete v0.4.1 acceptance.

## Reproduced failure before production changes

Regression-only published commit:
`48d1a36e072409ca70fa4c607a44f9b2bec9e3f1`, tree
`b121b4d89627c7c765ab451629888e4217078458` (identical to local
`2e42a2aa8938b882b5c2f6aecf6221a47dc468c1`).

The first signed, concretely leased Retire removes the actual owned listener
and nft rule and commits a durable retired journal. A receipt-delivery fault
before reservation release is modeled by the same durable
NEEDS_RECONCILIATION transition the Agent records on a failed POST. A fresh
signed Retire then receives a new single-use lease but fails because the
executor still requires a live owned listener or an unfinished process-local
stop record. The reservation cannot complete that ordinary retry.

The LAB uses actual signatures, held authority, protected inventory, sockets,
nftables, durable journal/reservations and a separate sole-process helper in a
private network namespace. The orchestrator's private mount namespace keeps
policy changes off the host. It does **not** perform BCC HTTPS receipt delivery
or prove Agent service restart/systemd boot availability.

Hosted workflow: [operational recovery 41096753430](https://github.com/zarkmakerburg/BAFT/actions/runs/41096753430).

- Attempt 1, job 124204371040: RED from incomplete process inventory during
  setup/first retirement, including one original test. This is retained but
  **not** counted as the behavioral regression. Artifact 12997221254 ZIP
  SHA-256: `e62b3345ac0cceabd6b68091d4f7db51e455c5ab6f34214364b559b63ac7aa29`.
- Only that failed job was rerun once. Attempt 2, job 124206353158: exactly
  15 top-level tests PASS, the new positive retry test FAIL, zero SKIP. Its
  result explicitly reached `stage:retry`, `first_retired:true`, fresh
  `job:retire-retry-2`, then returned `ingress restart ownership cannot be
  verified`. The foreign-listener and spent-job negative cases both PASS.
  Artifact 12996119071 ZIP SHA-256:
  `68bb06d53c0639ac8527e1066b409316915cdcfd12e93d955e4cf4e471153ab1`.

Downloaded ZIP digests matched GitHub's recorded digests. Original test2json
events are retained under `evidence/v041-retire-retry/`. Production code was
unchanged until this second, correctly attributed behavioral RED was verified.

## Fix and trust boundary

The operational executor adds a read-only completed-retirement path:

1. Require the normal concrete live lease and exact signed Retire command.
   Existing issuance, expiry, supersession, policy and replay checks remain.
2. Retain the normal transaction-to-guard lock order. Match the exact retired
   journal owner, port and nonzero original nft handle.
3. Pin installed execution scope and revalidate it inside the held guard.
4. Collect fresh complete numeric-port socket evidence: deny any listener,
   stopped-service claim, unknown inventory, wrong-port row or inode-bearing
   socket of the Agent UID. Kernel remnants remain observations, not ownership.
5. Independently read back the pinned nft chain/layout and require absence of
   this owner tag, port and original rule handle. Require no runtime session.
6. Recheck socket evidence and unchanged journal under the same held guard;
   only then return exact retirement evidence. No listener, nft or journal
   mutation occurs on this completion path.

The Agent still sends the ordinary command-bound retired receipt and performs
the separate guarded `ReleaseVerified` step. The change does not mint leases,
accept caller Boolean authorization, manufacture a receipt, adopt a listener or
clear BCC bookkeeping. It changes no wire schema and retains the normal
live/partial-retire path. A released reservation or spent job cannot mint the
lease needed to enter this path, preserving existing restart anti-replay tests.

## Validation at this checkpoint

- Helper/orchestrator build-only tests and vet: exit 0. No local positive
  kernel execution was claimed; this container denies namespace creation.
- Affected ownership package `go test -race -count=1 -timeout=5m`: exit 0.
  Existing opt-in LAB cases are not promoted to local acceptance.
- Vet for ownership, Agent, recovery helper/orchestrator: exit 0.
- The original 13 operational recovery cases remain mandatory, with the
  three new cases added to the zero-SKIP verdict (16 total). The separate
  pinned 28-case operational ingress manifest is unchanged.

The fixed published SHA must independently pass those 16 real kernel LAB
cases and all applicable hosted gates. Local results or the parent's GREEN
cannot satisfy that requirement. Fixed-head hosted evidence and any remaining
failures are recorded in PR #293 and HQ #67. Overall v0.4.1 remains NO-GO.

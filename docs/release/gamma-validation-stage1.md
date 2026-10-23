# Gamma v0.4.0 — operational release validation (LAB only)

Date: 2026-10-23. Gamma baseline: PR #270 candidate `d95ceea23242b3d153c7721ad93bdde2f6a35848`.
Branch: `agent-gamma/p0-release-validation`. **Decision: NO-GO for production.**
This file records exactly what Gamma executed or inspected; other agents' claims are not silently inherited.

## Fresh executable evidence

| Gate | Environment / command | Result | Coverage boundary |
|---|---|---|---|
| CLI EX→IR pairing + signed reply + transport | macOS ARM64 / Go 1.27.2; `E2E_MIB=1 E2E_CONNS=2 bash tests/e2e/pair_and_run.sh` | PASS; exit 0; 2 × 1 MiB roundtrips and SHA-256 equality; rogue reply rejected | Two **processes on loopback**, NOT independent Linux nodes, SSH denial, systemd, reboot or live WAN |
| Release provenance + backup/restore/PKI regressions | macOS ARM64; `go test -race -count=1 ./cmd/baft-pair ./tests/release ./internal/bcc -run 'TestGeneratedConfigsLoadAndPinEachOther|TestPKIServerCertificateVerifiesForHost|TestEncryptedBackupFreshRestoreFullStateAndAudit|TestBackupRejectsWrongKeyTamperAndSchemaWithoutChangingState|TestRestoreFaultInjectionLeavesStateAndAuditUnchanged|TestStaleLocalTagIsNeverUsed|TestCIEvidenceMustComeFromTheIntendedWorkflowFile' -timeout=8m` | PASS; exit 0 (255.04s including concurrency/contention) | Tests are fixture-driven; no stopped real BCC systemd service or restored real machine |
| Rejected pairing-code CLI state mutation | `go test -race ./tests/release -run '^TestGammaRejectedPairCodeDoesNotCreateIRState$' -count=1 -v` | PASS; exit 0; expired/bad-prefix inputs rejected without creating IR key/state/config | Actual binary subprocess, no two-host transport |
| Hosted current-candidate workflows | GitHub SHA `d95ceea...`, runs CI 39961755004 / ingress LAB 39961755129 / static 39961755110 / UI 39961755104 / R3 39961755117 | SUCCESS per GitHub connector, exact SHA | Green CI is not sufficient for deployment authorization |

## Independent Linux two-namespace acceptance (new Gamma CI)

The Linux GitHub Actions LAB on Gamma commit `9a32f8c7b3a7de90ad60569d9da368594e79d7f8` **RUN/PASS**:
[workflow run 39969944832](https://github.com/zarkmakerburg/BAFT/actions/runs/39969944832),
[job 120469652271](https://github.com/zarkmakerburg/BAFT/actions/runs/39969944832/job/120469652271).
Both preparation and execution steps completed SUCCESS. The job log shows
2 × 1,048,576-byte echo roundtrips with independently reported SHA-256 values,
an EX→IR TCP/22 deny test with an actual placeholder listener, IR→EX tunnel
traffic across a veth link, unchanged EX/IR config and Noise-key digests,
and continued reachability of an unrelated TCP/2053 listener after BAFT
shutdown. The script cleans up its temporary network namespaces and processes.

**Scope:** Two independent **network namespaces on one disposable Linux runner**,
not two physically independent servers. Port 22 was an isolated placeholder
TCP listener with a scoped nft drop rule; this demonstrates one-way transport
under packet-filter policy, not real sshd or the user's inaccessible Iran
server. systemd, installer, reboot, WAN/public reachability and live 3X-UI
remain unverified.

## P0-D: compiled BCC restore acceptance (disposable Mac LAB)

The actual `baft-bcc restore` binary is built and launched as a separate
process, not merely invoked through a Go library helper. These two tests
create only temporary private paths; **they do not stop a real BCC systemd
service or restore an actual second host**.

- `TestGammaStoppedBCCRestoreCLI` covers an encrypted snapshot, failure
  with a wrong key, refusal when another process holds the state lock, and
  successful `--yes --json` restoration to the earlier generation after the
  lock is released. Backup file permissions remain private.
- `TestGammaEncryptedBackupRestoreToFreshDirectory` verifies a separate
  destination directory (a simulated second-node filesystem). The CLI
  requires explicit pre-provisioning of that private directory; preview
  refuses mutation until confirmation, restore then reopens the original
  backed-up node, and the original source database remains unchanged.
  Sensitive node tokens and the key are checked against subprocess output.
- Both independently passed on macOS ARM64 under `go test -race` with exit
  status 0, and `go vet ./tests/release` returned 0. Their exact branch
  commits are tracked in PR #274. Linux hosted acceptance for the newest
  branch SHA is separate and must not be inferred from macOS.

Reproduce on disposable hosts only:

```sh
go test -race ./tests/release -run 'TestGammaStoppedBCCRestoreCLI|TestGammaEncryptedBackupRestoreToFreshDirectory' -count=1 -timeout=5m
go vet ./tests/release
```

**Operational caveat:** the restore CLI currently obtains its backup
decryption key from `BAFT_BCC_BACKUP_KEY` in the process environment; this
is not the same as running the service with `--backup-key-file`. Do not put
key material in command arguments, public logs or CI artifacts. Actual
stopped-service restore, permissions on the target host, and recovery under
systemd remain release gates.

**New P0-D key-file trust regression (RED, not merged):** On Mac LAB,
the real `bcc.BackupKeyFromFile` accepted both mode-`0644` key material and
a symlink to a key file. An isolated `TestGammaBackupKeyMustRejectPublicModeAndSymlink`
reproduced both failures (exit **1**) against `5814918`; no actual secrets
were used and the failing scratch test has **not** been added to the PR.
Ownership remains with Beta; [HQ evidence](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6368745255)
and [Beta coordination](https://github.com/zarkmakerburg/BAFT/pull/275#issuecomment-6368745965)
request fail-closed regular-file, ownership/mode and symlink validation
with regression tests. The key-file permission criterion is **OPEN**,
independent of successful encrypted backup content and restore drills.

## P0-C: real v0.3.7 BCC upgrade and cold rollback (isolated Linux)

The actual `v0.3.7` tag is pinned to
`94c55dccf5af7cd126089374d991463931413121`.
A disposable Ubuntu LAB compiles the genuine tagged `baft-bcc`
and the development candidate, boots the old BCC over loopback,
persists a node, shuts it down and snapshots its SQLite state and
sidecars. The new binary then performs a forward schema migration and
persists an additional node. Re-running the old binary against the
migrated DB must **fail closed** without corrupting the two writes.

With both BCC processes stopped, the LAB restores the preserved
pre-upgrade filesystem state, proves that the original binary starts
and sees only the pre-upgrade node, then upgrades again to verify
recovery. The test runs in isolated temporary directories, with no
systemd or production service operations.

- Script: `tests/e2e/gamma_v037_upgrade_rollback.sh`.
- Hosted Ubuntu exact-commit first success:
  [run 39977674832](https://github.com/zarkmakerburg/BAFT/actions/runs/39977674832) /
  [job 120496417536](https://github.com/zarkmakerburg/BAFT/actions/runs/39977674832/job/120496417536)
  — SUCCESS with emitted `[gamma-v037] PASS`.
- Release eligibility now demands this workflow on the **exact tagged SHA**,
  not a lookalike workflow or an earlier successful attempt. Falsification
  tests `TestGammaTaggedReleaseRequiresV037UpgradeLAB` passed on Mac
  against `5814918`, exit 0, across six cases: absent,
  lookalike, failed, running, later-failed and green.

These are **BCC state-schema and cold-snapshot recovery** proofs, not
an EX/IR binary downgrade from v0.3.7 to v0.4.0, live zero-downtime
upgrade, whole-host rollback, systemd reboot or restoration of active
tunnel sockets. A rollback necessarily discards writes made after the
backup; automation must surface that data-loss window. Real node
upgrade/recovery remains a blocking acceptance drill.

## P0-C: EX/IR transport v0.3.7 → candidate (Linux namespaces)

The pinned genuine BAFT **v0.3.7** executables were built from tag
`94c55dccf5af7cd126089374d991463931413121` alongside candidate executables.
The older `baft-pair` generated EX/IR Noise keys, PKI and one-time PairCode;
the original v0.3.7 `baft run` binaries carried two SHA-checked 1 MiB
connections between two separate Linux network namespaces. Both were
stopped and the candidate `baft run` binaries launched **without
re-pairing, replacing config, or rotating secrets**. Two more 1 MiB
SHA-checked connections succeeded. EX→IR TCP/22 was blocked by nftables;
an unrelated IR:2053 service remained reachable before/after restart.
EX/IR config and key digests remained bit-for-bit unchanged.

- Script: `tests/e2e/gamma_v037_tunnel_upgrade_namespace.sh`.
- Workflow: `.github/workflows/gamma-v037-tunnel-upgrade-lab.yml`.
- Hosted Linux result:
  [run 39979913583](https://github.com/zarkmakerburg/BAFT/actions/runs/39979913583) /
  [job 120504182399](https://github.com/zarkmakerburg/BAFT/actions/runs/39979913583/job/120504182399)
  **SUCCESS** (source `d55ebd09`; job log emits explicit v0.3.7→candidate PASS).
- Release admission: `scripts/release/accept.sh` requires the exact tagged
  candidate commit's `v037-two-node-upgrade` job to have completed
  successfully when the workflow is included in that tag. Mac
  `TestGammaTaggedReleaseRequiresV037TunnelUpgradeLAB` passed on
  `3bae76c` (Exit Code 0).

**Boundary:** This is a *stopped-binary upgrade* under network namespaces,
not two physical hosts, systemd upgrade, durable kernel resource adoption,
an unattended installer migration or uninterrupted existing TCP sockets.
No server was modified. Real-host service recovery and signed
installer Upgrade/Rollback remain release gates.

## P0-E: preserve the active service's actual port (dynamic port LAB)

The existing live listener in both Gamma network-namespace acceptance
scripts is now selected by `GAMMA_EXISTING_PORT`, with `2053` only the
historical *test fixture default*, **never a privileged or hard-coded
protected BAFT port**. Invalid values and collisions with the test's
own EX/IR/target/metrics ports fail before creating namespaces. Both
CI workflows now test:

- `2053` with the original active service preserved through pairing,
  traffic, stop/restart and tunnel upgrades.
- `24443` with the same preservation checks on a different real
  listener port.
- `18443` as a deliberately colliding input: rejected before any
  namespace mutation, with unchanged namespace inventory.

Changes: `tests/e2e/gamma_two_node_namespace.sh`,
`tests/e2e/gamma_v037_tunnel_upgrade_namespace.sh`,
`.github/workflows/gamma-two-node-lab.yml` and
`.github/workflows/gamma-v037-tunnel-upgrade-lab.yml`.
### Verified results on integrated QA candidate

On exact QA SHA `03f9e5c42ff12159b2621e7eef24cb34a151f758`, both hosted Linux workflows passed:
- [Two-node isolated LAB](https://github.com/zarkmakerburg/BAFT/actions/runs/39992929862), with services preserved on **2053 and 24443**, blocked EX→IR TCP/22, process restart and rejected conflicting service port 18443.
- [Genuine v0.3.7 EX/IR upgrade LAB](https://github.com/zarkmakerburg/BAFT/actions/runs/39992929849), with original pairing secrets/configs preserved and both service-port scenarios accepted.
- [v0.3.7 BCC upgrade/rollback](https://github.com/zarkmakerburg/BAFT/actions/runs/39992929851), [Public Ingress LAB](https://github.com/zarkmakerburg/BAFT/actions/runs/39992929836), [BCC UI](https://github.com/zarkmakerburg/BAFT/actions/runs/39992929959) and static analysis also passed. The **main CI** was queued at observation.

On subsequent integrated QA SHA `fe245f161d50821644ace410b78c2c7b3a750e6b`, independent Mac `go test -race` on Agent/Ingress Ownership and Gamma release acceptance plus `go vet` passed (exit 0); hosted P0 ingress, two-node, v0.3.7 upgrade and rollback, and static analysis succeeded, while main CI was queued and BCC UI in progress at last verification. See [HQ evidence](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6370860173).

## Owner-authorized LAB-only environmental policy

**The owner accepts representative disposable LAB results as the final environment-level evidence.** No physical host, production SSH access or production deployment is a separate release prerequisite. The **functional contract is still mandatory**: reviewed BCC request and plan hash -> signed Agent Apply with durable authoritative reservation -> real IR TCP listener/firewall and end-to-end data -> verified signed receipt -> unchanged Wizard/client config port; plus foreign/stopped-service ownership, fail-closed uncertainty, interruption/rollback, and restart recovery. The existing isolated pairing and v0.3.7 upgrade LABs do **not** prove this BCC-to-Agent vertical slice; until it passes, P0 #269 remains OPEN / NO-GO.

## P0-A through P0-F release acceptance matrix

| P0 | Criterion | State | Still required |
|---|---|---|---|
| A | Install/pair and SSH denial with existing services safe | PARTIAL: Linux namespace pairing, SSH-port denial and real data verified | Representative disposable Linux multi-node/process installer and repair lifecycle, TCP continuity, and service restart/crash evidence |
| B | Safe tunnel lifecycle, crash and reboot recovery with authoritative resource ownership | BLOCKED | Disposable Linux service/process crash, restart/reboot-equivalent recovery, durable ownership and kernel listener/firewall inventory acceptance |
| C | v0.3.7 to candidate upgrade and rollback | PARTIAL: EX/IR binary continuity and BCC state rollback in Linux LAB passed | Disposable Linux installer/systemd-style service migration, failed-upgrade rollback, preserved active sockets where promised, and restart recovery |
| D | Encrypted backup/restore and trusted secret handling | PARTIAL: restore/lock/wrong-key drills passed; QA key-file symlink/permission regression race passed | Stopped-service encrypted restore and independent disposable Linux node/process recovery, with key/file permissions, wrong-key atomicity and redacted logs |
| E | Port collision refusal, SSH protection, multiple protocols/families | PARTIAL: isolated Linux 2053/24443 preserved, 18443 rejected; other ingress LAB passed | Foreign active and persistently reserved ownership across IPv4/IPv6, UDP where supported, collision/uncertainty refusal and disposable Linux network/process failure scenarios |
| F | Exact release SHA provenance, CI and release acceptance | PARTIAL | Final integrated SHA all workflows green, remaining P0 acceptance and authorized HQ release decision |

## Reproduction and operating boundaries

From the repository root, run **only in isolated disposable LAB environments**, not production:

```sh
go test -race ./tests/release -run '^TestGamma' -count=1 -timeout=6m
bash -n tests/e2e/gamma_two_node_namespace.sh
bash -n tests/e2e/gamma_v037_tunnel_upgrade_namespace.sh
# Dedicated Linux namespace host as root, with disposable binaries only:
sudo env GAMMA_EXISTING_PORT=24443 bash tests/e2e/gamma_two_node_namespace.sh /path/to/baft-bin
```

A Linux network namespace is **not a physical host** and proves neither independent server reachability nor systemd/whole-host rollback. Temporary generated secrets stay local. Keep integrations in Draft; **NO-GO**, with no merge/tag/release/deploy without HQ authorization.

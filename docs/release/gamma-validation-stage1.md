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
Exact Gamma SHA `4251eef3a775c5ad9e8f78b6b9ced0dc811e2a14`
passed `bash -n` (both scripts),
`go test ./tests/release -run '^TestGammaV037UpgradeWorkflowSyntaxAndCommands

| P0 | Operational criterion | Gamma status | Required next proof |
|---|---|---|---|
| A | Two independent Linux nodes: install, pair, EX→IR SSH blocked, reinstallation, preservation of live tunnels | PARTIAL: isolated Linux namespace pairing, port-22 filtering and service preservation PASS; real two-host installation remains BLOCKED | Disposable two-node Linux LAB and then approved real-host acceptance; prove **both** install/repair and traffic continuity |
| B | Create/start/stop/restart/delete, reboot/crash, cross-process ownership and safe resource reconciliation | BLOCKED | End-to-end process/service crash and reboot with authoritative nft/systemd inventory, existing-service hashes |
| C | v0.3.x→v0.4.0 upgrade and compatible downgrade, failed-upgrade rollback | PARTIAL: genuine v0.3.7 BCC migration/rollback plus stopped EX/IR v0.3.7→candidate binary recovery with pairing/config/key preservation PASS in isolated LAB | Still requires installer/systemd upgrade, active socket continuity and physical-host recovery; no unsafe in-place downgrade |
| D | Encrypted backup, same/second-node restore, key permissions and secret-free logs | PARTIAL: real CLI Mac LAB with wrong-key/held-lock failures, stopped-state restore and isolated fresh-directory recovery passed | Still requires real stopped-service Linux drill, separate physical node, key handling and artifact/log review |
| E | TCP/UDP, IPv4/IPv6 when admitted, network loss, nftables, port conflicts, SSH/service preservation | PARTIAL: Linux nft and one-port preservation PASS earlier; new dynamic-port matrix added with Linux CI pending | Exact-SHA 2053/24443/18443 dynamic port jobs, then real host resource ownership, UDP/IPv6 and network failure acceptance |
| F | Exact candidate race/vet/CI plus all release gates, provenance/signatures | PARTIAL | Exact candidate hosted workflows green; Gamma local scoped race green; final integrated SHA and operational P0 gates still absent |

## Reproduction and operating boundaries

Run from repository root. Use only private disposable LAB nodes, no production host:

```sh
E2E_MIB=1 E2E_CONNS=2 bash tests/e2e/pair_and_run.sh
# root-only disposable Linux GitHub runner (not macOS/production):
sudo bash tests/e2e/gamma_two_node_namespace.sh /path/to/compiled/baft-binaries
go test -race ./tests/release -run '^TestGammaRejectedPairCodeDoesNotCreateIRState$' -count=1 -v
go test -race ./cmd/baft-pair ./tests/release ./internal/bcc -run 'TestGeneratedConfigsLoadAndPinEachOther|TestEncryptedBackupFreshRestoreFullStateAndAudit|TestRestoreFaultInjectionLeavesStateAndAuditUnchanged' -count=1
```

Secrets are generated only in temporary directories, not uploaded or logged. A macOS loopback test does not simulate the Linux firewall, network namespaces, physical routes or loss of EX→IR SSH. Keep PR #270 Draft, #269 OPEN, and do not merge/tag/release/deploy until separate HQ authorization and accepted final gates.
 -count=1`,
`go vet ./tests/release` and `git diff --check` on Mac (exit 0).
Hosted Linux on this new port matrix SHA was **queued** at observation;
do not promote the new `24443` or collision claims to PASS until the
exact Linux job succeeds. Earlier Linux success only established `2053`
on older SHA. No physical host or existing Imperial Crown service touched.

## P0-A through P0-F release acceptance matrix

| P0 | Operational criterion | Gamma status | Required next proof |
|---|---|---|---|
| A | Two independent Linux nodes: install, pair, EX→IR SSH blocked, reinstallation, preservation of live tunnels | PARTIAL: isolated Linux namespace pairing, port-22 filtering and service preservation PASS; real two-host installation remains BLOCKED | Disposable two-node Linux LAB and then approved real-host acceptance; prove **both** install/repair and traffic continuity |
| B | Create/start/stop/restart/delete, reboot/crash, cross-process ownership and safe resource reconciliation | BLOCKED | End-to-end process/service crash and reboot with authoritative nft/systemd inventory, existing-service hashes |
| C | v0.3.x→v0.4.0 upgrade and compatible downgrade, failed-upgrade rollback | PARTIAL: genuine v0.3.7 BCC migration/rollback plus stopped EX/IR v0.3.7→candidate binary recovery with pairing/config/key preservation PASS in isolated LAB | Still requires installer/systemd upgrade, active socket continuity and physical-host recovery; no unsafe in-place downgrade |
| D | Encrypted backup, same/second-node restore, key permissions and secret-free logs | PARTIAL: real CLI Mac LAB with wrong-key/held-lock failures, stopped-state restore and isolated fresh-directory recovery passed | Still requires real stopped-service Linux drill, separate physical node, key handling and artifact/log review |
| E | TCP/UDP, IPv4/IPv6 when admitted, network loss, nftables, port conflicts, SSH/service preservation | PARTIAL: actual Linux nft and TCP namespace check PASS; remaining protocol and host gates OPEN | Real Linux kernel ingress LAB exists in #270; no final physical-host/network reachability or complete protocol matrix |
| F | Exact candidate race/vet/CI plus all release gates, provenance/signatures | PARTIAL | Exact candidate hosted workflows green; Gamma local scoped race green; final integrated SHA and operational P0 gates still absent |

## Reproduction and operating boundaries

Run from repository root. Use only private disposable LAB nodes, no production host:

```sh
E2E_MIB=1 E2E_CONNS=2 bash tests/e2e/pair_and_run.sh
# root-only disposable Linux GitHub runner (not macOS/production):
sudo bash tests/e2e/gamma_two_node_namespace.sh /path/to/compiled/baft-binaries
go test -race ./tests/release -run '^TestGammaRejectedPairCodeDoesNotCreateIRState$' -count=1 -v
go test -race ./cmd/baft-pair ./tests/release ./internal/bcc -run 'TestGeneratedConfigsLoadAndPinEachOther|TestEncryptedBackupFreshRestoreFullStateAndAudit|TestRestoreFaultInjectionLeavesStateAndAuditUnchanged' -count=1
```

Secrets are generated only in temporary directories, not uploaded or logged. A macOS loopback test does not simulate the Linux firewall, network namespaces, physical routes or loss of EX→IR SSH. Keep PR #270 Draft, #269 OPEN, and do not merge/tag/release/deploy until separate HQ authorization and accepted final gates.

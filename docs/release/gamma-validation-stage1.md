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

## P0-A through P0-F release acceptance matrix

| P0 | Operational criterion | Gamma status | Required next proof |
|---|---|---|---|
| A | Two independent Linux nodes: install, pair, EX→IR SSH blocked, reinstallation, preservation of live tunnels | PARTIAL: isolated Linux namespace pairing, port-22 filtering and service preservation PASS; real two-host installation remains BLOCKED | Disposable two-node Linux LAB and then approved real-host acceptance; prove **both** install/repair and traffic continuity |
| B | Create/start/stop/restart/delete, reboot/crash, cross-process ownership and safe resource reconciliation | BLOCKED | End-to-end process/service crash and reboot with authoritative nft/systemd inventory, existing-service hashes |
| C | v0.3.x→v0.4.0 upgrade and compatible downgrade, failed-upgrade rollback | BLOCKED | Snapshot-first Linux host upgrade drill with verified complete restore |
| D | Encrypted backup, same/second-node restore, key permissions and secret-free logs | PARTIAL | Unit/race tests passed; actual stopped-BCC restore drill and artifact scans still needed |
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

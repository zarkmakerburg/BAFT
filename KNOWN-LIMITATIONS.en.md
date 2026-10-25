# Known Limitations

> Persian: [KNOWN-LIMITATIONS.md](KNOWN-LIMITATIONS.md)

- BAFT remains research software and is not declared production-ready.
- The current Stage-C soak for multi-Flow/slow-receiver and its race sample are green; public benchmarking, a real pilot, and production readiness are still unproven.
- Passing COR-01 is a correctness result on runner/local networking, not a public-network benchmark.
- allocator, TWRL, PADL, and control scheduling are in the data path; the shared multi-Shard budget and the current repeated soak are tested.
- same-process ECRL carrier replacement, same-process epoch fencing, and bounded replay are implemented and covered by Step 5.7 runtime tests.
- durable ECRL session snapshots, process-restart resume, and machine-reboot resume are not implemented or claimed.
- endpoint-pool/relay production paths are incomplete.
- H3 and Worker paths are not enabled in the baseline core.
- formal 60s × 5 benchmarking and profiling are incomplete.
- the rerunnable installer (inspect, plan, apply, verify, roll back config/binaries/systemd service state; doc 33), safe uninstall (doc 34), the secret-safe support bundle (`baft support-bundle`) and A4 transactional certificate rotation are implemented and tested in CI on systemd; they have not been exercised on a large real fleet. Update and Repair in the `baft` menu are still planned (use the installer).
- v0.4.0 public ingress: reboot adoption of a live ingress is not implemented (a rebooted host needs a new signed Apply or Retire), uncertain partial changes need reconciliation, and loopback or namespace LAB results do not certify external reachability. Clients that send nothing first see up to 50 ms added connection latency on an ingress port. There is no in-product Update Center; upgrade by re-running the verified installer. Downgrading across SQLite schema 14/15 or backup schema 3/4 requires a verified pre-upgrade snapshot.
- **TCP only.** BAFT carries TCP streams; it does not carry UDP. VLESS/VMess/Trojan over TCP, WebSocket, gRPC or HTTPUpgrade work (including UDP carried inside them); Hysteria2, TUIC, WireGuard and other raw-UDP protocols do not.
- no large real Iran↔EX pilot has been run; BAFT is new and not field-proven at scale. From at least one Iranian datacenter, foreign TCP on ports other than 22 was cut after about 8.7 KB, so each path must be tested (`sudo baft-net`).
- no universal throughput, undetectability, or guaranteed-connectivity claim has been established.
- the IR public listener does not authenticate end users automatically; if it is exposed beyond loopback, separate service-layer security is required.


## Step 5.7 qualification status

Baseline `29125392566021a286af20ff8f3a4907c28dbee8` passed five independent Recovery Soak attempts, Stage-C soak, and full CI. This is a freeze candidate; formal ACCEPT/FREEZE remains an HQ decision, and the scope limitations below are unchanged.

## Step 5.7 recovery scope
ECRL runtime recovery is limited to **same-process carrier replacement** for an already-live session. Same-process epoch fencing and bounded replay from validated ECRL plans are implemented and tested. Recovery commit uses explicit pre-commit preparation, a canonical plan digest, a two-sided readiness/commit barrier, idempotent commit identity, and separate post-commit failure handling. It does not persist ECRL session snapshots across process restart or machine reboot, and it does not claim process-restart resume. A peer BootID change during recovery fails closed. Subscription Engine work is out of scope. Record Shaping and Morphing behavior are unchanged by Step 5.7.

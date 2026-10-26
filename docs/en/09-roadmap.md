# 09 — Roadmap and Stages A–H

Each Stage has deliverables and an evidence gate.

- **A — contract/spike:** TLS/H2 ADRs, protocol vectors, config schema, full-duplex mTLS Carrier. Completed for its scope.
- **B — secure vertical slice:** fixed Routes, Flow protocol, identity/authorization, revocation, CLI, and COR-01. Completed for its defined scope.
- **C — resources/multi-Flow:** global allocator, receive/replay reservation, DRR, bounded control queue, multi-Flow/multi-Shard, slow-receiver/no-OOM gates. In progress.
- **D — precise resume:** session/boot/epoch fencing, snapshots, replay, tombstones, duplicate-free Carrier replacement.
- **D2 — endpoint/relay:** bounded preconfigured direct/relay endpoint selection after core resume.
- **E — performance:** reproducible benchmarks, profiling, baseline comparison, raw evidence.
- **F — operations:** systemd, idempotent installation, config transactions, drain/stop, certificate rotation, rollback, secret-safe support bundle.
- **G — optional transport/research:** H3 and other bounded experiments behind explicit capability/flags and measured gates.
- **H — real-server pilot:** limited Route, rollback path, real health, multi-window evidence.


## After v0.4.0: BCC-managed shared multiplexing

v0.4.0 ships shared multiplexing as a command-line path only (`baft-pair ex-accept --join` and a live reload; see KNOWN-LIMITATIONS). The next release plans the full design, gated by its own tests and LAB:

- a tunnel-transaction type that adds or removes one tenant on a running EX listener by reload, not restart, with its own backup, health check and rollback that never touches other tenants;
- an ownership model where one instance (marker, generation, unit, metrics claim) carries several tunnels, so discovery, drift, retire and rollback stay exact;
- BCC planning, the Wizard and drift for shared-listener tenants, chosen only when both peers report the capability;
- several tenants per session and the `multiplex-v1` wire capability, negotiated in HELLO;
- live reload for mTLS-only listeners, or an explicit refusal;
- a two-host LAB: tenants joined and removed while others stream, crash of one tenant, reboot.

The project does not promise resume across process restart unless a future persistence design explicitly adds and tests that property.

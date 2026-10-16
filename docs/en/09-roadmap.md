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

The project does not promise resume across process restart unless a future persistence design explicitly adds and tests that property.

## Approved post-v0.3.2 expansion (2026-10-16)

[v0.3.2](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.3.2) is publicly published with independently verified signatures; that is NOT live IR↔EX field/deployment acceptance. Full acceptance and dependencies: [PLAN.en.md](../../PLAN.en.md), [roadmap #199](https://github.com/zarkmakerburg/BAFT/issues/199), and [HQ master addendum](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6223076976).

- **P0:** evidence-fresh Link Test/Network Doctor; bounded failover across multiple addresses of the **same** authenticated peer; independently prove ECRL preservation of an EXISTING TCP flow, not only reconnect with fresh traffic.
- **P1:** guided/manual BCC/Aurora Hybrid transport choice showing real crypto/capabilities, per-tunnel concurrent connections and aggregate bandwidth limits, measured Quiet/Balanced/Boost profiles.
- **P2:** isolated KCP/QUIC/UDP/WireGuard/SOCKS5 and raw-packet research under threat-model/privilege review; reproducible Stage E and R-003 comparisons. No field effectiveness claims without evidence.

Multi-Address is NOT Multi-Exit across separately authenticated foreign nodes. Protect ECRL, replay memory bounds, sibling isolation and existing BCC fallback. No new transport activation, Merge, Release or Deploy without explicit HQ approval and exact-source gates.


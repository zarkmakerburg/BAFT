# Development Plan

> Persian: [PLAN.md](PLAN.md)

## Stage A
Completed: repository contract, Go pin, BAFT/1 codec/vectors, strict configuration, test PKI, full-duplex H2+mTLS/cancellation, independent Shard transports, and pinned-toolchain CI.

## Stage B
Completed for the defined vertical slice: new-session handshake, authorized Routes, Flow framing, half-close, certificate negative tests, duplicate handling, active revocation, fixed RESET codes, 1 GiB COR-01, strict YAML/dependency lock, and real IR/EX CLI path.

## Stage C

Completed/integrated:
- [x] global data allocator
- [x] independent receive/replay pools
- [x] per-Flow reservation
- [x] replay release on ACK
- [x] credit/memory backpressure
- [x] byte-based DRR in DATA path
- [x] bounded control queue
- [x] control priority with finite anti-starvation burst
- [x] concurrent multi-Flow integration test

Remaining:
- [x] stabilize slow-receiver liveness gate with TWRL
- [ ] longer slow-receiver/soak tests
- [x] multi-Shard shared-budget exhaustion/reuse gate
- [x] privacy-bounded conservation metrics
- [ ] close Stage C after the independent Stage-C soak workflow passes without OOM/race/deadlock

## Stage D
Boot/session/epoch fencing, snapshots, replay, tombstones, duplicate-free Carrier replacement, state-machine fuzz, and recovery correctness gates.

## D2/E/F/G/H
Endpoint/relay work, reproducible performance benchmarking, operations/packaging, bounded research transports, and real-server pilot work remain future stages.

## R3.1 current review candidate

Probabilistic normal/Laplace padding, tunable jitter, pinned Noise runtime wiring and pre-authentication HTML fallback are implemented. See [R3.1 guide](docs/en/21-stealth-pro.md) and [Hoosha report](reports/HOOSHA-R3.1.md). Full local tests/race/vet and ECRL differential pass. Stage D remains paused. Fresh-VM provisioning, jitter performance acceptance and real-path detectability validation remain open; v0.2-Pro is not released.

## Post-v0.3.2 roadmap expansion — Arange-tun comparative review (2026-10-16)

**Public release:** [v0.3.2 is published and independently signed/verified](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.3.2); this does **not** certify an Iran↔Germany live rollout or deferred native field tests. The [HQ master addendum](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6223076976) and [product epic #199](https://github.com/zarkmakerburg/BAFT/issues/199) are authoritative for prioritization. This section extends, rather than reinterprets, earlier historical stages.

### P0 — correctness-first network intelligence
- [ ] Extend existing Path Probe/Network Doctor (#145/#146/#147): TCP/UDP reachability, RTT, jitter, loss, goodput, resource impact, timestamps, freshness, uncertainty and explainable recommendations. UNKNOWN when unobserved/stale; no unsupported filtering label or automatic switching.
- [ ] Bounded same-peer multi-address failover: endpoint ≠ authenticated peer identity, controlled retries/hysteresis/cooldown, audit and rollback. Fault tests for blackholes, flaps, handshake failure, stale generations, leaks and sibling tunnel isolation.
- [ ] Separate acceptance for reconnect vs byte-perfect continuation of the PRE-EXISTING TCP flow through ECRL (ACK/WINDOW, FIN, bounded replay, multi-flow). No process-restart/reboot resume claims.

### P1 — guided UX and resource controls
- [ ] BCC/Aurora Hybrid guided/manual transport selection with BOTH peers' actual capabilities, accurate encryption and metadata disclosure, measured path inputs, explicit config preview/approval, audit/rollback and accessible FA/EN RTL/LTR Dark/Light/mobile parity.
- [ ] Per-tunnel concurrent connection and aggregate bandwidth limits with independent budgets, fairness, bounded memory, control-plane liveness, transactional change and rollback tests.
- [ ] Evidence-based Quiet/Balanced/Boost profiles (#199): safe limits, before/after benchmarks and fallback; no silent transport/cryptography change.

### P2 — optional research, not release blockers
- [ ] Lab feasibility for KCP/QUIC/UDP, WireGuard/AmneziaWG and SOCKS5 as isolated adapters, not implied BAFT/1 equivalents; UDP reachability is a prerequisite for UDP carriers.
- [ ] Root/raw-socket/ICMP/packet/spoof research under explicit security/capability review and separate HQ authorization; no 'undetectable' claims.
- [ ] Stage E comparative 60s × 5 benchmarks vs direct TCP and alternative tools on equal paths/load: goodput, tail latency, loss, existing-flow survival, CPU/memory, profiler and raw evidence.

**Topology boundary:** Multi-Address reaches the SAME authenticated server; Multi-Exit spans independent EX node identities and routes. No unsupervised multi-exit pooling/switch, merge, release, deployment or live VPS changes. Isolated draft PRs, exact SHA/CI and HQ #67 evidence are required.


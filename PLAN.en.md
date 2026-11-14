# Development Plan

> Persian: [PLAN.md](PLAN.md)

## Parallel UX-0 — non-technical first-time user priority

Detailed roadmap and acceptance gates: [UX-0 roadmap](docs/en/09-roadmap.md#parallel-ux-0-track--first-time-non-technical-users-p0).

This track takes priority before expanding complex features to general users: remove the empty-wizard dead end, make sign-in and first use clear, guide the first tunnel, use plain language, provide actionable states and understandable recovery, and validate real mobile/RTL/LTR browser behavior. The initial empty-state navigation and removal of the pre-auth tunnel-creation CTA are under review in the UX PR; they are not accepted until CI and browser evidence pass.

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
- [x] repeated real-path slow-receiver/Stage-C soak in CI — [run 43472903062](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903062) PASS (the recorded job ran for about 2m 11s; this is a bounded CI soak, not a long-duration endurance test)
- [x] multi-Shard shared-budget exhaustion/reuse gate
- [x] privacy-bounded conservation metrics
- [ ] close Stage C after reviewing the independent soak evidence and recording an explicit acceptance decision; the CI workflow has passed, but the owner/HQ acceptance gate remains open

## Stage D
Boot/session/epoch fencing, snapshots, replay, tombstones, duplicate-free Carrier replacement, state-machine fuzz, and recovery correctness gates.

## D2/E/F/G/H
Endpoint/relay work, reproducible performance benchmarking, operations/packaging, bounded research transports, and real-server pilot work remain future stages.

## R3.1 current review candidate

Probabilistic normal/Laplace padding, tunable jitter, pinned Noise runtime wiring and pre-authentication HTML fallback are implemented. See [R3.1 guide](docs/en/21-stealth-pro.md) and [Hoosha report](reports/HOOSHA-R3.1.md). Full local tests/race/vet and ECRL differential pass. Stage D remains paused. Fresh-VM provisioning, jitter performance acceptance and real-path detectability validation remain open; v0.2-Pro is not released.

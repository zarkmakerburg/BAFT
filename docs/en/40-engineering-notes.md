# 40 — Engineering notes

This page keeps the engineering material that used to be on the front page (`README.md`). The front page is now written for operators; this page is for reviewers and contributors. Nothing here was removed, only moved.

Persian: [../fa/40-engineering-notes.md](../fa/40-engineering-notes.md)

## Name and design rule

**Bounded · Authenticated · Fail-safe · Transactional**

BAFT is research software for securely relaying authenticated TCP byte streams between two operator-controlled agents. In the baseline architecture the **IR** agent initiates the Carrier connection to **EX**, while application data remains bidirectional. BAFT does not replace the target service, Xray, or the application protocol; it moves TCP bytes only through preconfigured, explicitly authorized Routes.

The repository is developed from content Blueprint v1.4 and Implementation Master Prompt v1.2 dated 2026-09-27. The historical Blueprint filename contains `v1.0`, but the normative content version is **1.4**.

Internal status badges previously shown on the Persian front page: `ECRL Runtime`, `RTL Safe`, `Logo Preserved`. They describe internal engineering gates (ECRL recovery wired into the runtime, the Persian RTL documentation test, and the rule that the BAFT logo is independent of the BCC theme), not user features.

## Development stages

BAFT is not declared production-ready.

- **Stage A:** protocol contracts, parser, configuration, test PKI, and the HTTP/2+mTLS Carrier spike are implemented and tested.
- **Stage B:** the secure TCP → BAFT → TCP vertical slice, peer authorization, Routes, Flow semantics, active revocation, CLI, and the 1 GiB COR-01 correctness gate are implemented.
- **Stage C:** global memory allocation, backpressure, byte-based DRR, bounded control scheduling, and multi-Flow testing are being stabilized.
- **Stage D and later:** resume, epoch fencing, replay, tombstones, formal benchmarking, operations, research gates, and real-server pilot work are not complete.

See [STATUS.en.md](../../STATUS.en.md), [PLAN.en.md](../../PLAN.en.md), and [KNOWN-LIMITATIONS.en.md](../../KNOWN-LIMITATIONS.en.md).

## What problem does BAFT solve?

A local service on IR can be mapped to one fixed, pre-authorized service on EX without allowing a peer to provide an arbitrary destination:

```text
local application
      │
      ▼
127.0.0.1:1443 on IR
      │
      ▼
BAFT IR (dialer)
      │
      │ HTTP/2 + TLS 1.3 + mTLS
      │ independent Shards
      ▼
BAFT EX (listener)
      │
      ▼
fixed authorized Route: 127.0.0.1:2443
      │
      ▼
target service
```

OPEN carries a `route_id`, not a target host supplied by the remote peer.

## Core terminology

| Term | Meaning |
|---|---|
| Node | BAFT process acting as `dialer` or `listener` |
| IR | baseline Carrier initiator |
| EX | baseline Carrier listener |
| Carrier | authenticated bidirectional byte stream that transports BAFT frames |
| Shard | independent Carrier with a bounded subset of Flows and its own scheduling |
| Session | in-memory state associated with one Shard |
| Flow | one bidirectional application TCP connection |
| Route | pre-authorized mapping between a local listener and a fixed target |
| ACK | contiguous byte acceptance by BAFT, not proof of final application processing |
| WINDOW / Credit | absolute maximum offset a sender may transmit |
| Replay | unacknowledged bytes retained for recovery semantics |
| Epoch | Carrier generation used by the future resume design |

See [11-glossary.md](11-glossary.md).

## Non-negotiable security properties

- TLS 1.3 minimum and mTLS are mandatory.
- There is no plaintext fallback.
- Certificate chain, hostname, validity time, and EKU verification stay enabled.
- Node identity is derived from a verified URI SAN; HELLO `node_id` alone does not establish trust.
- CA trust and Route authorization are separate.
- The peer cannot supply an arbitrary target destination.
- Frame sizes and types are validated before allocation.
- User payload is not written to logs or support bundles.
- Test and benchmark claims must come from real executions.
- The project does not claim universal undetectability, guaranteed connectivity, or guaranteed public-network throughput.

Read [Security model](03-security-model.md).

## Baseline architecture

```text
IR Node
  ├─ Shard 0 ─ TCP/TLS ─ H2 POST ─┐
  ├─ Shard 1 ─ TCP/TLS ─ H2 POST ─┤
  ├─ Shard 2 ─ TCP/TLS ─ H2 POST ─┤──► EX Node
  └─ Shard 3 ─ TCP/TLS ─ H2 POST ─┘

Inside a Carrier:
HELLO → HELLO_ACK → READY
                  │
                  ├─ OPEN / OPEN_OK
                  ├─ DATA
                  ├─ ACK
                  ├─ WINDOW
                  ├─ FIN / FIN_ACK
                  └─ RESET
```

Each Shard owns an independent transport in the baseline so multiple Shards are not silently pooled onto one TCP connection.

## Release trust chain

The installer accepts only artifacts that pass the release trust chain:

```text
Pinned Root
    |
    v
Release Key Certificate
    |
    +-- not revoked
    +-- valid time window
    v
Signed Manifest
    |
    +-- version
    +-- exact commit
    +-- artifact size
    +-- SHA-256
    v
Downloaded Binaries
```

If verification fails, the install stops. See [23-p1a-signed-releases.md](23-p1a-signed-releases.md).

## Native tunnel builder

Building a tunnel is transactional:

```text
PLAN
  |
  v
VALIDATE
  |
  v
PREPARE EX + PREPARE IR
  |
  v
COMMIT
  |
  v
HEALTH CHECK
  |\
  | \__ failure --> ROLLBACK BOTH SIDES
  |
  +---- success --> ACTIVE
```

The goal is that a failure in the middle never leaves one side half-active. See [27-p1e-tunnel-builder.md](27-p1e-tunnel-builder.md).

## Recovery scope

In the current scope BAFT focuses on recovery inside the same process/session.

Verified: Carrier replacement, epoch fencing, bounded replay, keeping an active Flow, multi-flow continuity, FIN / FIN_ACK recovery, commit validation before an authority change.

Not verified: resume after a process restart, resume after a machine reboot, durable session snapshots.

## Innovation and development method

Important BAFT decisions are evaluated on two explicit tracks:

1. **Baseline:** the smallest secure, bounded, testable mechanism with a rollback path.
2. **Research:** a conceptually distinct mechanism aimed at a documented gap, with falsifiable experiments and explicit failure criteria.

The 10x/100x rule is used as design pressure, not as a numeric claim. Research hypotheses, engineering results, and legal novelty/patentability are recorded as separate states.

Current examples:

- [TWRL — Tri-Watermark Receive Ledger](13-stage-c-twrl.md): engineering result supported in CI.
- [PADL — Pressure-Aged Deficit Leasing](14-stage-c-padl.md): research hypothesis under evaluation.

See [Innovation methodology](12-innovation-method.md).

## CI and engineering evidence

The main gates include:

- unit / integration tests
- race detector
- `go vet`
- protocol fuzz smoke
- installer E2E
- signed-release installer E2E
- Agent enrollment E2E
- SSH bootstrap against a real sshd
- Launch-1 end-to-end
- rollback tests
- multi-flow / slow-receiver soak
- recovery-specific soak

Passing these gates proves correctness within the tested scope; on its own it is not a public benchmark or a declaration of production readiness. See [08-testing-and-ci.md](08-testing-and-ci.md) and [TEST-RESULTS.en.md](../../TEST-RESULTS.en.md).

## Developer quick start

```bash
git clone https://github.com/zarkmakerburg/baft.git
cd baft
go build ./cmd/baft

go test ./...
go test -race ./...
go vet ./...
```

The repository currently targets Go 1.27.1 via `go.mod`.

Validate configuration:

```bash
./baft config validate --file configs/example-ir.yaml
./baft config validate --file configs/example-ex.yaml
```

After creating appropriate PKI material and adjusting addresses:

```bash
# EX
./baft run --file /etc/baft/ex.yaml

# IR
./baft run --file /etc/baft/ir.yaml
```

## Repository layout

```text
cmd/                 command-line binaries
internal/            runtime, protocol, BCC, agent, recovery
configs/             example configurations
tests/               integration, correctness and E2E tests
scripts/             release/build/test automation
release/             release trust material
docs/fa/             Persian documentation
docs/en/             English documentation
reports/             engineering and research reports
.github/workflows/   CI, soak, release and security workflows
```

## What BAFT is not

At the current stage BAFT is not a general-purpose VPN, an arbitrary-destination reverse proxy, a persistence layer for user payloads, or a finished recovery system. H3, relay, Worker, and real-path research remain outside the default core path until their later gates are explicitly implemented and tested.

## Technical documentation map

See the [documentation index](README.md) for the full list (01–37).

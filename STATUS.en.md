# Project Status

> Persian: [STATUS.md](STATUS.md)
> Report date: 2026-11-14

## Current snapshot — exact main SHA

- Repository: `zarkmakerburg/BAFT`
- Branch: `main` (protected)
- Exact HEAD: [`d4348c2ace51e56469b10aee2806f3ad4869c8fa`](https://github.com/zarkmakerburg/BAFT/commit/d4348c2ace51e56469b10aee2806f3ad4869c8fa)
- Latest published release: [v0.5.0](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.5.0), published 2026-11-02.
- The current main commit contains post-v0.5.0 work listed under `[Unreleased]` in `CHANGELOG.md`; v0.5.1 is not a published release.

### Exact-SHA hosted evidence

The following nine workflow runs are completed with `success` on the exact HEAD above:

- [ci](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903251): SUCCESS
- [operational recovery](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903271): SUCCESS
- [operational ingress v1](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903102): SUCCESS
- [shared operational ingress](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903312): SUCCESS
- [Two-host LAB](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903017): SUCCESS
- [stagec-soak](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903062): SUCCESS
- [advanced-adversary](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903026): SUCCESS
- [Guided tunnel flow E2E](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903373): SUCCESS
- [Step 5.7 recovery soak](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903463): SUCCESS

These are hosted GitHub Actions results. The Two-host LAB workflow is not a real-world field pilot. The repository has additional workflow definitions; this list does not assert that every workflow has run or passed on this SHA. No production-readiness or scaled field-pilot claim follows from these results.

### Current product gates

- **P0 / Issue #269:** OPEN. Full acceptance for public ingress, safe shared transport, wizard simplification, rollback, recovery and the end-to-end acceptance matrix remains incomplete.
- **Ingress integration candidate / PR #278:** OPEN and Draft, head `231bbf622f40bbd2dce96b2a54cd18271131fd39`. The latest adversarial review recorded unresolved P0 fencing gaps: listener Apply/Retire/rollback effects are not serialized under the held lease lock; numeric-port ownership is not freshly checked immediately before listener effects; restart Remove has a guard-close race. Details: [HQ board review](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6861034383). These findings concern the candidate branch and must not be represented as a merged-main defect without a separate main-branch verification.
- **Stage C:** the dedicated soak workflow passed on this exact main SHA. The plan's formal Stage-C closure item remains open pending explicit acceptance.
- **Stage D / ECRL:** only same-process live-session carrier replacement is implemented and tested. Process-restart resume, machine-reboot resume and durable session snapshots are not implemented.
- **Release:** v0.5.0 is the latest public release. The main-head changes under `[Unreleased]` are not a published or signed v0.5.1 release.

## Historical engineering detail

The sections below preserve selected prior qualification evidence and design history; they are not a complete archive. The Step 5.7 qualification baseline is reproduced immediately below. Its date, SHA and workflow IDs are historical and must not be read as current-main evidence.

## P0-I — Step 5.7 final qualification (2026-10-03)

Exact qualification baseline on `main`:

- SHA: `29125392566021a286af20ff8f3a4907c28dbee8`
- PR #60: merged; logical-session cancellation-cause instrumentation retained and the temporary diagnostic workflow removed.
- Full CI on this SHA: **PASS**, including unit/integration, race detector, vet, and protocol fuzz smoke.
- Stage-C soak on this SHA: **PASS**.
- Step 5.7 Recovery Soak: **PASS in 5 independent attempts on the same SHA**.
- Each Recovery Soak covers unresolved Class-A exact transactions, hard Class-A live-data recut, Class-B fresh epoch, ambiguous replay contract, Topology × ECRL authority isolation, HTTP handler lifecycle, P0-C lifetime/race, exact-rebind lifecycle, FD/resource lifetime, multi-flow replacement, distributed finalization/replay/FIN matrix, and concurrent Flow churn + race.

Engineering result: the recent rare Class-A evidence gap did not reproduce on this baseline and current qualification is green. This records a **Step 5.7 freeze candidate**; formal ACCEPT/FREEZE remains an HQ governance decision.

Scope remains limited to **same-process live-session carrier replacement**. This historical qualification does not establish process-restart resume, machine-reboot resume, durable ECRL session snapshots, or production readiness.

## Stage B evidence

Implemented/tested behavior includes new-session HELLO/HELLO_ACK/READY, fixed authorized Routes, Flow control frames, half-close, fixed RESET codes, mTLS identity/authorization, active peer revocation, strict YAML configuration, production CLI path, and end-to-end TCP→BAFT/H2+mTLS→TCP transfer.

Latest COR-01 run `36338439622` on `b1ddb445...` passed with Go 1.27.1, 1,073,741,824 bytes each direction, and SHA-256 `1efd9d3aab21f9e312a2a0b5a6886b2a640c810ecb1fbe33f64614b26cfb27e3` in about 10.56 seconds on local/runner networking.

## Stage C

The global receive/replay allocator, replay release on ACK, memory/credit backpressure, byte-based DRR, bounded control scheduling, and multi-Flow integration work are in the data path.

The historical slow-receiver failure triggered the TWRL redesign. TWRL now separates Accepted, Delivered, and Credit watermarks and passes the slow-receiver gate, race detector, and 1 GiB COR-01.

The active DATA scheduler is now PADL (Pressure-Aged Deficit Leasing), with classic DRR retained as a comparison baseline. Consolidated CI run `36341912070` and COR-01 run `36341912044` both pass on the PADL path.

Shared multi-Shard allocator exhaustion/reuse and conservation snapshots pass in CI run `36341666646`. Privacy-bounded conservation metrics pass in CI run `36341810504`.

### Stage-C soak — current status

The dedicated `stagec-soak` workflow repeats real multi-Flow and slow-receiver transfers 25 times and runs 5 rounds under the race detector.

Evidence:
- historical failure: run `36342169299` — **FAIL** with `TestConcurrentMultiFlowTransfer: unexpected EOF`.
- historical pass after the fixes: run `36342627897` — **PASS**.
- code-head evidence before the documentation commit: run `36519987991` on `6fcf41631dc963af6f9c124f245a3ce7a47bc2fe` — **PASS**, including the repeated soak and race sample.

The current Stage C gate is therefore green, but this is not a public benchmark, a real pilot, or a production-readiness claim.

## Not complete

Same-process carrier replacement, same-process epoch fencing, and bounded replay are implemented and tested in Step 5.7. Still incomplete are process-restart resume, machine-reboot resume, durable ECRL session snapshots/tombstones across restart, endpoint/relay production paths, the formal benchmark campaign, final operations/package work, and real-path pilot testing.

## R3.1 current review candidate

Probabilistic normal/Laplace padding, tunable jitter, pinned Noise runtime wiring and pre-authentication HTML fallback were recorded for the historical R3.1 review candidate. See [R3.1 guide](docs/en/21-stealth-pro.md) and [Hoosha report](reports/HOOSHA-R3.1.md). The statement that Stage D was paused describes that historical point; current ECRL status is the Step 5.7 release-branch section below. Fresh-VM provisioning and the broader release/operations validation remain open; v0.2-Pro is not released.


## Release branch Steps 5.1–5.7 status (2026-09-28)

On `release-v1-goldapp`, Steps 5.1 through 5.6 have passing CI regression gates for signed/idempotent telemetry, route monitoring, finance reporting, BCC hardening, backup/audit anchoring, and persistent telemetry reliability.

Step 5.7 integrates ECRL with live Runtime sessions for **same-process carrier replacement only**. Tested scope includes active-flow continuity, same-process epoch fencing, bounded replay, peer BootID fail-closed behavior, competing candidates, FIN/FIN_ACK recovery, multi-flow recovery, six-route identity isolation, and telemetry/finance continuity. Recovery commit safety uses pre-commit replay/materialization validation, a canonical plan digest, an explicit two-sided PREPARED/readiness/COMMIT barrier, committed/uncommitted results, idempotent COMMIT/COMMIT_ACK handling, old-epoch fencing after authority publication, forward recovery to the next epoch after post-commit carrier failure, and separate post-commit failure accounting.

This does **not** make BAFT production-ready. Process-restart resume, machine-reboot resume, and durable ECRL session snapshots are not implemented or claimed. Subscription Engine work is not part of Step 5.7.

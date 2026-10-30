# BAFT P0 #269 Signed Ingress LAB Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove that authenticated signed Agent jobs can safely prepare, apply, verify and retire a real public listener and exact-owned nft rule in an isolated Linux LAB, with no production mutations.

**Architecture:** Retain the data-only Prepare/Cancel store and add explicit signed Apply/Retire actions. A disabled-by-default Agent LAB adapter resolves canonical stored proof before invoking `ingressownership.NFTLabTransaction`; all live claims require independent listener/kernel evidence. Linux namespace integration and fault injection gate P0 closure.

**Tech Stack:** Go, existing `agentjob`, `agent`, `ingressmanifest`, `ingressownership`, Linux nftables network namespaces, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-23-p0-269-signed-ingress-lab-design.md`

## Global Constraints

- No production deployment, release, or mutation of existing host firewall chains.
- Default Agent configuration must fail closed on live mutation requests.
- Do not adopt listeners or nft rules based solely on journal records or ACKs.
- All mutations require exact owner, generation, canonical manifest digest and verified signed job.
- Preserve existing Prepare and Cancel semantics and all existing service ports.
- Use RED → GREEN → refactor, targeted race tests, isolated Linux LAB, and exact-head CI before claiming completion.

## Review Focus

- Replayed signed Apply after cancellation must never resurrect a tombstoned generation (Task 2).
- Concurrent distinct owners on the same public port must never mutate each other's resources (Task 3).
- Cancelled context during firewall install must not leak an adopted rule or claim success (Task 3).
- Restart with a verified journal phase but missing kernel rule must remain unverified (Task 4).
- A compromised or misconfigured LAB adapter must not operate on a host/default nft chain (Task 4).

---

### Task 1: Signed Apply and Retire job contracts

**Files:**
- Modify: `internal/agentjob/agentjob.go`
- Test: `internal/agentjob/public_ingress_validation_test.go`

**Interfaces:**
- Produces: `ActionPublicIngressApply`, `ActionPublicIngressRetire` with canonical required params matching Prepare, optional `instance_id`, and existing semantic port/target constraints.
- Consumes: existing `Job.Validate`, signing and verifier APIs.

- [ ] **Step 1: Write failing tests** for both action constants, signed round-trip, missing/tampered digest, invalid port, self-forwarding and SSH target; assert `Validate` rejects invalid jobs.
- [ ] **Step 2: Run** `go test ./internal/agentjob -run 'TestPublicIngress' -count=1`; **Expected:** FAIL before implementation.
- [ ] **Step 3: Implement** two allowlisted actions and matching semantic checks in `agentjob.go` without weakening existing actions.
- [ ] **Step 4: Run** `go test ./internal/agentjob -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit** `feat(ingress): define signed apply and retire job contracts`.

### Task 2: Proof-bound, disabled-by-default Agent LAB adapter

**Files:**
- Modify: `internal/agent/tunnel.go` and Agent `Config`/constructor at their existing declarations.
- Create: `internal/agent/public_ingress_apply_lab.go`
- Test: `internal/agent/public_ingress_apply_lab_test.go`

**Interfaces:**
- Consumes: Task 1 action names; `ingressmanifest.Open`, `Store.Inspect`, `ingressmanifest.Preview`; `ingressownership.NFTLabTransaction.Apply(context.Context, ingressmanifest.Spec, ingressmanifest.Proof) error` and `Retire(...) error`.
- Produces: `IngressLABExecutor` interface with `Apply(ctx, spec, proof) error` and `Retire(ctx, spec, proof) error`; Agent injection field explicitly nil by default; proof-bound JSON receipts whose live-verified flag is true only after transaction success.

- [ ] **Step 1: Write failing tests**: disabled Agent rejects both live actions without mutations; wrong digest/instance/node or missing prepared manifest rejected; cancelled generation and replay fail closed; valid prepared proof delegates exactly once to fake executor; signed receipt never claims live ownership on error.
- [ ] **Step 2: Run** `go test ./internal/agent -run 'TestPublicIngressLAB' -count=1`; **Expected:** FAIL.
- [ ] **Step 3: Implement** proof reconstruction from signed params, exact `Store.Inspect` matching, cancellation fencing, nil-executor guard, and dispatch. Never invoke an arbitrary shell command.
- [ ] **Step 4: Run** `go test ./internal/agent -run 'TestPublicIngressLAB|TestSignedIngressPrepare|TestIngressPrepare' -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit** `feat(ingress): gate signed live jobs behind proof-bound LAB adapter`.

### Task 3: Transaction ownership and rollback contract

**Files:**
- Modify only as tests require: `internal/ingressownership/nft_transaction_lab.go`, `journal_lab.go`, `journal_unix.go`.
- Test: `internal/ingressownership/nft_transaction_lab_test.go` and targeted new tests if needed.

**Interfaces:**
- Consumes: `NFTLabTransaction.Apply/Retire` and proof identity from Task 2.
- Produces: exact-owner, concurrency-safe Apply/Retire behavior with bounded rollback and explicit unverified state on ambiguous failure.

- [ ] **Step 1: Write failing regression tests** for two owners racing on same port, cancelled context after install, failure to journal phase after listener bind, duplicate Apply/Retire and stale generation; inspect actual runtime state and mocked nft ownership.
- [ ] **Step 2: Run** `go test ./internal/ingressownership -run 'TestNFTLab' -count=1`; **Expected:** regression FAIL before fix, or document existing coverage as PASS and avoid redundant change.
- [ ] **Step 3: Make only necessary minimal fixes**, preserving exact-handle deletion, bounded cleanup, and no journal-only adoption.
- [ ] **Step 4: Run** `go test -race ./internal/ingressownership -count=3`; **Expected:** PASS.
- [ ] **Step 5: Commit** only if code or test changes were required.

### Task 4: Isolated Linux nft namespace integration and restart gate

**Files:**
- Modify: `.github/workflows/p0-ingress-lab.yml` as needed.
- Create: `tests/integration/public_ingress_signed_lab_test.go` or existing Linux integration package file consistent with repository conventions.
- Modify: `docs/release/v0.4.0-readiness.md` for verified evidence only.

**Interfaces:**
- Consumes: Tasks 1–3 Agent actions and `NFTLabTransaction`, journal recovery and nft backend.
- Produces: repeatable isolated LAB acceptance command, proof of namespace containment, fail-closed restart behavior, and exact-head CI evidence.

- [ ] **Step 1: Write Linux LAB test** for real namespace-owned listener/rule, independent inspection, exact-owner retire, interrupted Apply, duplicate job and restart with missing rule; assert host nft state unchanged. Skip with explicit reason when privileges/nft unavailable.
- [ ] **Step 2: Run Linux LAB** on an isolated Linux runner; **Expected:** FAIL for missing integration before fix, not silently SKIP in required CI.
- [ ] **Step 3: Wire isolated namespace harness**, no generic host nft commands and no production chain reuse; add fault injection and teardown.
- [ ] **Step 4: Run** targeted Go race tests, `go test ./...`, Linux LAB integration and `git diff --check`; **Expected:** all PASS, required Linux tests not skipped.
- [ ] **Step 5: Commit** `test(ingress): verify signed Agent lifecycle in isolated Linux LAB`.

### Task 5: Review, evidence and P0 closure gate

**Files:**
- Modify: `docs/release/v0.4.0-readiness.md` with observed exact-head evidence.
- Update GitHub issues #269 and #67 with links to tested commit and CI run.

**Interfaces:**
- Consumes: Tasks 1–4 outputs and test evidence.
- Produces: audit-ready P0 closure decision, without premature release or deployment.

- [ ] **Step 1: Review entire branch** against spec, signed job trust boundaries, rollback and nft isolation; classify critical/important findings.
- [ ] **Step 2: For any critical/important finding**, write a failing test, fix it, and rerun the relevant suite; **Expected:** RED then GREEN.
- [ ] **Step 3: Verify exact branch HEAD and CI run**; **Expected:** required gates green, Linux LAB tests executed rather than skipped.
- [ ] **Step 4: Record evidence** in #269 and HQ #67 and readiness doc; **Expected:** links resolve to the tested SHA.
- [ ] **Step 5: Close #269 only if all acceptance gates are met**; otherwise retain OPEN with precise blockers. No release/deploy in this task.

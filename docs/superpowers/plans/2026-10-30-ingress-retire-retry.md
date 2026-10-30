# Ingress Retire Receipt Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the first bounded lifecycle repair: a fresh signed Retire can re-prove completed cleanup after receipt delivery failed, without reviving spent jobs or touching a foreign listener.

**Architecture:** Keep concrete leases, protected inventory and the mutation journal authoritative. Add a read-only completed-retirement branch before the existing live/partial Retire path; preserve the separate receipt and ReleaseVerified boundaries. Resume existing regression-first work rather than duplicate it.

**Tech Stack:** Go 1.27.3; Linux sockets, nftables and namespace LAB; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-30-fundamental-repair-design.md`, first engineering milestone and existing-fixes section. This plan covers only the completed-Retire retry part; failed Apply, ordinary-host support and restart availability require subsequent plans.

## Global Constraints

- Do not remove ownership checks, signature verification, replay fencing or unknown-state refusals.
- Do not infer control-plane convergence from local cleanup.
- Preserve an unrelated service on port 2053 and sibling resources.
- Report source, tests, main and published artifacts separately.
- No claim of ordinary-host or two-server acceptance from singleton loopback LAB evidence.

## Review Focus

- A replaced foreign listener after successful cleanup must remain alive; negative kernel case owns this assertion.
- A consumed original job must not mint another lease; spent-job kernel case owns this assertion.
- A stale or changed journal, chain identity or source pin must refuse success; inspect existing fences and exact-identity negative tests before qualification.
- A state save or receipt delivery failure must not imply a released reservation; positive kernel case retains the reconciliation reservation until verified completion.
- Build-only/opt-in skips must never count as kernel acceptance; the required sixteen-case zero-SKIP verdict owns this assertion.

---

### Task 1: Qualify the existing regression-first executor repair

**Files:**
- Test: `tests/e2e/operational/recovery/recovery_test.go`
- Test helper: `tests/e2e/operational/recoverylab/main.go`
- Modify: `internal/ingressownership/operational_executor.go`
- Create: `internal/ingressownership/operational_retired_retry.go`
- Test verdict: `tests/release/oprecovery/verdict.sh`

**Interfaces:**
- Consumes: `CommandV1`, concrete `*ingressauthority.Lease`, persisted `PhaseRetired`, protected installed inventory and pinned nft readback.
- Produces: `completedRetireRetry(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (RetiredIngressEvidence, bool, error)`; bool means this completed-journal branch handled the attempt, never authorization.

- [ ] Step 1: Confirm the existing tests-only commit `48d1a36` and recorded attempt 2 reached retry after actual first retirement: one intended FAIL, fifteen PASS, zero SKIP. Do not recreate already published tests.
- [ ] Step 2: Review local candidate `77bf230` against the spec and all authority/effect boundaries; fix review findings with RED then GREEN tests. Existing implementation was written only after the recorded behavioral RED.
- [ ] Step 3: Run affected race tests and vet; report any failed tests by name. Run the complete Go suite and web tests before final qualification; classify environment failures explicitly.
- [ ] Step 4: Publish the candidate source tree to the existing draft PR #293 with a branch-head lease. Do not merge or release.
- [ ] Step 5: Require the final published head's operational recovery workflow: all sixteen required real-kernel tests PASS, zero SKIP, host cleanup PASS. Inspect failures rather than blindly rerun.

Expected: positive retry reaches completion; foreign listener and spent original job remain refused; existing thirteen recovery cases remain required. This is not BCC HTTPS delivery acceptance.

### Task 2: Record the bounded result and next lifecycle dependency

**Files:**
- Modify: `docs/release/v041-retire-receipt-retry.md`
- Modify: this plan and the fundamental repair spec.

**Interfaces:**
- Consumes: Task 1 final source SHA, workflow conclusions and artifact metadata.
- Produces: a reviewable checkpoint that keeps failed-Apply/BCC convergence and production execution profile open.

- [ ] Step 1: Run fresh-context code review of the executor change and tests; resolve Critical/Important findings before claiming completion.
- [ ] Step 2: Record observed results and limits, including complete-suite failures and skipped local kernel tests; update draft PR description to describe final behavior.
- [ ] Step 3: Verify `git diff --check`, exact remote/local tree identity and final-head workflow results. Commit evidence/docs without claiming a new product release.

## Execution ruling

The user delegated technical choices and explicitly opted to let the agent proceed. Execute inline without another plan-approval round. Preserve existing RED and candidate commits. A fresh-context reviewer checks the final change. The broad repair remains a sequence of subsystem plans; completion of this plan is not completion of the whole program.

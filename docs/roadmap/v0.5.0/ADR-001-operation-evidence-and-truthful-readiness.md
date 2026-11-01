# ADR-001: Operation evidence and truthful readiness

- **Status:** Proposed for v0.5.0 implementation
- **Date:** 2026-10-31
- **Scope:** BCC-issued operations, Agent effects, ingress ownership/journal, user-visible connection/readiness state
- **Source baseline:** main `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`
- **Related evidence:** [Baseline audit](./BASELINE_AUDIT.md), [Issue #269](https://github.com/zarkmakerburg/BAFT/issues/269), [shared-ingress acceptance contract](../../release/v040-shared-ingress-acceptance.md)

## Context

The main source tree already contains a signed-job model, Agent validation, a held ingress admission/lease path, operational enqueue, restart reconciliation, ownership/journal machinery and a shared-ingress acceptance contract. These are implementation components, not proof that a complete operation has been applied and verified.

The key product/security risk is collapsing distinct states into one “active/healthy” indicator. A queued job is not an applied effect; an Agent acknowledgement is not necessarily a reachable service; a listening socket is not proof of correct routing; and stale readiness must not survive an uncertain mutation or failed reconciliation.

This ADR defines the contract that future implementation and tests must enforce. It does not claim the contract is fully implemented today.

## Decision

### 1. Keep operation states distinct

The persisted/API model and UI must not conflate these semantic states:

1. **Planned** — validated proposal only; no mutation requested.
2. **Authorized** — BCC has approved a specific immutable plan and resource scope.
3. **Queued** — a signed, scoped job was durably issued; no execution claim.
4. **Executing** — the Agent accepted the job and is attempting bounded effects.
5. **Applied** — the expected effects are evidenced by the owning subsystem/journal.
6. **Verified** — a post-apply probe proves the required user-visible path at a stated time.
7. **Failed** — an operation reported a definite failure and required cleanup/reconciliation is complete or explicitly pending.
8. **Uncertain / Reconciliation required** — effect may have occurred but outcome cannot safely be proved; this state must fence further conflicting operations.
9. **Retired** — retirement was authorized, owned effects were removed, listener/route state was verified closed, and a terminal receipt is durable.

Existing enum/state names may differ. Map existing models to these meanings rather than creating duplicate state machines without need.

### 2. Make authority immutable and scope-bound

- BCC is the authority for policy, resource scope, reviewed plans and signed job issuance.
- The Agent validates the actual signed envelope against the enrolled/pinned issuer key, target node, action allowlist, expiry/replay rules and canonical plan/resource digests.
- A UI field, caller-supplied boolean, transport driver or success-shaped response must never mint authority.
- Every sensitive effect must independently re-check the relevant live authority/lease and ownership at the effect boundary.
- The authorization binding must cover the exact node, operation ID, generation, listener/firewall/resource identifiers, config/manifest digest and intended action as applicable.
- Lease/guard lifetime must span the protected mutation and its evidence write; uncertainty revokes reusable authority and fences conflicting work.

### 3. Evidence before success

A successful API call, job acknowledgement, open port or process start is insufficient to claim **Verified**. The operation must collect evidence suitable to its claim:

- authenticated, correlated Agent receipt tied to the exact job/operation/generation;
- durable journal/ownership evidence for each mutation;
- a real path-level probe for connectivity claims, with target identity/marker where feasible;
- freshness timestamp and clear stale/unknown behavior;
- explicit failure or uncertainty when receipt delivery, probe, reconciliation or rollback cannot be confirmed.

Do not make the UI display “connected”, “active” or “healthy” merely because an operation is queued or because an old positive result remains cached after a relevant mutation.

### 4. Recovery and rollback are ownership-scoped

- Record intent before irreversible effects and checkpoint each effect/result.
- Make retries idempotent and correlate retries to the same operation and generation.
- After crash, timeout or lost receipt, inspect durable journal and actual host state before deciding whether to retry, retire, roll back or remain uncertain.
- Remove only resources whose ownership and scope are independently proven. A shared listener, unrelated tenant, foreign firewall rule or prior system setting must survive rollback/retirement.
- If ownership or effect state cannot be proven, fail closed and require reconciliation; do not “repair” uncertainty by deleting resources based on names/comments alone.

### 5. Secret and diagnostic policy

Receipts and journal records may include stable IDs, digests, timestamps, typed outcomes and redacted diagnostic reasons. They must not include private keys, enrollment tokens, bearer tokens, full signed secret-bearing payloads or unredacted credentials. Tests must assert redaction for both success and failure paths.

## Required acceptance tests

The following are implementation gates, not claimed results:

| Case | Required result |
|---|---|
| Wrong issuer key, wrong node, expired job, replay, modified plan digest | Rejected before host mutation; no success/readiness evidence |
| Correct job with stale/revoked lease | Rejected at the effect boundary |
| Same request retried after response loss | No duplicate unsafe effect; durable correlation/reconciliation |
| Effect succeeds but receipt is lost | State remains uncertain until reconciliation; no false Verified card |
| Effect partially fails after one mutation | Exact operation-owned rollback or explicit reconciliation-required state |
| Crash after intent / after first effect / before receipt | Deterministic restart reconciliation; conflicting operations fenced while unresolved |
| Probe fails or target marker is wrong | Not Verified; previous positive state cannot mask current failure |
| Retire one tenant using shared listener | Tenant resources removed; owner and unrelated tenants remain functional |
| Ownership evidence is absent or ambiguous | Fail closed; no deletion of foreign/shared resource |
| Logs/receipts include crafted secret values in errors | Secrets redacted in all paths |

Each result must be tied to the exact tested commit SHA and execution environment. Separate unit, integration, Linux-kernel LAB, container/simulation and physical-host evidence.

## Implementation mapping to current source

These are observed paths on the baseline SHA, not a claim that every gate already passes:

- Signed job/action validation: `internal/agentjob/agentjob.go`
- Agent polling/execution: `internal/agent/agent.go`
- Held ingress admission/lease: `internal/agent/ingress_signed_held_admission.go`
- Canonical ingress command mapping: `internal/agent/public_ingress_v1_command.go`
- BCC signing and operational enqueue: `internal/bcc/jobsign.go`, `internal/bcc/ingress_operational_enqueue.go`
- Startup reconciliation: `cmd/baft-agent/ingress_reconcile.go`
- Operational ingress provisioning: `cmd/baft-agent/ingress_provision.go`
- Ownership/firewall/journal/recovery: `internal/ingressownership/`
- UI and browser tests: `web/bcc-app.js`, `web/bcc-operations-v3.js`, `web/bcc-action-wizard.js`, `web/tests/`
- Combined shared-ingress acceptance: `docs/release/v040-shared-ingress-acceptance.md`

## Consequences

- **Positive:** A single state/evidence vocabulary reduces false success, unsafe retries and ambiguous operator decisions.
- **Positive:** It makes existing components composable without granting transport/UI layers independent authority.
- **Cost:** Some workflows will show “uncertain / reconciliation required” instead of optimistic success when proof is missing. That is intentional.
- **Compatibility:** Preserve existing public schemas where possible. Any required schema/state migration must be versioned and tested; do not silently reinterpret old persisted states.
- **Next step:** Map each acceptance case to the existing test name and identify missing tests/failures on a reachable execution environment. Then implement the smallest confirmed P0 defect and run the relevant exact-SHA tests before extending the vertical slice.

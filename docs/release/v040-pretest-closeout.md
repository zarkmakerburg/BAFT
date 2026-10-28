# v0.4.0 pre-test closeout

Date: 2026-10-28  
Scope: finish implementation-only work before the owner-authorized final acceptance tests. This file is a tracking boundary, not evidence that an item is implemented or passed.

## Canonical candidate

- Integration PR: #278, branch `claude/v040-integration`, base `qa/integration-20261023-latest`.
- Observed PR head at this checkpoint: `1824ae460f5e7e9860384ea9055e1f3eaf8b35100fb`.
- PR is open and draft. Do not merge, tag, publish, or deploy before all P0 and release gates pass.
- The readiness document's historical checkpoint header contains an older SHA; use live PR metadata and exact-SHA workflow evidence as canonical. A previous 8/8 workflow pass on `e1101ec7b2040b2977b24e5ea3455e1f31686dcd` is not a pass for the current head.

## Implementation-only work, in order

### 1. BCC tenant lifecycle and transport capability contract
- Current known boundary: shared join/reload exists through CLI, but BCC transactions do not create/list/remove joined tenants end-to-end.
- `multiplex-v1` is not implemented on the wire. Do not advertise or enable shared transport without a negotiated, authenticated capability and tenant isolation proof.
- Implement BCC-owned create/list/edit/remove lifecycle, acknowledgement and rollback only after inspecting the canonical shared-multiplex branch and resolving its merge base with the current integration candidate.
- Until this is genuinely integrated, expose dedicated-port mode only and explain why shared mode is unavailable. Do not imply that port 443 alone provides censorship resistance.

### 2. Public ingress recovery and ownership
- Reboot adoption/reconciliation of live listener + firewall ownership remains blocked.
- Implement explicit reconciliation from trusted durable owner/generation evidence; unknown or partial state must remain fenced and must never be adopted based only on a listening socket, config file, caller boolean, or stale lease.
- Preserve rollback and exact-owner cleanup semantics. Keep the existing Arno 0.3.7 tunnel and unrelated services untouched.

### 3. Safe dedicated-port fallback and preflight
- Ensure the wizard/API selects a dedicated public TCP port by default when shared capability is absent or the candidate port is unavailable.
- Preflight must cover numeric-port conflicts across address families, persistent stopped-service claims, routing, firewall ownership, destination reachability, transport compatibility, and preservation of unrelated services.
- Never acquire or stop an existing service just because its port number is used as an example. The existing active 2053 listener is protected by actual ownership, not by a hardcoded port rule.
- Ensure destination 3X-UI inbound port and client public port defaults are consistent; if the operator chooses a different public port, clearly flag the external subscription-rewrite requirement. Do not add a subscription-domain field to BCC.

### 4. Final-candidate backup/restore and provenance
- Complete final-candidate encrypted backup/restore fault-injection and stopped-service restore drill documentation/artifacts.
- Confirm independent trusted build provenance/signing and artifact checksum acceptance procedure. Existing historical signing or restore results on other SHAs do not satisfy this candidate's gate.

### 5. Release documentation and exact-head evidence
- Keep the readiness ledger's candidate SHA synchronized with the actual PR head.
- Once implementation is complete, run the required exact-head workflows and verify each final status; no inherited or predecessor-SHA green check is accepted.
- Only after the owner-authorized test phase closes all remaining gates may HQ consider merge, tag, signed publication, and deployment.

## Deferred by explicit owner instruction

The owner asked to do all non-test work first and defer tests until the Mac is online. Do not start Mac or real-host tests before that instruction changes. This does not authorize claiming unrun tests as passing.

## Current release gate

**NO-GO.** Known implementation gaps remain in BCC shared-tenant lifecycle/capability, reboot reconciliation, and final-candidate backup/restore/provenance. Exact-head hosted workflow status must be rechecked after the last implementation commit. No merge, release, or deployment is authorized by this checklist.

# v0.4.0 candidate acceptance and remaining gates

Updated: 2026-10-30. This ledger distinguishes implemented behavior from
acceptance evidence. The owner's newer HQ instruction authorizes integration,
merge, tagging and release after the gates pass. Production deployment remains
a distinct action. This document records technical readiness, not permission.

## Candidate and history

- PR #289 (`hq/p0-candidate-20261029`, base `hq/p0-continuation-20261029`)
  supersedes this document's old PR #278 checkpoint. Read live PR metadata;
  neither an earlier SHA nor a green parent supplies checks for a new head.
- PR #290 (`hq/cloud-continuation-20261029`) adds held-authority restart fencing,
  signed shared-tenant lifecycle corrections and the supplementary combined
  ingress acceptance. It remains separate from the shared QA branch.
- The earlier `1824ae460f5e7e9860384ea9055e1f3eaf8b35100fb` checkpoint and
  implementation-only/Mac-wait instructions are historical. The owner's newer
  HQ handoff authorizes independent test execution in the available cloud LAB.
- The isolated-LAB policy in `v040-lab-acceptance-policy.md` remains the
  environmental contract. Production hosts are not a release prerequisite;
  untested critical behavior still blocks release.

## Implemented behavior requiring exact-head validation

- BCC reviewed shared joins allocate separate IR instances, retain an explicit
  EX owner, and carry tenant identity/digests through signed normal Agent jobs.
  Same-IR joins, tenant restart/removal and a persistent owner flow are exercised
  by `TestBCCSignedSharedSameIRRealFlowsPreserveOwner` with real binaries,
  certificate-validated HTTPS and a test process supervisor instead of systemd.
- Tenant actions must pass the normal BCC signing allowlist and strict scoped
  job schema. Finalizing a shared join retains the physical-owner transaction
  and sibling observation records. Physical-owner replacement refuses dependent
  tenants instead of superseding their ownership records.
- Public ingress has a pinned 28-case operational gate. Restart recovery has a
  separate 13-case zero-SKIP gate covering real process death and exact cleanup.
  Remove-only reconciliation is not adoption of an old live session; unknown
  ownership or partial evidence must remain fenced.
- Recovery authority, fresh socket evidence, exact rule readback and deletion
  share one held guard. A late foreign listener or revoked authority prevents
  cleanup rather than acknowledging a destructive effect after the fact.
- Independent peer sessions sharing an authenticated Noise listener are not a
  `multiplex-v1` wire capability. Do not advertise unsupported engines, carriers
  or per-session multiplexing. Keep dedicated mode explicit when no compatible
  reviewed shared owner exists.

## Remaining acceptance

1. On the final candidate SHA, require full Go/race/vet, UI units and real-browser
   checks, applicable security scans and hosted workflows. Preserve failures,
   skips and reruns; no parent-green substitutions.
2. Combine reviewed BCC provisioning, real shared transport, operational ingress
   listener/nft, authenticated receipts and the verified client card in one
   acceptance path. Separate transport and ingress successes do not prove this.
   Include partial failure, cold start, sibling/foreign preservation and cleanup.
   The [combined acceptance contract](v040-shared-ingress-acceptance.md) now has
   positive and real partial-Add failure cases, including tenant Runtime restart,
   retirement, authenticated receipts/cards and durable component reconstruction.
   Local WIP runs passed with zero SKIPs and independent review; final committed
   workflow evidence is still required. Reopening Agent objects/stores is not
   process death. Automatic BCC port reuse after failed Apply remains unavailable.
3. Install exact candidate artifacts in the six-node disposable LAB. Synthetic
   labels, application delay and echo probes are not geographic measurements,
   kernel netem or product auto-selection evidence. Record unsupported cases.
4. Complete final-candidate backup/restore, upgrade/rollback and artifact
   provenance/checksum evidence. CLI updater proposals and disabled external
   engine roadmap issues must be truthfully scoped rather than marked complete.
5. Reconcile the P0 ledger and Issue #67, then record a final GO/NO-GO decision.
   Trusted offline signing and final signed-asset verification remain necessary;
   the owner's merge/tag/release authorization is already recorded in HQ.

## Current disposition

**NO-GO pending final-SHA evidence.** Source presence and bounded local tests do
not close #269. Final committed shared-ingress workflow evidence and the complete
release gate/provenance set remain required.
No production host or protected existing service is a substitute for a LAB.

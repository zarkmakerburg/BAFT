# Fundamental repair of BAFT

Status: implementation direction selected under the user's delegation; **no capability is completed by this document**.

Baseline: released v0.4.0, commit `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`.
Date: 2026-10-30.

## Outcome

An operator must be able to install BCC, enroll two ordinary Linux servers,
create a native TCP tunnel, exchange data through its public port, recover from
an interrupted operation, and maintain the installation through restart,
backup, restore and upgrade. The default documented installation must provide
this outcome without a private LAB namespace or undocumented flags.

The repair preserves working native transport and authentication code. It does
not start by replacing the entire repository. It also does not remove ownership
checks, signature verification, replay fencing or unknown-state refusals to make
a demonstration pass.

## Selected approach

Repair incrementally on the existing native engine, rather than rewrite the management and installation layers wholesale. A rewrite offers cleaner initial boundaries but adds migration risk and repeats previously solved authentication and transport work. The user explicitly delegated this choice ("khodet pish boro"); it is an engineering decision, not a claim that the user reviewed every detail.

## Product contract

1. There is one primary Create Tunnel flow. It uses enrolled node IDs, the live
   engine catalog, server-side planning, the reviewed plan hash and existing
   authenticated job execution. Preview experiments are explicitly separate.
2. Basic inputs are the IR server, EX server, destination address/port and public
   client port. Direction and native TCP are explicit defaults. Transport port,
   instance naming and metrics settings move to Advanced, with resolved values
   visible in Review. Automatic defaults must pass the same conflict checks.
3. Review distinguishes the public client port, inter-node transport port,
   destination port and local administration/metrics port. It explains which
   side owns each listener. It never implies that changing a port also rewrites
   an external application's subscription or client configuration.
4. An online Agent is not automatically a deployment-ready node. Before Plan,
   BCC reports missing enrollment, stale heartbeat, unsupported host profile,
   missing policy/chain, incomplete inventory and ownership conflicts with an
   actionable next step. A missing capability does not become enabled by UI.
5. Accepted, applied, ingress-reachable and destination-verified are different
   states. A nonce echo or TCP connect cannot label an application healthy.
6. Every mutating operation has a persisted outcome, a recoverable interruption
   path and an explicit ownership boundary. Retry does not repeat an arbitrary
   shell command or adopt another service's socket or firewall rule.
7. Installation success reports installed components separately from readiness
   and end-to-end verification. Unsupported hosts are rejected before mutation.
8. Release status distinguishes source, tests, main and published artifacts.
   A merged branch or successful fixture is not a delivered capability.

## Order of implementation and exit gates

Work proceeds in the order below. A later demonstration cannot waive an earlier
gate. Changes should be narrow enough to review, but accepted against complete
operator scenarios rather than only the edited function.

| Stage | Implementation | Exit gate | Audit findings |
| --- | --- | --- | --- |
| 1. Ingress lifecycle | Resolve failed-Apply reservations across BCC and Agent; lost receipt and fresh signed Retire retries; restart/reboot lifecycle; authoritative reconciliation | Fault injection after each persisted/effect boundary; no stale receipt releases a newer generation; no foreign socket/rule is changed; retry converges or returns a precise unresolved state | A03, A04, A05, A13, A14 |
| 2. Ordinary host support and installation | Define a supported production inventory/execution profile; provision policy and firewall prerequisites; connect installer output to readiness | Two fresh supported Linux VMs, standard network namespaces, real systemd, ordinary services present; documented default install and enrollment; real external client reaches destination | A01, A06, A07, A23, A24, A32 |
| 3. One operational creation flow | Replace the primary preview entry with the authoritative native TCP flow; discover nodes and capabilities; explain ports; show persisted progress and verification | Create and add a second tunnel without re-pair; stale Plan cannot deploy; revoked/stale/unready node and conflict fail with useful reasons; replacement requires exact reviewed ownership | A02, A08, A09, A31 |
| 4. Day-two operations | Unify verified version catalogs; define update scope per component; implement role-aware upgrade/rollback; document and exercise full backup/restore; fix rotation and repair guidance | Upgrade BCC, Agent and data plane separately; interrupted upgrade remains recoverable; restore a complete sample installation with compatible state/schema; rotate certificates without re-pairing siblings | A19, A20, A21, A22, A25, A26, A27 |
| 5. Truthful health and operations | Separate listener, tunnel and destination health; describe uptime/latency evidence; label map location; qualify WAN paths without promising universal reachability | Destination down while ingress is up is represented correctly; unknown is preserved; published WAN results name the tested paths, dates and conditions | A15, A16, A17, A18 |
| 6. Versioned feature delivery | Decide scope and separate acceptance for multiplex-v1, reverse, UDP and additional engines | Each included feature has an authenticated capability, deployable artifact, isolation/resource limits, positive and negative end-to-end tests; unsupported features stay labeled unavailable | A10, A11, A12 |
| 7. Release acceptance | Reconcile docs, CLI, UI, lifecycle contracts and release inventory; simplify coupled code where needed to make those contracts maintainable | Exact final artifacts pass the standard installation and maintenance scenarios; capability matrix matches actual behavior; superseded PR/HQ states are closed or clearly historical | A28, A29, A30, A33, A34 |

Stage 6 items require an explicit included/deferred decision before calling a
release complete. Listing them here does not silently expand the immediate
native TCP repair into a promise to implement every catalog entry. The accepted
scope and any deferral must be visible to the operator.

## First engineering milestone: convergent ingress operations

Before integrating the UI, document and implement the lifecycle of one public
port across BCC desired state, signed job, lease, Agent journal, listener,
firewall rule and receipt. Persisted states must distinguish intent, resources
present, resources absent, receipt pending, released and unresolved ownership.

For every interruption boundary, specify:

- which component is authoritative for each fact;
- which evidence is required to retry or reconcile;
- whether retry is read-only or can mutate resources;
- how lease expiry and supersession invalidate work;
- how BCC learns completion if the original response was lost;
- what prevents an old operation from releasing a new binding;
- how the operator sees the first blocking condition.

Required failure cases are failed Apply before and after listener creation,
failed firewall mutation, lost Apply receipt, successful Retire with lost receipt,
Agent termination at each stage, foreign listener appearing during recovery,
stopped-service port claims, and concurrent old/new generations. Explicitly
cover BCC reservations; local cleanup alone is not control-plane convergence.

Production host support is a separate gate. Existing singleton loopback LAB
evidence may prove lifecycle behavior within that scope, but cannot qualify the
ordinary host profile or a real two-server installation.

## Treatment of existing fixes

- PR #292 contains a limited release-tail metadata repair and its evidence.
  Review it on its own merits and verify its final SHA before integrating it.
- PR #293 contains tests for a completed Retire whose receipt was lost. A local
  candidate fix exists outside this baseline. It must pass the positive retry,
  foreign-listener and spent-job cases on the final code before integration.
- Neither change completes ordinary-host support, failed-Apply reconciliation,
  durable Agent recovery or the primary Wizard.
- The repair branch starts from the released commit so these distinctions stay
  reviewable. Do not copy unrelated unpublished changes into it implicitly.

## Required acceptance scenarios

| Scenario | Required observation |
| --- | --- |
| Fresh default installation | Correct role components, keys and policy; services running; readiness result explains outstanding external prerequisites |
| Enrollment | Manual and configured SSH paths work; authentication and host identity failures do not become generic success |
| First tunnel | Enrolled IR/EX selected in primary Wizard; reviewed Plan committed; external client exchanges data with a real destination |
| Second tunnel | No re-pair; no change to the first tunnel or an unrelated listener; separate status and ownership |
| Apply failure | Every partial state has a recorded outcome; signed reconciliation permits safe reuse of the port |
| Lost Retire receipt | Fresh signed retry verifies absence and completes; replay of the spent job remains refused |
| Agent restart/reboot | Listener restoration or a clearly declared recovery policy is exercised on real systemd; stale resources are never mistaken for active service |
| Destination failure | Ingress may remain reachable, but destination/application verification is failed or unknown |
| Upgrade failure | Role-specific versions and state are known; interrupted change and rollback preserve compatible data and keys |
| Restore and uninstall | Full sample installation restored; removal respects ownership and preserves a separate service and its firewall rules |

These are product gates in addition to existing security, kernel and protocol
tests. Mocked browser fixtures, test skips, installer plans and loopback-only
kernel tests must retain their actual evidence scope.

## Evidence and release record

For each completed stage record the source SHA, artifact hashes, test environment,
commands and outcomes, unsupported cases and recovery behavior. Report failed
tests and skips explicitly. Final qualification runs on the actual published
candidate artifacts and the default documented install path. Never infer it
from a source-only test run.

Do not assign a completion percentage from the number of commits or tests.
Track each audit finding as open, implemented/unverified, verified in a named
environment, or delivered in a named release. A test-only or local fix remains
open for the operator until the appropriate delivery gate passes.

## Current checkpoint

All 34 audit findings start open for delivered-product purposes. The first
implementation plans qualify existing completed-Retire retry work and repair a
newly reproduced native listener shutdown race. Their named evidence and release
status must be recorded separately; neither completes ordinary-host ingress,
failed-Apply/BCC reconciliation, reboot durability or the primary Wizard.

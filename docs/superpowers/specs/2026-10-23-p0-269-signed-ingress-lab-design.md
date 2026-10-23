# BAFT P0 #269 — Signed Public Ingress LAB Integration

## Intent and scope

Close P0 #269 by proving that a signed Agent job can safely control a real listener and a strictly owned nftables rule in an isolated Linux LAB. Count a P0 as closed only after its acceptance gates pass, not after individual commits. No production deployment, release, or mutation of existing host firewall chains is in scope.

## Current boundary

The Agent's `publicIngressPrepare` and `publicIngressCancel` currently write or fence canonical private manifest bytes and explicitly do not mutate live resources. `ingressownership.NFTLabTransaction` separately binds a listener, writes phase records to a journal, installs a rule in a preprovisioned nft chain, and verifies ownership; it is not an authenticated Agent executor. `RecoverLabJournal` does not authorize adopting resources after restart. The integration must preserve all three properties rather than treating a signed manifest receipt as a live-ownership proof.

## Chosen architecture

Use a narrow, explicitly LAB-enabled Agent execution adapter. The existing authenticated Agent job validation remains the sole entry point. The adapter resolves the canonical prepared manifest and its exact digest, validates node, instance, tunnel, generation, port, target, and the signed job action, then invokes a transaction interface backed by `NFTLabTransaction`. Production/default Agent configuration does not instantiate the adapter and must fail closed on live mutation requests. Never infer live ownership from a job ACK, a journal phase, or a stored manifest.

A new signed Apply action is distinct from Prepare and Cancel. Prepare remains data-only and durable; Apply requires exact prepared proof, exclusive port ownership, generation fencing, and a configured LAB executor. Apply returns a live receipt only after independent socket and nft verification. Cancellation before Apply fences the prepared generation; cancellation after Apply must use an exact-owner retirement flow with independently verified resource handles. A failed or timed-out Apply must not claim success; rollback is bounded and leaves ambiguous states blocked for reconciliation.

## Component contracts

- `internal/agent`: authenticated job dispatcher, action validation, LAB-only adapter injection, proof-bound receipts, explicit fail-closed behavior when disabled.
- `internal/ingressmanifest`: canonical manifest/digest, immutable generation proof, prepared/cancelled fencing; no host mutation.
- `internal/ingressownership`: transaction and durable phase journal, exact-owner socket and nft inspection, conflict prevention, bounded rollback, fail-closed restart assessment.
- Linux LAB harness: disposable network namespace, isolated nft table/chain, controlled listener and target, fault injection, teardown, evidence capture. Never use production nft table or live service ports.
- BCC: may request the signed lifecycle and display independently verified state, but cannot bypass the Agent or grant ownership based on a UI receipt.

## Lifecycle and failure semantics

1. Validate signed job and canonical digest before any host mutation.
2. Prepare durable initial generation 1 with idempotent same-proof replay; conflicting ownership or cancelled generation is rejected.
3. Apply only with the exact stored proof and configured LAB executor; persist intent before listener bind, then listener-bound, firewall-installed, and verified phases.
4. Verify the bound listener and exact nft handle/rule independently before returning a ready receipt.
5. Retire/cancel using exact proof, handle and ownership; never delete an unrelated firewall rule or listener. A cancellation before Apply creates a durable tombstone.
6. On cancellation, failure, or timeout, bound cleanup and report uncertain ownership as unverified. On reboot, require independent kernel/socket reconciliation before adoption or cleanup; never trust the journal alone.
7. Concurrent jobs on one public port and stale generations must fail closed; replay of the same completed job is idempotent only when live ownership is independently verified.

## Acceptance gates

- Signed job tampering, wrong node/action, invalid digest, stale generation, missing LAB flag, and absent prepared manifest cannot mutate host resources.
- Two owners cannot claim the same port; reserved SSH and existing service ports remain protected.
- Real Linux LAB namespace proves listener + nft install, exact-owner verification, and owner-safe retirement without affecting external resources.
- Inject failures at intent, listener bind, firewall install, verify, journal persist, and retire; prove bounded rollback or explicit unverified reconciliation state.
- Lost ACK, duplicate Apply/Cancel, concurrent Apply, cancelled context, restart and journal recovery never silently adopt or leak ownership.
- Targeted race tests, Go package tests, Linux LAB integration, exact-head CI, and evidence links in HQ issue #67 and P0 #269 are green. Only then close #269 and consider release gates.

## Implementation boundaries and sequencing

First define the signed Apply/Retire contract and adapter with tests that fail before implementation. Next integrate proof-bound manifest lookup, transaction execution, and receipts. Then add Linux LAB namespace scenarios and fault/restart evidence. Finally review the branch, record exact commit/CI evidence, and evaluate P0 closure. Keep unrelated BCC/Aurora design and pairing work in their own P0 tracks.

## Non-goals

No automatic production deployment, no generic shell-command execution by signed jobs, no nft rule adoption based solely on journal data, no forced reuse of occupied port 443, and no relaxation of existing signature, cancellation, or generation fences.

# BAFT v0.5.0 — Baseline Audit (Stage 0)

**Audit date:** 2026-10-31  
**Repository:** `zarkmakerburg/BAFT`  
**Working branch:** `codex/v0.5.0-foundation-20261031`  
**Scope:** Live GitHub refs, releases, PRs #290/#292–#295, Issue #67, main recursive tree, and targeted source reads.

> This is an evidence-based source/GitHub audit, not a local build or runtime test. The connected Desktop Commander devices were offline. No runtime tests were run during this audit.

## 1. Verified live baseline

| Item | Observed evidence |
|---|---|
| Main SHA | `9c32ba369fe2be2c549f2878d1de5dbf092c8f64` |
| Main commit | Merge v0.4.0 native ingress candidate (#291) |
| Main tree | `e065727d66ce97a917b9fc64e4e7ec7db9d73e3c` |
| Main recursive tree | 1,285 entries; response not truncated |
| Go module / language version | `github.com/zarkmakerburg/baft`, Go `1.27.3` |
| Latest published release | GitHub Releases API confirms `v0.4.0`, published 2026-10-30T13:14:53Z; draft=false, prerelease=false |
| Main Actions | Latest returned main run: `static-analysis`, SHA above, completed/success, run 41193394932. This is one workflow, not proof that all required gates passed. |
| v0.5.0 branch head | `743ad38f5039bd8c5f4dcc5d19e36c2961a86a91` |
| Branch head commit | `docs(v0.5.0): add canonical autonomous agent handoff`; unsigned per GitHub verification metadata |
| Working branch runtime diff | No runtime implementation commit is present in the branch history observed here; branch work so far is planning/handoff documentation. |

Links: [main](https://github.com/zarkmakerburg/BAFT/tree/main), [v0.5.0 branch](https://github.com/zarkmakerburg/BAFT/tree/codex/v0.5.0-foundation-20261031), [v0.4.0 release](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.4.0), [latest main static-analysis run](https://github.com/zarkmakerburg/BAFT/actions/runs/41193394932).

## 2. Version-workstream conflict

Draft PR [#295 — docs(v0.4.1): establish execution plan and acceptance gates](https://github.com/zarkmakerburg/BAFT/pull/295) is open against main and has head SHA `95e6f21573b78c8858edfd35356abde9e19f7b21`. The user-directed workstream in this branch is v0.5.0. These are competing next-version plans. This audit preserves the owner's explicit v0.5.0 target and does not merge, rename, or overwrite PR #295. Record the version sequencing decision in Issue #67 before duplicating implementation work.

## 3. Live ingress/lifecycle dependency state

At audit time the following PRs were open drafts:

| PR | Head SHA | Base | Evidence and implication |
|---|---|---|---|
| [#292 — recover committed port releases with identity fencing](https://github.com/zarkmakerburg/BAFT/pull/292) | `f300a73204078ad38557b0d4f379a336b807d626` | `main` | Recovery/fencing change remains unmerged; do not treat as main behavior. |
| [#293 — recover fresh signed Retire after receipt delivery failure](https://github.com/zarkmakerburg/BAFT/pull/293) | `9fc1c355b97547215dbeb0e4490b026ae14e4ccd` | `codex/v041-release-crash-recovery-20261030` | Stacked on #292; its checks and dependencies must be evaluated as a chain. |
| [#294 — join native listener shutdown and unblock hijacked WebSockets](https://github.com/zarkmakerburg/BAFT/pull/294) | `632d8b16c9607d661e485b3a43a106390ccbcf82` | `main` | Lifecycle/shutdown repair remains unmerged. |
| [#290 — preserve live authority and shared tenant lifecycle](https://github.com/zarkmakerburg/BAFT/pull/290) | `ced78f509f2acc8b3cc08fb94d1baa97092191c7` | `hq/p0-candidate-20261029` | Stacked candidate; not main behavior. |
| [#295 — v0.4.1 execution plan](https://github.com/zarkmakerburg/BAFT/pull/295) | `95e6f21573b78c8858edfd35356abde9e19f7b21` | `main` | Competing documentation-only next-version plan. |

[Issue #269](https://github.com/zarkmakerburg/BAFT/issues/269) remains a critical dependency to re-check before any ingress/lease/listener/firewall/recovery runtime work. Issue #67 remains open and is the governance/evidence board. Do not assume historical CI results remain applicable to current heads.

## 4. Main source inventory

The recursive tree contains these actual subsystem roots:

- `internal/bcc/` — control plane, job issuance, ingress policy, Wizard capability/progress.
- `internal/agent/`, `internal/agentjob/` — agent polling/execution and signed-job validation.
- `internal/ingressauthority/`, `internal/ingressmanifest/` — ingress authority/lease and manifest/proof.
- `internal/ingressownership/` — firewall executor, ownership, journal, rollback, reconciliation and tests.
- `internal/ingressinventory/`, `internal/publicingress/` — ingress inventory and public-ingress path.
- `internal/recovery/`, `internal/tunnelnode/` — recovery engine and node lifecycle/state.
- `internal/config/`, `internal/release/`, `internal/uninstall/` — configuration, release verification, uninstall.
- `cmd/baft/`, `cmd/baft-agent/` — CLI and Agent entrypoints.
- `web/` — BCC UI and browser tests.
- `install.sh`, `tests/e2e/`, `.github/workflows/` — installation, E2E and CI.

### Targeted source files inspected at main SHA

- `internal/agent/agent.go`
- `internal/agentjob/agentjob.go`
- `internal/agent/ingress_signed_held_admission.go`
- `internal/agent/public_ingress_v1_command.go`
- `internal/bcc/jobsign.go`
- `internal/bcc/ingress_operational_enqueue.go`
- `internal/bcc/ingress_policy.go`
- `cmd/baft-agent/ingress_provision.go`
- `cmd/baft-agent/ingress_reconcile.go`
- `internal/config/config.go`
- `docs/release/v040-shared-ingress-acceptance.md`

The source documents a pinned Ed25519 signed-job model, node/action/time/replay checks, a held-ingress admission/lease boundary, operational ingress enqueue, startup reconciliation, ownership/journal machinery, and a shared-ingress acceptance contract. This proves that components exist, not that the entire end-to-end product path is production-ready.

## 5. Subsystem findings

| Subsystem | Evidence | Audit classification | Required next proof |
|---|---|---|---|
| BCC control plane | `internal/bcc/`, job signing/enqueue and ingress policy | Implemented but unverified end-to-end | Trace reviewed plan → signed job → Agent receipt → durable UI state. |
| Agent and job authorization | `internal/agent/`, `internal/agentjob/`, signed held admission | Implemented but unverified on live hosts | Test key pinning, target scope, expiry/replay, denial paths and effect-boundary checks. |
| Ingress authority/lease | `internal/ingressauthority/`, Agent mint path | Implemented but unverified as a production path | Verify held-lock lifetime, single-use lease, revocation and uncertain-state fencing. |
| Firewall/ownership | `internal/ingressownership/`, nft backend, operational executor, inventory | Extensive implementation; host/LAB result unknown | Run exact-SHA Linux kernel/namespace tests; prove cross-tenant and system resources survive retirement/rollback. |
| Journal/recovery | journal/recovery code and tests; startup reconciliation | Extensive implementation; crash matrix not run here | Inject crash at each effect boundary; verify restart reconciliation and durable receipt retry. |
| Public ingress | manifest, operational enqueue, Agent command mapping | Code exists; release acceptance not established | Real signed enqueue → apply → payload probe → receipt → truthful readiness → retire. |
| Shared listener/tenant lifecycle | acceptance contract plus open #290 and lifecycle PRs | Main has acceptance contract; related changes remain unmerged | Reconcile dependencies and prove owner survives tenant join/restart/retirement on exact candidate SHA. |
| Wizard/UI | `web/bcc-action-wizard.js`, `web/bcc-app.js`, `web/bcc-operations-v3.js`, browser tests | UI/tests exist; complete user flow unknown | Run JS/unit/browser matrix; verify capability-backed status, failure clarity, Persian RTL and English LTR. |
| Installer/upgrade/rollback | `install.sh`, `internal/uninstall/`, E2E scripts | Implemented surfaces; release test result unknown | Verify signed artifact and run install/upgrade/rollback/uninstall in isolated LAB. |
| Release/CI | published v0.4.0; workflows present | v0.4.0 release verified; v0.5.0 gates unknown | Collect required checks on exact candidate SHA; no inherited green checks. |

## 6. High-priority acceptance gaps and risks

1. Issue #269 and the ingress/lifecycle PR stack must be rechecked before v0.5.0 touches shared listener, lease, firewall, receipt or recovery.
2. #292, #293 and #294 are unmerged at the audited heads; their code and green checks cannot be attributed to main.
3. #290 is based on another candidate branch and is not main behavior.
4. #295 creates a v0.4.1 planning stream in parallel with this owner-directed v0.5.0 stream. Avoid duplicate implementation until Issue #67 resolves sequencing.
5. A successful static-analysis workflow is not a full acceptance pass. No full local tests or LAB execution occurred in this audit.
6. Current main CI status beyond the specific Actions run linked above is **unknown** from the inspected evidence.

These are evidence gaps/dependencies, not claims that every listed subsystem is defective.

## 7. Test and action log

- Live branch and release refs: inspected.
- Open PRs #290, #292–#295: inspected at the listed heads.
- Issue #67: inspected; open, 1,726 comments at query time.
- Main recursive tree: inspected (1,285 entries, non-truncated).
- Targeted source reads: completed as listed above.
- Local Go tests: **not run**; connected Mac/Desktop Commander devices were offline.
- UI/browser tests: **not run**.
- Linux kernel/systemd LAB: **not run**.
- Runtime code changes: **none** in this audit.
- Merge/tag/release/deploy: **none**.

No unrun test is reported as passed.

## 8. Next executable sequence

1. Recheck Issue #269, PRs #290/#292/#293/#294, latest heads/checks and the v0.4.1/v0.5.0 sequencing note immediately before runtime changes.
2. Secure a reachable local checkout/CI execution environment; do not substitute source browsing for tests.
3. Start with the smallest unresolved P0 that is confirmed against main and has a reproducible failing or missing acceptance test. Prefer completing the dependent lifecycle/reconciliation chain over adding new transport protocols.
4. On the selected exact SHA, run targeted tests first, then race/vet/relevant integration/UI checks; record commands, exit codes and SHA.
5. Only then implement the vertical slice: BCC plan → signed job → scoped Agent effect → ownership/journal evidence → real probe → truthful receipt/readiness → rollback/recovery under injected failure.
6. Keep all changes on the dedicated v0.5.0 branch; no merge/tag/release/deploy without the applicable explicit authorization and acceptance gates.

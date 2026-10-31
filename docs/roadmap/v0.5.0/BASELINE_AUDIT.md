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

## 9. Stage 1 — first local execution and first P0 fix (2026-10-31)

### 9.1 Live state re-checked before runtime work

| Ref / item | Observed |
|---|---|
| `main` | `9c32ba369fe2be2c549f2878d1de5dbf092c8f64` (unchanged) |
| v0.5.0 branch head before Stage 1 | `fe063f919f222d131cc88bfc905888322c2d4d02` |
| PR #292 / #293 / #294 / #295 | still open drafts at the heads listed in §3 |
| PR #290 | still an open draft, **but its head `ced78f50…` is the same commit that #291 merged into `main`**. Its content is already in `main`; treat #290 as superseded rather than as a pending dependency. (Correction to §3/§6.) |
| Issue #269 | open, no-go; last comments 2026-10-29 still list shared-transport lifecycle and E2E acceptance as missing. |

### 9.2 Execution environment (honest limits)

- Cloud Linux sandbox, amd64. The pinned toolchain `go1.27.3` could not be downloaded (egress policy denies `proxy.golang.org`/`go.dev`), and building it from source was not permitted. Tests ran with the locally installed **Go 1.25.1** on a scratch copy whose `go.mod` directive was lowered to `go 1.25`; nothing in that scratch change is committed.
- Dependencies were fetched from the upstream GitHub mirrors at the exact `go.sum` versions. Every module zip and `go.mod` was hashed and **matched its `go.sum` `h1:` line byte-for-byte** before use (x/sys, x/crypto, noise, yaml v3, utls, compress, brotli, sqlite, humanize, uuid, isatty, strftime). `x/sys`, `x/crypto` and `sqlite` declare `go 1.26.0`; their go directive was lowered in the scratch copy only.
- `modernc.org/libc|mathutil|memory` are only on GitLab (blocked), so **`internal/bcc`, `cmd/baft-bcc*` and every test that imports `internal/bcc` could not be built**. For `internal/agent`, the 8 test files that import `internal/bcc`/`oplab` (or helpers defined in them) were held aside in the scratch copy; the remaining 32 test files ran.
- No IPv6 `/proc/net/tcp6` and no access to `/proc/1/ns/net` in the sandbox; the Linux-kernel/namespace "LAB" tests fail for that reason, not as product evidence either way.

### 9.3 Baseline result at `fe063f91` (Go 1.25.1, `go test -count=1 ./...`)

- **40 packages ok**: cmd/baft, cmd/baft-agent, cmd/baft-pair, agentjob, benchpath, carrier/h2, carrier/utlsdial, carrier/ws, cluster, clustersync, config, dnsfailover, failover, geoip, identity, ingressauthority, ingressmanifest, ingressunit, metrics, node, protocol, recordshape, recovery, release, resources, routes, scheduler, securityinternal, session, sshboot, sshmigrate, telemetry, tunnelengine, tunnelnode, uninstall, tests/camouflage, tests/correctness, tests/e2e/operational/recovery, tests/installer, tests/release/opgate.
- **9 setup failures (environment: modernc libc unavailable)**: cmd/baft-bcc, cmd/baft-bcc-audit-verify, internal/agent (full set), internal/bcc, tests/e2e/operational, tests/integration, tests/release, tests/uninstall, web/qa/fixture.
- **4 test failures, all environment-caused** (`/proc/net/tcp6` missing or `/proc/1/ns/net` denied, plus cascades): internal/ingressinventory, internal/ingressownership (NFT LAB tests), internal/publicingress (forwarder/listener tests), tests/docs.
- This is **Locally tested (partial, non-pinned toolchain)** evidence only. It is not CI evidence and not a substitute for the Go 1.27.3 gates.

### 9.4 First confirmed P0 defect and fix — ADR-001 §5 secret redaction

Reproduced on `fe063f91` with new tests (both **failed before the fix**):

1. `TestFailedJobACKAndResultAreRedacted` — a failing `health` job whose `baft doctor` output contains the agent bearer token, a PEM private key, a pairing code and URL credentials leaked all of them into `Result.Detail` (logged by `Agent.Run`) and the ACK `Message` stored by BCC.
2. `TestOperationalReceiptFailureCauseIsRedactedAndDeliverable` — a long executor error in `FailureCause` made the receipt exceed `maxIngressReceiptBytes`; the Agent refused to send it (`operational ingress receipt exceeds bound`), so **BCC received no failure evidence** (ADR-001 §3). The cause was also unredacted.

Fix in commit `1cdef92a3a3bae00f727297516c928a8d0340ec3` (`fix(agent): redact and bound diagnostics at every outbound boundary`): new stdlib-only `internal/redact`; Agent ACK message/output, `Result.Detail` and receipt `FailureCause` (bounded to 1024 bytes) are redacted at the boundary; the support bundle shares the same rule set. Diff: 6 files, +462/−33 (tests included).

After the fix (Go 1.25.1, `-race`): both new tests pass; `internal/redact` (unit + 30 s fuzz, ~119k execs), `cmd/baft`, `internal/agentjob`, `internal/release` pass; `internal/agent` (32 of 40 test files) passes except `TestPublicIngressPreflightAcceptsLinuxTCP6Header` (`/proc/net/tcp6` missing — environment). `go build` of `internal/agent`, `cmd/baft`, `cmd/baft-agent` and `go vet` of the changed packages are clean.

### 9.5 ADR-001 acceptance mapping progress

| ADR-001 case | Status after Stage 1 |
|---|---|
| Secrets in errors/receipts/logs redacted | **Agent side implemented + locally tested** (ACK, Result/log, operational receipt). BCC-side defense-in-depth on ingest (`/api/agent/ack` message, receipt `FailureCause` before persist/UI) **not yet implemented** — needs a build environment with `modernc.org/libc`. |
| Failure evidence must reach BCC | Oversized failure receipts are now deliverable (Agent side). |
| Wrong key / wrong node / expired / replay / modified digest | Existing tests in `internal/agentjob` and `internal/agent` pass locally (partial set). Note: `agentjob.Verifier` silently skips replay checks when `Seen == nil`; both nil-`Seen` callers (`ingress_operational.go`, `ingress_signed_held_admission.go`) rely on `RunOnce`'s durable claim and the single-use lease mint. Not exploitable as observed, but fail-open by construction — candidate hardening. |
| Remaining cases (stale lease, lost receipt, partial failure, crash matrix, shared-listener retirement, ownership ambiguity) | Unchanged: need a full Go 1.27.3 + Linux LAB environment and the #292/#293/#294 decisions. |

### 9.6 Next executable actions

1. Run the full suite on the pinned Go 1.27.3 toolchain (CI or a host with module-proxy access) at the new branch head; record exact-SHA results.
2. Add BCC-side redaction/bounding on ACK and receipt ingest using `internal/redact`, with `internal/bcc` tests.
3. Harden `agentjob.Verifier` so a nil `Seen` cannot silently disable replay protection (explicit opt-out for callers that hold a durable claim), with tests.
4. Decide #292/#293/#294 sequencing in Issue #67 before touching lease/listener/recovery code.

## 10. Stage 2 — first exact-SHA CI, a caught regression, and C-BAFT integration (2026-10-31)

Owner direction (2026-10-31): combine C-BAFT's essential capabilities and ease of use with BAFT's BCC into one product on this branch, with full authority to take the necessary steps. Merge/tag/release/deploy remain separate decisions.

### 10.1 CI on the pinned toolchain (GitHub Actions, `ci.yml` via `workflow_dispatch`)

`codex/**` branches do not trigger CI on push, so runs were dispatched manually.

| Run | SHA | Result | Finding |
|---|---|---|---|
| [41229286650](https://github.com/zarkmakerburg/BAFT/actions/runs/41229286650) | `e156fd7f` | **failure**: `test`, `e2e-launch1` | Real regression from `1cdef92a`: redacting the ACK **Output** replaced the EX pairing code with `[redacted pairing code]`, so BCC could not issue IR prepare/trust jobs (`tunnel 1 ended as 'preparing_ir'`; rotation flow `pull jobs: HTTP 500`). Not reproducible locally because those tests import `internal/bcc`. |
| [41229725348](https://github.com/zarkmakerburg/BAFT/actions/runs/41229725348) | `9c32ba36` (main) | control run | Dispatched to separate time-dependent flakiness from the regression; `e2e-launch1` passed on main. |
| [41230304771](https://github.com/zarkmakerburg/BAFT/actions/runs/41230304771) | `26a86708` | `test` failed **only** on `tests/docs` RTL check | Rotation/tunnel tests pass after `5392b404`. Remaining failure: `EXECUTION_PLAN.fa.md` (from `2b275764`) lacked the canonical RTL wrapper — fixed in `3f51221e`, `tests/docs` passes locally. (Correction to §9.3: the local `tests/docs` failure was this defect, not the sandbox.) |
| [41230317598](https://github.com/zarkmakerburg/BAFT/actions/runs/41230317598) | `26a86708` | **success** | `BCC UI tests` (JS unit + Playwright/Chromium browser regressions). |
| [41230590263](https://github.com/zarkmakerburg/BAFT/actions/runs/41230590263) / [41230592672](https://github.com/zarkmakerburg/BAFT/actions/runs/41230592672) | `7b417b2f` | running at time of writing | Results must be read from the runs, not inferred. |

Lesson recorded for the next agent: ACK `Output` is an authenticated one-time channel (pairing/reply codes, rotation plan) and must not be redacted; only failure text and logs are. `TestSucceededACKOutputCarriesPairingCodeUnchanged` guards it.

### 10.2 C-BAFT inventory and decision

C-BAFT (`zarkmakerburg/C-BAFT` `ed57639`) was cloned and inspected: a fork of C-Tunnel by CIAUB (AGPL-3.0 with §7(b) attribution and §7(e) name terms; some files from BackPack), 28 kernel/UDP methods, UDP-Crypt AES-GCM, `rudp-fec`, reverse TCP carrier, one-line `ctunnel://` links with a shared token, a web panel and a Connection Test. Feature-by-feature decisions, licensing obligations and slice order are in [ADR-002](./ADR-002-c-baft-capability-integration.md). No C-BAFT code has been copied.

### 10.3 Implemented in this stage

| Commit | Change | Local evidence |
|---|---|---|
| `5392b404` | Keep ACK Output intact; failure text/log redaction kept | `internal/agent` subset (-race) passes; new regression test fails on `1cdef92a`, passes here |
| `26a86708` | Wizard **Connection Test**: signed path probe IR → EX on the chosen port + common TLS ports, live bilingual/RTL board, only `FULL_DATA` reads as passing, selection via the evidence-checked auto-select, 3 s polling capped at 10 min, cancel, invalidated on server change | `web/tests/connection-test-wizard.test.cjs`; JS suite 168/168; `node --check` of the dashboard script; Chromium screenshots EN/FA at 1000 px and 390 px |
| `7b417b2f` | Probe evidence adds `attempts` and `connect_spread_ms`; board shows "38 ms ±4 · 3/3 ok". No throughput figure (64 KiB flows incl. connect time would mislead) | classification test in an isolated scratch package (Go 1.25.1); JS suite 169/169 |
| `3f51221e` | RTL wrapper for `EXECUTION_PLAN.fa.md` | `tests/docs` passes locally |

### 10.4 Next executable actions

1. Read the results of the runs for `7b417b2f` and the final head; fix anything red before new features.
2. ADR-002 slice: a bounded, signed throughput action (separate from reachability probes) so the board can show a real speed figure.
3. ADR-002 slice: Aurora wizard parity with the live classic wizard (test → recommend → review → deploy).
4. Transport-driver contract ADR (WireGuard first candidate) before any kernel/UDP transport from C-BAFT.
5. BCC-side redaction on ACK/receipt ingest and the `agentjob.Verifier` nil-`Seen` hardening (from §9.6).

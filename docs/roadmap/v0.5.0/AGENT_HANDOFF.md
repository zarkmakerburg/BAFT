# BAFT v0.5.0 — Agent Handoff & Execution Contract

**Purpose:** This is the canonical entry point for any agent continuing BAFT v0.5.0. Read this file first, then the execution plan and current GitHub evidence. Do not require the project owner to paste this context into a chat.

- Repository: `zarkmakerburg/BAFT`
- Governance/evidence board: [Issue #67](https://github.com/zarkmakerburg/BAFT/issues/67)
- Version plan: [EXECUTION_PLAN.fa.md](./EXECUTION_PLAN.fa.md)
- Baseline audit (Stage 0, live evidence): [BASELINE_AUDIT.md](./BASELINE_AUDIT.md)
- First architecture contract: [ADR-001 — operation evidence and truthful readiness](./ADR-001-operation-evidence-and-truthful-readiness.md)
- Working branch recorded at handoff: `codex/v0.5.0-foundation-20261031`
- Base SHA recorded at branch creation: `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`
- Plan commit recorded: `2b2757641943f79ce93a39a2e6589f6734b0c246`

> These are historical checkpoints, not a claim about live status. Verify current GitHub state, branch head, main, open PRs/issues and CI before acting.

## 1. One-line mission

Move BAFT from its verified current state toward a stable, secure and user-friendly v0.5.0 by inspecting the real code, fixing the highest-priority gaps, implementing and testing real behavior, and leaving reproducible evidence for the next agent.

**Do work, not just plans.** Never claim work, tests, LAB runs, CI, release or deployment that did not actually happen.

## 2. Source of truth and state labels

Use this order when evidence conflicts:
1. Live GitHub refs, PRs, issues, commits, releases and checks.
2. Actual code at the exact SHA under inspection.
3. Reproducible local/CI/LAB results.
4. Issue reports and evidence-linked engineering notes.
5. Roadmap documents and previous agent summaries.
6. Chat history.

Label every capability accurately:
- **Planned** — proposed only.
- **Implemented** — code exists.
- **Locally tested** — test actually ran locally.
- **CI verified** — checks passed on the named SHA.
- **LAB verified** — lab test actually ran; state whether physical, VM, container or simulated.
- **Production verified** — observed in real production.
- **Released** — published artifact/tag/release independently confirmed.

Never infer one label from another. Unknown means **unknown**, not complete.

## 3. Known starting point — revalidate first

The owner reported that v0.4.0 was published. Treat this as a reported fact until the release/artifact is independently checked.

The v0.5.0 branch was created as:
`codex/v0.5.0-foundation-20261031`

Recorded base:
`9c32ba369fe2be2c549f2878d1de5dbf092c8f64`

The initial planning document:
`docs/roadmap/v0.5.0/EXECUTION_PLAN.fa.md`

Recorded plan commit:
`2b2757641943f79ce93a39a2e6589f6734b0c246`

The plan commit establishes planning documentation only. It does **not** prove that runtime code, architecture, tests or release criteria have been completed.

Previous reports flagged Issue #269 and PRs #292, #293 and #294 as related to the v0.4.0 ingress/lifecycle work. Their current status is unknown until checked. For each, verify state, latest SHA, CI, acceptance criteria and whether it belongs in v0.5.0 or is already superseded.

## 4. Product goal for v0.5.0

Build a coherent system that is safe to operate and straightforward for ordinary users:
- BCC is the central authority for identity, policy, authorization, plans and job issuance.
- Agents execute only authentic, authorized, scoped jobs; they do not invent authority.
- Journal, ownership, recovery and rollback make state changes auditable and recoverable.
- Firewall and OS resource changes are ownership-aware and must not delete other tenants' or system resources.
- Transport drivers provide connectivity mechanisms, not independent authorization.
- Wizard and UI guide users through server selection, connection choice, real testing and apply/verification.
- Installer, upgrade, rollback and uninstall are controlled, observable and tested.
- Claims such as “connected”, “active” or “healthy” must be backed by evidence appropriate to the claim.
- Documentation, tests and GitHub evidence make future agent handoffs cheap and reliable.

Do not rewrite healthy subsystems without a demonstrated reason. Tie every substantial change to a concrete defect, product need, security risk or acceptance criterion.

## 5. Target architecture — validate against actual code

### BCC
Central source of truth for identity, authorization, tenant/resource scope, policy, connection plans, job issuance, lifecycle and reported state. UI input, local flags or a caller-supplied boolean must not create authority.

### Agent
Validate job authenticity, issuer, authorization scope, expiry/replay rules and target resources before sensitive effects. Enforce controls at the actual effect boundary, not only at an API or UI boundary. Record outcomes and support recovery.

### Journal
Determine whether state and side effects can diverge across crashes/restarts. Test idempotency, interrupted operations, replay/recovery, auditability and secret redaction.

### Ownership and firewall
Never remove a resource merely because its name, interface or comment looks familiar. Prove BAFT ownership, scope effects, preserve prior state where required, and test rollback. Shared listeners and multi-tenant resources must survive unrelated tenant retirement.

### Transport drivers
Inventory real implementations, OS/kernel/CPU constraints, privilege needs, timeout/retry behavior, MTU, secret handling, health probes and rollback. A protocol name does not prove encryption or authentication.

### Wizard and UX
Target flow:
1. Add/select servers.
2. Choose connection and essential settings.
3. Run a real connection test.
4. Apply and verify actual state.

Show clear differences between configured, requested, applied, reachable and verified. Support mobile/desktop, Persian RTL and English LTR, accessible errors and progressive disclosure of advanced options.

### C-BAFT feature inspiration
C-BAFT is a separate project, not BAFT's authority model. Candidate ideas to assess—not promises for v0.5.0—include Connection Test, Direct/Reverse, path selection, filters, metrics, MTU discovery, Multi-Lane, FEC, GRE/IPIP, VXLAN/GENEVE and UDP-Crypt. Before reuse, inspect actual code, security properties, dependencies and licenses. Prefer capability adaptation over copying insecure ownership, installer or authorization patterns.

## 6. First mission: evidence-based baseline audit

### A. GitHub and branch state
- Fetch current main SHA and v0.5.0 branch head.
- Compare refs and inspect commits since branch creation.
- Review open/closed/merged PRs, P0/P1 issues, release/tag state and CI checks on exact SHAs.
- Inspect Issue #67 directives and latest evidence.
- Identify concurrent work before editing.
- Do not reset, force-push, delete branches/stashes, or overwrite another agent's work.

### B. Real code inventory
Locate actual code and tests for BCC, Agent, Journal, firewall, ownership, recovery, Wizard/UI, transports, configuration, authentication/authorization, installer, upgrade/rollback, logging/metrics, CI and security tests. Do not assume directory names or package boundaries.

### C. Audit table
For every subsystem record:
- actual paths and exact SHA;
- implemented behavior and public interfaces;
- callers/dependencies;
- existing tests and tests actually run;
- security risks and missing behavior;
- measurable acceptance criteria;
- next action and evidence links.

Classify findings as:
- **Verified complete**
- **Implemented but unverified**
- **Incomplete or broken**
- **Not implemented**
- **Unknown** (not inspected / insufficient evidence)

Use file paths, line numbers, commit links and CI links wherever possible.

### D. Persist handoff state
Write the results to `docs/roadmap/v0.5.0/BASELINE_AUDIT.md` (or an existing, more appropriate canonical path after checking the repository). Update Issue #67 with a concise evidence-linked summary. Do not modify runtime behavior until the baseline and relevant contracts are understood, except for an urgent, clearly scoped security fix.

## 7. Second mission: architecture contracts

Stage 0 baseline audit is committed at [BASELINE_AUDIT.md](./BASELINE_AUDIT.md). The first contract is [ADR-001 — operation evidence and truthful readiness](./ADR-001-operation-evidence-and-truthful-readiness.md). It defines operation-state semantics, authority/scope invariants, evidence-before-success, ownership-scoped recovery, secret redaction and required acceptance cases. It is a proposed contract, not proof that current runtime behavior passes.

After the audit, document the gaps between actual implementation and target architecture. Add focused ADRs where needed for:
- BCC/Agent responsibility boundaries;
- job authenticity, authorization and replay protection;
- resource ownership and shared resources;
- Journal consistency and recovery;
- transport driver contract;
- truthful connection/health state;
- secrets and logging;
- installation, upgrade and rollback;
- compatibility and migration.

Do not design against imagined code. Reference real paths, interfaces and failing or missing acceptance tests.

## 8. Third mission: finish one vertical slice

Once the baseline and contracts support it, implement the smallest high-value end-to-end flow:

**BCC connection plan → validate and issue job → Agent verifies and executes within scope → ownership evidence → Journal record → real probe → truthful result → rollback/recovery under injected failure.**

Adapt to existing architecture; this is an acceptance goal, not permission to invent new interfaces blindly.

Acceptance criteria:
- unauthorized/expired/replayed jobs are rejected as specified;
- out-of-scope resources are not modified;
- duplicate requests do not cause unsafe duplicate effects;
- reported success is supported by a real probe or appropriate effect evidence;
- failures/timeouts remain failures or unknown, never false success;
- restart/crash recovery is deterministic and tested;
- rollback affects only resources owned by the operation;
- shared/other-tenant resources survive;
- sensitive values are not leaked;
- tests and results are tied to the exact commit SHA.

## 9. LAB plan

A six-node target topology has been proposed:
`IR-01`, `IR-02`, `EU-01`, `EU-02`, `EU-03`, `EU-04`.

These are logical labels; do not claim six physical hosts exist until verified. For each node, record whether it is physical, VM, container or simulated; OS/architecture; actual location if known; network path; latency/loss source; transport; and limitations.

Test healthy connection, latency/loss, abrupt path loss, agent/controller restart, expired/duplicate job, partial apply, rollback, recovery, tenant retirement and preservation of shared resources.

Never describe simulated latency or a container test as a real multi-country physical test. Keep LAB previews out of GitHub unless explicitly needed and authorized.

## 10. Priorities

**P0 — security and correctness**
- authorization bypass;
- unproven ownership or cross-tenant deletion;
- lifecycle/recovery/rollback corruption;
- false active/healthy status;
- secret leakage;
- critical CI/install/release blockers.

**P1 — core product path**
- complete BCC-to-Agent execution;
- reliable Journal and ownership;
- transport contract and real connection testing;
- usable Wizard and truthful state;
- predictable installation and upgrade.

**P2 — expansion**
- additional transports;
- automatic path selection;
- advanced metrics;
- Multi-Lane/FEC and other optional features.

Re-evaluate priority against current evidence. P2 work must not delay unresolved P0s.

## 11. Security and change constraints

- No caller flag or UI field grants authority.
- No resource deletion without verified ownership and correct scope.
- No secrets in logs, commits, public artifacts or handoff reports.
- Make sensitive operations bounded, auditable, replay-safe and rollback-aware.
- Preserve other agents' work and unrelated local changes.
- Do not modify production, merge, tag, release or deploy without the owner's explicit authorization and the applicable current governance.
- Do not force-push or destructively reset.
- Verify repository rules and Issue #67 before write actions.
- If permissions or infrastructure are missing, state the exact blocker; never simulate a successful write.

## 12. Tests and proof

Choose tests based on changed behavior: unit, integration, race, static analysis/vet, fuzz/protocol, fault injection, ownership/authorization, rollback/recovery, install/upgrade and LAB.

Record exact command, environment/tool versions where relevant, SHA, exit code, pass/fail counts if available, skipped tests and limitations. A green CI result only proves the named SHA and jobs.

Definition of done requires the relevant implementation, tests actually run, regression/security review, updated documentation, traceable commit and Issue #67 evidence. Not every documentation-only change requires runtime tests, but explicitly say when none ran.

## 13. Efficient autonomous workflow

1. Inspect before changing; avoid duplicate work.
2. Use targeted searches and file ranges instead of repeatedly reading the whole repository.
3. Reuse existing CI evidence only when SHA and scope match.
4. Fix root causes rather than hiding failures.
5. Parallelize independent read-only investigation; coordinate shared writes.
6. Keep changes small, testable and reversible.
7. Commit and push only to the authorized working branch.
8. Update baseline/handoff notes as work advances so the next agent can continue without repeating discovery.
9. Keep interim reporting minimal; continue executing while meaningful authorized work remains.

## 14. End-of-stage report

Include:
- stage and baseline SHA;
- concrete code/docs changes;
- added/deleted LOC from the actual diff;
- tests run, exact result and limitations;
- commit SHA/link;
- Issue/PR/CI evidence;
- remaining P0/P1 and real blockers;
- next executable action.

Do not count new-file line count as total diff statistics unless that is what the metric explicitly represents.

## 15. Immediate startup checklist

1. Verify current GitHub state, main and branch heads, Issue #67, related PRs/issues and exact-SHA CI.
2. Read this handoff and `EXECUTION_PLAN.fa.md`.
3. Produce the real subsystem inventory and `BASELINE_AUDIT.md`.
4. Identify confirmed P0/P1 gaps and avoid repeated work.
5. Document necessary architecture contracts.
6. Implement the highest-value safe change; test, fix, commit and push to the authorized branch.
7. Record evidence and refresh the handoff state.
8. Do not merge/release/deploy without explicit authorization.

**Final rule:** Act autonomously within granted access and safety boundaries. Never substitute a report for feasible work, and never fabricate implementation, tests, CI, LAB, production or release evidence.

# ADR 0008: Incremental Aurora frontend and trusted provider boundary

Date: 2026-10-15. Status: development direction; implementation and acceptance pending.
Baseline: `1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7`.
Authority and source inventory: [Aurora execution ledger](../aurora/README.md).

## Context

BCC currently embeds HTML/CSS/JavaScript in its Go binary. The server injects
per-request configuration, renders public/login/dashboard modes and owns
secret-path access, session cookies, CSRF and classic fallback. An operations
bridge mounts the existing reviewed tools. There is no existing Node package
or TypeScript build configuration to extend.

The owner approved a modern Aurora product with nine workstreams, strict
TypeScript, FA/EN, RTL/LTR, dark/light, mobile operation and verifiable mock
versus real data. A wholesale framework replacement would add migration and
security risk before the product/API boundaries are inventoried.

## Decision

1. Keep the Go server, authentication and authoritative network/tunnel logic.
   Introduce Aurora on an isolated feature branch and opt-in preview surface.
   Preserve the existing dashboard and classic fallback until independently
   tested rollout approval. This ADR does not change routing or default UI.
2. New Aurora application modules use strict TypeScript and typed domain
   models. Compile to browser JavaScript before embedding. Do not add an npm
   install or Node service to the production runtime. Review a pinned build
   toolchain and deterministic generated-asset policy in the A1 implementation.
3. Define a provider boundary shared by deterministic mock fixtures and
   schema-validated same-origin real adapters. Inputs begin as `unknown`.
   Presentation models explicitly carry scope, observation time, validity and
   unavailable values. Unknown never becomes zero, offline or healthy by default.
4. New preview surfaces are explicitly DEMO and use synthetic identifiers and
   reserved example addresses. Mock mutations update only memory. Never use
   an environment guess to enable operational writes.
5. Real adapters retain cookie authentication, CSRF, redirect refusal, safe
   errors and session-expiry behavior. No bearer-token prompt, credentials in
   localStorage, automatic login fallback or direct cross-origin calls.
6. Migrate one surface at a time with explicit ownership of DOM nodes,
   listeners, timers and pending requests. Do not duplicate the global fetch
   interception or mount old and new controllers over the same operation.
7. Start with framework-independent models/adapters and shared semantic design
   tokens. React/Next.js, React Flow, ECharts, Liquid Glass and Kiranism are not
   approved dependencies by this decision. Evaluate candidates against the
   existing implementation before committing to a component framework.
8. Use only existing supported operations. New Studio writes, acknowledgement,
   restore and profile changes stay demo-only until authenticated API contracts,
   permission behavior, confirmation, uncertainty/retry and audit are reviewed.

## Dependency admission record

Before adding any dependency, record exact package/version, primary license
and notices, upstream maintenance/release evidence, known security advisories,
browser/Go embedding compatibility, optional SSR implications, compressed
bundle delta and alternatives. Pin the chosen build graph and lockfile. This
slice adds no dependency, so no license/security verdict on named candidates
is implied. Do not assume an upstream demo's license covers copied code/assets.

## Incremental PR sequence

| Slice | Output | Required gate / rollback |
|---|---|---|
| 0 | Inventory, risks, ADR and readiness ledger | Documentation review; no runtime change |
| A1 | Strict models, validators and isolated providers/toolchain | Typecheck/build and invalid/stale/wrong-scope fixtures; no live adapter activation |
| A2/B | Bilingual dual-theme shell/primitives/mobile layouts | Eight basic language/theme/device combinations plus 320px, keyboard and reduced motion; keep old page |
| C/E | Topology/Inspector and telemetry views | Scope/freshness/error tests and measured rendering/transfer cost; no fabricated health |
| D/H | Studio, journey/history and diff/preview | Plan-hash, uncertainty and rollback UX tests; existing backend gates retained |
| F/G | Incident Center and palette | Reviewed #202/#203 evidence/read contracts, permissions and focus management |
| I | Optional motion/3D | Functional gates already passed; measured cost and reduced-effects fallback |

Roll back a UI slice by disabling its opt-in entry or selecting the preserved
existing page; do not reset/rebase published history or revert backend state.
No destructive cleanup or default cutover is authorized here. If actual API
integration changes schema/state, it requires a separate migration/rollback plan.

## Evidence and completion

Record exact source SHA, changed files, toolchain, commands/results, failures,
SKIP/BLOCKED and preview/screenshot artifacts. Matrix: FA/EN × dark/light ×
desktop/mobile, with 320px, keyboard, focus return, reduced motion, session
expiry and malformed/stale data checks. Baseline transfer/runtime measurement
precedes performance budgets. Source contract tests cannot prove visual QA.

DEMO, INTEGRATED and PRODUCTION-VERIFIED are separate readiness levels.
Native two-node tests and candidate handshake/payload before cutover remain
OPEN and owner-deferred. P0/backend acceptance, merge, release and operational
deployment gates are not waived by frontend work or green CI.

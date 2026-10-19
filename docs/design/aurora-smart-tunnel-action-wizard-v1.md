# BAFT Aurora — Smart Tunnel Action Wizard v1

Status: product UX specification; no production behavior or deployment.

## Product objective
Replace fragmented BCC forms with one reusable accessible overlay per action. Preserve all existing BCC functionality, integrate Aurora design language (not replace BCC), support Persian/English, RTL/LTR, dark/light and mobile.

## First-class tunnel architecture selection
Before node selection, expose **Tunnel topology: Direct / Reverse** and **Tunnel engine: Smart Auto / BAFT Native / installed compatible engines** as prominent first-class steps. The user-facing service intent (3X-UI/TCP/custom) remains separate. Treat topology, engine, and connection initiator direction as separate typed properties. Reverse must have an explicit engine-specific definition, including which node initiates outbound transport, listener placement and NAT/firewall implications. Do not imply reverse support until engine capability discovery proves it. Engine registry should publish supported topologies, protocol families, platform/version requirements, required parameters, health capabilities, rollback capabilities and deployment readiness. Unsupported combinations are disabled with an explanation, not silently substituted. Smart Auto may select only verified compatible installed engines. Engine-specific fields appear progressively; the preview shows topology, engine, initiation direction, endpoint and host mutations. Keep old tunnel configs backwards compatible and do not reinterpret existing tunnel direction.

## Smart tunnel creation flow
1. **Intent:** choose 3X-UI, generic TCP or custom; smart defaults, advanced mode on demand.
2. **Nodes:** context-select IR and EX, show live Agent health, versions, capacity, compatibility. Never infer live readiness from UI mock data.
3. **Route:** separate user-facing public TCP port, IR loopback route listener, EX transport listener and EX 3X-UI target. Smart suggestions must validate each resource. 3X-UI owns UUID/subscriptions. No BCC subscription-domain field. No assumed multiplexing on TCP/443.
4. **Preview:** show proposed exact host/service/firewall changes, owned resource IDs, preflight results, downtime impact, conflicts, rollback plan and immutable plan hash. No writes at preview.
5. **Confirm/execute:** revalidate plan and permissions; durable idempotent operation ID; signed allowlisted Agent jobs; prepare tunnel; commit/verify tunnel; prepare/commit/inspect public ingress; finalize. Present real-time evidence and errors. Preserve existing BAFT.goldapp.tr:2053.
6. **Result:** verified endpoint, health, configuration history, audit, retry/recovery; do not claim success without observed Agent evidence.

## Floating Liquid Glass visual contract
Use a floating translucent glass modal with subtle Aurora accent glow, soft shadow, thin light-catching border and layered depth. Blur and dim the BCC dashboard backdrop while open, adapting tint and opacity to dark/light themes. Maintain WCAG-readable text and control contrast with solid-enough inner surfaces; never let glass transparency compromise legibility. Desktop modal max-width approximately 680–840px; mobile uses near-fullscreen responsive overlay with safe-area support and no horizontal scrolling. Animate entry/exit gently (~180–260ms) with reduced-motion support. Blur/glow must be performance-conscious (avoid large expensive filters on low-end devices); preserve focus trap, keyboard handling, safe dismissal and operation persistence. This is the universal Action Wizard shell across tunnel/server/backup/update/delete operations, not a one-off screen. Avoid harsh neon or heavy bloom.

## Universal overlay shell
Context-aware launcher from tunnel/server card and command palette. Clear title, progress, back/next, contextual help, keyboard focus trap, ESC and backdrop safe handling, responsive fullscreen on narrow mobile, no redundant fields, save draft, accessible validation and loading states. Long-running operation persists independently of modal; closing overlay never cancels a running job. Destructive operations require stronger explicit confirmation and impact preview.

## Backend contract
Typed per-action step schema, server-side validation and authorization, immutable plan hash, preflight and ownership proofs, idempotent execution, persistent operation lifecycle, telemetry, audit, safe rollback. Never use arbitrary remote shell. Fail closed on stale/missing ownership evidence.

## Acceptance tests
- New user can complete normal tunnel setup within one overlay without page navigation.
- Only essential inputs in default mode; advanced inputs are progressive.
- Preview and actual execution are distinguishable; no host mutation before confirmation.
- Invalid ports, offline nodes, conflicts, stale plans, unauthorized actions and failed health checks block deployment.
- Reopening operation shows durable live status; retries are idempotent.
- Keyboard/screen-reader/mobile/FA/EN/RTL/LTR/dark/light tested.
- Existing tunnel and BCC functions are preserved.

## Delivery priorities
P0: tunnel create/edit/delete, public ingress and verified execution; P1: server upgrade/downgrade, backup/restore; P2: remaining actions and bulk workflows. No merge, release or deploy without HQ authorization.

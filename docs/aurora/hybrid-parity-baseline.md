# BAFT Aurora Hybrid — release parity baseline (Stage 104)

**Owner decision: MERGE, DO NOT REPLACE.** This is a source-evidenced acceptance checklist, not a screenshot or browser-verified visual approval.

| Existing BCC surface | Evidence in `internal/bcc/dashboard.go` | Hybrid disposition | Gate before replacing legacy surface |
| --- | --- | --- | --- |
| Nodes/UP/DOWN/UNKNOWN KPI cards | `kNodes`, `kUp`, `kDown`, `kUnknown` | KEEP + MERGE into telemetry | UNKNOWN remains distinct from down and zero; fresh observed timestamps |
| 7-Day Traffic & Uptime | `historyCanvas` | KEEP + ENHANCE | Same node selection, time range and truthful missing data |
| Tunnel plan and edit | `tnPlan` | KEEP + MERGE into Tunnel Control Studio | Hash-bound plan review, auth, audit and rollback; no silent write |
| Change History | `chRows` | KEEP + MERGE into Configuration History | Secret-free append-only records and correct per-instance identity |
| Route Doctor | `rdResult` | KEEP + MERGE into Incident Center | Evidence age and unknown/stale handling |
| Connectivity Matrix | `pmMatrix` | KEEP + MERGE into Network Intelligence Map | PASS requires fresh FULL_DATA; never mark stale green |
| Recent Jobs | `jobs` | KEEP + ENHANCE | Permission and failure visibility retained |
| Layout/motion | 820px/520px breakpoints; reduced-motion rule | KEEP + ENHANCE | Rendered 320/390/768/1440px and reduced-motion browser tests |

**Automated guard:** `node --test web/aurora/tests/hybrid-parity.test.mjs`. It checks source-level retention of key legacy controls and fallback policy only. It is intentionally a narrow, independent test; it cannot prove that the browser renders correctly or that backend flows work.

**Not yet accepted:** actual screenshot/visual parity, FA/EN × dark/light rendered matrix, keyboard/screen reader, 320px overflow, performance, live BCC route integration, API write controls, native two-server tests (owner-deferred). The existing BCC UI remains the fallback. PR207 combines Aurora wire and navigation only; do not advertise it as a completed Hybrid dashboard.

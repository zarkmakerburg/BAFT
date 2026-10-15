# Hybrid preservation contract — default BCC and classic

HQ Stage181 requires Aurora to integrate with the existing product. Stage188
identified a gap: the older source parity guard inspects `dashboard.go` but does
not protect the authenticated default Command Center V3 and its visual assets.
This slice adds tests and inventory; it does not mount or activate Aurora.

| Surface to retain | Real source and serving path | Evidence in this slice |
| --- | --- | --- |
| Default dashboard shell, navigation and search | `web/bcc-app.html`, `web/bcc-app.js`; secret-path root through `access.go` | Source guard protects shell IDs, ten routes and dashboard control IDs |
| Map, Earth, welcome globe and operations | `bcc-map.js`, `bcc-earth-v3.js`, `bcc-welcome-globe-v3.js`, `bcc-operations-v3.js`, `assets/world-land.js` | Guard checks nonempty source, real `go:embed` declaration, page dependency order and bridges; HTTP tests fetch assets |
| Current KPI, traffic range/chart, servers, events, health, costs and quick actions | `dashboardView` in `bcc-app.js` | Negative cases delete each control from an in-memory source copy and require rejection |
| FA/EN, RTL/LTR, dark/light and reduced motion | Current inline first-paint bootstrap and `bcc-app.css` | Actual bootstrap executes in Node VM for four preferences, invalid values and blocked storage; source guard retains reduced-motion CSS |
| Native operations: UP/DOWN/UNKNOWN, history, tunnel plans, Doctor, matrix, jobs | `dashboard.go` inserted into authenticated `operationsHTML/operationsJS`; `bcc-operations-v3.js` | Actual Go HTTP test decodes default page configuration, verifies control IDs/loaders and shared CSRF |
| Reversible classic fallback | `classicLink`, `access.go` secret-path `classic/` | Source guard and HTTP test require same-session access to both pages |
| Session/CSRF and logout boundaries | Existing `accessHandler`, `sessionAdmin`, tunnel plan endpoint | HTTP tests reject missing, short-wrong and same-length wrong CSRF; valid CSRF reaches invalid-JSON rejection without creating a tunnel; logged-out default/classic expose no operations |

Tests use existing Node built-ins and existing Go HTTP fixtures; no new package
or production module is added. The preflight path filter expands from
`web/aurora/**` to `web/**`, so changes to current BCC assets/tests also exercise
the pinned combined Aurora × Core workspace. Existing pin freshness, read-only
permissions, recoverable bundle and all mandatory tests remain intact.

Run locally where available:

```sh
node --test web/tests/bcc-hybrid-preservation.test.cjs
node --test web/tests/*.test.cjs
go test ./internal/bcc -run '^TestHybrid' -count=1
go test -race ./internal/bcc -run '^TestHybrid' -count=3
```

The first new Node run passed all ten cases, including deliberate deletion,
unembedding, dependency order and fallback/motion mutations. They are negative
test inputs, not product modifications or a claim that existing UI was broken.
The follow-up includes a same-length invalid CSRF token: testing only a short
wrong token would miss a regression that checked length without token equality.
This is test hardening, not evidence that the current CSRF implementation is bad.
The source guard is deliberately conservative: reviewed structural refactors
must update the contract with equivalent evidence. It does not prove pixel,
layout, chart rendering, keyboard/screen-reader behavior or preservation of every
possible code path merely because source markers remain present.

Actual Go execution and comprehensive current-head/combined CI are reported at
their exact SHA in HQ #67. The development environment lacks local Go/gofmt;
unexecuted commands must not be reported as passed. No production source changed.
Initial optional `internal/bcc/operations_v3.go` read was ENOENT; inventory found
the real module at `web/bcc-operations-v3.js` before editing.

Next P1 gate: reviewed isolated integration candidate, reversible feature flag,
typed and validated read contracts with fresh/UNKNOWN data, and actual rendered
FA/EN × dark/light ×320/390/768/1440 matrix covering both retained surfaces.
Browser accessibility/performance and owner-deferred native lab acceptance remain
open. No main merge, release, deploy or production acceptance follows from these
source/VM/HTTP tests.

# Current BCC ↔ Aurora read-contract qualification

This slice pairs selected synthetic wire fixtures with actual Go HTTP handlers
and the existing TypeScript real-provider decoder. It is a test contract, **not a
new/versioned API**, live integration, merged incident contract or production UI.
Parent source: PR204 `de446b5df8f045c69ca388b3c500515143f7812a`.
Ownership: #67 claim6192411548, independent API-contract lane in HQ6190282468.

## Producer and consumer

`internal/bcc/aurora_wire_contract_test.go` runs the existing `Server.Handler()`
in process against a throwaway Store, with a fixed nanosecond timestamp and
synthetic nodes/tunnels. No server socket, operational job or live node is used.
It checks actual authenticated `GET /api/overview` and `GET /api/tunnels` output
against `web/aurora/tests/wire/*.json`, including exact array order/cardinality.
The object comparison is recursive and permits additive unconsumed fields.
Fixtures are a consumed-field subset, not full generated response captures; no
claim is made that every backend field or frontend feature is covered.

The Node test uses those SAME files as injected responses to `createRealProvider`.
No real request is sent. Pairing proves compatibility only once both producers'
and consumers' tests pass on the same source SHA. Node success alone does not
qualify the Go producer. No fixture is imported by the runtime/UI or mixed into
live data. Existing fixtures and provider tests stay intact.

| Case | Go side | Node side |
| --- | --- | --- |
| Empty inventory | Actual handlers encode empty arrays | Empty arrays stay empty; no invented resources |
| Measured zero | Synthetic recent counter samples yield zero rates/latency | Zero stays zero, but per-metric freshness remains UNKNOWN without its own timestamp |
| Missing/revoked | Real node projection emits null metrics/revoked flag | Null remains null, revoked node cannot appear UP |
| Location | Existing IR/DE country anchors | Clearly country-anchor, not server GPS |
| Time | Go RFC3339Nano with9 fractional digits | Original source string retained; cached data expires without renewing timestamps |
| Legacy + scoped intent | Same IR/EX, distinct ports/IDs and optional instance ID | Legacy absence becomes null; scoped ID retained; active never implies payload delivery |
| Authorization/read-only | Missing credentials refused; GET queues no jobs or intent changes | Provider performs only same-origin cookie GETs; existing session/CSRF tests remain separate |

The Go harness uses only the existing legacy test-admin boundary. It does not
exercise native browser cookie login, secret-path sessions, CSRF writes, incident
SQLite persistence or deployed agent access. Full authenticated integration and
schema versioning remain separate convergence requirements.

## Execution/evidence

No production code, route, API field, transport, embedding, DOM or dependency is
changed. New Go test source is not part of shipped binary. A separate branch/PR
stacks on current Aurora; PR200/203/197/204 remain untouched.

Local Node strict/build, adapter/DOM/classic tests are executable. Local Go/gofmt
are absent at checked paths, so Go compile/execution is NOT claimed locally;
actual exact-SHA CI must qualify it after draft publication. A compile/runtime
failure must be retained and fixed before producer/consumer acceptance.

Attempts before publication: optional guessed monitoring/glob source reads failed
and were corrected to store.go; no AGENTS or Go tool found in checked inventory.
Locked install uses existing dependencies with lifecycle scripts disabled. No
browser/server/dependency download, native node or deployment attempt occurred.
Final test counts, exact SHA, CI and any later failures are logged in #67/#199.

First producer attempt on `e646fac` compiled but failed in test setup: fresh
Store lazily leaves its tunnel map nil, so direct synthetic insertion panicked.
The correction initializes only that test map. The full failing CI job is retained
at [immutable evidence](https://github.com/zarkmakerburg/BAFT/blob/0143ac246304eb8a34c1566c54bebb1176401809/docs/evidence/aurora/wire-e646fac/first-go-failure.md).
This correction requires a new exact-source CI pass; Node success is insufficient.

Browser/layout/mobile320/screenshots/rendered accessibility/performance remain
OPEN. Native IR↔EX/candidate remains owner-DEFERRED/OPEN. This test slice does not
authorize main merge/release or claim controlled convergence is complete.

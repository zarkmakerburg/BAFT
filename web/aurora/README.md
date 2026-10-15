# Aurora A1 — typed data foundation

This package is an isolated foundation, not a deployed dashboard. Its generated
modules are committed but are not embedded by `web/assets.go`, imported by the
current UI, or exposed through a new BCC route. Production needs no Node service.

From the repository root:

```sh
npm --prefix web/aurora ci --ignore-scripts --no-audit --no-fund
npm --prefix web/aurora test
# After an intentional source change:
npm --prefix web/aurora run build
```

The single build dependency is TypeScript 6.0.3, pinned with registry SHA-512 in
the lockfile. See [toolchain review](../../docs/aurora/toolchain-review.md).
`test` runs strict typechecking (including negative capability tests), an isolated
rebuild and exact generated-file comparison, module byte budgets and behavioral
tests against the committed ES2022 output. No runtime package dependency exists.
Generated assets are not edited by hand. Missing, changed or orphan output fails.

## Provider contract

`ReadProvider` has exactly `getOverview` and `getTunnels`; mode is explicit.
`createMockProvider` returns DEMO-labeled deterministic synthetic resources. It
does not fetch, poll, read cookies or fall back to a live provider. Its optional
phase-edit illustration only changes that provider's memory; callers receive
independent data and one mock instance cannot mutate another.

`createRealProvider` is explicitly constructed by a future authenticated shell:

```ts
const provider = createRealProvider({
  origin: location.origin,
  basePath: trustedBootstrap.base, // server-injected path, not operator input
  onSessionExpired: expireCurrentSession
});
```

This example is an integration contract, not code activated in the current page.
Only existing `api/overview` and `api/tunnels` GETs are allowed. Base paths are
path-only, slash-terminated ASCII segments; traversal, encoded segments, query,
fragment and cross-origin bases are refused. Future enrollment/bootstrap changes
must review that restriction rather than silently normalize an untrusted path.
Reads use session cookies, `no-store`, redirect refusal and JSON Accept; they do
not accept a bearer token, credential, arbitrary endpoint or request method.
There are no live writes to add CSRF headers to. Any later write provider must
review the actual session/CSRF/permission/confirmation contract separately.

The default deadline is 10s, covers headers and streamed body, and has a 30s
configuration ceiling. Responses have a 1 MiB decoded-byte ceiling; declared
length does not bypass streamed-byte checks. Error bodies are not displayed.
Cancellation closes streams and prevents late responses from expiring a session.
401 calls the explicit session-expiry boundary. Network/HTTP/schema failures
produce safe error codes; there is no silent mock fallback or automatic retry.
No requests were sent to production in this implementation or its tests.

## Evidence semantics

- Unknown JSON is validated and only selected presentation fields leave the
  adapter. Node IDs and topology references are scoped; duplicate resources,
  dangling edges, invalid dates, control characters and invalid values reject.
- Overview node health remains node-reported status, not tunnel delivery.
  Missing/future/stale observations and revoked nodes cannot become effective UP.
- `active` is a desired tunnel phase; all overview/list delivery health stays
  `unknown`. Tunnel `updated_at` is lifecycle age, not a measurement timestamp.
- Overview has no per-metric timestamp. Numeric rates/latency retain their value
  (including measured zero) but have UNKNOWN freshness; missing remains null.
  Reuse neither node `last_seen` nor HTTP fetch time to certify metric recency.
- Country coordinates are labeled `country-anchor`, not invented server GPS.
- `reassessOverview(snapshot, now)` must run when repainting cached evidence;
  it never renews generated/fetched timestamps. Three-minute TTL is a conservative
  client policy, not discovery of the server's configurable staleness setting.
- `exactCounter` preserves validated uint64 decimal strings; already-rounded JSON
  numbers outside the safe integer range become unavailable. A finance adapter
  still needs a reviewed exact-integer wire contract before integration.

Strings are plain text values. Future rendering must use text-safe primitives;
these validators are not an HTML sanitizer. Unknown newer roles/statuses degrade
to unknown. New API shapes need reviewed fixtures rather than unchecked casts.

A1 itself has no page or visual acceptance. The incremental A2 slice now adds an
opt-in static DEMO entry at `preview/index.html`, with FA/EN, RTL/LTR, dark/light
tokens, preview preferences and responsive resource read views. It is not BCC
embedded/routed, and cannot accept a live provider. No real reads/writes or polling
are activated. [A2 scope and open evidence matrix](../../docs/aurora/a2-shell.md)
distinguishes implemented source from browser/visual/mobile acceptance, which is
NOT RUN in this environment. The existing BCC/auth/CSRF/classic page is unchanged.

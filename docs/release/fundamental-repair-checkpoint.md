# Fundamental repair checkpoint — 2026-10-30

The implementation direction is incremental repair on the existing native
engine. The user delegated technical choices and asked to proceed using
Superpowers. The full program contract is in
`docs/superpowers/specs/2026-10-30-fundamental-repair-design.md`.

This checkpoint is **not a new release or whole-product acceptance**.

## Completed-Retire receipt retry

Draft PR #293 source candidate `9fc1c355b97547215dbeb0e4490b026ae14e4ccd`
matches local `77bf230` exactly at tree
`60c2067cbbec1262c72c0fced6c09814d49f12f6`.

- Tests-only `48d1a36`, recovery run `41096753430` attempt 2: fifteen PASS,
  one intended retry-stage FAIL, zero SKIP.
- Fixed candidate recovery run `41122465939`, job `124291958885`: all sixteen
  required real-kernel cases PASS, zero SKIP; host cleanup PASS.
- Artifact `13009476898` has GitHub-recorded ZIP SHA-256
  `6cbb23cd5046a37f89b1dafdbd0fbd1452d5614099ae9f8499acacacce67de55`.
- Operational ingress v1 `41122465995`, shared ingress `41122466115`, P0 LAB
  `41122466053`, static-analysis `41122465993`: success.
- General CI `41122466296`: fourteen jobs success; test job failed
  `TestCrossTenantRouteAccessRefused` when an expected denial reset arrived
  during connect. This run is not green. The test-oracle correction below is
  separate from the ingress executor patch.
- Fresh-context review found no Critical/Important defect; additional direct
  completed-Retire coverage of journal/chain/inventory/source-pin drift and TCP6
  rebound remains a minor recommendation.

This evidence covers the isolated executor/kernel LAB, not BCC HTTPS receipt
delivery, ordinary-host execution scope or durable service availability.

## Native listener lifecycle

Broad local tests exposed `TestNoiseMorphingRuntime` returning with one flow
and 65536 receive bytes still allocated. A twenty-run reproduction failed.
The new deterministic session-teardown barrier failed on the old code with
`listener Runtime.Run returned while session teardown was held`.

HTTP/2 reset can mark a stream idle before its handler finishes. The repair
tracks application handlers independently of HTTP connection state, closes new
admission, cancels the child lifetime, closes owned connections, joins handlers
and joins the serving goroutine on exit. The normal and recovery session paths
share this lifetime.

WebSocket hijacked connections need their own cancellation close. Real TLS
tests for an idle Noise handshake and an authenticated blocked read both failed
before the change. Cancellation now closes their underlying socket.

Fresh-context review caught an additional Important close race: graceful Close
could hold `closeOnce` while blocked writing a control frame. Forced close now
closes raw I/O before waiting for that Once. A real net.Pipe/write-entry barrier
test reproduced the failure and then passed after correction. Its final RED
log contains the assertion, not a build error or a test-framework deadlock.

Observed scoped results:

- Session-teardown barrier plus Noise round-trip: ten race-detector repetitions
  PASS.
- Node, H2, utlsdial, WS and session package race suites: PASS after the review
  fix.
- Initial complete local Go run with listener changes: fifty packages PASS;
  five inventory tests FAIL in the restricted environment, including inability
  to inspect PID1/process namespaces and chown protected fixtures. It is not an
  all-green run. Final source qualification and any further failures are recorded
  in the PR; do not infer them from these scoped results.

Source plans and RED/GREEN excerpts live under `docs/superpowers/` and
`docs/release/evidence/fundamental-repair/`. A missing planned direct Serve-error
regression is a deferred minor coverage gap; the error path was inspected but
not claimed independently exercised.

## Cross-tenant authorization test

The expected EX `ROUTE_DENIED`/`ROUTE_NOT_FOUND` makes the IR abortively close
the local socket. A fast RST can surface either during connect or during Read.
The original test incorrectly required connect to succeed. A sixty-run local
reproduction also failed before the correction.

The test now waits for the actual route-bind event, accepts only ECONNRESET as
the alternate connect outcome, and requires the exact route-correlated EX
protocol refusal in both cases. It still requires zero delivered bytes, no
cross-tenant target data and a working authorized route. Sixty race-detector
repetitions PASS. No production authorization behavior changed.

## Open lifecycle and product work

Failed Apply still needs BCC/Agent reservation reconciliation. The production
host execution profile, listener restoration after Agent death/reboot and the
primary operational Wizard remain open. Further native shutdown issues found
while tracing this bug include the Runtime metrics-error branch not joining its
role and dialer cleanup not explicitly joining every peer/recovery goroutine;
they require separate regression-first work. No published release contains the
changes described here at this checkpoint.

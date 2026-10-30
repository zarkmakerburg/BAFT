# Native Listener Shutdown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make listener Runtime.Run completion include owned carrier-handler teardown.

**Architecture:** Track admitted HTTP handlers with a mutex-protected shutdown gate and WaitGroup in a focused node lifecycle file. Cancel the request lifetime and close the HTTP server before joining handlers. Explicitly close hijacked WS underlying sockets on request cancellation.

**Tech Stack:** Go 1.27.3 standard library; existing H2/WS/Noise/session fixtures.

**Spec:** `docs/superpowers/specs/2026-10-30-runtime-shutdown-design.md`.

## Global Constraints

- No protocol, authentication, recovery authority or resource-accounting changes.
- No arbitrary sleep replaces the lifecycle completion barrier.
- Shutdown racing requests return HTTP 503 before creating sessions.
- Keep this change separate from ingress ownership and preserve unrelated services.

## Review Focus

- Idle H2 connection after RST does not imply completed handler cleanup: barrier regression.
- An upgraded WS client sends no Noise bytes: server cancellation closes the underlying socket.
- An authenticated WS stream is blocked reading: cancellation interrupts the read.
- A handler arrives after shutdown gate closes: 503 and no invocation.
- Serve fails without parent cancellation: the listener child lifetime still cancels and joins handlers.

### Task 1: Join the listener's handler lifetime

**Files:** Create `internal/node/listener_lifecycle.go`; modify `internal/node/node.go`; create `internal/node/listener_lifecycle_test.go` and `tests/integration/runtime_shutdown_test.go`.

**Interfaces:** Consumes parent context and final carrier http.Handler. Produces `newListenerLifecycle(context.Context, http.Handler) *listenerLifecycle`, implementing `http.Handler`, with child `ctx` and `close(*http.Server)` that closes admission, cancels, closes connections and joins admitted handlers.

- [ ] Add `TestListenerShutdownWaitsForSessionTeardown` using the existing real runtime-pair fixture and run-exit observer barrier; run it and observe early-return FAIL before code changes.
- [ ] Add lifecycle tests for rejected late requests and joining a canceled handler whose cleanup is held by a channel.
- [ ] Implement the focused lifecycle and wire server Handler/BaseContext and deferred cleanup in `runListener`; no broad formatting rewrite.
- [ ] Run the barrier test and existing `TestNoiseMorphingRuntime` repeatedly; authenticated WS cancellation prerequisite is Task 2, so do not claim the complete node lifecycle is ready before it passes.

### Task 2: Unblock hijacked WebSockets on lifetime cancellation

**Files:** Modify `internal/carrier/ws/noise.go`; create `internal/carrier/ws/noise_cancel_test.go`.

**Interfaces:** Consumes request-context cancellation from Task 1 and existing `Conn.closeUnderlying()`; does not change the public carrier interface.

- [ ] Add real-TLS-server tests for canceled idle Noise handshake and canceled authenticated blocked read; run them before code changes, expect handler completion timeout.
- [ ] Immediately after successful Upgrade register `context.AfterFunc` calling `conn.closeUnderlying`, and defer stopping the callback plus closing the underlying connection for all paths.
- [ ] Run WS tests and both node lifecycle regressions under race; all must pass without waiting for the Noise handshake timeout.
- [ ] Run node, carrier and session suites, then the full project suite; record every failure and opt-in skip. Obtain fresh-context review and address confirmed important defects with regression-first changes.
- [ ] Commit the isolated repair and publish a draft PR with accurate validation and open product limitations.

## Execution

Inline execution selected under the user's delegation. Task 1's handler join depends on Task 2 unblocking hijacked connections, so qualify and commit both together. Do not merge or claim whole-product readiness from this repair.


### Additional observed regression: route denial may reset connect

Hosted CI41122466296 and a local sixty-run reproduction failed
`TestCrossTenantRouteAccessRefused`: the expected OPEN_ERR abortive close can
reach the local client during connect, before Read. The test must accept only
ECONNRESET at Dial and still require the route-correlated protocol denial and
unchanged target counter. Await `DialerReady()` instead of probing denied routes;
use buffered refusal events rather than polling. No production auth changes.
The corrected test must pass sixty race-detector repetitions. This ruling adds
only a test-oracle correction supported by an observed failure and confirmed
kernel connect/SO_ERROR behavior.

Review fix: forced WS close must close the raw socket BEFORE waiting on the
shared graceful-close Once. `TestForcedCloseInterruptsBlockedGracefulClose`
uses a real net.Pipe and an entry-to-Write signal to reproduce the blocked
control-frame path; it failed before the change and passes after it. This is
part of Task 2's cancellation guarantee, not an additional feature.

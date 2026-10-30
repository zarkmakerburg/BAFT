# Native listener shutdown joins owned handlers

The fundamental repair preserves the native engine while fixing actual lifecycle
failures. Broad tests and a twenty-run reproduction exposed
`TestNoiseMorphingRuntime`: `Runtime.Run` returned while the listener still held
65536 receive bytes and one flow. The HTTP/2 server can mark a reset stream idle
before its handler finishes `Peer.Run` teardown. `http.Server.Shutdown` is not an
application-handler completion barrier. WebSocket hijacked connections are also
outside its shutdown ownership.

Decision delegated by the user: repair this lifecycle in place, in a separate
change from ingress ownership. Do not alter protocol, recovery authority or
resource accounting to make snapshots appear empty.

## Contract

- Listener runtime exit closes handler admission, cancels the listener lifetime,
  closes server-owned connections and waits for every admitted handler to finish.
- Register HTTP handlers before they can create sessions, including authenticated
  and pre-handshake requests. Requests racing shutdown are refused with HTTP 503.
- The request context retains normal HTTP cancellation and derives from the
  listener lifetime through `http.Server.BaseContext`.
- WebSocket cancellation closes the underlying socket without waiting for a
  protocol Close frame or write mutex. This covers stalled Noise handshakes and
  active blocked reads/writes, which otherwise prevent the handler join.
- Listener Serve failures execute the same cleanup as parent cancellation.
- Existing authentication, cover behavior, tenant reload and Noise timeouts stay
  in force. No arbitrary sleep replaces the lifecycle completion barrier.

## Alternatives

Waiting for resource counters to fall to zero would hide which work owns them.
Increasing test sleeps would leave runtime callers exposed to early completion.
Explicit handler ownership is selected because it closes the missing boundary
without changing session semantics. This is an engineering choice under the
user's instruction to proceed autonomously.

## Acceptance

A real two-runtime recovery-enabled H2 session is stopped while its existing
run-exit observer deliberately holds teardown. Listener Runtime.Run must not
return before that observer is released. After release, shutdown and resource
release complete. WebSocket tests use a real TLS server and upgraded socket:
canceling server lifetime must interrupt both an idle Noise handshake and an
authenticated stream read. Existing Noise round-trip tests and native node,
carrier and session tests run under the race detector. The full project suite
is run with all failures/skips reported; environment restrictions are not green.

This change does not supply reboot durability or managed-ingress production-host
support. The separate Runtime.Run metrics-error/role join is recorded for its
own regression rather than silently bundled here.

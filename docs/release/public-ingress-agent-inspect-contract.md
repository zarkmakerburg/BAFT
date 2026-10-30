# P0 Public Ingress — Signed live-inspect contract (implementation gate)

Status: **DESIGN CONTRACT ONLY — NOT IMPLEMENTED**. This document does not
permit Agent mutation, BCC `APPLIED` transitions, merge, or release.

## Admission and identity

1. Dispatch only via existing BCC Ed25519-signed Agent jobs. Verify pinned key,
   destination node, expiry, unique job ID, strict action allowlist and exact
   typed parameters before touching the filesystem or networking.
2. Require BCC's independently persisted expected generation and manifest
   digest, tunnel/instance identity, and distinct listener/firewall resource IDs.
   Job parameters must bind this whole tuple; never infer expected values from
   the Agent's own response.
3. Read only a pre-existing trusted Agent-owned manifest directory. Verify
   prepared bytes under `ingressmanifest.Store.Inspect(expected)` and reject
   cancelled, missing, mismatched, or uncertain generations. A prepared manifest
   proves neither an installed listener nor firewall ownership.

## Host observation

4. Observe the configured listener in the correct network namespace *and*
   prove its controlling BAFT-owned service identity, exact listen address,
   target, and generation. Port occupancy alone is not ownership; generic
   `/proc/net/tcp*` snapshots are insufficient.
5. Observe the scoped firewall rule by BAFT-issued immutable owner ID and
   exact protocol/address/port/direction. A firewall binary's presence or an
   arbitrary rule on the same port is insufficient. Do not rewrite unrelated
   rules, including existing 2053 listeners.
6. Return a versioned, bounded, duplicate-key-rejecting receipt bound to the
   signed job ID, node, generation, manifest digest and both resource IDs,
   plus listener/firewall observed+owned booleans. Include inspection time,
   boot identity and explicit uncertainty flags; reject stale or ambiguous
   observations. Inspect must never install, restart, delete or alter anything.

## BCC acceptance and recovery

7. Accept the receipt only against its dispatched job and independent durable
   expected proof, with replay/expiry fencing. Unknown ownership, wrong
   generation, foreign resources, and conflicting port mapping are BLOCKED.
8. Mark APPLIED only if both resources are live, owned and proof-matching
   (`TransitionVerifiedPublicIngressApplied`); mark REMOVED only after actual
   proven absence following authorized retire. A partial deployment is
   INSPECT/explicit recovery, never blind reapply (`PlanPublicIngressRecovery`).
9. On restart, re-inspect every persisted resource before making state claims.
   No inference from cached READY or prior manifest bytes. An incomplete
   inspection must preserve the last safe state and produce an actionable alert.

## Acceptance matrix before enabling the job

- Wrong node, signature, job ID, expiry, replay, manifest digest or generation;
  each fails without host mutation.
- Listener exists but is owned by another process; firewall exists but belongs
  to an unrelated service; both fail ownership checks.
- Listener only / firewall only; reports partial and requires explicit recovery.
- Reboot, crash after prepared manifest, crash after listener, crash after
  firewall, and incomplete rollback; verify no unsafe APPLIED claims.
- Verify 2053 existing service remains untouched, plus 54619 client path and
  documented limitations of 3X-UI subscription rewriting.

Only after the above and end-to-end tests pass should a separate owned-resource
prepare/commit/rollback implementation enter the Agent allowlist.

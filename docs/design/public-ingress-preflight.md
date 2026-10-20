# Public ingress: read-only host preflight

Status: implemented on the v0.4.0 development branch; **not public ingress deployment**. The floating preview wizard does not invoke this API yet.

## Request and result

An authenticated BCC administrator can POST to `api/ingress/preflight` under the configured BCC base path. Browser sessions require the normal CSRF header. BCC must have its job-signing key configured.

```json
{
  "ir_node": "ir-node",
  "tunnel_id": "existing-tunnel-id",
  "public_port": 54619,
  "local_target": "127.0.0.1:15443"
}
```

The node must be enrolled, unrevoked, have an eligible IR role and authenticated agent contact within five minutes. The tunnel must already belong to that IR node, and the loopback target must match its canonical route listener. Requests reject unknown fields, trailing JSON and bodies over 4 KiB. At most one queued/dispatched ingress preflight per IR node is allowed. A pending job is not a port reservation.

If delivery or ACK is lost, a new request after five minutes atomically marks the old pending read-only job failed and queues a fresh job with a new identity. The replacement audit records the expired job IDs; a failed save rolls both changes back. Late ACKs cannot revive expired jobs. This recovery applies only to read-only ingress preflight and never retries a mutating job or grants host ownership.

BCC atomically persists the job and audit intent and returns HTTP 202 with `read_only: true` and `ready_to_apply: false`. The normal authenticated pull/sign/verify/ACK protocol executes `public_ingress_preflight`; the agent needs a build containing this handler. Older agents are not certified as compatible by this endpoint.

An authenticated GET to `api/ingress/preflight?id=JOB_ID` returns only this action's job ID, node, status, timestamps and validated evidence. Other job types are not exposed. No generic job parameters, secret outputs or arbitrary job messages are returned. Successful evidence is checked again when read; invalid stored evidence is refused.

## What is observed

The agent reads bounded `/proc/net/tcp` and `/proc/net/tcp6` snapshots in its own network namespace. It does not bind or connect to a port, send a probe, execute shell/firewall commands, create configuration, restart a service or modify a tunnel. Firewall executable discovery only checks availability; it does not inspect effective firewall policy.

| Evidence | Meaning |
|---|---|
| `occupied` | At least one TCP LISTEN socket on the requested public port was observed. Ownership is unknown. |
| `not_observed` | Complete snapshots did not show a listener. This is **not** proof that a bind will succeed, and not a reservation. |
| `unknown` | Snapshot data was missing, malformed, truncated or over its bound. Missing data never means free. |
| `target_listener_observed` | A matching loopback/wildcard listener was observed. No connection or end-to-end delivery was tested. |
| `firewall_tools` | Executables found on the agent PATH; no command was executed or policy verified. |

`ready_to_apply`, `ownership_verified` and `firewall_policy_verified` must explicitly be false. Successful ACKs reject missing, duplicate, unknown or null fields, malformed/trailing JSON, contradictory socket states and a tunnel/port/target tuple different from the dispatched job. Authorizing claims are refused before any ACK state change. Failed and legacy job ACK behavior is unchanged. Observations are historical, untrusted host claims; they never authorize ingress apply.

## Verified scope and remaining work

Tests exercise real BCC HTTP dispatch, signed pull, live local listener observation without a connection, ACK validation, durable reopen and authenticated result retrieval. They also cover revocation/offline/role/ownership/target failures, signing requirements, CSRF, replay rejection and malformed/authorizing receipts.

Still missing: durable public-port ownership/reservations; wizard and deployment orchestration; generation/digest-bound prepare/commit/inspect/rollback/retire actions; managed listener/firewall ownership; reboot reconciliation; public-client end-to-end traffic and production staging acceptance. Existing socat-owned legacy listeners, SSH and 3X-UI remain externally owned and untouched.

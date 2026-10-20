# Public ingress: durable desired reservations

Status: development API. A reservation is BCC bookkeeping; it does **not** bind a socket, own a host resource, open a firewall or authorize deployment. IR public ingress remains separate from IR–EX transport.

## API

An authenticated administrator may POST the same four-field spec used by [read-only preflight](public-ingress-preflight.md) to `api/ingress/reservations` under the BCC base path. Browser sessions require CSRF. Bodies are limited to 4 KiB; duplicate, unknown, missing and trailing JSON fields are rejected.

```json
{
  "ir_node": "ir-node",
  "tunnel_id": "existing-tunnel-id",
  "public_port": 54619,
  "local_target": "127.0.0.1:15443"
}
```

The enrolled, unrevoked IR node and the existing tunnel must match, including its canonical loopback route. The same immutable spec is idempotent; another tuple for that tunnel or another tunnel on the same node/port is refused. Concurrent creates serialize under the store lock. The desired record and audit intent commit together; failed persistence restores both in memory.

Authenticated GET lists validated desired reservations. Responses explicitly report `desired_only: true`, `ownership_verified: false` and `ready_to_apply: false`. No agent job or host mutation is created by this endpoint.

## Conflict protection

Port 2053 is protected for the existing externally owned service; port 22 and recorded management/SSH migration ports are protected. Reservations also reject managed transport, loopback route, metrics and pending path-probe port conflicts. The reverse gates protect reservations when planning/creating tunnels, updating a node's management address, planning/starting SSH migration or creating explicit/automatic path probes. Unknown host listeners still require a later agent ownership/bind check; BCC bookkeeping cannot certify them absent.

There is intentionally no edit/delete/release endpoint yet. Failed or removed tunnels do not silently free these desired ports. An orphaned reservation remains allocated during restore. This conservative behavior can retain an unused port; generation-bound agent ownership and recovery must exist before an operator can safely release it through the product.

## Persistence, restore and downgrade

SQLite schema 14 persists reservations in their own table. Encrypted backups written by this candidate use schema 3; legacy schema 2 backups are accepted only without the new reservation state. Older binaries refuse the new database/backup versions rather than silently dropping allocations.

Restore preview and commit preserve current reservations when an older backup lacks them and reject changed tuples or conflicts with the merged restored resources. Restore never acts as a reservation release mechanism. Retain a verified **pre-upgrade** database/backup and previous trusted binaries for an actual downgrade; an old binary cannot open the newly migrated database or restore a schema 3 backup. No production downgrade or restore drill is claimed by these development tests.

## Remaining operational gate

The floating ingress wizard is still preview-only. Durable agent generations/digests, prepare/commit/inspect/rollback/retire, owned listener/firewall resources, reboot reconciliation and public-client traffic acceptance remain prerequisites for ingress deployment. Reservations and historical preflight observations cannot substitute for those gates.

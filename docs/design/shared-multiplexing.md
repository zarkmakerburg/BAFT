# Shared transport multiplexing (v0.4.0, P0 #2 / #269)

Status: design + first slice. Branch `claude/v040-multiplex`.

## 1. What exists today (verified in code)

- One EX listener (`internal/node.runListener`) already serves one TLS/Noise
  endpoint, authenticates every IR by its pinned Noise static key
  (`noise.allowed_peer_public_keys`: identity -> key) and builds one
  `session.Peer` per authenticated carrier.
- Inside a session, flows are already multiplexed: `OPEN{route_id}` is resolved
  by `routes.Table.Resolve(peerID, routeID)` where `peerID` is the identity
  proven by the Noise handshake. A route has a fixed `target` and an
  `allowed_peers` allowlist, so ACL is enforced on the EX from authenticated
  identity, never from a field the IR supplies (the route id only selects).
- Per-session cap `maxFlowsPerShard`=64 (advertised in HELLO_ACK), node-wide
  `limits.max_flows` (`resources.FlowSlots`), shared memory `Allocator`, and a
  per-session DRR sender (`internal/scheduler`) for fairness between flows.

What is missing: (a) the table, the allowed-peer set and the Noise handler are
built once at start, so adding a tenant means restarting the EX (kills every
live session); (b) route ids are one global namespace, so two tenants cannot
both use `port-443`; (c) no per-tenant flow cap; (d) the pairing tools only
create a new listener+port+unit per instance.

## 2. Model

- Sharing unit = one EX listener/port/process. Tenant = one authenticated IR
  identity (Noise key). One tenant = one session (x shards) to the EX; all its
  tunnels (routes) share that session. Different tenants do NOT share a
  session or a Noise key: a shared session would need in-session tenant
  authentication, which widens the trust boundary for no gain.
- A flow carries only `route_id` (existing OPEN frame, unchanged). The EX
  resolves `(authenticated peer identity, route_id) -> target`. Route ids are
  scoped per tenant: the same id may exist for different tenants if their
  `allowed_peers` sets are disjoint; overlapping duplicates stay rejected.
- Isolation: a flow naming a route the identity is not allowed on gets
  `ROUTE_DENIED` (existing id on other tenant) or `ROUTE_NOT_FOUND`; the
  resolved target always comes from EX config. Removing a tenant swaps the
  Noise handler first (no new auth), then cancels that identity's sessions.
- Backpressure/fairness: inside a session, DRR across flows (existing).
  Across tenants, transports are independent (own carrier, window, replay
  buffers). Shared scarce resources get a per-tenant ceiling:
  `limits.max_flows_per_peer` (child of the node-wide slot pool, so one tenant
  cannot starve others of flow slots or, via the per-flow memory caps, of the
  shared Allocator).
- Failure isolation: a tenant stall/crash only ends its own session context;
  other tenants' sessions, flows and targets are untouched. A tenant reload
  never touches a session whose identity is still allowed.

## 3. Live tenant change (the actual multiplexing gap)

`Runtime.ReloadListener(cfg)` (SIGHUP in `baft run`, re-reads the config file):
validates the new config; refuses (keeps old state, returns error) if anything
other than `routes`, `server.allowed_peer_identities`, `noise.allowed_peer_*`
changed; otherwise atomically installs the new route table and Noise handler.
Existing flows and sessions of unchanged tenants are not interrupted. Only
Noise listeners support it; mTLS-only listeners return an error (no fallback).
Config files are written by `baft-pair ex-accept --join` (atomic rename,
validated before write), which appends the tenant to an existing EX config.

## 4. Compatibility / negotiation

The first slice changes no wire frame. v0.3.7 IRs already speak route-scoped
OPEN over their own session, so they interoperate unchanged and keep the
per-tunnel model (new EX config fields are optional, old configs load as is).
Capability `multiplex-v1` is reserved for later wire features (explicit
per-tenant credit, route-bind frames, several tenants per session). It will be
advertised and used only when both HELLOs carry it (branch
`claude/v040-negotiation` owns the capability set; not present when this was
written, so nothing here depends on it).

## 5. Smallest honest first slice

Live add/remove of tenants on one EX listener; tenant-scoped route ids;
`max_flows_per_peer`; `baft-pair ex-accept --join`. Tests: concurrent
independent traffic with checksums, third tenant added while two stream,
removal/crash of one, cross-tenant refusal, caps, restart, `-race`, and a
real-binary e2e (two IR tunnels, one EX listener).

Not in the slice: several tenants in one session, wire capability, per-route
caps, shared-listener creation through `tunnelnode` transactions (systemd
units, metrics claims) and BCC exposure; those remain gaps in the gate row.

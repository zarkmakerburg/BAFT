# P0 — One-click tunnel creation and automatic public ingress

Tracking: #269; HQ #67. This document is a design checkpoint, not a deployed implementation.

## Existing code integration points
- `internal/bcc/tunnel.go`: signed two-node prepare/commit/health/finalize and rollback lifecycle.
- `internal/bcc/ingress_policy.go`: route-level IR eligibility/selection, not a public TCP port allocator.
- `internal/bcc/smart_ingress.go`: desired/applied endpoint publication, not proof of host-level TCP listener creation.
- `internal/tunnelnode/`: agent-side tunnel lifecycle.
- `web/bcc-app.*`: wizard UI integration.

## Product contract
Basic wizard: select IR node, EX node, EX target TCP port, and public client port. Default public client port = EX 3X-UI inbound port. Subscription domain is NOT a BCC field; 3X-UI manages links and users. Optional Advanced panel exposes dedicated/shared inter-node transport and compatibility details. Never imply changing the 3X-UI custom share address creates an ingress listener.

## Provisioning contract
Preflight IR port occupancy and ownership; allocate/reuse only owned resources. Bind public TCP listener to the specific tunnel's local ingress. Apply firewall change narrowly, persist across reboot, verify actual socket and end-to-end delivery, record desired/applied status, and rollback atomically. Preserve existing 2053 Imperial Crown and all non-owned services. On edit/decommission, preserve sibling tunnels and shared resources until last owner releases them.

## Shared transport gate
Default to TCP/443 only when available and compatible; port alone is not a stealth guarantee. Multiple logical tunnels can share 443 ONLY if native engine proves authenticated stream multiplexing/demultiplexing, per-tunnel routing/isolation, quotas, concurrency and teardown. Otherwise use dedicated listeners and present an actionable explanation. Never bind two independent TCP listeners to the same address:port.

## Required tests
Unit: port conflict, idempotency, ownership, shared refcount, rollback, restart. Integration: 54619 public -> IR local ingress -> BG 127.0.0.1:54619; existing 2053 unaffected; multi-tunnel shared listener; subscription output uses IR address and matching port; FA/EN RTL/LTR mobile. Real-host test is a separate gated activity. No merge/release/deploy without explicit HQ permission.

## Next implementation
1. Map current agent listener/firewall and route provisioning functions and existing tests.
2. Add public-ingress desired-state model and validation to tunnel plan.
3. Implement agent-side owned TCP listener and firewall reconciliation with rollback.
4. Wire simplified wizard and prove CI and end-to-end behavior.

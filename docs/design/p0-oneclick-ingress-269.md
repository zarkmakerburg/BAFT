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

## P0 — Linux installer audit and optimization (added 2026-10-20)

**Scope:** `install.sh` and its called shell helpers, supported Linux distributions, release/offline/source install paths, SSH pairing, BCC-only and agent-only installation, update, downgrade, recovery and uninstall. This is a P0 safety and reliability track alongside Public Ingress, not authorization to change live servers.

**Priority P0-A: preservation and rollback.** Verify every installer path preserves existing BAFT and 3X-UI traffic, especially the legacy BAFT.goldapp.tr:2053 listener. Require an explicit confirmation before replacing an occupied port, service unit, config or firewall rule. Back up config, binaries, units and managed firewall state before mutation. On failure, restore only installer-owned resources and report exact recovery steps; never delete unrelated tunnel state.

**Priority P0-B: supply-chain trust.** Keep signed release verification against the pinned root key, enforce revocations, checksum and version/downgrade rules. Verify offline bundles with the same policy. Treat `BAFT_INSTALL_FROM=source` as a development-only path and do not silently fall back from verified release to unverified source. Ensure downloads have bounded retries/timeouts and atomic staging.

**Priority P0-C: predictable Linux behavior.** Audit root/dependency detection, systemd availability, package manager support, Linux architecture, firewall backend (nftables/iptables/ufw), SELinux/AppArmor and IPv4/IPv6 handling. Validate all ports and bind addresses before service changes. Avoid package installation or global firewall flushing when unnecessary.

**Priority P0-D: UX and observability.** Add a safe preflight / dry-run report covering selected role, target paths, ports, occupied sockets, compatibility, planned actions, backup location and explicit blockers. Print actionable Persian/English messages and stable exit codes; redact tokens, pairing codes and private keys from logs.

**Priority P0-E: reproducible tests.** Run `bash -n`, ShellCheck, shell unit tests and disposable Linux container/VM install-update-rollback scenarios across supported distributions. Include network outage, corrupt artifact, signature mismatch, disk full, service restart failure, rerun/idempotency, noninteractive SSH pairing and preservation of existing tunnel 2053. Run staging integration only after CI; no production deployment without HQ approval.

**Acceptance gates:** audited install matrix; tests and CI green; zero unintended host changes; proven rollback/idempotency; security review of privileged operations; documentation updated; HQ permission before any merge, release or deploy.

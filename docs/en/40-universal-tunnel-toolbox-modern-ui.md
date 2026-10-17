# BAFT Stage267 — Universal Tunnel Toolbox and Modern UI Research

Product goal: After installing BAFT and BCC, an operator should eventually create, configure, monitor and manage supported independent tunneling engines without third-party ad-hoc installer scripts. **Current truth:** native BAFT Secure is the only driver with reviewed deployment; FRP, Rathole, Backhaul, WireGuard, SSH SOCKS, WSS and QUIC are roadmap entries, not deployed engines. WSS/QUIC are carrier transports rather than independent engines. A capability being listed is NOT evidence that the binary is installed or the path works.

**Implemented:** authenticated, GET-only `/api/tunnel-engines` returns a server-owned, fail-closed catalog. The BCC Aurora Toolbox cards show family/direction, actual build readiness, prerequisites, and plan entry only for BAFT. The real BCC plan/create API additionally rejects every unimplemented engine. If catalog fetch fails, the UI clears and disables all actions. The catalog performs no downloads or writes. Native BAFT's connection is IR dials EX, not the reverse EX dials IR used by proposed reverse adapters.

## Original UI/UX study (design patterns only, no third-party code/assets reused)

| Reference | Source license | Applicable pattern | Priority |
| --- | --- | --- | --- |
| [Arange-tun](https://github.com/MAHDI-byte64/Arange-tun) | AGPL-3.0 | Topology/family chooser, driver tutorials, separate control and exposed ports, lifecycle | P0 |
| [Homarr](https://github.com/homarr-labs/homarr) | Apache-2.0 | Customizable widgets, responsive card dashboard and search | P1 |
| [NetBird dashboard](https://github.com/netbirdio/dashboard) | AGPL-3.0 | React Flow-inspired topology and peer inspector | P1 |
| [Beszel](https://github.com/henrygd/beszel) | MIT | Freshness-aware agent metrics, trends and actionable alerts | P1 |
| [Dockge](https://github.com/louislam/dockge) | MIT | Per-service lifecycle status, logs and safe restart actions | P1 |
| [Pangolin](https://github.com/fosrl/pangolin) | AGPL-3 + commercial | Secure resource setup flow and access control | P2 |
| [3x-ui](https://github.com/MHSanaei/3x-ui) | GPL-3.0 | Xray inbound and client lifecycle distinct from inter-node tunnel | P2 |
| [Homepage](https://github.com/gethomepage/homepage) | GPL-3.0 | Compact, searchable widgets and service shortcuts | P2 |

These references are a grounded repository review, not an exhaustive interactive audit. Maintain existing BCC operator features while integrating the Aurora visual language. Required design: mobile-first, bilingual FA/EN, RTL/LTR, light/dark, accessible controls, reduced motion, and accurate evidence-backed state. No misleading live UI using dummy values. No GPL/AGPL code, graphics or CSS has been imported.

## Implementation gates

- **P0:** #228 real driver interface, #229 FRP, #230 Rathole. Signed and pinned artifacts, approval-gated installation, independent services, strict ownership, PlanHash, rollback, license provenance, two-node E2E.
- **P1:** Real public ingress and multiple port mappings, preflight, lifecycle actions, diagnostics, backups and a health-aware network graph; never silently assume public IR ingress exists.
- **P2:** Customizable workspace widgets, command palette, advanced Incident Center, gradual migrations from legacy scripts with dry-run and explicit ownership consent.

**Stage267 acceptance:** authenticated GET-only catalog; all planned engines refused in backend; UI enables only BAFT; loss of catalog data never marks an unavailable engine operational. Preserve unmodified signed v0.3.3 on real nodes; no production deployment without independent release gates.

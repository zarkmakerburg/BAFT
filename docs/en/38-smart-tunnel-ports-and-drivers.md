# BAFT Tunnel Toolbox — Service-Agnostic Engine Platform

BAFT and BCC exclusively install, orchestrate, configure, monitor, repair and remove **tunnel engines**, not applications behind tunnels. Application accounts, subscriptions, application protocols and service-specific client configs are outside BAFT.

## Operator flow

1. Choose a genuinely implemented tunnel engine and its direction and two endpoints. Only native BAFT is deployable today; all other engines remain unavailable until tested.
2. Choose the inter-server EX listener port separately from the **EX service destination IP:port**. Enter a fixed IPv4 or bracketed IPv6 endpoint directly. The destination starts empty and must be explicitly chosen. The optional Suggest destination port button uses crypto.getRandomValues and bounded rejection sampling for 20000–59999. It preserves the entered host; only an explicitly requested suggestion on an empty target uses 127.0.0.1. Current form ports and known occupied ports on the selected EX node are excluded. Suggestions do not scan or reserve OS sockets and never configure the destination service. Backend fixed-IP/port validation and real deployment gates remain authoritative.
3. The EX outer listener, local IR route, optional externally wired IR public ingress, and EX destination service port are different. This version does not automatically provision a public IR ingress; confirm independent ingress wiring if required.
4. Review PlanHash, conflict gates, Agent health, safe sibling isolation and rollback before deploy. Verify actual tunnel traffic without assuming a particular target application.

## Approved engine roadmap

P0: capability-driven fail-closed engine/driver contract; secure artifact provenance, pinned/signed binaries, licensing, safe agent jobs, per-instance lifecycle, secret handling, telemetry, backup, rollback, uninstall, restart persistence and two-node E2E.

P0: independently operational FRP and Rathole drivers, including topology roles and genuine TCP/UDP support where verified. Their UI options must remain disabled until all real gates pass.

P1: additional Backhaul, direct/reverse modes, WS/WSS, QUIC/UDP and composable recipes, multiple port forwards per tunnel, supported-by-agent inspection and dependency-free installs for the operator.

P1: ONE BCC toolbox for discovery, install, create, edit, start/stop, delete, monitor, diagnose, rollback and update. Preserve and merge existing BCC features with Aurora, not replace them. RTL/LTR, light/dark, mobile responsive, Network Intelligence Map, Studio and Incident Center all remain.

P2: field-test-grounded path admission, fallback and recommendations. No unsupported anti-detection claims, unauthorized deployments or unreviewed third-party code copying.

## Stage279 integration checkpoint

The service-port behavior from PR #231 and direction chooser from PR #234 are reconciled in draft PR #240 with the authenticated catalog from #237/#239 and the IPv6 edit fix from Stage278. This is a code integration on a feature branch, not a merge or production deployment. The redundant loopback-only port field is intentionally not retained: one target field is authoritative for manual entry, generation, editing and the reviewed request. Changing or suggesting a target invalidates the existing PlanHash; failed suggestions preserve the target and current plan.

Direct means IR initiates the inter-server link to EX. Reverse remains disabled and is rejected before a plan request; it is not encoded as a newly supported backend field. Catalog generation/token guards and unavailable-engine admission remain in place. Automated controller tests cover catalog → Studio → IPv6 port suggestion → reviewed plan → deployment request, plus stale-plan invalidation and unsupported direction/engine rejection. These fixtures are not live-node or browser E2E evidence.

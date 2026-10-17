# BAFT Tunnel Toolbox — Service-Agnostic Engine Platform

BAFT and BCC exclusively install, orchestrate, configure, monitor, repair and remove **tunnel engines**, not applications behind tunnels. No Xray, VPN app accounts, subscribers, inbound-management panels, application protocols or service-specific client configs belong to BAFT.

## Operator flow

1. Choose a genuinely implemented tunnel engine and its direction and two endpoints. Only native BAFT is deployable today; all other engines remain unavailable until tested.
2. Choose the inter-server EX listener port separately from the **arbitrary EX destination service port**. Enter it manually or generate a candidate from 20000–59999 using crypto.getRandomValues. Known reserved EX BAFT ports are excluded, but this is not a live operating-system socket-availability probe. Native BAFT target is 127.0.0.1:PORT (advanced target address is independently editable).
3. The EX outer listener, local IR route, optional externally wired IR public ingress, and EX destination service port are different. This version does not automatically provision a public IR ingress; confirm independent ingress wiring if required.
4. Review PlanHash, conflict gates, Agent health, safe sibling isolation and rollback before deploy. Verify actual tunnel traffic without assuming a particular target application.

## Approved engine roadmap

P0: capability-driven fail-closed engine/driver contract; secure artifact provenance, pinned/signed binaries, licensing, safe agent jobs, per-instance lifecycle, secret handling, telemetry, backup, rollback, uninstall, restart persistence and two-node E2E.

P0: independently operational FRP and Rathole drivers, including topology roles and genuine TCP/UDP support where verified. Their UI options must remain disabled until all real gates pass.

P1: additional Backhaul, direct/reverse modes, WS/WSS, QUIC/UDP and composable recipes, multiple port forwards per tunnel, supported-by-agent inspection and dependency-free installs for the operator.

P1: ONE BCC toolbox for discovery, install, create, edit, start/stop, delete, monitor, diagnose, rollback and update. Preserve and merge existing BCC features with Aurora, not replace them. RTL/LTR, light/dark, mobile responsive, Network Intelligence Map, Studio and Incident Center all remain.

P2: field-test-grounded path admission, fallback and recommendations. No unsupported anti-detection claims, unauthorized deployments or unreviewed third-party code copying.

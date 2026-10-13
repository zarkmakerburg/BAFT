# BAFT Documentation Guide

[مستندات فارسی](../fa/README.md) | [English root README](../../README.md) | [User guide](guide/README.md)

## Start here: the user guide

If you want to install BAFT and use it with x-ui, read the **[user guide](guide/README.md)** first: [install](guide/01-install.md), [use with x-ui](guide/02-use-with-x-ui.md), [panel](guide/03-panel.md), [network and BBR](guide/04-network-and-bbr.md), [update, backup, uninstall](guide/05-update-backup-uninstall.md), [troubleshooting](guide/06-troubleshooting.md), [FAQ](guide/07-faq.md).

## Operator documentation

How the installer, panel and tools behave in detail:

- [06 — Building and running IR/EX](06-running-ir-ex.md)
- [23 — Signed release artifacts and the offline bundle](23-p1a-signed-releases.md)
- [24 — BCC web access and SQLite state](24-p1c-bcc-access.md)
- [25 — Secure agent jobs](25-p1d-agent.md)
- [26 — Server inventory and SSH bootstrap](26-p1d-ssh-bootstrap.md)
- [27 — Native tunnel builder](27-p1e-tunnel-builder.md)
- [29 — BCC backups and restore preview](29-bcc-backup-restore-preview.md)
- [30 — BCC layered health](30-bcc-layered-health.md)
- [32 — The `baft` menu](32-baft-menu.md)
- [33 — The rerunnable installer (panel install, SSL, `baft-panel`, `baft-net`)](33-rerunnable-installer.md)
- [34 — Safe uninstall](34-safe-uninstall.md)
- [35 — Transactional certificate rotation](35-cert-rotation.md)
- [37 — Running BAFT behind Cloudflare (WebSocket carrier)](37-cloudflare-websocket.md)

## Technical documentation

This part is written for readers with no prior BAFT context. Recommended order:

1. [01 — Overview, goals, and scope](01-overview.md)
2. [02 — Architecture, roles, and data flow](02-architecture.md)
3. [03 — Security, identity, and trust boundaries](03-security-model.md)
4. [04 — BAFT/1 protocol and state machine](04-protocol-baft1.md)
5. [05 — Configuration and Routes](05-configuration.md)
6. [06 — Building and running IR/EX](06-running-ir-ex.md)
7. [07 — Memory control, backpressure, and scheduling](07-resource-control.md)
8. [08 — Testing, CI, and acceptance gates](08-testing-and-ci.md)
9. [09 — Roadmap and Stages A–H](09-roadmap.md)
10. [10 — Repository layout and module responsibilities](10-repository-layout.md)
11. [11 — Glossary](11-glossary.md)
12. [12 — Innovation methodology](12-innovation-method.md)
13. [13 — Stage-C TWRL hypothesis](13-stage-c-twrl.md)

## Source hierarchy

When documents disagree, use this order:

1. content Blueprint 1.4 and Implementation Master Prompt 1.2;
2. accepted ADRs for implementation decisions;
3. schema, wire tests, and golden vectors for executable contracts;
4. STATUS and TEST-RESULTS for what has actually been built and measured;
5. these guides for explanation.

A feature described by the Blueprint but not recorded as implemented in STATUS is a **planned contract**, not a current capability.

14. [14 — Stage-C PADL hypothesis](14-stage-c-padl.md)

15. [15 — Conservation telemetry and multi-Shard budget](15-conservation-telemetry.md)

16. [16 — Privacy-bounded conservation metrics](16-conservation-metrics.md)

17. [17 — Single-flight correctness gates](17-correctness-gates.md)

18. [18 — Stage-C soak gate](18-stage-c-soak.md)

19. [19 — Bounded terminal quarantine](19-terminal-quarantine.md)

20. [20 — superseded ECRL note](20-stage-d-ecrl.md)

15D. [ECRL — prior art, threat model, and formal invariants](15-stage-d-ecrl.md)

21. [21 — R3.1 Morphing (Stealth Pro, experimental)](21-stealth-pro.md)
22. [22 — Launch-1 roadmap](22-launch-1-roadmap.md)
23. [23 — P1-A: signed release artifacts](23-p1a-signed-releases.md)
24. [24 — P1-C: BCC web access and SQLite state](24-p1c-bcc-access.md)
25. [25 — P1-D: secure agent jobs](25-p1d-agent.md)
26. [26 — P1-D: server inventory and SSH bootstrap](26-p1d-ssh-bootstrap.md)
27. [27 — P1-E: native tunnel builder](27-p1e-tunnel-builder.md)

28. [28 — BCC visual system](28-bcc-visual-system.md)
29. [29 — BCC backups and restore preview](29-bcc-backup-restore-preview.md)
30. [30 — BCC layered health with hysteresis](30-bcc-layered-health.md)
31. [31 — Discovery of existing tunnels (report only)](31-tunnel-discovery.md)
32. [32 — The `baft` menu and the terminal header](32-baft-menu.md)
33. [33 — The rerunnable installer](33-rerunnable-installer.md)
34. [34 — Safe uninstall](34-safe-uninstall.md)
35. [35 — Transactional certificate rotation](35-cert-rotation.md)
36. [36 — Adversarial resilience soak](36-adversarial-resilience.md)
37. [37 — Running BAFT behind Cloudflare (WebSocket carrier)](37-cloudflare-websocket.md)
40. [40 — Engineering notes (moved from the front page)](40-engineering-notes.md)

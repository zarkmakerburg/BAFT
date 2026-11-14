# 09 — Roadmap and Stages A–H

Each Stage has deliverables and an evidence gate.

## Parallel UX-0 track — first-time, non-technical users (P0)

Goal: a new operator can sign in and create the first tunnel through a safe, understandable in-panel flow, without reading engineering documentation or running shell commands. This track runs alongside protocol/engine stages and is a prerequisite for broadening the general-user experience.

### P0 — remove dead ends and complete first use

- [ ] **Empty wizard recovery:** when either the Iran or foreign server role is missing, show one clear “Go to Servers” action, close the wizard, and navigate directly to server enrollment. Initial implementation is in the UX PR; final acceptance requires CI and browser evidence.
- [ ] **Unambiguous entry:** the public page offers product information and sign-in only; tunnel creation is available only after authentication inside BCC. Keep the public product page separate from the private console entry.
- [ ] **First-run guide:** show the sequence: sign in → add foreign server → add Iran server → verify both are online → create tunnel → confirm the outcome. Explain prerequisites, waiting states and the next action at each step.
- [ ] **Plain language:** keep internal terms such as ECRL, DRR, Carrier, Epoch, Shard and engine names out of the normal user path; expose them only under technical details with a short explanation.
- [ ] **Actionable states:** empty, loading, error, offline, stale, partial and unknown states each need a clear explanation and next action. UNKNOWN or stale data must never be presented as healthy.
- [ ] **Understandable failure/recovery:** preview impact before changes; after failure, show rollback outcome, what did/did not change, and the safe next step. Retry is enabled only when operation state is known.
- [ ] **Progressive disclosure:** keep common settings in the main path and technical options under Advanced. Explain defaults; auto-selection may use only verified capabilities.
- [ ] **Accessibility and mobile:** run real-browser checks for Persian RTL and English LTR, mobile/desktop widths, keyboard/focus, 200% zoom, reduced motion, contrast and readable text sizes.

### UX-0 acceptance criteria

1. No primary path ends in a dead end when a server, data point or permission is missing.
2. A first-time user can create the first tunnel using in-panel guidance; shell access and engineering docs are not required for the normal path.
3. Before deployment, the plan and its changes are understandable and reviewable; required consent is never bypassed.
4. Success is reported only from BCC-confirmed state; partial, stale or unknown outcomes remain explicit.
5. All primary flows are tested in a real browser across both languages and target viewport sizes; DOM or preview checks alone are not acceptance.

**Execution order:** fix dead ends and first-run guidance (P0), simplify dashboard/status language (P1), then visual polish and customization (P2). This track does not replace or bypass the security/correctness gates in Stages A–H.

### Execution slices from the independent UX audit

1. **This PR gate — empty-state recovery and accessibility:** keep initial focus inside the Wizard after inventory loading; give the Servers empty state a visible action that opens enrollment; cover both in real-browser regression tests. The final SHA must also have a successful latest required CI attempt and be mergeable under GitHub policy.
2. **UX-0.1 — enroll the first server (P0), implemented; awaiting usability-gate evidence:** explain above the SSH form what VPSs are needed (one foreign and one Iran server), where to find host/IP, SSH port, username and authentication method, include a safe example, explain EX/IR/master roles, and link to the install guide. Never place real credentials in examples or logs.
3. **UX-0.2 — one tunnel-creation path (P0):** make the Tunnels page a tunnel list plus one primary create action; hide/collapse unavailable engine cards and the legacy studio from the normal path, and foreground only the supported workflow.
4. **UX-0.3 — reliable Persian localization (P0):** user-visible Go/API strings must have localization keys or valid Persian fields; remove long English engineering sentences from primary pages and keep raw server responses under technical details only.
5. **UX-0.4 — recover from failure (P0):** failed/partial/attention outcomes need a plain-language summary and a direct “Diagnose” action that opens diagnostics for the affected tunnel; retry is enabled only when operation state is known.
6. **UX-0.5 — trust, mobile and accessibility (P1):** review progressive navigation, the mobile public page, version stamping, error copy and placeholders, focus in every dialog, contrast and 200% zoom.

**UX-0 exit gate:** at least five testers without SSH experience, given two test servers and only in-panel guidance, complete sign-in through verified tunnel creation in Persian and English at 390 px and 1280 px within 20 minutes; no terminal failure is left without a recovery action. Until this gate passes, BAFT is not considered ready for public release to non-technical users.

- **A — contract/spike:** TLS/H2 ADRs, protocol vectors, config schema, full-duplex mTLS Carrier. Completed for its scope.
- **B — secure vertical slice:** fixed Routes, Flow protocol, identity/authorization, revocation, CLI, and COR-01. Completed for its defined scope.
- **C — resources/multi-Flow:** global allocator, receive/replay reservation, DRR, bounded control queue, multi-Flow/multi-Shard and slow-receiver/no-OOM gates are integrated. The recorded [Stage-C CI soak passed](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903062) (about 2m 11s; a bounded CI soak, not long-duration endurance). Formal Stage-C closure remains open pending independent soak-evidence review and an explicit owner/HQ acceptance decision.
- **D — precise resume:** session/boot/epoch fencing, snapshots, replay, tombstones, duplicate-free Carrier replacement.
- **D2 — endpoint/relay:** bounded preconfigured direct/relay endpoint selection after core resume.
- **E — performance:** reproducible benchmarks, profiling, baseline comparison, raw evidence.
- **F — operations:** systemd, idempotent installation, config transactions, drain/stop, certificate rotation, rollback, secret-safe support bundle.
- **G — optional transport/research:** H3 and other bounded experiments behind explicit capability/flags and measured gates.
- **H — real-server pilot:** limited Route, rollback path, real health, multi-window evidence.


## After v0.4.0: BCC-managed shared multiplexing

v0.4.0 ships shared multiplexing as a command-line path only (`baft-pair ex-accept --join` and a live reload; see KNOWN-LIMITATIONS). The next release plans the full design, gated by its own tests and LAB:

- a tunnel-transaction type that adds or removes one tenant on a running EX listener by reload, not restart, with its own backup, health check and rollback that never touches other tenants;
- an ownership model where one instance (marker, generation, unit, metrics claim) carries several tunnels, so discovery, drift, retire and rollback stay exact;
- BCC planning, the Wizard and drift for shared-listener tenants, chosen only when both peers report the capability;
- several tenants per session and the `multiplex-v1` wire capability, negotiated in HELLO;
- live reload for mTLS-only listeners, or an explicit refusal;
- a two-host LAB: tenants joined and removed while others stream, crash of one tenant, reboot.

## After v0.4.0: third-party tunnel engines (direct and reverse)

Today BCC deploys one engine, native BAFT, and only in the **direct** direction (IR dials EX). The engine catalog (`internal/tunnelengine/catalog.go`) lists the planned engines below as unavailable and refuses them in plan and deploy. Every BCC tunnel is created by signed agent jobs: `tunnel_prepare_ex`/`tunnel_prepare_ir`, `tunnel_commit_ex`/`tunnel_commit_ir`, `tunnel_health`, `tunnel_finalize`, `tunnel_rollback`, `tunnel_retire` and `tunnel_discover`. Each job runs a `tunnelnode` transaction that writes config, unit and ownership marker for one instance and can roll it back. Binaries arrive only through the signed release (`update_baft` replaces `baft` and `baft-pair`).

Terms. **Direct**: the IR server opens the link to the EX server; users connect to IR and traffic leaves through EX. **Reverse**: the EX server opens the link to the IR server, which listens for users; useful when IR cannot dial out but EX can reach IR.

| Engine | Upstream license (checked 2026-10-26) | Direct (IR dials EX) | Reverse (EX dials IR) | Link transports | Plan |
|---|---|---|---|---|---|
| FRP (`fatedier/frp`) | Apache-2.0 | frps on EX, frpc on EX for the target, an stcp/xtcp visitor frpc on IR | frps on IR, frpc on EX publishes the EX target | TCP, KCP, QUIC, WebSocket/WSS | P0 driver |
| Rathole (`rathole-org/rathole`) | Apache-2.0 | not natural (the server binds the public port on its own host) | server on IR binds the user port, client on EX forwards to the target | TCP, TLS, Noise (default `Noise_NK_25519_ChaChaPoly_BLAKE2s`), WebSocket | P0 driver, reverse only |
| Backhaul (`Musixal/Backhaul`) | AGPL-3.0 | not documented | server on IR ("Local, IRAN" in its examples), client on EX | tcp, tcpmux, udp, udp-over-tcp, ws, wsmux, wss, wssmux | P1 driver, reverse only |
| WireGuard (kernel, `wireguard-tools`, `wireguard-go`) | kernel module in Linux 5.6+; `wireguard-go` MIT; tools license not verified (page unreachable) | either side may initiate; IR forwards user ports into the tunnel with nft DNAT | same, with EX as initiator | UDP only (layer 3) | P1 driver, needs route and firewall rollback |
| SSH | OpenSSH (system package) | `ssh -L` from IR, or dynamic SOCKS (catalog entry `ssh-socks`) | `ssh -R` from EX; IR sshd needs `GatewayPorts` for a public bind | TCP over SSH | P1, scoped key, never the operator's account |
| GOST v3 (`go-gost/gost`) | MIT | forward listener on IR over relay/TLS/WS to EX | `rtcp` / tunnel handler (reverse support since v3.0.0-rc) | TCP, TLS, WS, WSS, QUIC, KCP and more | candidate, not yet in the catalog |
| Chisel (`jpillora/chisel`) | MIT | client on IR opens a local port to the server on EX | server on IR with `R:` remotes, client on EX | HTTP/WebSocket, SSH-encrypted | candidate, not yet in the catalog |
| WSS, QUIC | (BAFT carriers) | carriers of the native engine, not separate engines | | | P1 carrier work |

What every engine needs before its catalog entry can be enabled (the gates already in `docs/en/38-smart-tunnel-ports-and-drivers.md` and `docs/en/41-engine-inventory-evidence-contract.md`):

1. **Artifact provenance.** Pinned upstream version and SHA-256 per architecture, rebuilt or verified and shipped inside the signed BAFT release with its license file and source offer. The agent never downloads an engine from upstream.
2. **Install and inventory.** A signed agent job that installs or verifies the engine binary, plus the read-only inventory report of doc 41, so BCC admits an engine only on two nodes that both report the expected version and digest.
3. **Driver contract.** One driver per engine that renders the engine config from a reviewed plan (no free-form fields), with direction-aware roles (which side listens, which side dials), per-instance systemd unit, secrets created on the node and never shown, and port reservations through the existing conflict checks.
4. **Transactions.** The same prepare, commit, health, finalize, rollback, retire and discover lifecycle as native BAFT, with exact ownership markers, restart persistence and safe uninstall.
5. **BCC and Wizard.** The Wizard offers direction (direct or reverse), engine and transport only when the catalog entry is enabled and both nodes report the engine; plan hash, drift, metrics and health per engine.
6. **Evidence.** Unit and failure tests, then a two-node LAB per engine and per direction (traffic, restart, reboot, rollback, uninstall) before the option is enabled.

Sources: https://github.com/fatedier/frp, https://github.com/rathole-org/rathole (and its `docs/transport.md`), https://github.com/Musixal/Backhaul, https://github.com/go-gost/gost, https://gost.run/en/blog/2023/reverse-proxy/, https://github.com/jpillora/chisel, https://github.com/WireGuard/wireguard-go, https://wiki.gentoo.org/wiki/WireGuard.

## Connection toolbox plan for Iran conditions (from the 2026-10-26 research)

Basis: the research report (Persian) at `docs/research/tunnel-engines-iran-2026.fa.md`. Its main finding: in a crisis, IP, SNI and protocol whitelists decide what survives, not encryption strength. UDP is blocked first, and connections from abroad into Iran are fragile in a crisis. **Direct** (IR dials EX) therefore stays the default, and reverse is for normal conditions only. A phase is done only when its exit criterion is met with recorded evidence.

### Phase 0 — harden what exists (v0.4.x, short)

- Tunnel health checks push more than 64 KB both ways, so a silent cut after a few packets or kilobytes is detected; "connected" is not enough.
- A per-connection byte/packet cap for the carrier: at the cap a fresh connection is opened and flows move over without interruption.
- Carrier SNI and server name configurable in the Wizard (an allowed SNI or the operator's own domain).
- The Wizard and docs state which server dials which, not just "direct" or "reverse".
- **Exit:** unit and e2e tests of rotation under load; in the LAB, the health check detects a firewall that cuts after N packets.

### Phase 1 — BCC platform for external engines (v0.5.0)

- A signed manifest per engine binary (engine, version, arch, url, sha256) shipped in the BAFT release, with a local mirror. A hash or signature mismatch stops the install; never warn and continue.
- A signed install/verify job and the inventory report of doc 41 from both nodes.
- Driver contract: configs are rendered only from typed schemas that reject newlines, control characters and free-form fields (the lesson of the 3x-ui `PostUp` advisories). The engine's own config check runs before any switch-over.
- One hardened systemd unit per instance: separate user, `NoNewPrivileges`, `ProtectSystem`, and no periodic reset timers.
- Engine dashboards and APIs on localhost only; secrets created on the node and never displayed.
- An install ledger with uninstall driven only by it; firewall rules tagged with comments; MTU/MSS clamping for stacked tunnels.
- Automatic rollback when the Phase 0 health check fails.
- **Exit:** one external engine from Phase 2 installs, runs, reboots, rolls back and uninstalls through this platform in a two-node LAB.

### Phase 2 — first engines, direct before reverse (v0.5.x)

1. **GOST v3** (MIT, 3.3.0 or later): direct and reverse, TLS/WS/gRPC transports and Prometheus metrics. The first driver, since it covers both directions.
2. **FRP** (Apache-2.0, 0.71 or later, wire protocol v2): reverse, and direct via an stcp visitor; the version is pinned on both servers.
3. **Rathole** (Apache-2.0) and **Backhaul** (AGPL-3.0): reverse only, for normal conditions. Re-check Rathole's maintenance status before starting.
4. **Chisel** (MIT, 1.10.0 or later) as a light HTTP/WebSocket option if needed.
- Every engine, in every direction, passes its own two-node LAB (traffic, restart, reboot, rollback, uninstall) before its catalog entry is enabled.
- **Exit:** the Wizard offers an engine only when both nodes report it, and auto-select knows these engines.

### Phase 3 — native engine resistance to whitelists and DPI (v0.6.0)

- **No TLS-in-TLS:** keep one outer TLS carrier with Noise inside, as today; no mode nests TLS.
- The first bytes look like a real protocol (TLS or HTTP), not uniformly random data.
- A real web server next to the listener (fallback site) so the server fingerprint is authentic; accept TLS 1.2 as well.
- An XHTTP-like mode: upload and download on separate connections, with multiplexing.
- A **two-hop domestic relay** mode: user to IR, IR to a domestic relay with a whitelisted IP or SNI, relay to EX (the pattern reported working in Russia).
- A CDN-fronted carrier mode (WebSocket or HTTP/2 on the operator's domain) for when only CDN ranges are open.
- **Exit:** a LAB with a whitelist-policy simulator and a cut-after-N-packets firewall, plus basic detection tests (first-byte entropy, TLS fingerprint). No "undetectable" claims.

### Phase 4 — UDP paths and emergency channels (v0.6.x)

- **AmneziaWG 2.0** and **Hysteria2 with BBR** drivers, for normal conditions only and labelled "blocked first in a crisis".
- A **DNS** fallback (dnstt or slipstream) only for the agent's control channel and small messages, not user traffic; evidence on its survival in a full shutdown conflicts.
- **Shutdown mode:** IR serves users over the domestic network, and BCC shows plainly that the international link is down.
- **Exit:** a LAB per driver, and an operations note "what to do in a shutdown" in Persian and English.

### Phase 5 — automatic selection and fallback, field measurement (continuous)

- Extend auto-select (`/api/tunnels/auto-select`) to every engine and transport, with fallback chains driven only by fresh probe evidence, never by periodic restarts.
- Path probes also classify the cut: after N packets or bytes, SNI-dependent, or UDP.
- A field-measurement programme on real servers in Iran, with dated results.
- Re-verify the report's list of unconfirmed claims (licenses, versions, CVEs, the "~8 KB in Iranian datacenters" premise).

The project does not promise resume across process restart unless a future persistence design explicitly adds and tests that property.

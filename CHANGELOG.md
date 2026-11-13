# Changelog

All notable changes to BAFT are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). Every release is signed; the installer verifies it before use.

## [Unreleased]

- **Dialers can enforce revocation and pin the listener certificate.** `revocation.file` is now accepted on dialers (same list format as on listeners): a revoked listener certificate (identity, serial or SHA-256) is refused on every new handshake, including recovery redials. New optional `peer.certificate_sha256` pins the listener's exact leaf certificate. Established carriers are not cancelled on a dialer; only new handshakes are refused. Config schema and tests updated.

## [0.5.0] - 2026-11-02

Release notes: [release/notes/v0.5.0.md](release/notes/v0.5.0.md). Real Iran↔abroad evidence: [REAL_HOSTS_LAB.md](docs/roadmap/v0.5.0/REAL_HOSTS_LAB.md) and issue #67.

- **New tunnel, guided and live**: the Command Center's "+ New tunnel" dialog now builds real tunnels in five steps (servers, service, connection test, review, deploy) on BCC's reviewed APIs. It reports "done" only when the tunnel is active and, if an automatic public port was requested, that port is verified; rollbacks are shown as such. It replaces the earlier preview-only wizard. It offers the automatic public Iran port only when the Iran agent reports it can open one; otherwise it says so, requests none, and shows the tunnel's entry address on the Iran server to point a panel inbound at.
- **Fixed: connection tests could never pass.** Since automatic path discovery (2026-10-07) BCC asked for a 16 KiB trickle at 1 s per KiB, which cannot finish inside the agents' 10 s probe deadline, so no path was ever `FULL_DATA` and auto-select never chose a port. BCC now sends 8 KiB at 500 ms (works with existing v0.4.0 agents), and impossible trickles are refused at signing.
- **Fixed: a path probe started while others were running failed at once** ("current probe job is missing"); it now waits for a free slot (up to 10 minutes), and cancelling a queued probe sends no job.
- **Fixed: `install.sh --agent-only --provision-ingress-policy` left public ingress off** until a manual restart, because the agent started before the policy and nft chain were written. They are now provisioned first, and a rerun that changes them restarts the agent.
- **Truthful ingress capability**: an agent advertises `operational-ingress-v1` only where Apply can run (today the isolated LAB namespace); host agents log the reason instead of letting BCC queue a public port every Apply refuses.
- **Guided-flow E2E**: a new workflow runs the wizard's exact API sequence against a real BCC and two agents installed from a signed release of the commit (one host plays both servers).
- **Connection test**: an on-demand, signed path test between the two servers on the chosen and common TLS ports, with a live per-port board (only full data counts as passing), connect time, attempt success and connect-time spread; the port is chosen by the evidence-checked auto-select.
- **Speed in the connection test**: agents advertise `path-probe-speed-v1`; when both ends do, BCC asks for a 5-second, 64 MiB-capped throughput flow whose byte count the destination must confirm exactly. The figure is a single-stream raw-path estimate, not tunnel capacity. Older agents are never sent the new parameter.
- **Secret redaction**: failure messages, agent logs and operational receipt causes are redacted (tokens, private keys, pairing codes, URL credentials, key=value secrets) and bounded on the Agent, and failure ACKs again on BCC ingest. Oversized failure receipts are now delivered instead of being dropped. The ACK output channel that carries pairing codes is unchanged.
- **Safe host mode (opt-in): BAFT opens the public Iran port on ordinary servers.** `install.sh --agent-only ... --enable-host-ingress` installs a `host-v1` policy; the agent then reports `operational-ingress-host-v1`. Before opening a port BAFT refuses one held by a running program (IPv4/IPv6), a systemd socket, another BAFT tunnel, an Xray/3X-UI inbound, an nginx `listen`, an HAProxy `bind` or a Docker published port (configuration read only; an unreadable one is a refusal). The reviewed plan needs a per-port operator confirmation that nothing else is configured for the port (programs started by hand are not visible to BAFT); it is bound to the plan hash and asked again when the port changes. BAFT opens only its own listener and its own rule in its own firewall chain, marks the port ready only after BCC itself reaches it, and removes exactly those on decommission. Off by default; the isolated-LAB contract is unchanged. ([ADR-003](docs/roadmap/v0.5.0/ADR-003-real-host-public-ingress.md))
- **Fixed: a decommissioned independent tunnel blocked every new tunnel on the same server** ("sibling instance has no verifiable config", ending `rollback_failed`). A retired instance is now recognised, and a rollback that has nothing to undo succeeds.
- **Fixed: no speed shown on fast paths.** A connection-test speed flow that moves its full 64 MiB cap in under 500 ms (any path faster than ~1 Gbit/s) is now reported as a floor ("≥ N Mbps") instead of being dropped; sub-millisecond connect times show as "<1 ms".
- **Two-host LAB**: a new workflow runs the whole product path between two KVM guests (separate kernels and IPs) with BCC on HTTPS: connection test, safe-host-mode public port Ready by BCC's own probe, client traffic, delay/loss, link cut and recovery, decommission.
- **Fixed: the agent refused its own host-mode policy** at the authority guard, blocking every host-mode Apply before any effect.
- **Fixed: an agent restart or reboot closed the public Iran port for good.** The listener lives in the agent process; after a restart the agent rolled the port back safely but BCC refused its report, so the tunnel stayed active with no public port. BCC now accepts that exact, authenticated report and issues a fresh signed Apply for the same reviewed plan at the next generation; the agent re-checks everything (port free, other services, policy) before opening it. The port is down for the restart plus one re-apply (about 10 s in the LAB). A decommissioned tunnel's approval still cannot be reused.
- **Automatic path discovery is now off by default.** It opened short-lived test listeners on every server (ports 80, 443, 8443, 9443, 49152) and sent probe flows between every pair of servers every 30 minutes; on a production server that briefly takes free ports nobody chose (the real-hosts LAB once deployed a tunnel on port 80 because of it). The tunnel wizard's own connection test and the manual "Run discovery" action are unchanged. Set `BAFT_BCC_DISCOVERY_PORTS=443,2053` (1-16 ports, never 22) on the BCC to run it automatically on those ports only.
- **Connection tests no longer show a fixed signature on the wire.** Every probe flow used to start with a plaintext `BAFTP1 <probe id> <size>` header, which a filtering system can recognise and use to find BAFT servers. Agents now advertise `path-probe-tls-v1`; when both ends do, BCC runs the flows inside TLS 1.3: a browser-profile ClientHello (Chrome, via utls) without SNI, and a certificate both agents derive from the probe ID, which only BCC and the two agents know and which is no longer on the wire. Verification stays on (the source trusts only that key). Older agents keep the plaintext flows until upgraded.
- **Recovery fixes merged** (#292, #293, #294): committed port releases after a crash, a fresh signed Retire after a lost receipt, and the native listener now waits for its handlers and closes hijacked WebSockets on shutdown.
- **Real-hosts LAB**: a manual workflow (`real-hosts.yml`, repository secrets) runs the whole product path between the owner's Iran and foreign test servers: preflight (read-only), install, connection test, deploy, Ready, real traffic, agent restarts, firewall drop (must not stay Ready), tunnel path cut and recovery, optional reboot, decommission and full cleanup. The same driver is self-tested on the two-host LAB's KVM guests. Guide: [REAL_HOSTS_LAB.md](docs/roadmap/v0.5.0/REAL_HOSTS_LAB.md); acceptance map: [P0_ACCEPTANCE.md](docs/roadmap/v0.5.0/P0_ACCEPTANCE.md).
- **Shared transport on the real product path**: a tenant joins an existing tunnel's foreign listener through BCC's reviewed plan (`join_existing_tunnel_id`), on the same port, with its own public Iran port and its own target. The real-hosts LAB now checks, between the owner's Iran and foreign servers, that tenant traffic reaches only its own target, that the owner's traffic and one long-lived owner connection continue across the tenant's join and retirement, and that retiring the tenant removes only its port and rule.
- **Fixed: after a managed tenant was retired, BCC stopped advancing every tunnel.** The adopted tenant-removal receipt was re-checked on every tick and refused, and the background loop gave up before stepping any tunnel: no drift check finished, and no deploy or decommission moved (the owner's decommission was refused). A receipt is now adopted exactly once; an unadoptable receipt is audited once and fails closed for its own tunnel only.
- Design records: [ADR-001](docs/roadmap/v0.5.0/ADR-001-operation-evidence-and-truthful-readiness.md), [ADR-002 (C-BAFT capability integration)](docs/roadmap/v0.5.0/ADR-002-c-baft-capability-integration.md), [ADR-003 (public ingress on ordinary servers)](docs/roadmap/v0.5.0/ADR-003-real-host-public-ingress.md), [C-BAFT comparative audit](docs/roadmap/v0.5.0/COMPARATIVE_AUDIT.md).

## [0.4.0] - 2026-10-30

See [readiness and upgrade gates](docs/release/v0.4.0-readiness.md).

- Read-only ingress preflight now expires from its persisted creation time across queue dispatch, signed payload and acknowledgement; expired dispatch writes a durable security-audit intent. TCP/2053 is governed by resource ownership and OS bind conflicts instead of a blanket port ban. Existing unrelated listeners remain owned by their services.

### Added

- **Transport capability negotiation (capability set v1)**: `HELLO`/`HELLO_ACK` now carry and act on `carrier.h2`, `carrier.ws`, `noise.v1` and `shape.v1`. Each side advertises what its config enables, the Session enables the intersection, and a missing required capability is refused with a typed reason (no silent downgrade; `ws` still requires Noise). New optional `transport.require_capabilities`. Wire-compatible with v0.3.7 in both directions (proved against a frozen copy of the v0.3.7 codec and real v0.3.7 binaries); negotiated and baseline sessions and refusals are exported as `baft_capability_*` metrics. HELLO capability lists must now be strictly ascending.
- BCC Update Center (P0): operator-imported signed release bundles verified against the pinned release root (revocation, version floor, re-tag checks), per-server installed-vs-latest status from component reports, upgrade notice badge, hash-bound upgrade/rollback plans, and an admin-confirmed apply that queues the existing signed `update_baft` job for one server. BCC never updates anything by itself. SQLite schema 16 and encrypted backup schema 5 persist imports; restore never forgets an import or resurrects an older revocation list. The `baft` menu Update item is now read-only guidance. Design: [docs/design/update-center.md](docs/design/update-center.md).
- Read-only Public Ingress host preflight with signed BCC-to-Agent dispatch, strict receipt validation and authenticated result retrieval for an existing owned tunnel. It observes listeners without binding, connecting or changing host state; ingress apply remains unavailable.
- Durable desired Public Ingress port reservations with authenticated API, concurrent conflict protection, atomic audit, restart persistence and allocator/restore guards. They do not bind ports or authorize host changes. SQLite schema 14 and encrypted backup schema 3 prevent older binaries from silently losing allocations; legacy schema 2 backups remain readable without reservation data. Retain a verified pre-upgrade snapshot for downgrade.
- BCC now persists a pending Public Ingress operation proof bound to an exact reservation, generation, manifest digest and listener/firewall IDs. Conflicting retries and restore snapshots are refused. SQLite schema 15 and encrypted backup schema 4 protect this cursor; it does not run an agent action or prove a live resource.
- Authenticated read-only Public Ingress inspection now queues a signed Agent job using the persisted operation proof, with bounded lifetime, duplicate/expiry handling and a validated receipt. It still cannot mark ingress applied or verify host ownership.
- BCC checks the expected manifest digest against a canonical, read-only preview of the reserved ingress configuration before persisting an operation intent; arbitrary caller-supplied digests are rejected.
- BCC can issue a signed, five-minute Public Ingress prepare job from the fenced initial operation. The Agent writes only canonical private manifest bytes and returns proof-bound evidence; installer provisions the Agent's private state path. No listener or firewall is applied. The installer also wraps narrow terminal tables instead of forcing a 64-column layout.
- An authenticated cancellation request can now queue a signed generation-1 tombstone job after prepare was issued. The Agent fences either prepared or unseen manifest bytes; BCC rejects late prepare ACKs after cancellation intent. This is manifest recovery only, with no listener or firewall rollback.
- The isolated Public Ingress LAB now has a durable mutation journal, exact-handle nftables adapter, generation-bound listener/firewall transaction, and a disposable Linux network-namespace acceptance workflow. A proof-bound systemd unit can be rendered and safely staged without enabling it. None of these LAB paths enables production ingress.

- **Tunnel Toolbox in the panel**: the engines a server can run are listed from the panel's own catalog; only the native BAFT engine can be selected. Other engines are shown as not available and every action on them is refused before a plan is made.
- **Smart Tunnel Wizard**: create a tunnel in five steps, with port suggestions checked against the catalog (a suggested port is not a reservation) and a truthful mapping for x-ui/Xray ports.
- **Engine inventory evidence contract**: a versioned format for what an agent reports about installed engines. It is not yet connected to live probes.

- **Public ingress operational path (release-gated, default-closed)**: the Wizard's reviewed plan fixes the IR client config port equal to the actual public ingress port (`ConfigPort == Port`) together with nodes, target, transport, generation and plan hash. BCC sends signed Apply/Retire jobs to the normal enrolled Agent; the Agent opens the listener and nftables rule only under a held, locally verified lease. BCC marks the ingress Verified/Ready and shows the deployable client config card only after the Agent's authenticated HTTPS receipt (Agent to BCC, certificate-validated, no downgrade) and the BCC-side probe agree. Nothing is opened by default; a tag is accepted only when the exact-SHA `operational ingress v1` workflow passes with zero skipped required tests.
- **Installer provisions the pinned nftables chain**: the Agent install creates the empty `inet baft ingress` chain with the pinned layout (idempotent; foreign nft objects are never touched; a differing layout is refused; opt out with `--no-ingress-chain`).
- **Release acceptance**: `scripts/release/accept.sh` now requires the `operational ingress v1` run at the tag SHA for every tag, and the BCC UI, vulncheck and delta-ingress-pin workflows whenever the tagged commit carries them.

### Changed

- The ingress probe responder waits up to 50 ms for a new client's first bytes before treating the connection as ordinary traffic, so clients that send nothing first (server-speaks-first protocols) see up to 50 ms of added connection latency on an ingress port.
- Tunnel Studio no longer assumes a particular destination service; the destination is any IPv4 or IPv6 address and port.
- Classic and Aurora panels reach the same cards and actions (parity documented in `docs/release/r1-classic-aurora-parity-stage286.md`).

### Fixed

- Public ingress: the Agent now holds the lease's guard lock across the final authority check, the fresh socket evidence and the single nftables Install/Remove (`LiveLease.WithLockedIngress`). A concurrent guard close, supersession or uncertain-state record can no longer slip between the last check and the firewall change.

- Test reliability: the late foreign-port-owner ingress test no longer picks an ephemeral port that parallel tests can take, and the slow-receiver backpressure fixture uses 8 KiB instead of 1 KiB buffers to avoid kernel zero-window stalls (assertions unchanged).

- Public welcome/sign-in headers wrap on mobile with the preview launcher visible, preventing horizontal overflow. Full browser checks now cover public control bounds as well as authenticated pages.

- Generated `baft-panel` and `baft-net` menus accept Up/Down, Tab/Shift+Tab and numbered choices through the same action dispatch, with numeric fallback for dumb terminals and redirected input. The Linux Go console also supports arrow/Tab selection, Back and Exit, restores terminal state before actions and on interruption, and keeps its plain input fallback.
- Lost read-only ingress preflight delivery or ACK no longer blocks a node forever: a fresh request after five minutes atomically expires the pending inspection, records the replacement in the audit trail and rejects late ACKs for the old job.
- Network tuning requires explicit consent (including SSH-assisted installs), cancels on prompt EOF, and saves the exact previous values of the keys it changes, restores them on partial failure, and keeps retryable rollback state. Legacy tuning without a snapshot is refused rather than guessed.
- BCC certificate acceptance requires DNS SAN and matching EC/RSA private key; new ACME downloads are pinned to a verified immutable archive and fallback requires explicit consent; failed ACME replacement restores the previous serving files. Renewal timer failure is reported without promising automatic renewal.
- Signed ingress preflight jobs reject invalid ports and loopback endpoints, and lifecycle helpers reject malformed owned resources. Operational ingress application is still incomplete.
- Floating wizard assets are embedded in BCC so their HTTP routes no longer return 404.

- Creating a tunnel from the Aurora panel works again, and edit plans for one instance no longer touch its siblings.
- Monitoring summaries require current, dated evidence; stale data shows as unknown instead of healthy.
- The tunnel catalog ignores late responses after a newer refresh or after the admin token changes.
- Native tunnel endpoints are validated, and a scoped rollback keeps the identity of the instance it restores.
- Panel on phones: no horizontal overflow, readable stage colours in both themes, and unset discovery dates are shown as unset. Switching language no longer overwrites live values.

## [0.3.3] - 2026-10-16

### Fixed

- **Adding a second outside server from the panel replaced the existing tunnel.** New tunnels are now separate instances next to the existing one, and the original tunnel keeps running.

## [0.3.2] - 2026-10-16

### Added

- **Installer in the x-ui style**: run one line, choose "Outside Iran", "Inside Iran" or "Panel", and answer a few questions.
- **Web panel offered with the outside server**: at the end of an outside-server install the installer offers to install the panel on the same server (`--with-panel`, `--no-panel`).
- **SSL for the panel**: a domain or the server's IP gets a Let's Encrypt certificate (90 days for a domain, 6 days for an IP), renewed automatically; a self-signed fallback if issuance fails.
- **`sudo baft-panel`**: panel status, link and username, new login, restart, logs, domain and SSL, renew, update.
- **`sudo baft-net`**: BBR and TCP buffer tuning on or off, speed test (also reports a foreign TCP connection cut after a few KB), latency, route and port tests.
- **BBR tuning** offered once during install (`BAFT_TUNE_NETWORK=1|0`).
- **Carried ports for x-ui** (`--ports 443,2053`): the Iran server listens on each port and forwards it to the same port on the outside server, so users change only the server address in their links.
- **Path test** from the Iran server to the outside server before pairing.
- Panel: servers are placed on the map from their IP address, and servers that are reachable or whose agent polls recently show as online.
- User guide in Persian and English, `SECURITY.md`, issue templates and this changelog.

### Changed

- **License: BAFT is now free software under the GNU AGPL-3.0.** The BAFT name and logo remain trademarks and are not licensed under the AGPL (see `COPYRIGHT`).
- The front pages (`README.md`, `README.fa.md`) are rewritten for users; the engineering material moved to `docs/*/40-engineering-notes.md`.

### Security

- A release tag must be an approved tag on a `main` commit, and the unsigned CI candidate is checked against a clean build of the same source before it is signed offline.
- Release signing can be done on a phone (Android Termux) with trust checks; see `docs/release/android-termux-signing.md`.

### Fixed

- The HTTP/2 carrier reports the TLS phase of a failure correctly when a Noise write fails at the same time.
- Panel telemetry history is pruned relative to the report time, so a server with a skewed clock does not lose its history.

## [0.3.1] - 2026-10-12

This entry also covers v0.3.0 (tagged the same day), the first release after v0.2.0.

### Added

- **New Command Center (panel)**: a rebuilt dashboard with an Earth globe and maps, measured server and tunnel counts, and active alerts.
- **Add a server over SSH from the panel**, signed end to end, and **edit an active tunnel** by a reviewed in-place replacement.
- **Encrypted panel backups** from the Command Center, with inspection of listed backups.
- **Installer**: a clearer, staged terminal look; asks what the server is for; can **install the Iran server over SSH** from the outside server; first-class panel install.
- **Network tools in the panel**: automatic path discovery, a connectivity matrix between servers, a read-only route doctor, safe SSH port migration, and a configuration change ledger.
- **Multiple Iran servers**: health-based active/standby and weighted distribution between Iran entry servers.
- **More carriers**: WebSocket (including behind Cloudflare) and uTLS, with failover between HTTP/2 and WebSocket.
- **Guarded tunnel removal** from the panel.

### Fixed

- A session no longer ends on a late or foreign PONG.
- Clearer diagnostics when a real WAN connection fails during the handshake.
- Installer preflight checks for paths and service-user access; `baft doctor` explains release provenance problems.
- Panel: less dashboard polling, durable security audit, and a compatible panel binary is kept after a failed rollback.
- Release signing secrets removed from repository CI; the signer is isolated and the supply chain pinned.

### Upgrading

Run the installer again on each server with the same options. Update the Iran and outside servers together.

## [0.2.0] - 2026-10-04

### Fixed

- **Recovery no longer drops the session on a connection error**; a rare loop that could keep recovery stuck is fixed.
- **No more long stalls** (seconds to a minute) on local services under load.

### Added

- **Rerunnable installer**: running it again inspects the server, shows a plan, changes only what is needed and rolls back on failure. One-line install.
- **Safe uninstall** that removes only what is proven to be BAFT's, keeps your data by default and can be undone.
- **Transactional certificate rotation** with rollback.
- **Signed offline installation bundle** for servers without internet access.
- **Panel**: stronger web security (security headers, login limits, Argon2id passwords), layered health with alerts and webhooks, backup restore preview and an offline restore command.
- **Tunnels**: plan preview before changes, drift detection, ownership markers, and a report of existing tunnels.
- **Operator tools**: `baft doctor` v2 and the `baft` menu.
- Third-party license notices ship with every release.

### Upgrading

Run the installer again on each server with the same options (`--agent-only` for servers managed by the panel). Update both sides together.

## [0.1.1] - 2026-10-02

### Added

- **Tunnel builder in the panel**: builds a tunnel between an outside and an Iran server and rolls both back if anything fails, times out or is cancelled.
- **Add servers over SSH**: the panel pins the server's SSH host key and installs the agent; SSH credentials are never stored, logged or audited.
- Dashboard cards for adding servers and for tunnels.
- Agent tunnel actions with strict parameter checks; pairing secrets are wiped from the panel after use.

### Removed

- The old `enroll_peer` job and `/api/enroll` endpoint (replaced by the tunnel builder).

### Upgrading

Run the installer again on each server (`--agent-only` for enrolled servers) so the agents get the tunnel actions before you build tunnels.

## [0.1.0] - 2026-10-02

First signed release.

[Unreleased]: https://github.com/zarkmakerburg/baft/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/zarkmakerburg/baft/releases/tag/v0.3.1
[0.2.0]: https://github.com/zarkmakerburg/baft/releases/tag/v0.2.0
[0.1.1]: https://github.com/zarkmakerburg/baft/releases/tag/v0.1.1
[0.1.0]: https://github.com/zarkmakerburg/baft/releases/tag/v0.1.0

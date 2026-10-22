# Changelog

All notable changes to BAFT are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). Every release is signed; the installer verifies it before use.

## [Unreleased]

- Read-only ingress preflight now expires from its persisted creation time across queue dispatch, signed payload and acknowledgement; expired dispatch writes a durable security-audit intent. TCP/2053 is governed by resource ownership and OS bind conflicts instead of a blanket port ban. Existing unrelated listeners remain owned by their services.

Development target: **v0.4.0**, draft PR #270. Not released. See [readiness and upgrade gates](docs/release/v0.4.0-readiness.md).

### Added

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

### Changed

- Tunnel Studio no longer assumes a particular destination service; the destination is any IPv4 or IPv6 address and port.
- Classic and Aurora panels reach the same cards and actions (parity documented in `docs/release/r1-classic-aurora-parity-stage286.md`).

### Fixed

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

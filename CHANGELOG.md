# Changelog

All notable changes to BAFT are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). Every release is signed; the installer verifies it before use.

## [Unreleased]

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

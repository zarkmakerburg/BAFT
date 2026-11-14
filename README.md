<p align="right"><a href="README.fa.md"><strong>فارسی ←</strong></a></p>

<p align="center">
  <img src="docs/assets/baft-readme-poster.webp" alt="BAFT" width="100%">
</p>

<h1 align="center">BAFT</h1>

<p align="center">
  <a href="https://github.com/zarkmakerburg/baft/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/zarkmakerburg/baft?style=flat-square&label=release"></a>
  <a href="https://github.com/zarkmakerburg/baft/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/zarkmakerburg/baft/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <img alt="License: AGPL-3.0" src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square">
</p>

<p align="center">
  <a href="README.fa.md">فارسی</a> ·
  <a href="docs/en/guide/README.md">User guide</a> ·
  <a href="https://github.com/zarkmakerburg/baft/releases/latest">Latest release</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

**BAFT is a secure, reliable tunnel from a server inside Iran to a server abroad, for people who run x-ui / 3x-ui (or any TCP service) on the foreign server, with a web panel to manage several servers.**

It is not a panel for selling users or managing subscriptions (like Marzban). It carries your existing service from Iran to abroad; your x-ui stays exactly as it is.

## Key features

- **One-line install** on the outside server; it can install the Iran server for you over SSH.
- **Carries your x-ui ports**: the Iran server listens on the same ports and forwards them to the outside server. Users change only the server address.
- **Path test before pairing**: checks that the Iran → outside path really carries data, not just that a port opens.
- **Web panel (BCC)** for several servers, on a domain or IP with a Let's Encrypt certificate.
- **Network tuning**: BBR and TCP buffers on or off with one command, plus speed, route and port tests.
- **Security**: signed releases verified against an offline root key, mutual TLS between servers, Argon2id panel login, agent jobs are signed and never run a shell.
- **Safe to rerun, safe to remove**: a rerun shows a plan and rolls back on failure; uninstall removes only what BAFT provably owns.

## Install

On the **outside** server (Debian/Ubuntu, as root):

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
```

Choose **1 (Outside Iran)** and answer the questions. Full walkthrough: [Install guide](docs/en/guide/01-install.md).

## Use with x-ui in 4 steps

1. Run the installer on the outside server (where x-ui runs), choose **1**, and give the ports to carry, for example `443,2053` (it suggests the ports your server already listens on, such as x-ui inbounds; `--ports 443,2053` in scripts).
2. Let it install the Iran server over SSH (or run the same line on the Iran server and paste the pairing code). It tests the Iran → outside path before pairing.
3. The Iran server now listens on each carried port and forwards it through the tunnel to the same port on the outside server.
4. In your x-ui links, change only the server address to the **Iran server's IP**. The port stays the same.

Details: [Use with x-ui](docs/en/guide/02-use-with-x-ui.md).

## What works, and the limits

**Works** (TCP): VLESS, VMess and Trojan over TCP, WebSocket, gRPC and HTTPUpgrade, including UDP that is carried *inside* those protocols (for example DNS or games through VLESS). Any other plain TCP service works too.

**Does not work** (UDP-based): Hysteria2, TUIC, WireGuard, and anything else that needs raw UDP. BAFT carries TCP only.

**Status, plainly**: BAFT is new. It is tested in CI and in lab setups, but it has **not yet been proven in the field at scale**; no large real Iran ↔ abroad pilot has been run. Iranian networks differ by datacenter: from at least one Iranian datacenter, foreign TCP on ports other than 22 was cut after about 8.7 KB. Test your path (`sudo baft-net`) before you rely on it. See [Known limitations](KNOWN-LIMITATIONS.en.md).

## Management commands

| Command | Where | What |
|---|---|---|
| `baft` | every server | status menu: status, doctor, logs, certificates, support bundle, uninstall |
| `sudo baft-panel` | panel server | panel link and login, new login, restart, logs, domain and SSL, update |
| `sudo baft-net` | every server | BBR and tuning on/off, speed test, latency, route (mtr), port test |
| `baft doctor` / `baft version` | every server | health check / installed version |
| `journalctl -u baft -f` | every server | live tunnel logs |

## Current release and development status

**Latest published release: [v0.5.0](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.5.0)** (published 2026-11-02). The current `main` head is [`d4348c2ace51`](https://github.com/zarkmakerburg/BAFT/commit/d4348c2ace51e56469b10aee2806f3ad4869c8fa) and contains post-v0.5.0 changes listed under **[Unreleased](CHANGELOG.md)**, including unified server updates, agent self-update and dialer-side revocation/certificate pinning. These changes are **not a published release**. For production installs, use the latest signed published release until a newer release is formally published.

**P0 status:** [Issue #269](https://github.com/zarkmakerburg/BAFT/issues/269) remains open. v0.5.0 includes a guided tunnel flow, opt-in public ingress and shared-transport tenant operations, but the complete P0 acceptance is not closed. In particular, do not infer that general-purpose `multiplex-v1` is implemented or supported. The current ingress integration candidate [PR #278](https://github.com/zarkmakerburg/BAFT/pull/278) is still a draft and has unresolved fencing findings recorded in the [HQ board](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6861034383).

**Engineering status:** Stage A/B are complete for their defined scopes. The Stage-C soak workflow passed on the current `main` SHA (see [run 43472903062](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903062)); formal Stage-C closure remains an explicit acceptance decision. Stage D is partial: ECRL supports same-process live-session carrier replacement, but process-restart resume, machine-reboot resume and durable session snapshots are not implemented. BAFT is not declared production-ready and the current `main` SHA is not evidence of a scaled field pilot.

The nine GitHub Actions runs listed in [STATUS](STATUS.en.md) completed successfully on the exact current `main` SHA. This is not a claim that every workflow in the repository has run or passed on that SHA.

- `install.sh`: installation, update and generated `baft-panel` / `baft-net` management tools.
- `internal/bcc/`: BCC backend, reviewed tunnel plans, public-ingress policy and operational coordination.
- `web/`: BCC browser UI, including the guided tunnel flow.
- `tests/installer/` and `tests/e2e/`: installer contracts and hosted end-to-end workflows.
- `docs/design/` and `docs/roadmap/`: design decisions, implementation plans and acceptance gates; a design document alone is not evidence of shipped behavior.

**Compatibility and ownership:** Aurora is intended to complement the existing BCC, not replace it. Existing 3X-UI subscriptions and services remain externally owned; BAFT does not take ownership of or replace them.

See [P0 implementation and acceptance gates](docs/design/p0-oneclick-ingress-269.md), [Project status](STATUS.en.md), [Development plan](PLAN.en.md), [Known limitations](KNOWN-LIMITATIONS.en.md) and the [changelog](CHANGELOG.md).



## Documentation

- **[User guide](docs/en/guide/README.md)**: [install](docs/en/guide/01-install.md), [x-ui](docs/en/guide/02-use-with-x-ui.md), [panel](docs/en/guide/03-panel.md), [network and BBR](docs/en/guide/04-network-and-bbr.md), [update, backup, uninstall](docs/en/guide/05-update-backup-uninstall.md), [troubleshooting](docs/en/guide/06-troubleshooting.md), [FAQ](docs/en/guide/07-faq.md)
- [Technical documentation](docs/en/README.md) and [engineering notes](docs/en/40-engineering-notes.md) (architecture, protocol, stages, CI evidence)
- [Changelog](CHANGELOG.md) · [Known limitations](KNOWN-LIMITATIONS.en.md) · [Project status](STATUS.en.md)

## Support and security

- Bugs and questions: [GitHub issues](https://github.com/zarkmakerburg/baft/issues/new/choose). Never paste pairing codes, passwords or keys.
- Security vulnerabilities: report privately, see [SECURITY.md](SECURITY.md). Please do not open a public issue.

## License

BAFT is free software under the [GNU AGPL-3.0](LICENSE). The BAFT name and logo are trademarks and are not licensed under the AGPL; see [COPYRIGHT](COPYRIGHT). Third-party notices: [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

## Support the project

If BAFT is useful to you, please **[give it a star on GitHub](https://github.com/zarkmakerburg/baft/stargazers)**; it helps others find the project.

Donations in crypto are welcome:

| Network | Wallet address |
|---|---|
| TRON (TRC-20) | `TP13BYbfbcFbtomvdLc9rKxAWbWyk195VH` |
| TON | `UQAN4TZrv29BRagXzFD_4_tkWJvx1wf8mIEeFHymwhlIatLa` |

Please double-check the address and network before sending; transfers cannot be reversed.

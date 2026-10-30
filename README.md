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

## Development status and code organization

**Next planned release: v0.4.0** — this is a development target, **not a published release**. For production installs, use the latest published release; the installer command above targets the default branch, not the development branch.

- [P0 implementation and acceptance gates](docs/design/p0-oneclick-ingress-269.md) — installer safety, BCC TLS, public ingress, network diagnostics and Linux menu accessibility.
- [Draft v0.4.0 integration PR](https://github.com/zarkmakerburg/BAFT/pull/270) and [HQ progress log](https://github.com/zarkmakerburg/BAFT/issues/67) — review and CI evidence, not release approval.
- `install.sh`: installation, generated `baft-panel` and `baft-net` management tools.
- `internal/bcc/`: BCC backend, including pure public-ingress planning and reservation checks. **Planning does not provision a public listener or firewall rule.**
- `web/`: BCC browser UI and preview-only Aurora wizard components.
- `tests/installer/`: installer contract tests; static checks are not a substitute for VM and live-path validation.
- `docs/design/`: proposed P0 work and engineering decisions, not proof that each feature has shipped.

**Known development gaps:** The generated `baft-panel` and `baft-net` menus and the Linux Go console support Up/Down arrows, Tab/Shift+Tab and numbered choices, with plain input fallback; the complete command catalog and localization still need completion. Signed read-only public-ingress preflight is available, but agent apply/reconcile, fully reversible adaptive network profiles, and the v0.4.0 end-to-end release gates remain under development. Aurora is intended to complement the existing BCC, not replace it. Existing 3X-UI subscriptions and services remain externally owned.

See the [v0.4.0 readiness matrix and upgrade gates](docs/release/v0.4.0-readiness.md) for tested changes and remaining blockers.

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

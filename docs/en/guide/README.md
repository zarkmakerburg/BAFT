# BAFT user guide

[فارسی](../../fa/guide/README.md) · [Front page](../../../README.md) · [Technical documentation](../README.md)

This guide is for people who run x-ui / 3x-ui (or another TCP service) on a server outside Iran and want users inside Iran to reach it through a server inside Iran. Read the pages in order the first time.

| # | Page | What it covers |
|---|---|---|
| 1 | [Install](01-install.md) | outside server first, Iran server over SSH or with a pairing code, offline install |
| 2 | [Use with x-ui](02-use-with-x-ui.md) | carried ports, changing client links, what works and what does not |
| 3 | [Panel](03-panel.md) | the web panel (BCC): domain or IP, SSL, login, adding servers |
| 4 | [Network and BBR](04-network-and-bbr.md) | `sudo baft-net`: BBR, speed, route and port tests |
| 5 | [Update, backup, uninstall](05-update-backup-uninstall.md) | rerunning the installer, panel backups, safe removal |
| 6 | [Troubleshooting](06-troubleshooting.md) | SSL, GitHub blocked, expired pairing code, cut connections, panel unreachable, logs |
| 7 | [FAQ](07-faq.md) | short answers |

Words used in this guide:

- **Outside server** (EX): the server abroad, where x-ui runs.
- **Iran server** (IR): the server inside Iran that users connect to.
- **Panel** (BCC, BAFT Command Center): the optional web panel; usually on the outside server.
- **Carried port**: a port the Iran server listens on and forwards to the same port on the outside server.

BAFT is new and not yet proven in the field at scale. See [Known limitations](../../../KNOWN-LIMITATIONS.en.md).

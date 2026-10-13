# 3 — The web panel (BCC)

[Guide index](README.md) · [فارسی](../../fa/guide/03-panel.md)

The panel (BAFT Command Center, BCC) shows your servers and tunnels in one place, adds servers over SSH and builds tunnels between them. It is optional: a single tunnel works without it. It is not a user-selling panel; your users stay in x-ui.

## Installing the panel

- **With the outside server**: at the end of an outside-server install the installer asks whether to install the panel on the same server. `--with-panel` says yes without asking, `--no-panel` never asks.
- **On its own**: run the install line and choose **3 (Panel)**, or `sudo bash install.sh --bcc-only`.

The installer asks:

1. **Domain or IP.** Enter a domain whose DNS A record points at this server, or press Enter to use the server's public IP.
2. **Port.** Press Enter for a random free port, or give one.

It then gets a **Let's Encrypt** certificate with acme.sh: a domain gets the usual 90-day certificate, an IP a short-lived 6-day one. Both renew automatically every day (`baft-bcc-cert.timer`). **Port 80 must be free and reachable from the internet while the certificate is issued.** If no certificate can be issued, the installer offers a self-signed one; adding servers over SSH stays off until a real certificate is in place.

Non-interactive: `--bcc-only --bcc-domain panel.example.com --bcc-port 8443`.

## Signing in

The last screen shows the **link** (it includes a secret path), the **username** and a **one-time password**. Save them; the password is stored only as an Argon2id hash and cannot be shown again. Repeated failed logins are rate-limited and locked for a while.

## `sudo baft-panel`

| # | Item | What it does |
|---|---|---|
| 1 | status | running or stopped, version, SSL expiry |
| 2 | link and username | shows the panel link and username |
| 3 | new login | makes a new link, username and password; everyone is signed out |
| 4 | restart | restarts the panel |
| 5 | logs | last panel log lines |
| 6 | set domain / get SSL | change the domain or IP and get a certificate |
| 7 | renew SSL | renews the certificate now |
| 8 | update | updates the panel to the latest signed release |
| 9 | network | opens `baft-net` |

Each item also works directly, for example `sudo baft-panel link` or `sudo baft-panel ssl`.

## Adding servers and tunnels

In the panel, **Add server over SSH** asks for the server's address, SSH user and password or key. The panel pins the server's SSH host key, installs the BAFT agent on it and verifies the release. The SSH password or key is used only for that request and is never stored, logged or written to backups.

With an outside and an Iran server added, **Tunnels** builds a tunnel between them: it shows a plan first, then prepares both sides, starts them and checks health. If anything fails, both sides are rolled back to how they were.

The agent on each server only runs a fixed list of signed jobs; it has no shell access.

More detail: [BCC access](../24-p1c-bcc-access.md), [SSH bootstrap](../26-p1d-ssh-bootstrap.md), [Tunnel builder](../27-p1e-tunnel-builder.md), [Layered health](../30-bcc-layered-health.md).

## Security tips

- Prefer a domain over a bare IP, and keep the secret link private.
- Do not open the panel port to everyone if you can limit it in your firewall.
- After sharing a screen or a screenshot by mistake, make a new login (`sudo baft-panel` → 3).

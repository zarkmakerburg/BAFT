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

## Creating a tunnel step by step

On the **Tunnels** page press **+ New tunnel**. A five-step dialog opens; nothing changes on any server until BCC has reviewed the plan:

1. **Servers.** Pick the Iran server (where users connect) and the foreign server (where the service runs). Each server shows whether its agent is online; one that has not reported in the last 5 minutes is flagged.
2. **Service.** Enter the service port on the foreign server (for 3X-UI, the inbound port). If the Iran server's agent reports the automatic public port capability, the port users connect to on the Iran server defaults to the same number; port 22 and a port another tunnel already uses on that Iran server are refused, and you must explicitly allow BAFT to open it once the tunnel is active. Otherwise the dialog says so plainly and requests no public port (see "When the automatic public port is not available" below).
3. **Connection test.** Press **Run connection test**. Through signed jobs, BCC asks the foreign server to open a short-lived test listener and the Iran server to send real data to it on several common TLS ports. Green means full data passed, yellow means it connects but data is cut, red means blocked. Connect time, successful attempts and — when both agents support it — an approximate speed in Mbps are shown. The tunnel port is chosen only among ports that passed full data. Under Advanced you can pick a port manually without test evidence.
4. **Review.** BCC reviews the plan and shows its safety checks as FAIL, WARN and PASS. Deploy stays disabled unless BCC accepts the plan.
5. **Deploy.** Progress is shown step by step: prepare foreign server, pair Iran server, install, health checks, finalize, active. "Done" appears only when the tunnel is active and, if an automatic public port was requested, that port on the Iran server has been verified independently. If a step fails, BAFT rolls back its own changes and says so.

### When the automatic public port is not available

In this version BAFT opens a public port automatically only inside its isolated LAB. On a normal server the Iran agent does not report that capability and logs why (`unsupported execution scope`). The tunnel is still built in full and listens on the Iran server itself; its entry address (for example `127.0.0.1:1444`) is shown in the review step and the final message. To let users connect, point a public port on the Iran server at that address in your panel: for example a 3X-UI `dokodemo-door` inbound on the public port that forwards to `127.0.0.1:1444`, or an equivalent port forward.

You can close the dialog at any time; the build continues in BCC and appears in the tunnel list. The same connection test is also in the Tunnel Studio, which adds shared-listener joins and editing.

More detail: [BCC access](../24-p1c-bcc-access.md), [SSH bootstrap](../26-p1d-ssh-bootstrap.md), [Tunnel builder](../27-p1e-tunnel-builder.md), [Layered health](../30-bcc-layered-health.md).

## Security tips

- Prefer a domain over a bare IP, and keep the secret link private.
- Do not open the panel port to everyone if you can limit it in your firewall.
- After sharing a screen or a screenshot by mistake, make a new login (`sudo baft-panel` → 3).

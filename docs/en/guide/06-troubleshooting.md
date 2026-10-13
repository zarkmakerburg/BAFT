# 6 — Troubleshooting

[Guide index](README.md) · [فارسی](../../fa/guide/06-troubleshooting.md)

Start with these on the server that misbehaves:

```bash
baft doctor              # checks config, keys, service, release and reachability
baft status
journalctl -u baft -f    # live tunnel logs (Ctrl+C to stop)
```

When you ask for help, attach `baft doctor` output and, if asked, a support bundle (`baft` menu → 10). They contain no payload, keys or passwords, but read them before posting and never post a pairing code or password.

## SSL for the panel fails (port 80 busy, DNS)

Let's Encrypt checks that it can reach your server on **port 80** while issuing the certificate.

- **Port 80 is busy**: see what uses it with `ss -ltnp | grep ':80 '`. Often it is nginx, Apache, or an x-ui/Xray inbound on port 80. Stop it for a minute, run `sudo baft-panel` → 6, then start it again.
- **Port 80 is blocked**: open it in the server firewall (`ufw allow 80/tcp`) and in your provider's firewall panel.
- **Domain does not point at this server**: `dig +short panel.example.com` must print this server's public IP. Behind Cloudflare, turn the proxy (orange cloud) off for the panel record while issuing, or use the IP instead of a domain.
- **IP certificate**: works without a domain; it is short-lived (6 days) and renews automatically.

If issuance still fails, the installer offers a self-signed certificate. The panel works with it, but adding servers over SSH stays off until a real certificate is in place.

## GitHub is blocked from the Iran server

Downloads from GitHub often fail or are very slow from Iran.

- **Install the Iran server from the outside server** over SSH (installer option on the outside server, or `--ir-ssh`). The outside server does the downloading.
- Or use the **offline bundle**: download `baft-offline-<version>.tar.gz` on a machine that can reach GitHub, copy it to the Iran server, and run `sudo bash install.sh --offline . ...` (see [Install](01-install.md#offline-install)).

## Pairing code expired

A `BAFTPAIR1:` code is valid for **15 minutes** and only once. If the Iran server says the code is expired or already used, run the installer again on the outside server: an outside server that is still waiting for pairing gets a fresh code. Paste the new code on the Iran server within 15 minutes. To start a new pairing on servers that were already paired, use `--re-pair --yes` (this interrupts the tunnel until pairing completes). Installing the Iran server over SSH avoids copying codes by hand.

## Connection cut after a few KB

Symptom: clients connect, then traffic stops; small pages load but downloads hang; or the installer's path test fails.

Some Iranian datacenters cut foreign TCP connections after a few kilobytes on most ports (in one case about 8.7 KB on every port except 22).

1. On the **Iran server** run `sudo baft-net speed`. A line *"the transfer stopped after N bytes"* confirms the cut.
2. Run `sudo baft-net port` against the outside server's IP and the BAFT port.
3. Try another port for BAFT or the carried service, then test again.
4. If every port is cut, the problem is the datacenter; another Iranian provider may behave differently.

## The panel is unreachable

- Is it running? `sudo baft-panel status` (option 1). If stopped, `sudo baft-panel restart` and read `sudo baft-panel logs`.
- Right link? `sudo baft-panel link` shows the full link including the secret path; the bare address without the path does not open the panel.
- Firewall: open the panel port in `ufw` (`ufw allow PORT/tcp`) and in your provider's firewall.
- `https://` vs `http://`: with a certificate the link is `https://`.
- Certificate expired: `sudo baft-panel renew`, or check `systemctl status baft-bcc-cert.timer`.
- Forgot the password: `sudo baft-panel` → 3 makes a new login.

## Users cannot connect through the Iran server

- On the Iran server, `ss -ltn` must show the carried port. If not, check `baft status` and `journalctl -u baft -f`.
- The port must be open in the Iran server's firewall and the datacenter's firewall.
- Is the protocol supported? UDP-based protocols (Hysteria2, TUIC, WireGuard) do not work; see [Use with x-ui](02-use-with-x-ui.md#what-does-not-work).
- Only the server address in the link should change; the port, SNI and the rest stay as they were.

## Logs

```bash
journalctl -u baft -f              # tunnel, live
journalctl -u baft -n 200          # tunnel, last 200 lines
baft logs -n 200 -f                # the same through baft
sudo baft-panel logs               # panel
journalctl -u baft-agent -n 100    # agent (servers managed by the panel)
```

Logs never contain user traffic. Still, read them before sharing and remove IPs if you prefer.

## The installer refuses to continue

- `BROKEN_INSTALL`: the config does not validate or a key is missing; nothing was changed. Run `baft doctor` to see why.
- "downgrade refused": the release is older than the installed one; this is protection against rollback attacks.
- "verification failed": the release did not match the pinned root key. Do not work around it; report it (see [SECURITY.md](../../../SECURITY.md)).

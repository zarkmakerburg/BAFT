# 2 — Use with x-ui / 3x-ui

[Guide index](README.md) · [فارسی](../../fa/guide/02-use-with-x-ui.md)

## How it works

```text
user (in Iran)  ──►  Iran server :443  ══ BAFT tunnel ══►  outside server :443 (x-ui inbound)
                     Iran server :2053 ══ BAFT tunnel ══►  outside server :2053 (x-ui inbound)
```

For every **carried port**, the Iran server listens publicly on that port and forwards each connection through the encrypted tunnel to the **same port** on the outside server. x-ui does not change, and its inbounds keep their settings, keys and users.

## The 4 steps

1. **Pick the ports.** In x-ui, note the port of each inbound you want to reach from Iran. When the installer asks for ports on the outside server it suggests the ports the server already listens on; keep the ones that are x-ui inbounds. In scripts: `--ports 443,2053`.
2. **Install and pair** (see [Install](01-install.md)). The installer tests the Iran → outside path before pairing.
3. **Check the Iran server** listens on those ports:
   ```bash
   ss -ltn | grep -E ':(443|2053)\b'
   ```
4. **Change the client links.** In each user's link or subscription, replace only the server address with the **Iran server's IP** (or a domain pointed at it). Keep the port, UUID/password, transport, path, SNI and everything else the same.

Example:

```text
before:  vless://UUID@OUTSIDE_IP:443?type=ws&path=/abc&security=tls&sni=example.com#user
after:   vless://UUID@IRAN_IP:443?type=ws&path=/abc&security=tls&sni=example.com#user
```

With TLS/Reality inbounds the SNI and certificate stay those of the outside server; only the address the client dials changes.

## What works

TCP only:

- VLESS, VMess and Trojan over **TCP, WebSocket, gRPC and HTTPUpgrade** (with or without TLS / Reality).
- UDP that travels **inside** those protocols (for example DNS, voice or games carried through VLESS/VMess/Trojan).
- Any other plain TCP service (a web server, SSH on a non-22 port, and so on).

## What does not work

Anything that needs raw UDP between the client and the server: **Hysteria2, TUIC, WireGuard**, OpenVPN over UDP, plain Shadowsocks UDP relay, KCP/mKCP and QUIC transports. BAFT does not carry UDP.

## Adding or changing ports later

Run the installer again on the outside server; it shows a plan before changing anything. See [Update, backup, uninstall](05-update-backup-uninstall.md).

## Things to keep in mind

- The Iran server's carried ports are public. They do not check who connects: x-ui's own authentication (UUID, password) protects the service, exactly as before.
- Open the carried ports in the Iran server's firewall and in the datacenter's firewall panel if it has one.
- Do not run another service (for example a second x-ui) on the Iran server on the same ports.
- If users connect but traffic stops after a few KB, the Iran datacenter may be cutting foreign connections on that port. Test with `sudo baft-net` and try another port; see [Troubleshooting](06-troubleshooting.md#connection-cut-after-a-few-kb).

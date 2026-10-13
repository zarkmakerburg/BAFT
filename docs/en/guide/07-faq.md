# 7 — Frequently asked questions

[Guide index](README.md) · [فارسی](../../fa/guide/07-faq.md)

**Is BAFT a replacement for x-ui or Marzban?**
No. BAFT does not create users or sell subscriptions. It carries your existing x-ui (or any TCP service) from an Iran server to the outside server.

**Do I need to change anything in x-ui?**
No. Only the server address in the client links changes, to the Iran server's IP. Ports and settings stay the same.

**Does it work with Hysteria2, TUIC or WireGuard?**
No. BAFT carries TCP only. VLESS, VMess and Trojan over TCP, WebSocket, gRPC or HTTPUpgrade work, including UDP carried inside them.

**Is it proven in Iran?**
Not yet at scale. It is tested in CI and lab setups, but no large real Iran ↔ abroad pilot has run. Test your own path with `sudo baft-net` first. See [Known limitations](../../../KNOWN-LIMITATIONS.en.md).

**Does BAFT make my traffic undetectable?**
No such claim is made. The tunnel uses TLS 1.3 with mutual authentication, but no tool can guarantee it will never be detected or blocked.

**Do I need the web panel?**
No. One outside server and one Iran server work without it. The panel helps when you have several servers.

**Do I need a domain?**
No. The panel can get a certificate for the server's IP. A domain is easier to remember and its certificate lasts longer.

**Can one outside server serve several Iran servers?**
Yes, through the panel: add the servers and build a tunnel for each pair.

**Does BAFT store my SSH password?**
No. It is used for the one install step and then discarded; it is never written to disk, logs or backups.

**How do I know the downloaded release is genuine?**
The installer verifies every release against a root key pinned in `install.sh` (kept offline by the project) and refuses anything that does not verify, or that is older than what is installed.

**Is BAFT free?**
Yes. It is free software under the GNU AGPL-3.0. If you run a modified version as a network service, the AGPL requires you to offer your users its source. The BAFT name and logo are trademarks and not covered by the AGPL.

**Where do I report a bug or a security problem?**
Bugs: [GitHub issues](https://github.com/zarkmakerburg/baft/issues/new/choose). Security problems: privately, see [SECURITY.md](../../../SECURITY.md).

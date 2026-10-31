# ADR-002: Integrating C-BAFT capabilities into BAFT

- **Status:** Accepted for v0.5.0 (first slice implemented; later slices planned)
- **Date:** 2026-10-31
- **Owner direction:** combine the essential, useful capabilities of C-BAFT and its ease of use with BAFT's BCC into one final product on the v0.5.0 branch.
- **Sources inspected:** `zarkmakerburg/C-BAFT` `main` at `ed57639` (module `ctunnel`, Go 1.22, ~12.1k non-test LOC); BAFT `codex/v0.5.0-foundation-20261031`.
- **Related:** [ADR-001](./ADR-001-operation-evidence-and-truthful-readiness.md), [Baseline audit](./BASELINE_AUDIT.md), [Agent handoff](./AGENT_HANDOFF.md)

## Context

### What C-BAFT is (verified from source)

- A fork of **C-Tunnel by CIAUB** (AGPL-3.0, with Section 7 additional terms). Parts of its Manage menu and `pkg/socks`, `pkg/localproxy`, `pkg/schedule`, `pkg/netscore`, `pkg/cli/manage.go`, `pkg/cli/health.go` come from **BackPack** (AGPL-3.0).
- Product model: one Iran server, many foreign servers; each tunnel gets its own `/30`, interface and UDP port; users connect to an Iran port that is forwarded through the tunnel.
- Data plane: 28 methods in `pkg/catalog` — kernel L3/L2 tunnels (gre, ipip, sit/6to4, ip6gre, gretap, vxlan, geneve, eoip, etherip, l2tpv3, fou/gue, xfrm-ipsec, wireguard, openvpn-dco …) and UDP transports in `cmd/ctunnel-engine` (dtls-1.3, quic-v1, wg-stealth, dnscrypt-udp, ntp/srtp-direct "look-alike" headers, `rudp-fec`), plus a reverse multi-stream TCP carrier (`revcarrier.go`).
- Crypto for UDP-Crypt (`pkg/crypto/aead.go`): AES-256-GCM per direction, HMAC-SHA256 key derivation from a shared token, 2000-packet anti-replay window, random padding. Protocol look-alike headers are framing, not protocol compliance.
- Usability: one-line `ctunnel://` setup link (host, ports and a shared token), terminal menu, dark responsive web panel with create/edit/delete, live traffic, **Connection Test** (`pkg/conntest`: sends real packets per method, shows pass/blocked, measures speed and recommends a method), path-MTU discovery (`pmtu_linux.go`), speed test (`pkg/speedtest`: latency, jitter, loss, 4-stream TCP throughput) and "deploy best".
- Control/authority: a shared token in the link is the credential; the panel acts directly on the local host; port forwards are iptables rules tagged per tunnel and removed by tag.

### What BAFT already has

BCC is the single authority (enrollment, Ed25519-signed scoped jobs, replay/expiry checks), ownership-proven public ingress, journal/recovery, TLS + Noise carriers (h2, ws, uTLS), signed `path_probe_*` jobs with evidence classes (`FULL_DATA`, `BYTE_CEILING`, …), a path matrix and an evidence-checked auto-select, plus a classic dashboard with the live Smart Tunnel Wizard and the newer Aurora UI. The wizard could choose an EX port **only from pre-existing evidence**; the Aurora wizard is preview-only.

## Decision

C-BAFT's **user experience and measurement ideas** are adopted into BAFT; C-BAFT's **authority, ownership and installer patterns are not**. Every adopted capability runs as a BCC-issued, signed, scoped Agent job and reports evidence under ADR-001 state semantics. Data-plane methods are added only through a transport-driver contract with tests and a security review.

| # | C-BAFT capability | Decision | How it lands in BAFT / why |
|---|---|---|---|
| 1 | Connection Test board + recommendation | **Adopted — implemented** (`26a86708`) | Wizard "Run connection test" starts a signed path probe IR → EX on the chosen port + common TLS ports; live bilingual board; only `FULL_DATA` reads as passing; selection goes through the existing evidence-checked auto-select, never around it. |
| 2 | Speed test (latency, jitter, loss, throughput) | **Adapt — next slice** | Extend `path_probe_run` evidence (bulk flows already record bytes and duration) with RTT samples, jitter and throughput; rank by evidence in auto-select; show in the board. No separate unauthenticated responder. |
| 3 | "Deploy best" / measured deployment | **Adapt** | Keep BAFT's plan → review → deploy with revalidation; add a one-click "use recommended" path in the wizard that pre-fills, never auto-deploys. |
| 4 | Web panel ease of use (create/edit/delete, live traffic, status dots, mobile) | **Adopt incrementally** | Bring the classic wizard's live flow into Aurora; reuse BAFT telemetry for live traffic; keep ADR-001 distinctions (configured / queued / applied / verified). |
| 5 | Path-MTU discovery | **Defer to UDP/L3 transports** | BAFT carriers are TCP-based and rely on kernel PMTUD; PMTU becomes a signed probe when a UDP or kernel transport lands. C-BAFT's probe also accepts replies from any source address — must bind to the peer. |
| 6 | Kernel tunnels (GRE, IPIP, SIT, VXLAN, GENEVE, …) | **Defer (P2), via transport-driver ADR** | Unencrypted encapsulations may only be underlays beneath an authenticated layer. Interfaces, routes and firewall rules need ownership proof and rollback like public ingress. WireGuard is the strongest first candidate (authenticated encryption, kernel-native). |
| 7 | UDP-Crypt family, `rudp-fec` (2+1 XOR FEC), look-alike headers | **Defer (P2), crypto review first** | Requires independent review of key derivation from a shared token, nonce/sequence handling and replay window under loss; look-alike framing gives no DPI guarantee and must be labelled that way. |
| 8 | Reverse carrier (EX dials IR) | **Defer; reuse BAFT TLS + Noise** | BAFT already plans Reverse; implement it with BAFT's authenticated carriers, not TLS-record look-alike framing. |
| 9 | `ctunnel://` one-line link with a shared token | **Rejected as an authority mechanism** | A link that carries a bearer secret bypasses BCC authority and scope. The ease-of-use equivalent is BCC's one-line enrollment command and the wizard. |
| 10 | iptables forwards removed by comment tag | **Rejected** | ADR-001 §4 forbids deletion based on names/comments; BAFT's ownership-proven public ingress stays the only path. |
| 11 | sysctl "optimize" | **Defer** | Host-wide mutation needs a signed job, recorded prior values and a tested rollback before it can ship. |
| 12 | MikroTik RouterOS snippets | **P2, read-only** | Useful generated text only once matching transports exist. |
| 13 | SOCKS5/HTTP local proxy, netscore, scheduler (BackPack) | **Out of scope for v0.5.0** | Not part of the tunnel product path. |

## Licensing and attribution

BAFT and C-BAFT are both AGPL-3.0. Slice 1 copies **no C-BAFT source code**; it adapts the interaction idea and status vocabulary onto BAFT's own APIs. Any future port of C-BAFT code must, in the same commit:

- keep the Section 7(b) line "Based on C-Tunnel by CIAUB — https://github.com/CIAUB/C-Tunnel" visible and unaltered in BAFT's NOTICE/README, and the BackPack line for any file from the BackPack-derived set;
- not use the "C-Tunnel" name or logo for BAFT (Section 7(e));
- add the provenance to `THIRD_PARTY_LICENSES.md` and a header in each ported file.

## Acceptance for each adopted slice

- Runs only through BCC-signed, node-scoped jobs; no new unauthenticated listener or shared-token path.
- The UI never shows "passes", "connected" or "active" without the evidence ADR-001 requires; partial, pending and stale states stay distinct.
- Bilingual (Persian RTL / English LTR), mobile-first, accessible status text (colour is never the only signal).
- Unit/UI tests in the same commit; exact-SHA CI evidence recorded before claiming completion.

## Slice order

1. Connection Test in the wizard — **done** (`26a86708`, CI pending at time of writing).
2. Throughput/jitter/loss in probe evidence and ranking.
3. Aurora live wizard parity (test → recommend → review → deploy) with "use recommended".
4. Transport-driver contract ADR; WireGuard as first kernel transport candidate, with ownership + rollback.
5. PMTU probe and UDP transports only after (4) and a crypto review.

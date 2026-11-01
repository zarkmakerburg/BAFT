# C-BAFT → BAFT comparative audit (code level)

- **Date:** 2026-11-01
- **Sources (exact):** C-BAFT `main` at `ed57639` (module `ctunnel`, Go 1.22, standard library only, no third-party modules); BAFT `codex/v0.5.0-foundation-20261031` at `d84ea120`, `main` at `9c32ba36`.
- **Method:** source read of both trees, existing tests counted per package, licence files read. No benchmark was run for this audit; where a decision needs measured numbers it says so instead of guessing.
- **Builds on:** [ADR-002](./ADR-002-c-baft-capability-integration.md) (decision record), [ADR-003](./ADR-003-real-host-public-ingress.md) (host public ingress). This file adds the code-level evidence, the A/B/C/D classification required by the owner's advisory of 2026-11-01, and the licence position.

## 1. Classification used

| Class | Meaning |
|---|---|
| **A** | Port the C-BAFT code directly (reviewed, tested, compatible, licence clear). |
| **B** | Port the code behind a small adapter (algorithm good, API/lifecycle differs). |
| **C** | Adopt the idea; implement it on BAFT's contracts (code has security, architecture or authority problems). |
| **D** | Reject, with the reason recorded. |

## 2. Licence and provenance

- Both projects are **AGPL-3.0**; C-BAFT's code is therefore licence-compatible with BAFT.
- C-BAFT's `NOTICE` adds Section 7 terms that bind any port: **7(b)** keep "Based on C-Tunnel by CIAUB — https://github.com/CIAUB/C-Tunnel" visible and unaltered in NOTICE and README; **7(e)** the "C-Tunnel" name and logo are not licensed.
- A second origin exists inside C-BAFT: `pkg/cli/manage.go`, `pkg/cli/health.go`, `pkg/socks`, `pkg/localproxy`, `pkg/schedule`, `pkg/netscore` come from **BackPack (AGPL-3.0)** and carry their own required line ("Based on BackPack by Amin Mohammadi (AminMGMT)").
- **Status:** no C-BAFT file has been copied into BAFT so far, so no attribution obligation has been triggered yet. The first port of any file must add both the NOTICE/README line(s) and a `THIRD_PARTY_LICENSES.md` entry in the same commit (ADR-002 §Licensing). BackPack-derived packages are quarantined from porting: none is on the product path (§3, row 13).
- C-BAFT has no third-party Go modules, so a port adds no dependency risk.

## 3. Capability decision table

| # | Capability | C-BAFT (code, tests) | BAFT (code, tests) | Evidence / risk | Class → decision |
|---|---|---|---|---|---|
| 1 | **Connection test** | `pkg/conntest` 1 677 LOC / 234 test. UDP responder; every probe and control packet carries the 16-byte token **in clear**; token compared non-constant-time; a 1-packet `ctlSend` makes the responder stream at full rate for up to 5 s (one stream at a time) **to the request's source address** (spoofable → reflection once the token is sniffed; nonces only deduplicate). | Signed `path_probe_*` jobs (BCC Ed25519, node-scoped, expiry/replay), evidence classes, `FULL_DATA` only passes; proven end to end in `guided-flow-e2e`. | BAFT's authority model is strictly stronger; C-BAFT's design would add an unauthenticated amplifier. | **C** — done (ADR-002 slice 1, `26a86708`; P0 trickle fix `dc6e973e`). |
| 2 | **Speed test** (latency, jitter, loss, throughput) | `pkg/speedtest` 767 / 178; 4-stream TCP. | Attempt/spread metrics + signed bounded `BAFTS1` throughput flow (`12e4e3ec`), exact byte confirmation. | Same authority issue as row 1. | **C** — done. |
| 3 | **Recommend / deploy best** | `conntest.Recommend` picks a method from results. | Evidence-checked auto-select bound into the reviewed plan hash; deploy re-validates. | BAFT binds selection to the plan; C-BAFT deploys locally. | **C** — done (`033cb4d1`). |
| 4 | **Users connect to an Iran port** | iptables DNAT tagged by comment, removed by tag (`pkg/tunnel`). | Ownership-proven public ingress: owned listener inode + owned nft rule handle in `inet baft ingress`, journal, lease, independent BCC probe. Was LAB-only; **safe host mode (ADR-003) implemented on this branch** (`2c982c9e`, `d84ea120`). | Deletion by comment/tag can remove foreign rules; BAFT's never does. C-BAFT has **no port-conflict detection at all** (no live-socket, systemd or Xray check). | **C/D** — idea adopted via ADR-003; tag-based removal rejected. |
| 5 | **UDP-Crypt AEAD** | `pkg/crypto` 222 / 122. AES-256-GCM per direction; keys = HMAC(shared token, label): **static, no handshake, no forward secrecy, no rotation**. Nonce = IV ⊕ seq, seq seeded from `time.Now().UnixNano()` per start: a backward clock step across a restart can **reuse a GCM nonce under the same key**. Receiver replay window is in memory: after a receiver restart, captured old packets are accepted again until fresh traffic moves the window past them. | Noise (`noise-v1`) inside TLS carriers; per-session keys. | GCM nonce reuse is catastrophic (forgery, keystream reuse). Fixable only with a real handshake/epoch, i.e. not a direct port. | **D** for the crypto as is. Any future UDP transport must use a handshake with per-session keys (WireGuard or Noise over UDP). |
| 6 | **XOR FEC 2+1** (`rudp-fec`) | `pkg/dpi/fec.go`, small, framing tests; single-loss recovery per group of 2. | none (TCP carriers do not need it). | Algorithmically sound and dependency-free; value only exists with a UDP transport, and must sit **above** an authenticated layer. | **A (deferred)** — candidate for direct port with attribution when a UDP transport lands; requires loss-model benchmarks (1–10 % loss, burst loss) against no-FEC first. |
| 7 | **Protocol look-alike headers** (dtls/quic/ntp/srtp/dnscrypt) | `pkg/dpi/framing.go`. Framing only; no protocol compliance. | uTLS fingerprinting on real TLS. | Offers no DPI guarantee; misleading if labelled as the protocol. | **D** for v0.5.0; may return only labelled as "framing", never as protocol support. |
| 8 | **Reverse carrier** (EX dials IR, multi-stream TCP) | `revcarrier.go` 1 111 LOC engine / 423 test; first frame authenticated with the static AEAD of row 5. | Reverse direction planned (wizard shows "Planned"). | Authentication inherits row 5's weaknesses. | **C** — implement Reverse with BAFT's TLS+Noise carriers. |
| 9 | **Kernel L2/L3 tunnels** (GRE, IPIP, SIT, VXLAN, GENEVE, EoIP, …) | `pkg/catalog`, `pkg/tunnel`, `pkg/eoip`. Mostly unencrypted encapsulations. | none. | Need interface/route/firewall ownership proof + rollback like public ingress; unencrypted. | **C (P2)** — only as underlay beneath an authenticated layer, via a transport-driver contract (§4). WireGuard first candidate. |
| 10 | **Path-MTU discovery** | `pmtu_linux.go`; replies read with `ReadFromUDP` and the source address ignored. | kernel PMTUD on TCP. | Must bind replies to the peer. | **B (deferred)** — port with a source-binding fix when a UDP transport lands. |
| 11 | **`ctunnel://` link with shared token** | `conntest.EncodeLink`/`DecodeLink`. | BCC one-line enrollment + signed jobs. | A bearer secret in a link bypasses BCC scope. | **D**. |
| 12 | **Web panel UX** (create/edit/delete, live traffic, status) | `pkg/webpanel` 2 017 / 841. | Quick Tunnel wizard (live, bilingual, mobile), classic Tunnel Studio, Aurora. | UX ideas adopted; the panel acts as local root, which BAFT's BCC model forbids. | **C** — wizard done; per-tunnel live traffic and edit/delete inside the new flow remain. |
| 13 | **SOCKS/HTTP proxy, netscore, scheduler, manage menu** (BackPack-derived) | `pkg/socks`, `pkg/localproxy`, `pkg/netscore`, `pkg/schedule`, `pkg/cli`. | n/a | Not on the tunnel product path; separate attribution chain. | **D** (quarantined) for v0.5.0. |
| 14 | **sysctl optimise** | `pkg/cli`. | none. | Host-wide mutation; needs signed job + recorded prior values + tested rollback. | **C (deferred)**. |
| 15 | **Atomic file write** | `pkg/atomicfile` 25 LOC, no tests. | BAFT already has fsync'd, `RENAME_NOREPLACE` writers with identity checks. | BAFT's is stronger. | **D**. |
| 16 | **Multiplexing / shared transport** | multi-stream TCP inside one reverse carrier (flow-hash pinning). | Shared EX listener with digest-fenced tenants, per-tenant routes, tenant isolation tests (`TestCrossTenantRouteAccessRefused`), session recovery. | BAFT already has the stronger lifecycle (join/remove without disturbing siblings). | **D** — keep BAFT's. |

## 4. Transport contract (status)

BAFT already has the control-plane half of the contract the advisory asks for: `internal/tunnelengine` (engine catalog, `RequireDeployable`, signed inventory, `RequirePairInstalled`) and agent capability tokens (`carrier-h2`, `carrier-ws`, `noise-v1`, `path-probe-speed-v1`, `operational-ingress-v1`, `operational-ingress-host-v1`). BCC selects only from declared capabilities. The data-plane half (common deadline/cancel/backpressure/stats interface and a shared conformance test suite for every engine) does not exist yet; it is the prerequisite for rows 6, 8, 9 and 10 and is the next design item (ADR-002 slice 4). No new transport is added before it.

## 5. What this audit changed in the plan

1. Highest-value slice remains **real-host public ingress** (row 4): it is the product gap users hit first, and C-BAFT offers no safer code for it. Implemented on this branch as ADR-003 safe host mode; its host-mode e2e result is pending and will be recorded in BASELINE_AUDIT.
2. **No C-BAFT file qualifies for class A today.** The only clean class-A candidate (XOR FEC) has no consumer until a UDP transport exists.
3. Any UDP transport must not reuse C-BAFT's static-key AEAD (row 5).
4. Measured comparisons (latency/jitter/loss, CPU/memory) are owed before rows 6/8/9 are adopted; none are claimed here.

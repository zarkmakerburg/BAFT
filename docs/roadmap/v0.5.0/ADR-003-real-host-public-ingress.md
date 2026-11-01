# ADR-003: Operational public ingress on ordinary servers (host profile)

- **Status:** Accepted by the owner on 2026-11-01 (decisions in §8); implemented on the v0.5.0 branch (§9)
- **Date:** 2026-11-01
- **Scope:** Agent public-ingress Apply/Retire on an Iran (IR) server that runs in the host's init network namespace; BCC capability gating, review and readiness; installer opt-in
- **Related:** [ADR-001](./ADR-001-operation-evidence-and-truthful-readiness.md), [ADR-002](./ADR-002-c-baft-capability-integration.md), [BASELINE_AUDIT §12](./BASELINE_AUDIT.md#12-stage-4--the-product-path-proven-on-real-installed-agents-2026-11-01), Issue #269

## 1. Problem

Users connect to a public port on the IR server. Today BAFT opens that port automatically **only inside an isolated, loopback-only, single-process LAB namespace** (`internal/ingressinventory/coverage_linux.go`, `execution_lab_linux.go`). On every ordinary server the agent now (correctly) does not advertise `operational-ingress-v1`, and the user must point a panel inbound or port forward at the tunnel's IR route listener by hand. That manual step is the largest remaining usability gap and the core reason Issue #269 is no-go.

## 2. Why the LAB restriction exists (verified in code)

`CollectInstalledManagedTCP` must prove that applying a listener and firewall rule on port *P* cannot harm anything else. It combines:

1. **Persistent claims:** systemd socket units under the policy's source roots that name *P* (stopped or running).
2. **Live claims:** every kernel TCP entry on *P* (`/proc/net/tcp{,6}`), v4 and v6.
3. **Exclusive execution:** the agent is the *only* process in the namespace (`measurePrivateLinuxLAB`), so nothing can claim *P* between the scan and the effect, and nothing it cannot see (another manager, a container, a config file) can hold a latent claim.

On an ordinary server, (3) is false by definition, and (1) cannot be complete: Xray/3X-UI inbounds, nginx, haproxy, Docker publishers and hand-run daemons declare ports in their own configuration, not in systemd socket units. The current model therefore reports **coverage incomplete** and refuses — correctly for its contract, which is "prove no other claimant exists".

## 3. Options

| Option | What it is | Assessment |
|---|---|---|
| **A. Keep LAB-only** (status quo) | Manual exposure through the user's panel. | Safe; poor UX; leaves #269 open indefinitely. |
| **B. Host profile with bind-exclusive ownership** | A separate, opt-in capability whose contract is *not* "no other claimant exists" but "BAFT holds the port exclusively, never disturbs a current holder, and refuses known latent claims". | Recommended. Matches what can actually be proven on a shared host; keeps every ADR-001 invariant. |
| C. Dedicated network namespace per ingress (veth + DNAT) | Run the public listener in a private netns, DNAT the host port into it. | Still needs host-level DNAT ownership on the shared namespace — same proof problem, more moving parts. |
| D. Configure the user's panel (e.g. 3X-UI API) | BAFT creates a dokodemo-door inbound for the user. | Couples BAFT's authority to a third-party API and its credentials; versions vary. Possible later as an *assistant*, not as the ownership model. |

## 4. Proposed decision (Option B)

### 4.1 A distinct capability and contract

- New capability `operational-ingress-host-v1`, advertised only when: the installed managed-node policy declares `"profile": "host-v1"` (new field, schema v2), the operator enabled it at install time (`install.sh --enable-host-ingress`, default off), the owned nft chain and journal are usable, and the agent runs in the namespace named by the policy.
- `operational-ingress-v1` (LAB) is unchanged. BCC treats the two as different contracts; a v0.4.0 BCC never sees the new token.

### 4.2 What the host profile proves before an Apply

1. **No current holder:** no kernel listener on *P* for any address family (as today). A non-listening entry (TIME_WAIT etc.) is reported, not deleted.
2. **No known latent claim:** systemd socket units (as today) **plus** read-only detectors for common declarers on the same host, each returning *claims* or *unknown*:
   - BAFT's own inventory (other tunnels' client/route/metrics ports — already in BCC);
   - Xray / 3X-UI: inbound ports from its config/DB (read-only);
   - nginx / haproxy `listen` directives;
   - Docker published ports (`docker ps` / API, if present).
   An *unknown* from a detector that is installed but unreadable is a **refusal**, not a pass.
3. **Explicit residual-risk acknowledgement:** the reviewed BCC plan for that port carries a per-port, per-node operator acknowledgement: *"No other service on this server is configured to use port P."* It is bound into the plan hash and the signed job; changing the port, node or plan invalidates it. No UI default ticks it.

### 4.3 How ownership is held and proven

- **Bind-exclusive listener:** the agent's forwarder binds `[::]:P` (dual-stack where available) without `SO_REUSEPORT`. Once bound, the kernel prevents another process from binding the same port; `EADDRINUSE` at bind is a clean refusal with no effect. The socket inode recorded in the journal is the ownership proof (existing `SocketIdentity`).
- **Owned firewall rule:** exactly as today — a rule in BAFT's pinned `inet baft ingress` chain, identified by handle and journal entry, never by name or comment. Foreign tables/chains are never touched.
- **Reachability is evidence, not an assumption:** another base chain (ufw, firewalld, provider firewall) may still drop traffic. "Ready" continues to require BCC's independent probe round trip (ADR-001 §3); a failed probe shows the honest reason ("listener and rule applied; not reachable from BCC — check the server/provider firewall").

### 4.4 Retire, recovery and failure

- Retire removes only the journaled rule handle and closes only the recorded socket inode (existing machinery). If either proof is missing or ambiguous: fail closed, `NEEDS_RECONCILIATION`, nothing deleted.
- Agent restart: the listener is re-bound from the journal only if the port is still free and the plan/lease are still valid; otherwise the operation becomes `uncertain` and BCC stops showing it as ready.
- A later conflict (another service tries to start on *P* and fails) is the residual risk acknowledged in 4.2(3). BAFT surfaces it: the agent logs and reports any process that attempted and failed to bind *P* only if observable; it never kills or reconfigures it.

### 4.5 What does not change

- No caller flag, UI field or BCC request can enable the host profile; only the local installer option writes the policy (root, protected path).
- No deletion of any resource BAFT cannot prove it owns.
- LAB workflows and the LAB capability keep their stronger guarantees.

## 5. Acceptance tests (gates, not claims)

| Case | Required result |
|---|---|
| Host profile off (default) | No `operational-ingress-host-v1`; wizard shows the manual path (as today) |
| Port held by a live listener (v4 or v6) | Refused before any effect; holder untouched |
| Port declared by a stopped systemd socket / Xray inbound / nginx / Docker publisher | Refused with the detector's reason |
| A detector present but unreadable | Refused (unknown is not a pass) |
| Acknowledgement missing, or plan changed after acknowledgement | Plan gate FAIL; no signed job |
| Successful Apply | Listener inode + rule handle journaled; BCC probe round trip → Ready; client card shown |
| Provider/host firewall drops traffic | Applied but **not** Ready; reason shown |
| Another process binds *P* between scan and bind | Bind `EADDRINUSE` → refused, no rule left behind |
| Crash after bind / after rule / before receipt | Deterministic reconciliation from the journal; no false Ready |
| Retire with foreign rule in the same chain or foreign socket on *P* | Only owned handle/inode removed; foreign survives |
| Shared IR running several tunnels | Retiring one tenant leaves the others' listeners and rules intact |
| Real two-host run (IR and EX on different machines) | Guided flow ends Ready with a real client through the public port |

Each result must be tied to an exact commit SHA and labelled unit / container / namespace LAB / physical host.

## 6. Implementation slices (after approval)

1. Policy schema v2 (`profile: host-v1`), installer opt-in, capability token, BCC gating (no Apply yet).
2. Latent-claim detectors (systemd sockets, BAFT inventory, Xray/3X-UI, nginx/haproxy, Docker) with unknown-is-refusal tests.
3. Plan-bound acknowledgement in BCC review and the wizard (fa/en).
4. Host-profile executor: bind-exclusive listener + owned rule + journal; reuse Retire/recovery.
5. Guided-flow e2e on the host profile (public port end to end, firewall-drop case) and a two-VM LAB run.

## 7. Owner decisions requested

1. Accept Option B and its residual risk (a service *configured but stopped* on *P* that no detector recognises may fail to start later), mitigated by detectors + explicit per-port acknowledgement + reversible Retire.
2. Keep the host profile **opt-in** at install time (recommended) or make it default for new installs.
3. Which detectors are mandatory for v0.5.0 (recommended: systemd sockets, BAFT inventory, Xray/3X-UI; nginx/haproxy/Docker when present).

## 8. Owner decisions (2026-11-01)

1. **Option B accepted** — safe host mode with bind-exclusive ownership, latent-claim refusal and a plan-bound per-port acknowledgement, including its documented residual risk.
2. **Opt-in at install time** — off by default; enabled only by the local installer option (`--enable-host-ingress`) on the IR server.
3. **Mandatory latent-claim checks for v0.5.0:** BAFT's own inventory and systemd socket units (always) **plus Xray / 3X-UI inbounds** (read-only). nginx/haproxy and Docker detectors are not required for v0.5.0; until they exist, the acknowledgement text must name them as unchecked.

## 9. Implementation (v0.5.0 branch)

| Slice (§6) | Status | Where |
|---|---|---|
| 1. Policy schema 2 (`execution_profile: host-v1`), installer opt-in, capability token, BCC gating | done (`2c982c9e`, `d84ea120`) | `internal/ingressinventory/coverage_linux.go`, `internal/ingressauthority/policy_bootstrap_linux.go`, `cmd/baft-agent` (`--ingress-profile host-v1`), `install.sh --enable-host-ingress`, `internal/agent/ingress_capability.go`, `internal/bcc/ingress_enqueue_gate.go` |
| 2. Latent-claim detectors | done: live listeners (v4/v6), systemd sockets, BAFT inventory (BCC), **Xray/3X-UI inbounds**, and beyond the v0.5.0 mandatory set **nginx `listen`** (files under `/etc/nginx` except `*-available`, enabled-site links followed), **HAProxy `bind`** (`/etc/haproxy/*.cfg`) and **Docker published ports** (each container's `hostconfig.json`, running or stopped; ephemeral and UDP bindings excluded). All read-only; present-but-unreadable = refusal; all part of the execution pin. | `internal/ingressinventory/xray_claims.go`, `service_claims.go` |
| 3. Plan-bound acknowledgement | done: `client_port_ack` in `TunnelRequest`/`Tunnel`, bound into the plan hash; plan gate FAIL without it on a host-mode IR; enqueue gate refuses unacknowledged tunnels; Quick Tunnel wizard and classic Tunnel Studio checkboxes (fa/en), reset on any port/server change. | `internal/bcc/plan.go`, `web/bcc-action-wizard.js`, `internal/bcc/dashboard.go` |
| 4. Host-profile executor | reuses the existing owned listener + owned nft rule + journal/lease machinery; only the execution-scope measurement differs (no single-process/loopback-only requirement; host-complete checks kept). | `internal/ingressinventory/execution_lab_linux.go` |
| 5. Guided-flow e2e on the host profile | **done**: `guided-flow-e2e` host leg passed on `c13c4510` (run [41282272962](https://github.com/zarkmakerburg/BAFT/actions/runs/41282272962), job 124776431516); details in [BASELINE_AUDIT §13](./BASELINE_AUDIT.md#13-stage-5--safe-host-mode-proven-on-real-installed-agents-2026-11-01). One machine plays both servers. Firewall-drop case and two-VM run: not done. | `tests/e2e/guided_flow.sh` (`GF_MODE=host`) |

Upgrade rule: an existing schema-1 (LAB) policy is replaced by host-v1 only when it is bound to the host's init network namespace, where the LAB scope refuses every Apply, so it can never have backed a reservation, rule or listener. Any other differing policy is left unchanged and the installer stops with a clear message.

Known limits: a port that another service declares only somewhere BAFT does not read (hand-run programs, non-default config locations, stopped host-network containers) is covered by the operator's acknowledgement, not by a detector; Xray inbounds that exist only in the 3X-UI database and are not in the running Xray configuration are not seen.


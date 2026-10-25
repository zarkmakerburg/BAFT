# BAFT BCC Auto-Select: port and transport planning

Status: implementation contract for `hq/auto-select`; design only until API/UI integration is reviewed. Base: `claude/v040-integration@28d5cd187542f269953acaa4f633d34b9890c978`. This document does not claim carrier support or censorship resistance beyond verified capability and path evidence.

## Goals and invariants

- Recommend a viable route using fresh `internal/bcc` path-probe / path-matrix evidence and installed-node capability reports; never infer availability from a port number, a stale probe, a UI default, or a successful TCP connect alone.
- Keep the four port roles distinct: (1) public IR client/ingress port, (2) IR local route listener, (3) EX outer transport listener, and (4) EX application target (for example 3X-UI inbound). A number may coincide only when explicit validation proves it safe.
- The public client/ingress port defaults to the selected EX target port to preserve 3X-UI Custom Share Address semantics. If the operator overrides it, show the external subscription rewrite requirement; BAFT does not edit 3X-UI users, UUIDs, or subscription payloads.
- Auto-selection is a suggestion in a deterministic, side-effect-free plan. No reservation, listener, firewall, tunnel, or config mutation occurs before explicit Review/Deploy. Revalidate evidence and the plan hash immediately before execution.
- Never silently downgrade security, change topology, replace a carrier, choose an untested port, or describe a degraded path as full-data. If no eligible option remains, refuse with a specific reason and remediation.
- Preserve existing configurations and legacy peer behavior. No assumption that TCP/443 guarantees censorship resistance.

## Inputs and freshness

Required:
- source IR node, destination EX node, requested topology/direction, target host+port, address family, requested service/protocol, public client port preference (auto or explicit), and operator constraints.
- authenticated Agent liveness/version and installed engine/carrier capability reports for both nodes, including topology, protocol, platform, auth mode, health/rollback support, and supported tuning.
- path-probe candidates for the relevant ordered node pair and address family: class, port, connect/ACK success counts, RTT, byte ceiling, trickle result, detail, evidence timestamp, probe ID.
- candidate-listener occupancy and owned/reserved-port inventory on each affected node, plus firewall/service ownership and target reachability.
- policy: required capabilities, allowed carriers, whether a dedicated EX port is allowed, and explicit operator-selected constraints.

Freshness:
- Use the path-matrix freshness interval returned by BCC. Missing, stale, failed, unknown, incomplete or mismatched source/destination/family evidence is not positive evidence.
- A path candidate is eligible for ordinary bulk TCP only when its current probe class is `FULL_DATA`, the required trickle check passed, the probe is complete and fresh, and the destination listener/target are independently verified. `CONNECT_ONLY`, `HANDSHAKE_ONLY`, `BYTE_CEILING`, `TIMEOUT_UNREACHABLE`, `LOCAL_CONFLICT`, `UNKNOWN`, and `STALE` must never be silently promoted to full-data.
- Re-run/refresh probes when evidence is absent or stale only through an explicit preflight action; a wizard render must not create jobs or host mutations.

## Deterministic decision order

1. Validate user intent, node identities, address family, target literal/hostname policy, and all port ranges. Reject invalid or contradictory constraints before ranking.
2. Exclude revoked/offline/stale Agents, unsupported platform/version, incompatible protocol/topology, missing authenticated capability evidence, occupied ports, unowned resources, and candidates lacking a fresh successful path probe.
3. Enforce hard policy requirements first: explicit carrier/topology/capability requirements cannot be relaxed by the ranker. WebSocket requires negotiated `carrier.ws` and `noise.v1`; HTTP/2 requires negotiated `carrier.h2` and its actual implementation; Noise is required where the selected transport's security contract says so. Never infer support solely from a configuration flag.
4. Select the transport family from the remaining verified set. Prefer the highest policy-approved path class (`FULL_DATA` only for a full-data promise), then lowest measured RTT, then deterministic tie-break by carrier ID, address family, and numeric port. Weighting/metrics may break ties only if explicitly configured and deterministic.
5. Select EX listener port from verified compatible candidates. Prefer an existing compatible shared listener only when the multiplex feature and that exact protocol capability are reported on both peers and the tenant lifecycle/isolation gate is enabled; otherwise choose an available dedicated port. TCP/443 is only a candidate, never a guarantee or a magic default.
6. Select the public IR client port independently. Default it to the EX application target port; validate availability and ownership on IR. If the operator explicitly sets a different value, show a blocking acknowledgement before Deploy that subscription clients may require external Custom Share Address/port rewrite.
7. Build the immutable reviewed plan with selected and rejected candidates, evidence timestamps/IDs, port-role mapping, exact mutations, owners, rollback, health checks, and plan hash. At execution, refresh all mutable evidence and refuse if the selected candidate, owner, capability, reservation, or plan hash changed.

## Decision table

| Evidence / condition | Result | UI behavior |
|---|---|---|
| Fresh `FULL_DATA`, trickle passed, compatible authenticated carrier, all ports free | Eligible | Recommend with measured RTT and evidence age |
| Fresh `BYTE_CEILING` or `HANDSHAKE_ONLY` | Not eligible for full-data; optional only if user explicitly requested a bounded/limited diagnostic mode supported by the engine | Explain observed byte ceiling/handshake limitation; never label Ready/full-data |
| `CONNECT_ONLY` | Not eligible for a production tunnel | Explain that TCP connect does not prove stream delivery |
| `TIMEOUT_UNREACHABLE`, `LOCAL_CONFLICT`, `FAIL` | Reject candidate | Show node pair, family, port and next probe/remediation |
| `STALE`, `NOT_ASSESSED`, missing or mismatched probe | Unknown, fail closed | Offer explicit re-probe; no auto-deploy |
| Port occupied by foreign/unidentified service or conflicting owner | Reject that port; do not kill/reconfigure the service | Show conflict and safe alternatives |
| Port owned/reserved by another BAFT operation | Reject or reuse only when same operation/tenant ownership contract permits | Explain ownership; never steal reservation |
| Shared-listener/multiplex capability absent on either peer | Use dedicated listener only if policy permits; otherwise refuse | Disclose dedicated-port choice and why shared transport is unavailable |
| Required carrier/capability not negotiated or unsupported by installed version | Refuse; no downgrade | Name missing capability and upgrade/configuration action |
| No candidate satisfies all hard constraints | Refuse deployment | Show ranked rejection reasons and allow user to change constraints |
| Evidence or plan changes between Review and Deploy | Refuse and require new Review | Keep no partial side effects; show stale-plan reason |

## Port-role rules

- **Public client / IR ingress:** default equals the selected EX application destination port. This is a compatibility default, not proof the port is free or externally reachable. Reserve and validate it on the IR node; never touch unrelated services or the protected legacy Imperial Crown `2053` listener.
- **IR route listener:** loopback/local route endpoint for the client-facing ingress to forward into BAFT. Validate local binding and conflict/ownership separately from the public ingress port.
- **EX transport listener:** BAFT's authenticated outer transport. Pick only from compatible path-probed candidates and live occupancy/ownership inventory. Shared listener requires explicit negotiated multiplex support and per-tenant isolation; otherwise dedicated listener.
- **EX target:** the actual application service host/port, such as a 3X-UI inbound. Probe target reachability from EX independently; do not confuse it with the transport listener.
- IPv6 literals must be bracketed only at host:port formatting boundaries; internally parse/store address and numeric port separately. Do not compare raw strings to establish address equivalence.

## Carrier / topology choices

- Present topology (Direct/Reverse), engine, carrier, and connection initiator as distinct properties. Reverse is available only when the installed engine's capability report defines its listener placement, outbound initiator, NAT/firewall behavior, and rollback; otherwise disable it with a reason.
- Smart Auto may choose only installed and authenticated engine/carrier capabilities. It must not treat a planned, absent, or stale capability as installed support.
- Surface only carriers that are both implemented and negotiated on both nodes: for example `carrier.h2`, `carrier.ws`, `noise.v1`, and `shape.v1` as appropriate. A missing or required-but-unmet capability is a typed refusal, not a downgrade.
- Show uTLS/browser-fingerprint as a separate optional capability only when the selected carrier's installed implementation reports support and its server authentication still validates. Explain the compatibility/certificate requirements. Never claim that uTLS, WebSocket, HTTP/2, or port 443 guarantees censorship resistance.
- If the user explicitly selects a carrier, do not switch carriers silently. If it is unavailable, block and offer the alternatives as choices.

## UX contract (FA/EN, RTL/LTR, mobile)

Basic mode contains only: IR node, EX node, destination service/port, and public client port (auto by default). Show one compact transport recommendation with confidence/evidence age, then Review/Deploy. Advanced contains topology, engine, carrier, EX transport listener port, address family, optional tuning, and explicit capability constraints.

Every recommendation displays:
- selected carrier and topology; who initiates the connection;
- the four port roles in a simple labeled mapping;
- probe state/class, timestamp/age, RTT and trickle result;
- shared versus dedicated listener and whether multiplex capability is confirmed;
- any public-client-port mismatch and required external 3X-UI subscription rewrite;
- rejected alternatives with concrete reasons, not raw internal errors only.

Persian strings (canonical):
- `انتخاب پیشنهادی بر اساس آزمون واقعی مسیر`
- `شواهد مسیر منقضی شده؛ قبل از ادامه دوباره آزمون بگیر`
- `هیچ مسیر امن و سازگاری با محدودیت‌های انتخاب‌شده پیدا نشد`
- `اتصال TCP به‌تنهایی انتقال کامل داده را ثابت نمی‌کند`
- `پورت در اختیار سرویس دیگری است؛ آن سرویس تغییر داده نمی‌شود`
- `قابلیت لازم روی هر دو نود تأیید نشده؛ تنزل خودکار غیرفعال است`
- `پورت عمومی با پورت مقصد سرویس متفاوت است؛ ممکن است بازنویسی Subscription در 3X-UI لازم باشد`
- `حمل‌ونقل مشترک تأیید نشد؛ پورت اختصاصی پیشنهاد می‌شود`
- `شواهد یا طرح تغییر کرده؛ مرور را دوباره انجام بده`

English equivalents:
- `Recommended from measured path evidence`
- `Path evidence is stale; re-probe before continuing`
- `No secure compatible path satisfies the selected constraints`
- `A TCP connect alone does not prove full data delivery`
- `This port belongs to another service; BAFT will not modify it`
- `Required capability is not verified on both nodes; automatic downgrade is disabled`
- `Public client port differs from target port; 3X-UI subscription rewriting may be required`
- `Shared transport is not verified; a dedicated listener is recommended`
- `Evidence or plan changed; review again before deploying`

Maintain existing BCC behavior, accessible focus/keyboard behavior, FA/EN, RTL/LTR, dark/light, and mobile layout. Do not use color alone to communicate PASS/WARN/FAIL. Every block must explain what the user can do next.

## Acceptance tests

1. Deterministic ranking for the same fresh evidence, including ties; identical input yields identical recommendation and plan hash.
2. Stale, absent, future-dated, wrong-pair, wrong-family, incomplete, and failed probe evidence cannot produce a production recommendation.
3. `FULL_DATA` + passed trickle is eligible; every degraded/unknown class follows the table above.
4. Occupied, foreign-owned, reserved, and concurrently claimed ports are refused without modifying the owner; explicit conflict does not fall back to another port unless the user enabled Auto and the alternative passes every constraint.
5. Public client port defaults to target port; explicit mismatch shows FA/EN warning and requires acknowledgement; no claim that BCC rewrites 3X-UI subscriptions.
6. Port roles remain distinct for equal and unequal values; IPv4/IPv6 and bracket formatting are covered.
7. Shared listener is selected only with verified negotiated multiplex capability and tenant isolation; otherwise dedicated listener or explicit refusal.
8. Missing/required carrier capability, unavailable uTLS, unsupported topology, or explicit carrier mismatch refuses without silent downgrade.
9. Plan review performs no mutation; stale hash/evidence/ownership at Deploy blocks before effects.
10. Re-probe is an explicit operation with a durable job/result; wizard render is side-effect free.
11. FA/EN copy, RTL/LTR, mobile, keyboard/accessibility, dark/light, and error-remediation tests.
12. Existing 3X-UI, legacy tunnel, and protected port 2053 regression tests remain green.

## Non-goals and safety

This selector does not create subscriptions or credentials, bypass the peer capability handshake, guarantee censorship resistance, or certify external reachability from a private namespace. Path probes are evidence about the tested ordered pair/family/port at a time, not a permanent promise. No production host, main branch, release tag, or signing key is touched by this design.

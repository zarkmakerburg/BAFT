# Hybrid Application Gateway — Agent Execution Instructions

**Project:** BAFT  
**Architecture contract:** ADR-004 — Hybrid Application Gateway  
**Related contracts:** ADR-001 (authority, evidence, Journal and ownership), ADR-002 (C-BAFT capability integration), ADR-003 (host ingress)  
**Tracking:** https://github.com/zarkmakerburg/BAFT/issues/67  
**Architecture PR:** https://github.com/zarkmakerburg/BAFT/pull/297  
**Status:** Design and LAB instruction only. This file does not authorize merge, release, deploy or production network changes.

## 1. Mission

Implement the Hybrid Application Gateway only after validating the current repository and ADR-004 against actual code. Required behavior:

- Domestic users reach domestic BAFT and domestic services without Starlink.
- The domestic BAFT server has no international default route and is not an Internet router.
- A separately enrolled Edge node has a domestic-side connection and a Starlink WAN connection.
- Only explicitly authorized external application requests may traverse the Edge.
- The Edge must never become an open proxy or an uncontrolled bridge between WAN and domestic networks.
- Failure of Starlink, Edge, HQ or central Journal must not break direct domestic service access.
- All actions remain under BCC authority, signed policy, Agent effect fencing, Journal evidence and ownership-safe recovery.

Do not implement a generic VPN, transparent proxy, SOCKS server, CONNECT proxy, arbitrary TCP/UDP forwarder or IP router as a shortcut. Those require separate product/security decisions and a new ADR.

## 2. Mandatory startup — verify before editing

1. Read ADR-001 through ADR-004, this guide, the current v0.5.x execution plan and baseline audit.
2. Fetch live GitHub state: main, active development branch, PR #297, Issue #67, related open PRs/issues, tags/releases and exact-SHA checks.
3. Verify whether PR #297 is still open and whether ADR-004 has been merged or superseded. Never assume branch names or SHAs in this document remain current.
4. Inspect actual BCC, Agent, identity/enrollment, signed-job, policy, transport, Journal, firewall, DNS, installer, UI and test implementations. Record exact file paths and interfaces.
5. Search for existing request/response, reverse connection, streaming, TLS/Noise and policy mechanisms before designing new ones. Reuse only after proving their security and lifecycle semantics.
6. Check Issue #67 and owner governance before writes. Preserve concurrent work. No reset, force-push, destructive cleanup or branch deletion.

Do not start by writing a new subsystem. First produce a compact code map: existing capability, path/API, reusable parts, missing behavior, tests and exact-SHA evidence.

## 3. Non-negotiable architecture invariants

### Domestic server
- No Starlink NIC, international default route, NAT gateway role or forwarding path to the Edge WAN.
- It remains the authority for domestic user authentication, user policy, request admission, session/accounting and local audit.
- It must continue serving domestic users/services while Edge, Starlink and HQ are unavailable.

### Edge
- Separate domestic and WAN interfaces; no L2 bridge and no kernel IP forwarding between them.
- WAN ingress is deny-by-default. No unsolicited WAN-to-domestic access.
- It may access only explicitly allowlisted domestic dependencies.
- It executes only signed, bounded, unexpired policy for its own enrolled identity.
- It cannot grant user permissions, widen destinations, invent policy or silently bypass the domestic server.

### Domestic services
- User-to-domestic-service traffic remains direct.
- A domestic destination must never hairpin through Starlink.
- Ambiguous destination classification is denied, not guessed.

### Management and audit
- HQ/BCC is not an inline dependency for every user request.
- Locally valid, unexpired policy may continue only within its defined lease and revocation semantics.
- Local Journal must survive central Journal outages; synchronization must be idempotent.
- No UI status may claim ready/connected/active without the evidence required by ADR-001.

## 4. First vertical slice — strict application relay

Start with exactly one narrowly scoped HTTPS request profile. Do not expose arbitrary destination strings supplied by users.

1. User authenticates to domestic BAFT.
2. BCC/domestic policy authorizes a named operation for that user and destination ID.
3. BCC issues a bounded, signed request authorization bound to the enrolled Edge identity and current policy version.
4. Domestic server sends the request over an authenticated, integrity-protected Server–Edge channel. Prefer existing BAFT TLS/Noise facilities after audit.
5. Edge validates issuer, signature/binding, principal reference, Edge identity, scope, expiry, replay/idempotency key, policy version, destination ID and resource limits.
6. Edge resolves the destination and validates the actual resolved address immediately before connecting. It enforces method, header, body, response, redirect, concurrency and deadline limits.
7. Edge makes only the approved outbound HTTPS request over its WAN route.
8. The response is bound to the original request ID and authorization context; domestic BAFT validates it before returning it to the user.
9. Both sides persist required lifecycle/effect/result evidence without leaking secrets or unnecessary content.

Initial method/header/destination allowlists, TLS/SNI/Host rules, request/response size caps, timeout values, concurrency and rate limits must be explicit policy values with conservative defaults. If the initial real use case or these values are unspecified, stop before exposing a production interface; use documented test fixtures in LAB only.

Redirects should be disabled initially. If later enabled, every redirect target must be independently re-authorized and DNS/address-validated. Never trust a hostname check performed only at configuration time.

## 5. Security requirements

- Unique enrolled identity/certificate per Edge; mutual authentication and key rotation/revocation.
- No shared bearer token in a URL, setup link or client-side configuration as an authority mechanism.
- Bind every request to a request ID, user principal reference, Edge identity, policy version, destination ID, operation, expiry and idempotency key.
- Reject expired, replayed, cross-user, cross-tenant, wrong-Edge, wrong-policy and out-of-scope requests.
- Protect against SSRF, DNS rebinding, redirect abuse, Host/SNI mismatch, IPv4/IPv6 policy gaps and resolver bypass.
- Deny loopback, link-local, multicast, unspecified, private and management ranges unless a narrowly justified domestic dependency is explicitly approved by policy. External destination policy must never accidentally permit domestic/private ranges.
- Validate resolved IPs at connect time and bind the actual socket destination to the authorized result.
- No plaintext fallback if mTLS/channel validation fails.
- Bound queues, memory, concurrency, request/response bytes, connect/read/write time and retries. Implement cancellation and backpressure.
- Do not automatically retry non-idempotent operations after an ambiguous timeout or partial external effect.
- Redact credentials, cookies, authorization headers, private keys, tokens and sensitive bodies from logs, errors, metrics and audit output.
- Keep firewall and routing default-deny. Do not mutate production routes, firewall, DNS or host services during discovery or unit/LAB work.
- Do not claim Starlink availability, legality, censorship resistance or performance. A simulated uplink is not a physical Starlink test.

## 6. Required work sequence

### HAG-0 — Repository and environment discovery
Deliver:
- BCC/Agent/Journal/identity/transport/UI code map.
- Existing interfaces that can be safely reused and the gaps they leave.
- Threat model and explicit supported HTTPS request profile.
- LAB topology proposal: domestic user, domestic server, Edge with two isolated interfaces, domestic service, external test endpoint, HQ and optional central Journal.
- Record which components are physical, VM, container or simulated.

No production changes.

### HAG-1 — Isolation and connectivity LAB
Create an isolated, reproducible namespace/container/VM test environment. Prove:
- Domestic server has no international route.
- Edge has separate domestic and WAN paths.
- Server–Edge communication is mutually authenticated and bidirectional, including return path.
- Edge cannot forward arbitrary packets between interfaces.
- Domestic users can still reach domestic services with Edge/Starlink stopped.
- WAN-originated connections cannot reach domestic services.

Capture route tables, interface addresses, firewall rules, packet/connection evidence and exact commands. A namespace test proves namespace behavior only; it does not prove a physical Starlink network.

### HAG-2 — Protocol and policy contract
Specify and test:
- Versioned request/response envelope.
- Signed authorization and principal/Edge/destination binding.
- Expiry, replay and idempotency semantics.
- Destination ID registry and DNS/IP validation rules.
- Method/header allowlists and body/response caps.
- Deadlines, cancellation, backpressure and error taxonomy.
- Journal state transitions and uncertain external-effect behavior.

Fuzz malformed, oversized, duplicated, expired and cross-boundary messages before runtime integration.

### HAG-3 — Secure relay prototype
Implement the smallest vertical slice using existing BCC/Agent authority and channel patterns. Do not add a new unauthenticated listener or independent policy authority. Keep the relay behind an explicit LAB-only feature gate until all security gates pass.

Positive case: one allowlisted HTTPS operation succeeds and returns a request-ID-bound response.

Negative cases: unknown destination, arbitrary URL/IP/port, invalid identity, invalid signature, expired/replayed authorization, wrong Edge, changed policy version, private/link-local target, DNS rebinding, redirect, Host/SNI mismatch, oversized payload, slow receiver and rate/concurrency overflow are refused.

### HAG-4 — Failure, recovery and security validation
Inject Edge loss, Starlink loss, channel loss, HQ loss, central Journal loss, clock skew, process crash at each effect boundary and response loss after an external effect. Verify:
- Domestic paths remain operational.
- External operations fail closed.
- Uncertain external effects remain uncertain and are not unsafely retried.
- Local evidence survives and synchronizes idempotently.
- Revoked/expired policy is refused.
- No false success/readiness is displayed.

### HAG-5 — Product integration
Only after HAG-0 through HAG-4 pass:
- Integrate Edge enrollment and lifecycle with BCC.
- Add reviewed policy and destination management.
- Add UI with honest states and reasons.
- Support Persian RTL and English LTR, mobile/desktop, keyboard/accessibility and non-colour status cues.
- Make domestic vs external routing explicit to the operator.
- Ensure no user-facing option can widen policy or bypass review.

### HAG-6 — Release qualification
Run exact-SHA CI, security/static analysis, unit/integration/fuzz tests, install/upgrade/rollback tests and multi-node LAB. Review dependency and licensing provenance. Publish no release until the owner separately authorizes it and all required gates pass.

## 7. Mandatory acceptance matrix

Execute and record at minimum:

1. Domestic server has no international default route.
2. Edge WAN and domestic interfaces are isolated; forwarding/bridging is disabled.
3. Server–Edge mutual authentication and bidirectional return path work.
4. Domestic user reaches domestic service directly with Edge and Starlink disabled.
5. Approved HTTPS request succeeds through Edge.
6. Arbitrary URL/IP/port/CONNECT/open-proxy attempt is rejected.
7. SSRF, private/link-local target, DNS rebinding and unauthorized redirect are rejected.
8. IPv4/IPv6 and DNS behavior cannot bypass destination policy.
9. Replay and duplicate non-idempotent requests cannot repeat effects.
10. Oversized payload, slow receiver and concurrency exhaustion are bounded.
11. Starlink outage stops only external requests.
12. Edge outage does not affect domestic server/services.
13. HQ outage does not expand authority; expired policy is denied.
14. Central Journal outage preserves local evidence and idempotent later sync.
15. Partial external effect/response loss is reported as uncertain; no unsafe retry.
16. Unsolicited WAN ingress cannot reach domestic services.

For every test record: test ID, purpose, exact command/test name, source SHA, environment type, topology, expected result, actual result, exit code, logs/artifacts and limitations. Mark NOT RUN, BLOCKED, PASS or FAIL; never convert a skipped test into a pass.

## 8. Reuse and C-BAFT rules

- Reuse BAFT's BCC authority, node identity, signed jobs, expiry/replay, TLS/Noise, Journal and evidence model where they meet the contract.
- Do not copy C-BAFT's shared-token links, panel-direct host mutation, comment-based firewall cleanup or unreviewed crypto/transport code.
- C-BAFT features are inspiration, not an implementation dependency.
- Any future source-code port must follow ADR-002 attribution, license, provenance and per-file notice requirements in the same commit.
- Do not add SOCKS, local proxy, Multi-Lane, FEC, UDP-Crypt, kernel tunnels or PMTU work to this gateway slice unless a separate accepted design and security review requires them.

## 9. GitHub, branch and collaboration rules

- Verify Issue #67, current PRs, branch protections and owner directions before every material write.
- Work only on the currently authorized feature branch; do not assume PR #297's branch remains authorized after it is merged, closed or superseded.
- Never overwrite another agent's changes. Coordinate shared files and commits.
- No force-push, destructive reset, branch deletion or cleanup of unknown work.
- Keep LAB preview artifacts out of GitHub unless explicitly needed and authorized.
- Update this guide, ADR-004, baseline audit and Issue #67 when decisions or evidence change.
- Do not merge, tag, release, deploy or modify production hosts without explicit owner authorization.
- If a required credential, physical node, permission or environment is unavailable, record the exact blocker and continue independent safe work.

## 10. Efficient execution and token discipline

- Start from the current exact SHA; do not reread the entire repository repeatedly.
- Use targeted code search and inspect relevant file ranges. Record paths, symbols and interfaces once in the baseline audit.
- Parallelize independent read-only inspection; serialize shared writes.
- Reuse exact-SHA CI evidence only when it covers changed code and relevant acceptance gates.
- Do not create a new abstraction before searching for an existing one.
- Implement one vertical slice at a time; run the narrowest relevant tests first, then broader suites.
- Fix root causes. Never weaken tests, hide failures, skip security checks or relabel failures to make CI green.
- Keep commits focused, messages descriptive and diffs reviewable.
- Update the handoff/evidence as each stage completes so the next agent does not repeat discovery.
- Reports are short; engineering work and reproducible evidence take priority.

## 11. Definition of done

The gateway is not complete until:
- all relevant acceptance cases pass on the exact candidate SHA;
- security review confirms the relay cannot become an open proxy or domestic/WAN bridge;
- domestic service continuity is demonstrated with Edge and Starlink disabled;
- signed authority, node identity, request binding, replay/idempotency, Journal and recovery are verified;
- failure cases produce honest states and no false readiness;
- install/upgrade/rollback and UI are tested where changed;
- docs, tests, commits and Issue #67 evidence agree;
- remaining risks and untested physical conditions are explicit.

Never state that the gateway is implemented, LAB-verified, Starlink-tested, production-ready or released based only on this guide, diagrams, mocks, a passing unit test or a namespace simulation.

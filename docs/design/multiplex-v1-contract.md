# Shared Transport and `multiplex-v1` Contract (Proposal)

**Status:** Draft design contract; not an implementation claim  
**Scope:** BAFT Issue [#269](https://github.com/zarkmakerburg/BAFT/issues/269)  
**Baseline:** `main` at `d4348c2ace51e56469b10aee2806f3ad4869c8fa`  
**Related proposals:** PRs #287, #289 and #290 (all remain unmerged Drafts at the time this contract was prepared)

## 1. Purpose and current boundary

Allow multiple independently managed logical tunnels between the same IR/EX pair to reuse one physical inter-node listener when, and only when, both installed endpoints prove support for the same versioned multiplex protocol and the listener can safely admit another tenant.

This document defines the contract and acceptance requirements. It does not claim that `multiplex-v1`, BCC-managed end-to-end lifecycle, or public ingress is implemented.

The current safe behavior remains a dedicated listener per tunnel when shared multiplex is unavailable, unverified, incompatible, or unsafe. A port number (including TCP/443) is not evidence of censorship resistance or protocol capability.

## 2. Terms and ownership

- **Physical listener:** one OS listener/socket bound on EX, owned by exactly one BAFT listener-owner record.
- **Logical tunnel / tenant:** an independently authenticated and managed tunnel routed through a physical listener.
- **Owner tenant:** the tenant whose transaction created and owns the physical listener. It is not privileged to impersonate or mutate sibling tenant identities.
- **Tenant membership:** a durable, authenticated association between a tunnel identity and a physical listener generation.
- **Negotiated capability:** a capability supported by both installed endpoints and proven by fresh, authenticated evidence. UI selection or a configured port is not proof.
- **Listener generation:** a monotonically advancing generation binding owner identity, listener configuration, tenant membership and the corresponding durable ownership record.

The listener owner and each tenant must have distinct durable identities. Removing or replacing the owner must be refused while dependent tenants remain, unless an explicit, separately reviewed owner-transfer protocol is implemented.

## 3. Protocol negotiation and compatibility

### 3.1 Capability advertisement

1. Each endpoint advertises its installed implementation capabilities through the authenticated control channel.
2. The advertisement is tied to the installed binary identity/version and expires according to a documented freshness limit.
3. A stale, missing, unauthenticated, contradictory or unknown capability report is treated as unsupported.
4. The plan records the exact endpoint pair, binary identities, capability snapshots, listener identity/generation and selected protocol version.

### 3.2 Version selection

1. The initial protocol identifier is `multiplex-v1`.
2. Both endpoints must explicitly advertise `multiplex-v1`; otherwise shared multiplex is unavailable.
3. A client must not silently downgrade an explicitly required shared-multiplex plan to an older or weaker mode.
4. If the operator selected automatic mode, failure to negotiate may select the documented dedicated-listener fallback only after a fresh preflight and explicit review of the resulting plan.
5. Unknown major versions are rejected. Compatible minor extensions must be length-delimited or otherwise safely ignorable and must not change the meaning of existing mandatory fields.
6. Negotiation output is authenticated and bound to the reviewed plan. Deploy revalidates it atomically with the plan-hash comparison and resource reservation.

### 3.3 Framing and routing

The wire format must be versioned, bounded and unambiguous. Every logical stream/frame must carry:

- protocol version;
- authenticated tenant/tunnel identifier;
- stream identifier scoped to that tenant;
- frame type and bounded payload length;
- sequencing/flow-control metadata required by the transport.

The receiver must authenticate the peer and resolve the tenant identity before forwarding payload to a destination. A caller-supplied tenant ID is a routing hint, never an authentication credential. Unknown, retired, unauthorized or generation-mismatched tenants fail closed.

The exact byte-level frame layout, maximum frame size, stream-ID allocation, rekey behavior and cryptographic binding remain implementation decisions and require a protocol-specific review before code is accepted.

## 4. Isolation and resource controls

Each tenant must have independent:

- authenticated identity and authorization;
- stream namespace and lifecycle;
- destination/route binding;
- byte and connection accounting;
- configured connection, stream, buffer and rate limits;
- cancellation and error state;
- metrics and audit attribution.

A tenant must not consume unbounded shared memory, descriptors, goroutines, queues or CPU. Admission and runtime enforcement must include per-tenant limits and listener-wide aggregate limits.

Backpressure must be propagated without allowing one slow or malicious tenant to stall unrelated tenants. Queue saturation, quota exhaustion, malformed frames and tenant cancellation must be isolated to the affected tenant unless the physical listener itself is unhealthy.

Cancellation and teardown must release only resources owned by the affected tenant. A tenant failure must not close sibling streams or the physical listener while another valid tenant remains.

## 5. BCC lifecycle contract

All membership changes must flow through the ordinary reviewed BCC plan/deploy and signed Agent job/receipt path. A low-level add/remove endpoint alone is not end-to-end lifecycle integration.

### Add / join

1. Preflight verifies both endpoint capabilities, peer identity, listener ownership/generation, route reachability, destination health, port availability and resource headroom.
2. The reviewed plan identifies the existing physical listener and the exact new tenant identity/configuration.
3. Deploy revalidates the plan hash, capabilities, listener generation and ownership under the same durable transaction boundary used for admission.
4. The signed operation adds only the new tenant, validates staged configuration, applies it transactionally and returns authenticated evidence.
5. BCC marks the tenant active only after validating the receipt against the exact operation, tenant, listener generation and expected digest.

### Remove / retire

1. Removal names one exact tenant and expected listener generation/digest.
2. The operation refuses stale ownership, unexpected sibling state or a mismatched tenant identity.
3. Only that tenant's streams, configuration and accounting are retired.
4. The physical listener remains active while any valid tenant remains.
5. The physical listener may be retired only when the final tenant is removed and the owner transaction proves that no dependent membership or active stream remains.
6. Retries are idempotent and must not manufacture success from missing receipts or incomplete host evidence.

### Rollback and recovery

- Every mutating operation records durable before/after digests and an operation identity before host mutation.
- Recovery is idempotent and reconciles only resources whose ownership is proven by the durable transaction.
- Unrelated config, units, sockets, firewall rules and services must remain untouched.
- Ambiguous ownership, missing evidence or drift fails closed and requires operator remediation.
- Restart/reboot recovery must be tested separately from in-process rollback.

## 6. Public ingress and dedicated fallback

Shared inter-node transport does not replace the public ingress lifecycle. Public client ingress on IR remains a separately owned, health-checked and transactionally managed resource.

If shared multiplex cannot be proven, BCC must offer a dedicated EX listener only after checking port conflicts and preserving existing services. It must never take over an unrelated listener or change the existing Imperial Crown service on port 2053.

The default public client port and 3X-UI inbound port relationship must remain explicit in the wizard. BAFT must not claim to update 3X-UI subscription records automatically unless an authenticated integration actually performs and verifies that operation.

## 7. Wizard and API behavior

- Basic flow exposes IR, EX, destination and client public port, followed by one Review/Deploy action.
- Advanced controls expose transport and tuning without making unsupported modes selectable.
- Shared mode is shown as available only when fresh endpoint evidence proves compatibility and the exact listener has safe capacity.
- The review screen explains whether the plan reuses a listener or creates a dedicated one, including the reason for fallback.
- A change to endpoint pair, port, transport, capability evidence, listener generation or tenant identity invalidates the reviewed plan.
- Deploy revalidates all plan-bound evidence; the browser is never authoritative for capability or ownership.
- Create, edit, remove, retry and rollback use the same durable lifecycle invariants.
- Persian RTL and English LTR mobile flows must expose the same security decisions and errors.

## 8. Required acceptance matrix

| Area | Required positive evidence | Required negative / isolation evidence |
|---|---|---|
| Negotiation | Two compatible endpoints negotiate `multiplex-v1` and route two tenants | Missing, stale, mismatched and unknown capability fail closed; no silent downgrade |
| Authentication | Each tenant reaches only its authorized destination | Spoofed tenant ID, wrong key, replay and retired identity rejected |
| Concurrency | Multiple simultaneous streams and tenants make progress | Slow tenant, quota exhaustion and cancellation do not stall or terminate siblings |
| Lifecycle | Add, join, remove, re-add and final-owner retirement | Duplicate add, stale generation, owner removal with siblings and conflicting digest rejected |
| Failure | Partial apply rolls back exactly; retry is idempotent | Lost receipt, process restart and ambiguous host state never produce false success |
| Preservation | Existing tunnel remains byte-for-byte functional through sibling add/remove | Existing config, key, unit, firewall, socket and unrelated service remain unchanged |
| Fallback | Unsupported endpoints use reviewed dedicated mode | Occupied port or unsafe listener ownership blocks deployment |
| Public ingress | IR ingress routes to the selected 3X-UI inbound and survives restart | Existing port 2053 and unrelated services are preserved |
| UX | Wizard review reflects the actual selected mode and evidence | Stale asynchronous response or edited form cannot restore a deployable stale plan |

Every acceptance record must identify the exact source SHA, binary SHA(s), test command/workflow, environment, artifacts and result. Hosted CI, simulated network LAB, real-process tests and physical-host/systemd tests must be labeled distinctly; none may be substituted for another.

## 9. Release gates

Issue #269 remains open until all required behaviors are implemented and evidenced on an exact candidate SHA. At minimum:

1. Protocol and compatibility review approves the wire contract and downgrade behavior.
2. BCC owns the complete add/remove/retry/rollback/recovery lifecycle.
3. Per-tenant isolation and resource-limit tests pass under concurrency and failure injection.
4. Public ingress and dedicated fallback preserve unrelated services.
5. Wizard/API plan binding and RTL/LTR mobile acceptance pass.
6. Required CI and isolated-LAB workflows pass on the exact same candidate SHA.
7. Required real-process and host-level acceptance is completed according to the repository's current release policy.
8. Independent review finds no unresolved blocking security or correctness issue.

No merge, tag, release or production deployment is authorized by this document.

## 10. Open design decisions before implementation

These decisions must be resolved in a focused ADR or protocol review, not guessed in code:

- Whether the shared listener's transport security is TLS-only, Noise-only, or a composed authenticated design, and how tenant identity is cryptographically bound to the session.
- Exact frame encoding, stream multiplex semantics, flow control and maximum resource bounds.
- Rekey/credential rotation behavior for one tenant without disrupting siblings.
- Whether listener-owner transfer is needed; default policy is to reject owner replacement while tenants remain.
- Exact compatibility policy for future minor/major versions and capability freshness windows.
- How public ingress ownership and shared inter-node listener ownership are represented and reconciled without conflating their lifecycles.

## 11. Implementation sequence

1. Resolve the open security/protocol decisions and approve a byte-level `multiplex-v1` specification.
2. Add protocol negotiation and compatibility tests without enabling shared mode by default.
3. Complete native tenant stream routing and per-tenant resource isolation.
4. Wire join/remove into BCC's ordinary reviewed lifecycle, including owner/sibling digest synchronization.
5. Add transactional recovery and preservation/failure tests.
6. Enable Wizard capability display and plan-bound review only after backend evidence is authoritative.
7. Run exact-SHA CI, isolated LAB, real-process and required host acceptance.
8. Enable shared mode only after all release gates pass.

---
This proposal is intentionally conservative: until the complete path is implemented and verified, dedicated transport remains the safe fallback and `multiplex-v1` must not be advertised as supported.

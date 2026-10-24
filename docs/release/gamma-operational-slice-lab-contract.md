# Gamma — P0 #269 operational vertical-slice LAB acceptance contract

Scope: **disposable isolated Linux LAB only**. Physical/production hosts are not required. This is an execution checklist, not an implemented/deployed ingress flow. Gamma's `TestGammaRealIngressContractPreflightRefusalMatrix` checks real shared Go types and rejects unminted/zero authority, but **does not simulate Apply success**.

## Canonical real-path checkpoints, in order

1. **BCC reviewed request** (Beta): create and persist a reviewed plan with canonical **client/config TCP port**, the exact persisted BCC `Plan.Hash`/ReviewedPlanHash, stable operation and lease IDs, IR node ID, listen endpoint and local target. Explicit API path and durable read-back are required.
2. **Authenticated command dispatch** (Beta + Alpha): BCC enqueues a **signed**, bounded-expiration `ingressauthority.CommandV1` with action **`public_ingress_apply`** and the complete validated `BindingV1`; Alpha independently authenticates and verifies node/job/plan/binding identity, derives policy locally, obtains an actually held durable OS authority `*Lease`, and invokes a real executor. A valid `BindingV1.Validate()` or read-only preflight is **never** the authority to mutate.
3. **Real listener/firewall** (Delta): executor installs only lease-owned listener and narrowly scoped firewall rules in the isolated Linux namespace. Independently query kernel socket and nftables tables and send actual TCP bytes end-to-end to the declared target. Verify active unrelated sibling and stopped/persistently reserved foreign claims are preserved.
4. **Authenticated receipt** (Alpha/Agent -> Beta/BCC): enrolled Agent posts the **actual** `ingressauthority.ReceiptV1` to the **BCC HTTPS receiver** using its existing per-node credential, with certificate-validated server TLS. BCC matches the enrolled sending node with independently persisted command ID, action, full `BindingV1`, plan digest, operation/lease, listener/firewall evidence and generation; only verified physical kernel read-back allows active state.
5. **Config read-back** (Beta): the server card and generated user configuration expose exactly the *original reviewed* canonical client port. A late listener success cannot bypass a missing/invalid receipt.

## Refusal matrix — must fail closed without modifying foreign resources

| Fault injected in LAB | Required observed outcome |
|---|---|
| Wrong node/enrollment credential; untrusted BCC TLS | Receipt rejected; no active state |
| Wrong exact persisted plan SHA, operation/lease, generation, or config port | Command/receipt refused; no config-port alias |
| Missing/expired/forged signed command or noncanonical action | No Agent Apply |
| Zero/nil/expired/lost durable authority, wrong inode/policy/version | No listener/nft mutation; reconciliation flagged |
| Active IPv4/IPv6 alias listener, stopped foreign persistent reservation, unknown inventory | No bind/firewall changes |
| Interrupted Prepare/Commit/Apply, process SIGKILL, restart | Recover held journal/resources or report uncertain BLOCKED, no duplicate ownership |
| Partial listener success with firewall failure, or receipt delivery failure | Safe owned rollback/reconcile, never success UI |
| Trailing JSON, oversized whitespace, unknown/duplicate receipt keys, HTTP downgrade/redirect | Reject request; no authenticated receipt |
| Retire or restart with sibling ingress and unrelated services | Exact owned resources retired/recovered; sibling/foreign remains byte-for-byte unchanged |

## Exact missing upstream API seams required from HQ

- **Beta:** public BCC reviewed-plan request/commit/API endpoint, persisted plan-hash operation/lease read-back, signed Apply enqueue and ACK, enrolled-Agent HTTPS receipt receiver route plus credential lookup, and exact user-config/server-card read-back.
- **Alpha:** accepted signed `CommandV1` dispatch into Agent, trusted local managed-node policy constructor, actual held `*Lease` mint/check with durable cross-process restart/lock release, executor invocation, and Agent->BCC receipt POST.
- **Delta:** executable Apply/Retire adapter accepting the *held* `*Lease`, supported managed-node inventory coverage collector with stopped/active claims, nft/listener ownership identifiers and read-back/rollback hooks.
- **HQ:** publish the single exact common SHA and callable interfaces, plus accepted isolated-Linux fixture bootstrap and ownership of integration. Gamma does not edit Alpha/Beta/Delta code or QA branch.

**Current status:** **BLOCKED** for real operational BCC→signed Agent→held authority→TCP+nft→authenticated receipt→config. Existing HTTPS receiver fixtures and BindingV1 validation tests are **transport/data refusal only**; they do not prove real Apply.

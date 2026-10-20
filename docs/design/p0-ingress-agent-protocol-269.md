# P0 Public Ingress — agent protocol gate

Tracking: #269 / draft PR #270.

The existing agent job protocol (`internal/agentjob/agentjob.go`) uses an explicit action allowlist, strict parameter validation and signed jobs. Therefore, BCC must **not** implement public ingress via arbitrary shell commands or by overloading unrelated tunnel actions.

## Proposed explicit signed actions
- `public_ingress_preflight`: read-only inspect host bind availability, existing BAFT ownership, firewall manager, and target loopback listener; return structured evidence.
- `public_ingress_prepare`: stage an owned, idempotent ingress configuration with generation and digest; do not expose public port yet.
- `public_ingress_commit`: atomically enable only the owned TCP listener, apply narrow firewall access if required, and verify listener/target; fail closed.
- `public_ingress_inspect`: return ownership, generation, digest, socket state and firewall evidence.
- `public_ingress_rollback`: remove only changes owned by the matching tunnel/generation; never touch other services.
- `public_ingress_retire`: final removal with explicit ownership and digest checks.

Each action requires validated `tunnel_id`, `instance_id` where applicable, `public_port`, `local_target`, `generation` and/or `digest` as appropriate. The current read-only preflight and manual listener accept public ports 1024..65535; the BCC planning contract permits 1..65535 but that broader range is not an apply capability. Supporting a privileged port such as 443 requires a separately reviewed binding and privilege strategy before enabling it in the wizard. Port 2053 is not globally banned: preserve an occupied listener and admit it only when free and ownership is proven. Validate local targets as loopback only; reject duplicate port ownership. Avoid opening a wildcard listener until all checks pass.

## Transaction ordering
1. BCC resolves EX/IR tunnel and reserves port in durable state.
2. Agent preflight checks **actual host** socket occupancy (not only BCC state).
3. Existing tunnel EX/IR prepare/commit and health complete.
4. Public ingress prepare/commit/inspect verify that public client traffic reaches the correct loopback target.
5. Finalize both tunnel and ingress. On any failure, rollback ingress then tunnel, with durable retry and audit.

## Hard requirements
- A successful status transition is **not** proof of a running listener.
- On reboot, reconcile only BAFT-owned resources; fail closed on ownership ambiguity.
- Existing BAFT.goldapp.tr:2053 and other unrelated services must remain untouched.
- Shared TCP/443 requires proven multiplexing, never bypass `tunnelResourceConflict`.
- Owner authorization for merge/release/deploy is already recorded in HQ #67. Keep the PR draft and require all technical, security and real-host acceptance gates before using that authorization.

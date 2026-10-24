# Gamma — canonical operational ingress negative acceptance

Baseline: QA `ea10b24925823da1a0ae74af2a18b320286b7f37`, reviewed by HQ. This is a **disposable LAB-only** plan, not a production-host prerequisite.

## Executable blocked bridge

`GAMMA_ISOLATED_LAB=1 bash tests/e2e/gamma_operational_blocked_path_lab.sh`

This runs the **actual normal Agent.RunOnce signed job** negative receipt tests (`internal/agent/public_ingress_operational_receipt_test.go`) and the **actual BCC HTTPS receiver** bridge (`internal/bcc/ingress_receipt_tls_integration_test.go`). It cannot claim successful Apply; the receipt remains `phase=blocked`, `ownership_verified=false`, empty listener/nft evidence, and pending BCC readiness after reopen. Restart replay and improper executor injection are refused. The script runs durable held-policy/command-intent acceptance only under disposable Linux root conditions; other environments explicitly print BLOCKED for that portion.

## Real-path refusal matrix (no mock Apply PASS)

| Scenario | Canonical API / evidence | Gate |
|---|---|---|
| Unenrolled/wrong Agent credential, wrong origin/TLS or redirect | `Agent.RunOnce` -> actual BCC receipt HTTPS route, enrolled token and exact ACK | Must deny token leakage, receipt acceptance and ready config |
| Forged/replayed/stale signed job or command action | `agentjob` signed envelope -> `RunOnce` + `PendingGuard.RecordCommandIntent` | Deny admission or second Apply; never release held pending reservation |
| Installed policy absent, replaced inode, wrong node/netns or manager/source drift | `ProvisionInstalledManagedNodePolicy`, `PendingGuard.BindInstalledPolicy/CheckInstalledPolicy`, `ingressinventory` collector | Fail closed before listener/firewall mutation |
| Foreign active alias (IPv4/IPv6), stopped/persisted foreign port or unknown inventory | Independent collected source identity and `ingressownership` registry/runtime evidence | Deny mutation even if OS bind seems available |
| Own listener vs sibling owned listener, retire and restart | Authoritative matching lease, ownership evidence, `publicingress` runtime and narrow firewall IDs | Only own exact binding can be retired; sibling untouched |
| Interrupted Apply, partial firewall/listener failure, restart reconciliation | Durable journal/held lock, installed policy and Delta readback | Pending/reconciliation; no forged `applied` receipt or ready config |
| Wrong operation/node/planSHA/jobID/port, duplicate/extra JSON | Shared `ingressauthority.CommandV1/ReceiptV1/BindingV1`; enrolled Agent POST to BCC | Reject, keep canonical ConfigPort unchanged |

**Acceptance remains RED/BLOCKED for operational success.** Canonical `public_ingress_apply` cannot pass until Alpha publishes a live authorized lease/executor with held-policy checks, Delta completes supported inventory/listener/nft effect+readback and owns/sibling distinction, and Beta enables reviewed BCC enqueue+config readiness only after an authenticated exact receipt. Gamma will then use those concrete APIs in one Linux representative EX/IR process/service LAB with real TCP bytes, interrupted restart and foreign service preservation. Do not invent an interface, simulate an Apply success or use a new receipt JSON mirror.

# Shared transport and operational public ingress acceptance

This supplementary contract adds one combined path to the existing operational
ingress and recovery gates. The pinned 28-case manifest is unchanged. A passing
transport-only test and a passing ingress-only test cannot satisfy this contract.

## Actual path

`TestBCCSignedSharedSameIROperationalIngressLifecycle` uses the real TLS BCC
handler to review and create owner and same-IR tenant plans. Normally polling,
enrolled Agents verify signed jobs and use the real reservation store, installed
policy, held lease, durable journal, operational executor and nft backend. Public
TCP payloads cross the shared Noise/TLS transport to distinct marker targets.
Actual Agent receipts return over certificate-validated authenticated HTTPS;
the admin readiness API must return the matching Verified card.

The same owner public connection must survive tenant join, tenant Runtime restart
and reviewed decommission. The owner rule handle, detached receipt, config, key,
unit and state must remain unchanged. Tenant retirement requires its real signed
command and retired receipt, absent owned rule, closed public listener and no
ready card. A separate real TCP/2053 firewall table must remain byte-identical.
The final owner readiness probe runs after the API cache expires.

`TestBCCSignedSharedSameIROperationalIngressPartialAddFailure` delegates one
tenant Add to real nft, then injects a lost-response error. Normal Agent execution
must fail and persist a failed/reconciliation_required receipt without a false
Verified card. Exact rollback must remove the tenant rule and listener while
preserving the owner connection and resources. Reopening durable stores and
constructing a fresh Agent exercises the production restart reconciler: complete
tenant retirement evidence must release the local claim with a tombstone; the
live owner remains fenced and untouched. The old binding must remain unreplayable.

## Explicit boundaries

- Pairing and configuration validation use real `baft`/`baft-pair` binaries.
  The combined LAB supervises real `node.Runtime` goroutines to satisfy the
  installed-inventory sole-process contract. It does not exercise systemd or
  physical hosts. A separate binary-process regression remains required.
- Reconstructing Agent/stores while the owner listener remains alive is durable
  component recovery. It is not process death, a reboot, or proof that public
  connections survive the death of the whole Agent. The separate kill-9 recovery
  gate supplies its own evidence; its result is not substituted for this test.
- The partial failure covers one accepted nft Add followed by a response error.
  It does not cover every crash window or multiple simultaneous failing tenants.
- After this failed Apply, BCC retains its failed receipt and desired port
  reservation. Its signed Retire admission requires applied ownership proof;
  local release cannot manufacture that proof or an authenticated retired
  receipt. Automatic BCC reuse of this failed port remains unavailable. Do not
  weaken the gate or clear bookkeeping to obtain a passing retry.
- The readiness protocol itself is echoed by the ingress listener. The distinct
  marker payload checks independently prove that the target was reached.
- A foreign firewall table proves preservation of that table. It does not prove
  that an arbitrary live foreign service or a production host was exercised.

## Reproducible evidence and release decision

Run `bash tests/release/sharedop/run_lab.sh /absolute/new/evidence-directory` on
a disposable Linux LAB with Go from `go.mod`, root/sudo, iproute2 and nftables.
The output directory must be outside the checkout. Each exact test runs in a
fresh network namespace; Agent TestMain creates its private mount namespace.
The runner records source before/after, HEAD/tree, dirty content fingerprint,
binary hashes, exit codes and test events. Its verdict requires both exact cases
to run and pass, package completion, exit zero, stable source and zero SKIPs.
The workflow requires a clean exact-SHA checkout and preserves the evidence.

Results must be attached to the final candidate SHA in HQ #67. Local WIP passes,
parent checks and this contract alone do not close #269 or authorize a GO decision.
Backup/update/provenance, all applicable CI gates, trusted offline signing and
published-asset acceptance remain separate requirements.

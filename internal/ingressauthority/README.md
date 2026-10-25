# Operational ingress v1 shared boundary

This package is the single shared contract selected by HQ decision
[#6371376812](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6371376812).
The initial source was copied from Alpha's uncommitted `contract.go` and
`contract_test.go` under the bounded HQ handover; the originals remain untouched.
HQ fixed the port policy, explicit v1 marker, explicit defaults and capability
serialization before publication. Durable implementation ownership returns to
Alpha after the shared checkpoint is published.

## Compile-level API

- `BindingV1.Validate()` checks immutable resource shape, explicit effective
  manifest/config bytes, exact proof and canonical endpoint/target. `Port` and
  `ConfigPort` must match. It does not authenticate the reviewed plan or prove
  that a port is available. Port 443 is not globally prohibited; port 2053 has
  no special status. Existing SSH port 22 exclusion is retained.
- `BindingV1.MatchReviewed(expected)` checks exact resource identity equality.
- `CommandV1` contains `IngressProtocol="1"`, `JobID`, `Action`, `IssuedAt`,
  `ExpiresAt` and `Binding`. `Validate(now)` checks shape and a strict current
  admission window, not a signature. A fresh signed Retire command has a new
  job ID and the same persistent resource binding.
- Executors consume **`*Lease`**, an alias of concrete `LiveLease`, and call
  `CheckLockedIngress(ctx, command)`. They must not substitute a public
  interface or callback returning success. The command carries action/expiry;
  persistent ownership does not expire merely because the Apply job expires.
- `ReceiptV1` carries a binding, job/action, phase and typed listener/firewall
  evidence, ownership/rollback status and optional failure cause. It is inert
  data. Node authentication over validated HTTPS and exact expected-job/
  binding comparison are required before BCC accepts it.

## Supported lease scope

`PendingGuard.MintSignedManagedLABLease(ctx, envelope)` can mint only in the
independently measured singleton, loopback-only Linux LAB. It verifies the
original signature against fixed root-protected `/etc/baft-agent/bcc-job.pub`,
the node identity in the protected installed policy, the immutable recorded
command and the same OS-held durable reservation. The only live reservation
store is `/var/lib/baft-agent/ingress-authority`; an alternate otherwise valid
store cannot bypass its global numeric-port claim. The directory must already
exist with protected root-owned ancestry and mode 0700.

The private lease captures Delta's actual executable/process/source/netns and
cgroup pin. Every check rereads the original guard, installed policy, enrolled
key inode/bytes and execution pin. `WithLockedIngress` additionally serializes
one leaf effect with `Lease.Close` and `PendingGuard.Close`; its callback must
not reenter guard/lease methods. Nil, zero and internally flagged leases without
this private state still deny; lease JSON marshal/unmarshal is refused.

A fsynced one-shot JobID marker precedes issuance. Restart, Close and persisted
outcome bytes cannot re-mint the command. A separately signed Retire job can
use the same durable binding, but an Apply lease cannot execute Retire. Closing
the guard or lease never frees an uncertain durable reservation. Inert outcome
records are logs, not proof of recovered physical ownership or a release gate.

This is **LAB-only authority**, not production admission or complete restart
reconciliation. Full normal Agent listener/nft/TCP/authenticated receipt and BCC
acceptance need their own representative end-to-end evidence; a successful
constructor test does not satisfy that release gate. Unsupported, unknown,
drifted or incomplete managed execution coverage continues to deny.

## Import direction and exclusive writers

The package imports dependency-leaf `agentjob`, `release`, `ingressmanifest` and
`ingressinventory` in addition to the standard library. Agent, BCC and
the `ingressownership` executor may import it. Do not import it from
`ingressmanifest` or `publicingress`: manifest already depends on publicingress,
so that would create a cycle. Keep authority independent of runtime/transaction,
Agent and BCC packages. Alpha owns future shared API changes; Delta owns its
adapter/evidence capture, Beta owns BCC/web, Gamma owns acceptance tests.

The external signed action parameter map remains separate from these Go/JSON
types. Alpha must explicitly map and validate the v1 fields in `agentjob`;
serializing `CommandV1` is not a replacement for the signed envelope verifier.

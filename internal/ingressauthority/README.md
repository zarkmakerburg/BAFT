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

## Deliberate implementation boundary

No constructor can mint an operational lease yet. Nil, zero-value and even
internally flagged `LiveLease` values still deny mutation. Lease JSON marshal/
unmarshal is refused. There is no host mutation, new Agent dispatch, operational
signed job schema, BCC Apply enqueue or active-ingress success path in this
checkpoint. Validation and a receipt claiming success never establish authority.

Alpha still must implement signed admission, real held cross-process locks,
durable numeric-port reservations/generation fences and restart reconciliation.
Delta must implement complete trusted active/stopped-service inventory plus
independent exact listener/firewall observation and cleanup. Unknown authority,
lost lock or incomplete inventory must deny mutation. Beta persists exact
authenticated receipts and exposes active configuration only after verification.
Gamma tests the actual full path; this skeleton cannot satisfy that gate.

## Import direction and exclusive writers

The package imports only standard library and `ingressmanifest`. Agent, BCC and
the `ingressownership` executor may import it. Do not import it from
`ingressmanifest` or `publicingress`: manifest already depends on publicingress,
so that would create a cycle. Keep authority independent of runtime/transaction,
Agent and BCC packages. Alpha owns future shared API changes; Delta owns its
adapter/evidence capture, Beta owns BCC/web, Gamma owns acceptance tests.

The external signed action parameter map remains separate from these Go/JSON
types. Alpha must explicitly map and validate the v1 fields in `agentjob`;
serializing `CommandV1` is not a replacement for the signed envelope verifier.

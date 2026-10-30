# Shared transport and operational ingress LAB

This supplementary gate runs two combined-path tests, without changing the
pinned operational-ingress-v1 manifest:

- `TestBCCSignedSharedSameIROperationalIngressLifecycle`
- `TestBCCSignedSharedSameIROperationalIngressPartialAddFailure`

The tests use the reviewed BCC Plan and real HTTPS handlers, ordinary enrolled
Agent polling, the shared transport Runtime, the operational executor and
durable ownership journal, public listeners, kernel nftables, distinct target
payloads, authenticated receipts and the BCC readiness/card API. The positive
case joins, restarts and retires a tenant while holding the owner's public
connection open. The fault case loses a response after a real tenant nft add;
its assertions distinguish safe cleanup from a BCC fence that still requires
reconciliation. A passing fault test does not imply automatic port reuse or
complete reconciliation of the BCC record.

On a disposable Linux LAB runner with Go from `go.mod`, Python 3, iproute2,
nftables and passwordless sudo:

```sh
python3 -B tests/release/sharedop/verdict_test.py
bash tests/release/sharedop/run_lab.sh /tmp/baft-sharedop-evidence
```

The evidence directory must be new and outside the checkout. Each case executes
in a separate network namespace with only loopback. Agent TestMain creates a
private mount namespace with disposable copies of the fixed policy and systemd
unit paths. Missing isolation is a test failure in gate mode. The script never
provisions policy, LAB sentinels or nft rules in the host namespaces. Builds and
module downloads happen before entering the isolated namespaces. Real product
binaries are supplied to the filtered test; the TestMain binary-build shortcut
does not skip either required case.

The verdict requires one top-level RUN and PASS for each exact test, package
completion, exit zero and zero FAIL/SKIP events. Saved log hashes and source
snapshots must match. Evidence records HEAD, committed tree, dirty status,
digests of all tracked/untracked source files, Go version and built binary
digests. Local dirty runs are labelled as such; CI additionally requires the
expected exact SHA and a clean checkout. The verifier's fixtures check that
empty logs, RUN-only tests, hidden skips/failures, nonzero process exit, changed
source, wrong SHA and changed evidence cannot pass.

Transport supervision in these combined tests runs real Runtime goroutines in
one test process, as required by the installed socket-inventory LAB contract.
They do not prove physical-host isolation, external Internet reachability,
systemd process restart or process-death recovery. The workflow separately runs
the existing shared transport regression with real child processes and the
race detector; that complementary result does not enlarge these tests' scope.

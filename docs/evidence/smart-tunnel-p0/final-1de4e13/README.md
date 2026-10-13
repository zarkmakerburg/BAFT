# Exact-candidate validation: 1de4e13

Source commit: `1de4e132efcf5d9d937bd940fdbb8bf13cab7776`.
Source tree: `0468311b47ac39568d281945baa81d2d91625361`.
Clean detached checkout; Go 1.27.1, Linux. This evidence-only branch keeps the
reviewed code commit unchanged.

| Log | Command | Result |
| --- | --- | --- |
| unit-integration.log | `go test ./... -count=1 -timeout 10m` | PASS |
| race.log | `go test -race ./internal/carrier/h2 ./internal/bcc ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout 10m` | PASS; BCC 327.266s |
| live.log | `go test ./internal/tunnelnode -run '^TestTwoLiveInstancesKeepGermanyFlow' -count=3 -timeout 150s -v` | PASS 3/3 |
| ownership.log | `go test ./internal/tunnelnode -run '^TestPKIRollbackRefusesUnverifiedMaterialBeforeChangingLiveState$/^changed-live-owner$' -count=1 -v -timeout 90s` | SKIP: this root user namespace cannot chown to uid/gid 65534 (EINVAL) |

The real-process fixture uses actual BAFT H2/TLS/Noise and echo payload but
simulated service control. Bulgaria now changes public address and port, forcing
a new CA/leaf. Its original PKI and working payload return after rollback while
the same Germany TCP flow, same IR PID/generation and config/unit/marker/keys
remain intact through creation, edit, failure and retirement.

The uid/gid mutation skip is **not** ownership field acceptance. No live VPS,
actual systemd deployment, production mutation, main merge, tag or release was
performed. Preparation still changes live disk PKI before commit; fully staged
cutover and general deployment payload verification remain open P0 work.

All failed and successful development attempts are retained in the parent code
commit's `docs/evidence/smart-tunnel-p0` directory, including the old restart
order, six unsafe-material cases, chown environment failure and certificate
change during restart. Do not relabel failed attempts as successful repetitions.

GitHub status when this record was made: static-analysis 38468488702 SUCCESS;
CI 38468488711 and R3 38468488531 IN PROGRESS. Inspect those exact-SHA runs for
their eventual outcome. Prior 2737902's three successful workflows are not
successor acceptance. #199/#67/PR #200 retain the current review and gate status.

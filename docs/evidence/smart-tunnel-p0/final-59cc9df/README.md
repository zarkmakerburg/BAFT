# Staged PKI: exact-head evidence

PR #200 / epic #199. Reviewed published HEAD:
`59cc9dfe719a535914b58e358d2760938fad2f37`.
Tree: `66210d9867612d13c424624e8cb97fdfccd711ad`.
Baseline: `d8bb3b69decfba2ba57430bcfe61eab64a6ba3cc`.

The published commit was fetched from origin into a clean detached checkout.
All final commands below completed with exit 0 on that checkout:

- `go test ./... -count=1 -timeout=10m`: PASS (`59c-full.log`).
- `go test -race ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout=10m`: PASS (`59c-race.log`).
- `go test ./internal/tunnelnode -run '^TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle$' -count=3 -timeout=4m -v`: PASS 3/3 (`59c-live.log`).
- `go vet ./internal/tunnelnode ./internal/agent ./internal/agentjob`: PASS (`59c-vet.log`, intentionally empty success output).
- Clean status and `git diff BASE --check`: PASS.

Two real H2/TLS/Noise processes carry integrity-checked echo data. Germany's same
TCP flow/PID/generation/artifacts survive Bulgaria create, address/port/CA edit,
rollback, injected failure and retirement. The old Bulgaria route also transfers
payload during replacement preparation while its live PKI remains unchanged.
Local service control is simulated; this is not a production rollout or two-VPS
systemd acceptance. The current GitHub launch/systemd gate is separately listed
in the CI snapshot, without inheriting predecessor success.

## Retained attempts

| Log | Result / interpretation |
| --- | --- |
| before.log | Baseline FAIL: preparation changed active PKI; first preparation installed live PKI before commit |
| attempt1.log | FAIL: outdated PKI fault fixture targeted the live directory and expected a backup that preparation no longer creates |
| attempt2.log | PASS after adapting fault injection to candidate paths and preserving a changed live tree |
| expanded.log | FAIL: interruption fixture counted read-only is-active/is-enabled queries as mutations; real sibling lifecycle passed |
| expanded-retry.log | PASS after distinguishing state reads from service mutations; focused ownership subtest explicitly SKIP |
| full.log | FAIL: Agent secret-output fixture produced non-PEM placeholder certificates, correctly rejected by new validation |
| full-retry.log | PASS with real certificate generation in the Agent fixture |
| race.log | PASS full tunnelnode/agentjob race on development tree |
| agent-race.log | PASS targeted Agent tunnel/BCC/rotation race |
| legacy.log | PASS older committed-receipt compatibility |
| vet.log | PASS affected-package vet |
| 54e-full/race/live.log | PASS on local commit 54efae2; same exact tree as published commit, including 3/3 real-process cycles |
| 59c-full/race/live/vet.log | Final published-SHA PASS, as listed above |
| publication.txt | Initial CLI authentication failure and successful expected-head connector publication; no reset/rebase/force push |

No failed attempt is relabelled as a pass. UID/GID mutation remains an environment
SKIP: only the available mapped ownership can be checked here; privileged field
ownership acceptance remains open. The earlier d8bb3b6 evidence stays separate.

## Limits and next P0

This stages and validates certificate artifacts before activation. Candidate
config generation finishes at CommitEX after the IR reply; candidate
handshake/payload verification BEFORE coordinated cutover remains open. The
delivery gate is passive recent bidirectional DATA AFTER commit, BEFORE backup
release, not L7 challenge semantics, future uptime or zero downtime. Existing
old in-flight contracts, cross-process host reservations, real host preflight,
shared-binary upgrades and installer #197 coexistence need further qualification.
No production mutations, main merge, release/tag, new transport/shaping,
automatic restart recovery or standby role resumption occurred.

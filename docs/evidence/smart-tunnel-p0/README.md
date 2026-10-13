# Development attempts for #199 P0

These are bounded local test outputs with generated fixture identities. No
production logs or credentials are present. Preserve failed attempts alongside
successful ones.

Baseline source was `main@b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`.
Regression tests were introduced in the working tree before their corresponding
fixes. In-flight attempts are development evidence, not exact-final-SHA gates.
Final acceptance commands run on the immutable published candidate and are
recorded in the PR/#67 with its SHA; those gates are separate from this history.

| Output | Command/meaning |
| --- | --- |
| preflight-before.log | `go test ./internal/bcc -run '^TestPreflight' -count=1`: expected FAIL before collision/endpoint fixes |
| preflight-after.log | Focused preflight + scoped instance tests: PASS after the initial fix |
| rollback-before.log | Service-operation, corrupt-transaction and completion-receipt tests: expected FAIL |
| rollback-after.log | Focused rollback regressions + previous-state/ownership tests: PASS |
| two-live-attempt1.log | Real-process lifecycle: FAIL at Bulgaria port-edit rollback |
| two-live-attempt2.log | Same failure reproduced, with bounded fixture process logs |
| two-live-attempt3.log | PASS after restoring `activating`/`reloading` service intent |
| enable-before.log | Active-but-disabled previous service: expected FAIL |
| enable-after.log | Same regression after enablement restoration: PASS |
| health-before.log | Four missing/invalid restart-evidence cases: expected FAIL |
| health-after.log | Same cases after evidence validation: PASS |
| full-local.log | `go test ./... -count=1 -timeout 180s`: PASS before publication; source matches the published tree |

Toolchain: Go 1.27.1, Linux. The real-process fixture runs actual BAFT transport
binaries with simulated systemd. Certificate name and endpoints are local
loopback; this is not field evidence or production acceptance.

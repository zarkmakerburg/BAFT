# Final local validation for PR #200

Immutable code SHA: 2737902dd54c1ac59dce4b79df40b3219a4fb2b0
Code tree: 5db968abd5a36d81513e2c4feb67beaf459a31b7
Environment: Linux, Go 1.27.1; clean detached checkout, no production access.

All commands below completed successfully on that exact SHA:

- go test ./... -count=1 -timeout 10m
- go test -race ./internal/carrier/h2 ./internal/bcc ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout 10m
- go test ./internal/tunnelnode -run '^TestTwoLiveInstancesKeepGermanyFlow' -count=3 -timeout 150s -v
- go test -race ./internal/carrier/h2 -run 'TestOpenNoiseReportsTLSPhase|TestNoisePhaseTrace|TestNoiseTraceKeeps' -count=100

The live fixture passed 3/3 and retains one Germany TCP flow/PID/files across the sibling lifecycle. Service control is simulated. These are local unit/integration/race results, not two-VPS/systemd deployment or R-003 field acceptance. Failed development/earlier CI attempts are retained in the parent evidence directory.

This evidence-only branch keeps the reviewed code SHA fixed. GitHub Actions results and remaining P0 gates are tracked in PR #200 and issues #199/#67.

# Monitoring freshness — complete attempt ledger

Date: 2026-10-16. Worker claim: #67/comment6221933471.
All entries distinguish staged source from the published SHA. No CI retry or
cancellation was issued by this worker. Missing output is not a successful test.

| Attempt | Source / command | Observed result | Evidence / limit |
| --- | --- | --- | --- |
| Baseline falsification | Unchanged f39df857c6f888613af4e431f250122dad59b8f5 production files, new initial test; `go test ./internal/bcc -run '^TestMonitoringFreshness' -count=1 -v` on isolated Mac Go1.27.1 | Expected FAIL, exit1, 1.655s; all three top-level groups fail, not all22 matrix cases | Original `baft-stage249-baseline.log` retained on Mac; initial test is archived here. Raw-log recovery later timed out; no fabricated transcript. |
| First correction | Initial staged patch over f39df857; `go test -race ./internal/bcc -run '^TestMonitoringFreshness|^TestMonitoringStatus|^TestOverview|^TestMonitoringAnd' -count=1 -v` | PASS3.908s | `baft-stage249-fixed-focused.log` on Mac. This regex did NOT include existing TestMonitoringAlertsAndSevenDayHistory. Aggregate-rate assertion and deep-copy strengthening were not yet present. Not final-SHA acceptance. |
| Stronger negative test | Initial corrected per-node path, added aggregate assertion; `go test ./internal/bcc -run '^TestMonitoringFreshnessOverviewRejectsFutureRateAndAgent$' -count=1 -v` | Expected FAIL, exit1, 0.566s; aggregate rate remained nonnil | `baft-stage249-aggregate-before.log` on Mac. Fixed aggregate path afterwards; regression-strengthening.diff preserves the added assertion. |
| Final staged BCC package | Final five BCC files over f39df857; `GOTOOLCHAIN=go1.27.1 go test -race ./internal/bcc -count=1` | Launched; last process observation was active; final result UNKNOWN after remote read timeout | Mac shell pid57881, Go child57914, bcc.test57925; `baft-stage249-full-bcc.log`. Do not infer pass/failure/hang from quiet output or lost connection. |
| Live-base carry and publication | f39df857→2630c217→ae27cae reviewed before publication. All four touched existing BCC files byte-identical; new independent branch at ae27cae | Six-file local staged tree and remote tree identical86b8f2cc9a07e3ee3df6ddc628e7c5b7135ca2dd; published14cfc6a42186ece49393f5401b69ffa8046771c3 with expected_sha/force:false | Original modified worktree preserved. Exact clean Git fetch independently confirms source, tree, sole parent ae27cae and clean checkout. No reset/rebase/discard/force-push. |
| Exact published source | Linux CI38948263357, R3.138948263416, GoSec38948263379, attempt1, all metadata head_sha14cfc6a | Consult final-status.json for actual final outcomes | No green transferred from staged f39df857, other PRs, main/tag, Mac or a synthetic merge. |

Baseline, first focused and intermediate aggregate raw logs are not in this
archive unless later recovered explicitly. They remain at the isolated Mac's
`/Users/candoo.collection/code/baft-stage249-*.log` paths. The connected-device
listing said Online while last-seen was already8–11minutes old. read_process_output
returned an MCP request timeout; full-log read call b9161dd2-a996-42da-a570-1b90d386e607
ended `Timeout - no response from device`. Three separate read-only earlier-log
requests also ended with no response; all results are in remote-read-attempts.json.
No duplicate test execution,
kill, production connection, signing or workaround was attempted.

Minor unsuccessful inspections: an optional guessed monitoring.go path did not
exist; actual inventory found store.go/health_model.go/monitoring_status_test.go.
An exact-worktree AGENTS.md name search returned no match (exit1), not a test
failure. A guessed .github/workflows/r3.1.yml path did not exist; inventory
located r2-noise.yml, whose workflow name is r3.1-probabilistic-morphing. One
diagnostic process-list command printed a large base64 public-source command;
subsequent diagnostics use comm, not command. No production secret was read.
An evidence-only apply_patch attempted delete/add on the same JSON path and was
rejected before editing; an ordinary update patch corrected it. No product test
or source file was changed by that failed evidence edit.
Later optional inspection guessed alerts.go, assets/bcc-app.js and auth*.go
paths that were absent; no source was changed. A raw /actions/jobs/ID metadata
request was rejected400 by the connector's endpoint allowlist; an attempted
JSON parse of that error also failed. Supported fetch_workflow_run_jobs was
used afterwards. These are inspection/helper errors, not failed product tests.

Read projections only: old/future telemetry without current independent
evidence is UNKNOWN. A fresh TCP probe or authenticated agent poll can still
prove node management reachability, not tunnel/process/data-path health. Stale
routes remain UNKNOWN. Overview rejects old probe RTT and future current-rate
observations. JSON schema, write/auth/session/CSRF handlers, persistence, alert
engine and layered-health state machine are outside this patch. Historical
charts/uptime and per-route timestamps are not redesigned. Browser/rendered
accessibility/performance NOT RUN; native IR↔EX owner-DEFERRED/OPEN. No change
is promoted into HQ's independently owned v0.3.2 signing/release lane.

# Release-tail regression evidence

Code candidate: `e86405b6851c4cc82012f3af037d1ec7e5b65785`.
Baseline production code: `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`.
Regression-only commit: `ad9a11ae3f7db55cf2cdc5ac3e476e002a5eeabb`.
Toolchain: Go 1.27.3 linux/amd64.

- `release-crash-red.log`: initial four top-level regressions on the code now
  committed as ad9a11a, exit 1. No production changes had been made.
- `release-crash-complete-red.log`: the final five top-level regressions,
  including the subprocess test, overlaid on the ad9a11a worktree, exit 1.
  This was explicitly a dirty test-only overlay, not a clean committed result.
  The overlaid test file SHA-256 is `4c3fdf65501afc35fa13e5ec472bd2c71768d84e7fc65cab56e5fd723e14d8a9`.
- `ingressauthority-race.log`: precommit race run with the final fixed source,
  exit 0, 42 top-level PASS and 14 LAB-only SKIP. Not a final release gate.
- `baseline-inventory.log` / `.exit`: inventory package run on unchanged
  baseline production code (ad9a11a), exit 1, reproducing the five restricted
  environment failures. The test-only overlay affects another package.
- `candidate-*.sha` and `.exit`: exact code SHA and actual pipeline exit code
  for each corresponding postcommit log. Vet covers `./...`; the test command
  is `go test ./... -count=1 -timeout=10m`; race covers authority, ownership,
  Agent, BCC and the Agent CLI with `-race -count=1 -timeout=10m`.

The subprocess test seeds the preceding verified-host-retirement state, kills
only a private store-writer child, then reopens the disk store. It is not proof
of Agent, kernel firewall, systemd or reboot acceptance. Namespace creation is
unavailable here; no LAB-only result was promoted to PASS.

These logs are local evidence. Publication and candidate hosted checks remain
blocked pending explicit push authorization after automatic approval review
rejected the external write. No candidate PR, HQ comment, merge, tag, release
or production deployment was performed.

## Exact candidate outcomes

- Full vet: exit 0.
- Full Go tests: exit 1; five inventory failures reproduced on baseline code,
  with all other packages completing successfully.
- Five-package race: exit 1; authority, ownership, Agent and Agent CLI passed;
  BCC reached the 10-minute suite timeout during
  `TestUnreachableIncidentIsDeterministicAndRestartSafe`. The stack is retained.
  No race detector warning appeared, but the BCC race gate is NOT passed.

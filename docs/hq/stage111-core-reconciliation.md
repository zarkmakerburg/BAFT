# Stage111 — Core PR200 ↔ PR203 semantic reconciliation

Status: integration NOT performed. This file is an executable handoff, not proof of a merge.

## Fixed inputs
- PR200: `1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7`
- PR203: `39424cf6e4f5a79ec1282b7c2bc516ac306ea6aa`
- merge-base: `b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`
- working branch: `codex/core-reconcile-stage110`
- GitHub comparison: diverged, 72 ahead / 22 behind (PR200→PR203).

## Reproducible merge procedure (isolated working copy only)
```bash
set -euo pipefail
git fetch origin 1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7 39424cf6e4f5a79ec1282b7c2bc516ac306ea6aa
git switch codex/core-reconcile-stage110
test "$(git merge-base --is-ancestor 1816f2a35dfeb8a51d32575dbb57e6e0c4c27dc7 HEAD; echo $?)" = 0
# Stage111 documentation commit follows PR200, so HEAD is intentionally newer.
test -z "$(git status --porcelain)"
test "$(git merge-base HEAD 39424cf6e4f5a79ec1282b7c2bc516ac306ea6aa)" = b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f
if ! git merge --no-commit --no-ff 39424cf6e4f5a79ec1282b7c2bc516ac306ea6aa; then
  echo "Merge stopped for manual semantic conflict resolution"
fi
git status --short
```
Stop and resolve every conflict semantically before committing. Never use wholesale 'ours'/'theirs' on stateful BCC files.

## Review hotspots
- `internal/bcc/statedb.go`, `store.go`: migrations, singleton `incident_snapshot`, seed/idempotence, compatibility with existing SQLite databases.
- `internal/bcc/backup.go` and tests: backup/restore consistency and rollback behavior.
- `internal/bcc/telemetry_write_failure_test.go`: preserve assertions from both sides.
- `internal/incidentclass/*`: incident lifecycle/debounce/tracker/snapshot coverage.
- `tests/e2e/bcc_schema_upgrade.sh`: test upgrades from an existing persisted DB, not only fresh install.

## Acceptance before PR
1. Record final conflict list and rationale per resolved file.
2. Run `go test ./internal/bcc/... ./internal/incidentclass/...` plus complete repository test and CI workflows; run the schema-upgrade, backup/restore, uninstall, rollback and race suites.
3. Record exact combined SHA and workflow IDs, all passing. No skipped tests or weakened stress counts.
4. Open draft integration PR for review. Do not merge to main, tag, release or deploy without HQ release gate.

Aurora Hybrid PR208 is separately all-green in automated workflows, but rendered visual parity and integration remain unverified.

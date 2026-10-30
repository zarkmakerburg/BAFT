# BAFT v0.4.0 — conservative dead-code and repository cleanup audit

Status: **partial source audit; no wholesale dead-code deletion authorized by evidence yet**. This document separates confirmed cleanup from suspected unused code. Static source references alone cannot prove that shell entry points, dynamically registered Go handlers, web assets or generated installer commands are unreachable.

## Inspected scope
- `install.sh`: generated `baft-net` and `baft-panel` scripts, command dispatch and diagnostics.
- `internal/bcc/public_ingress_p0.go`, `public_ingress_conflicts_p0.go` and their tests.
- `tests/installer/network_tool_contract_test.go`.
- `README.md`, `README.fa.md`, and the P0 design specification.

## Confirmed low-risk cleanup on the development branch
- Normalized formatting and comments in ingress reservation validation; no intended behavioral change.
- Organized installer network safety contract test and removed redundant blank lines.
- Updated both GitHub-facing READMEs to explain the actual code layout, v0.4.0 target, and which P0 capabilities are still incomplete.
- Kept public ingress preflight separate from agent apply and kept the Aurora wizard labeled as preview-only.

## Candidates requiring evidence before deletion
1. Installer-generated `baft-panel`/`baft-net` functions: CLI entry points can be invoked externally; do **not** remove based only on absence of internal references.
2. `internal/bcc/public_ingress_*.go` desired-state helpers: some may currently be test-only or future agent contracts. Determine call graph and feature flag ownership before deleting.
3. `web/bcc-action-wizard.js`: preview-only does not imply dead; check HTML/template registration, build manifest and browser integration.
4. Older documentation, assets, examples, and compatibility aliases: search build, installer, release packaging, tests and public URLs before classifying as unused.
5. Generated artifacts and vendored dependencies: identify ownership, hashes, license obligations and reproducible generation before removing.

## Required next audit gates
- Enumerate the full Git tree and track references across Go imports, shell commands, systemd units, templates, JS modules, docs, release packaging and CI.
- Run `go test ./...`, `go vet ./...`, `bash -n install.sh`, shellcheck (where available), BCC UI tests, static analysis and full release workflows.
- Use coverage, dependency and call-graph evidence to classify each candidate as **reachable**, **compatibility-required**, **future/feature-gated**, or **confirmed unreachable**.
- Delete only confirmed unreachable files/functions in small, independently tested commits; preserve compatibility wrappers and rollback until verified.
- Verify legacy `BAFT.goldapp.tr:2053`, SSH, 3X-UI, existing tunnel/agent commands, FA/EN docs, and v0.4.0 HQ approval gate.

This is a traceable cleanup inventory, **not** a claim that a whole-repository dead-code scan or cleanup has finished.

# BAFT R0 — HQ Release Candidate Handoff (Stage 227)

## Immutable candidate and scope

- Repository: `zarkmakerburg/BAFT`; draft PR: https://github.com/zarkmakerburg/BAFT/pull/216
- RC source SHA: `41224168af097bd210f5e7469a00952d2f9eb398`
- Base SHA at PR inspection: `b028fd580fb2c3f0ebffe2fbf1a52f6b577cbf5f`
- Branch: `hq/rc0-release-readiness-stage209` (not merged).
- Code delta: `internal/bcc/store.go` changes telemetry history pruning cutoff from ingestion wall clock to report timestamp; delayed reports no longer erase preceding history prematurely. Existing read-side 7-day history filter remains unchanged.
- Documentation: `docs/release/r0-decision-record.md`.
- Aurora is NOT substituted for classic BCC; Aurora integration belongs to later reversible, separately qualified R1.

## Verified exact-SHA GitHub Actions

All three observed PR workflows for SHA `41224168af097bd210f5e7469a00952d2f9eb398` passed:

| Gate | Evidence | Outcome |
| --- | --- | --- |
| Main CI (15/15 jobs) | https://github.com/zarkmakerburg/BAFT/actions/runs/38792157039 | PASS |
| R3.1 | https://github.com/zarkmakerburg/BAFT/actions/runs/38792157084 | PASS |
| Static analysis / gosec | https://github.com/zarkmakerburg/BAFT/actions/runs/38792156910 | PASS |

Main CI includes unit/integration tests, race detection, installer fault injection, install/release/offline/SSH/BCC, schema upgrade, uninstall, and Launch1 break/rollback. Launch1 used throwaway signing keys; this is NOT a production signature.

## Release decision

**HOLD for production publication and deployment**, despite green CI.

1. HQ must explicitly authorize any merge, release tag, production release, or deployment. No authorization is implied by this handoff.
2. An independently verified production-key signature and signed release manifest/checksums are still required; the workflow's unsigned candidate and CI throwaway keys do not satisfy this gate.
3. Real Iran ↔ Germany carrier/continuity acceptance remains deferred at HQ direction and must be recorded as residual risk before any release decision.
4. Review PR diff for the event-time retention change and its delayed/out-of-order history implications. Confirm version, changelog, and rollback instructions for the intended release.

## Safe next actions

- Review PR #216 and this handoff without merging.
- When HQ approves the publication process, freeze and revalidate the exact source SHA, create the offline signing and verification evidence, and perform an explicit GO/NO-GO.
- Do not deploy to live Arno or Goldapp-de without a separate HQ approval.

This record documents observed CI results only; it is not a release authorization.

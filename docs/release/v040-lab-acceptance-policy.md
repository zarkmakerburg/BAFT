# BAFT v0.4.0 — LAB-only release acceptance decision

Decision date: 2026-10-23. The project owner explicitly accepts **successful, reproducible LAB validation** as sufficient *environmental evidence* for publishing v0.4.0. Testing on live/production IR or EX hosts is **not a prerequisite for publishing the release**. This supersedes prior *physical two-host required for release* language in QA notes. It does not waive code, security, availability or release-integrity gates.

## Release GO conditions (all must hold on the exact candidate SHA)

1. All applicable required GitHub CI workflows succeed, including full unit/integration tests, BCC browser tests, static analysis, P0 ingress kernel LAB and Gamma isolated two-node LAB; no stale/check-from-another-SHA substitutions.
2. The disposable LAB validates operational provisioning, pairing where EX cannot initiate SSH into IR, traffic flow, preservation of unrelated listeners/services, fault injection, cleanup, reboot/restart or an equivalent cold-start replay, and applicable downgrade/backup/restore paths. Each untested critical case must remain a release blocker or be explicitly scoped out of v0.4.0; passing an unrelated LAB cannot prove it.
3. P0 #269 features presented to users as available must actually be implemented and covered by acceptance tests. If automatic public ingress or shared inter-node transport is not admitted, BCC must truthfully block/label those capabilities; no UI may imply they work.
4. Candidate remains tied to reviewed exact-SHA artifacts, signed manifest and checksum/rollback evidence. No P0 vulnerabilities or integrity failures remain open at GO decision.
5. HQ #67 records PASS/FAIL/BLOCKED evidence and a final release GO decision. Release/tag/merge and any production rollout are distinct actions; publishing the release authorizes no changes to live hosts.

## Non-gate / scope boundaries

- No requirement to SSH into, install onto or run end-to-end tests on production servers before publishing v0.4.0.
- Linux network namespaces are acceptable for two-node LAB *release evidence*, provided limitations (same kernel and runner, no independent provider routing) are explicit.
- Successful LAB evidence proves only tested configurations and failure modes. It is **not** a guarantee of uptime/reachability under real-world filtering or production deployment authorization.

Disposition: apply the policy to QA/HQ release readiness; do not silently change code to make failed tests pass and do not mark #269 closed on policy grounds alone.

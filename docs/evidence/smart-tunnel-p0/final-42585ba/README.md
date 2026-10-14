# Candidate configuration identity — exact-source evidence

Recorded 2026-10-14. PR #200 is draft; #199 remains open.

## Source

- Baseline: 0da45e07d71c5f7a84f29bc07c9f98d644745f4b.
- Core published commit: 8ec52bed45ccf5a36b3e635d969f2981427b9fb8, tree b408a398cb6af963a14ae678e9af7aa3b55feb28.
- Final published source: 42585baa9e5967abac2977c028cd2406409ccf31, tree 47d8ecc416a7c7f4e9b5a68d89df10f767fcb827.
- Local core/fixture commits 80738c3 and dd7cd00 have those identical trees. Source branch update used expected_sha=0da45e0, force=false, preserving all existing commits.
- Final source was fetched into a clean detached checkout. Synthetic PR merge 558eb74463b19d743ad07b8829455c818110ccab was fetched and has exactly the same tree. See publication.json.
- This evidence is a separate documentation-only branch; it does not advance the reviewed source HEAD.

## Implemented behavior

PrepareIR now records candidate config bytes and config/input modes/uid/gid before returning its reply. PrepareEX records parameters, pending state, PSK and Noise-key identities; drift is rejected before accepting/consuming the reply. Completed EX config is validated and recorded before live PKI activation.

Material reads use one bounded descriptor with no final symlink following, refuse directory aliases/special files and sample metadata around the read. Config role, Noise key, CA/PKI and management paths must belong to the instance. The typed loader and installed binary validate before any live mutation, bytes/dependencies are rechecked around validation and before activation, and installation uses the captured validated bytes.

Refusal retains preparation and leaves live config/unit/marker/generation/certificates/service untouched. Restoring the recorded candidate permits explicit retry. Older receipts without preparation identities retain their prior identity contract while still receiving bounded syntax/path validation. Failed IR preparation now reports rollback errors.

## Exact-SHA validation

Go 1.27.1, Linux/amd64, source 42585baa9e5967abac2977c028cd2406409ccf31.

| Command | Result | Log |
| --- | --- | --- |
| go test ./... -count=1 -timeout=10m | PASS, exit 0 | logs/199-config-425-full.log |
| go test -race ./internal/tunnelnode ./internal/agent ./internal/agentjob -count=1 -timeout=10m | PASS, exit 0; 103.746s / 176.182s / 1.303s | logs/199-config-425-race.log |
| go test ./internal/tunnelnode -run '^TestPrepared' -count=3 -v | 3/3 PASS, no skips, 28.090s | logs/199-config-425-focused.log |
| go vet ./internal/tunnelnode ./internal/agent ./internal/agentjob | PASS, exit 0, no output | logs/199-config-425-vet.log |
| scripts/check_ci_supply_chain.sh | PASS, 13 workflows | logs/199-config-425-policy.log |

New regressions cover valid/invalid config edits, Noise key/CA bytes, config symlink/mode and key mode, EX parameter drift before PSK consumption, mutation after successful binary validation before PKI activation, explicit repair/retry and older receipt compatibility. Real baft/baft-pair tools generate the candidate files; local systemd is simulated.

The runtime's existing child-process namespace/FD, AF_UNIX and uid/gid mutation restrictions remain explicitly distinguished from executed passes. The separate environment log records those skips on this exact source; a package-level PASS does not turn skipped native cases into acceptance. The new candidate-drift suite itself had no skips.

## Current-source CI

[CI 38555751948](https://github.com/zarkmakerburg/BAFT/actions/runs/38555751948), job 115824341737: mandatory node-resource acceptance SUCCESS. It executes THREE native Linux runs with BAFT_REQUIRE_RESOURCE_EVIDENCE=1, converting resource environment skips into failures. This includes the two-real-process H2/TLS/Noise sibling lifecycle and Unix socket ownership/client-held cases. Its process-fixture service control is simulated; real-systemd launch is a separate job.

Current-source [BCC UI 38555751958](https://github.com/zarkmakerburg/BAFT/actions/runs/38555751958) and [static 38555751981](https://github.com/zarkmakerburg/BAFT/actions/runs/38555751981) SUCCESS. [R3 38555751962](https://github.com/zarkmakerburg/BAFT/actions/runs/38555751962) and aggregate CI were still running in the initial snapshots. The later final-status.json records the final observed state separately: CI/R3 still IN PROGRESS, and current-source real-systemd signed-release build/break/rollback job 115824342188 completed SUCCESS on a throwaway GitHub runner. This is not production acceptance. No all-green inference from predecessor 0da45e0, which did finish all four workflows.

## Attempts retained

All available logs for this slice are in logs/ and checksummed in manifest-sha256.json. Earlier uncommitted/development runs are not exact-final-SHA acceptance.

| Attempt | Observation |
| --- | --- |
| before | Baseline IR edits/key/CA/symlink accepted; invalid config triggered rollback service operations. Initial EX fixture accidentally omitted normalized metrics and was corrected rather than counted as EX reproduction |
| before-ex-corrected | Baseline valid EX target edit accepted and pending/PSK consumed |
| before-expanded | Baseline IR mode/key-mode failures; initial validator-change fixture had incorrect argument count and did not inject, so that case is NOT valid mutation evidence |
| before-validator-corrected | Actual post-validator config mutation reproduced active config/PKI replacement; injection asserted |
| after-first | Build FAIL: removed local staged variable still referenced by cleanup; corrected |
| after-fixed / after-identity | Focused candidate/PKI tests PASS for executed cases; PKI uid/gid environment skip retained |
| after-expanded | IR/EX cases PASS; validator fixture failed because it did not inject; corrected without weakening production checks |
| after-final-race | New candidate suite 3/3 race PASS on development tree |
| full-development / race-development | FAIL in Agent secret-output fixture: it returned a code without pending/PSK artifacts. The race wrapper's later vet exit 0 is NOT race success |
| agent-fixture-fixed | Real key/pending/PSK generation added to the adapter; secret-output test PASS |
| 425-* | Final fetched exact source validation above |

A tree-entry read hit output truncation before publication; it was retried with a complete payload. API tree hashes were compared to local trees before creating commits. No source branch update occurred until the fixture correction was included.

## Limits and next work

This closes candidate integrity and validation ordering gaps. EX configuration still completes within CommitEX after the IR reply; no separately running candidate process or coordinated candidate handshake/payload BEFORE either active service change exists yet. Passive payload proof still gates backup release after commit. No zero-downtime, L7 semantics, continuous-health or field/blocking claim.

Writes after the last identity check still require host-wide reservation/locking and service-credential access review. Existing installed/mixed-version/shared-binary compatibility and general OS permission hardening remain open. Installer #197 at fb1950b295442d891aa6a09481d030e49c507b24 is unchanged and separate; standalone service/path namespaces and shared binary/release/baft-net ownership need coordinated validation. No installer code is duplicated here.

Keep #199 open and #200 draft. No production/server change, main merge, tag/release, transport/shaping change, EX pool/exit replacement, restart-resume or standby role resumption. Board claim 6163435418 is released in the final report.

## Subsequent exact-source CI completion — 2026-10-14

The reviewed source HEAD is still 42585baa9e5967abac2977c028cd2406409ccf31. All four associated workflows are now SUCCESS: CI 38555751948, R3 38555751962, static 38555751981 and BCC UI 38555751958. Main CI's full race/vet/fuzz and all 15 jobs completed successfully; the R3 job also completed successfully. ci-complete.json records this newer observation without relabeling the earlier pending snapshots.

native-linux-complete.log retains the actual mandatory three-run resource-step output with BAFT_REQUIRE_RESOURCE_EVIDENCE=1: three two-live-instance passes and three held-Unix-client passes, no skips. This completes current-source native qualification; local process/Unix/uid-gid skips remain unchanged and are not called passes. No source code or reviewed source HEAD changed during this follow-up. Broader epic, coordinated pre-cutover candidate handshake/payload and authorized field/production acceptance remain open.


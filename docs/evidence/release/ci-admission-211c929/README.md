# Stage243/246 release CI admission evidence — initial checkpoint

Source: [211c929d41924fbe17ec3bb19615f7d0e8429490](https://github.com/zarkmakerburg/BAFT/commit/211c929d41924fbe17ec3bb19615f7d0e8429490), tree7f6d49880cdd422d80cdd3d6eb15e543f3e65044, parent27cca1a763310212a2f2a6b27c4cf7201b3f1b57. [Draft PR220](https://github.com/zarkmakerburg/BAFT/pull/220); [claim6221370689](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6221370689), [delivery Stage246](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6221508311).

This is an INITIAL evidence checkpoint, not full qualification or release approval.
HQ's separately frozen proposed candidate27cca1a remains unchanged. This follow-up
requires its own review and exact-source CI. All ancestor commits are preserved.

## Deterministic proof and local execution

- Added four test groups against the unchanged parent acceptance script:
  **22 negative subcases and the qualified-CI boundary case failed**, expected.
  The 26 failure rows are four groups plus22 subcases, not26 independent bugs.
- On the staged fix, Go1.27.1 on isolated macOS:
  `go test -race ./tests/release -count=1 -v` PASS, all7 top-level tests and22
  negative subcases. Existing remote-tag/latest-run/workflow-identity checks
  remain. The positive-CI boundary still rejects absent release assets.
- Fetched published211c929 into a second clean detached Mac worktree:
  HEAD/tree exact match; same full package command PASS (34.375s), clean before
  and after. Bash syntax, gofmt and diff PASS.
- Independent clean Linux scratch checkout has exact HEAD/tree and three-path
  delta. Bash syntax/diff and supply-chain13-workflow policy PASS. Explicit
  main-job list equals all15 jobs in current ci.yml. No local Linux Go toolchain.

Full original logs are included, not just summaries. Terminal newlines omitted
by the line reader were restored only after SHA256 verification against the
original Mac files:

| File | SHA256 |
| --- | --- |
| baseline.log | 8ebfef2a13ceb368e09ad390f0eec3ecca7ecd7881771a720b2bb15700a0e6e6 |
| fixed-staged.log | a7cdc66e8bd22cbe01d0becd22bbfef76720d7a6acfb2462ddf8a61ebda7e726 |
| exact-mac.log | 1a6ac79d5fd7942b57bd80578ed687a798168053c4c31576d19e940b85b6747c |
| linux-release-dry-run.log | d9668b6fbf5225e2b8f6a71f85c410eeaed3e3afeddc9bd75c051ce08682bcd1 |

## Linux exact-source evidence at initial snapshot

- [CI38943151691](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151691):
  IN PROGRESS; initial-status.json retains job/step state.
- Its [release-dry-run job117115547458](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151691/job/117115547458):
  SUCCESS, including independent provenance/tamper regression.
  These tests use throwaway keys; no production key or signature.
- [R3.138943151519](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151519):
  IN PROGRESS.
- [static38943151722](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151722):
  SUCCESS.
- No retries/cancellations requested. A running gate is not passed.
- GitHub check inventory showed GitHub Actions only; no Vercel check observed.
  This is a check read, not independent proof about every deployment provider.

## Attempts, scope and remaining risks

Scratch had been pruned; new public filtered clone succeeded. No AGENTS.md in
tracked source. An optional guessed telemetry_history_test.go read failed and
was corrected by source inventory; it was not a product test failure.
Local Go absent; supported Mac toolchain used. One remote read exceeded3s and
was retrieved using its returned call ID, without duplicating the request.
All expected baseline failures and actual fixed/exact passes are retained.

Only accept.sh, its regression fixtures and policy docs differ from PR219.
No workflow permissions, dependencies, runtime, signer, installer or UI edits.
Publication used expected_sha/force:false and exact staged/remote tree equality.
The evidence branch is not the tested source SHA and cannot inherit that claim.

This tightens current release-admission policy; historical releases missing
mandatory evidence may fail. No claim that their artifacts are defective.
The new CI rows do not substitute for manifest/signature/provenance/offline
bundle acceptance, independent upgrade/rollback or HQ's final decision.
No published release assets were tested by the fake-GitHub admission fixtures.
No main merge, tag, production signing, release, deployment or node changes.
Native Iran–Germany remains owner-DEFERRED. Browser/rendered UI acceptance is
not tested or claimed by this release-checker work. Aurora remains additive BCC.

Next: inspect complete current-source Linux CI/R3, preserve full final logs and
statuses, then close only this claim if justified. HQ owns final source choice.

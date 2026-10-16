# Stage247 — release CI admission qualification FINAL

**Bounded slice PASS; release HOLD.** Source [211c929d41924fbe17ec3bb19615f7d0e8429490](https://github.com/zarkmakerburg/BAFT/commit/211c929d41924fbe17ec3bb19615f7d0e8429490),
tree7f6d49880cdd422d80cdd3d6eb15e543f3e65044, parent27cca1a763310212a2f2a6b27c4cf7201b3f1b57.
[Draft PR220](https://github.com/zarkmakerburg/BAFT/pull/220) remains unmerged.
The HQ proposed candidate27cca1a and original PR216/218/219 are unchanged.
This source is a separate follow-up requiring explicit HQ scope selection.

## Exact-source gates

| Gate | Run | Result |
| --- | --- | --- |
| Main CI, all15 jobs | [38943151691](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151691) | SUCCESS, attempt1 |
| R3.1 | [38943151519](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151519) | SUCCESS, attempt1 |
| GoSec | [38943151722](https://github.com/zarkmakerburg/BAFT/actions/runs/38943151722) | SUCCESS, attempt1 |

All three API run head_sha values equal211c929. Main test job117115547710
has Race detector, Vet and Protocol fuzz smoke SUCCESS; the conditional
failure-report step is skipped as designed, not a missing mandatory test.
Installer faults, signed-fixture upgrade/offline/SSH/BCC/schema/uninstall and
Launch1 break/rollback jobs are successful. These use disposable fixtures and
throwaway keys; they are not production or native field acceptance.

R3 independently completed full correctness, whole-repository Go race/vet,
statistical/ECRL differential, fuzz and build/package gates. Its original log
contains tests/release PASS in correctness9.052s and race10.282s. No data-race
warning occurs in the complete main or R3 logs; no retry/cancel was requested.

GitHub synthetic mergef0c63d757fc7a78cf6849422f30f00f47c5b15b4
has exact source tree7f6d4988 and ordered parents mainb028fd58/source211c929;
Git data independently read. This is a PR test merge object, not a merge to
main or a Core–Aurora integration certification. Full metadata/job steps are
in final-status.json.

## Actual change and falsification

Only three paths differ from the parent: acceptance script, its Go fixtures,
and docs/release/ci-admission.md. It requires whole-workflow success, all15
current main jobs and independent exact-source GoSec/R3; absent R3 fails closed.
The older8-job/overall-conclusion/security gaps are demonstrated by the complete
baseline log:22 negative subcases fail, plus the policy-boundary fixture.
The four failure groups plus22 subcases are26 failure rows, not26 separate bugs.

After correction, all7 top-level release-admission tests and22 negative
subcases pass with -race, first staged (34.507s), then a clean independently
fetched exact source211c929 on Mac Go1.27.1 (34.375s). Worktree clean before/after.
Original remote tag freshness, workflow identity and latest-run tests remain.
The qualified-CI fixture still rejects missing release artifacts; signatures,
provenance, manifest and offline-bundle checks are not replaced by CI green.
No dependency or workflow-permission change.

## Recoverable evidence and attempts

README.md, initial-status.json, baseline.log, fixed-staged.log and exact-mac.log
retain the original pending snapshots and local failure/pass sequence.
Their hashes were checked against original Mac logs, including restoration of
the line-reader's omitted terminal newline. Additional full logs:

- linux-main-test.log:3158971 chars; Git blob7f889995d401f4977aa90892bb39dcfb9ee7d505.
- r3.log:41567 chars.
- static.log:50297 chars.
- linux-release-dry-run.log:22975 chars.
- linux-upgrade-rollback.log:35828 chars.
- linux-launch-rollback.log:22903 chars.

SHA256 values for the supporting CI logs are recorded in final-status.json;
the main log has its exact Git blob identity and is recoverable from this branch.
No private-key PEM marker found in collected main/R3/rollback logs; this narrow
scan is not a universal secrets/security audit.

All operational attempts remain accounted for: fresh public clone after
scratch pruning; no local Go, so supported Mac used; clean remote source/tree
verification and expected_sha/force:false publication; no retries. Optional
guessed telemetry_history_test.go, monitoring_test.go and scripts/test_gate.sh
reads returned missing-path errors; source/workflow inventory supplied actual
paths. No source or product test failed in those reads. One remote file read
exceeded3s and was retrieved by its returned call ID, not restarted. One README
readback was mistakenly parsed as JSON although the connector had returned
decoded Markdown; direct expected content and independent Git fetch corrected
verification without republishing. A combined-log digest helper exceeded the
OS argument limit before running; hashing files separately succeeded. These
helper/read failures are distinct from expected baseline test failures.
Narrow read-only freshness source review was outside this patch; no BCC change
or new runtime acceptance follows from it.

## Risks and next step

Historical tags without these stricter current gates can now be rejected;
that is not proof their signatures or binaries are defective. Complete-source
CI here is not unmerged Aurora/Core parity or real UI/browser acceptance.
No new auth/session/CSRF/API or actual BCC mounting change is made.

This bounded CLAIM6221370689 can be RELEASED after durable evidence readback.
Future code needs a fresh claim and live HEAD/ownership checks. HQ must choose
the final source (do not silently replace frozen27cca1a), review release scope,
then authorize any main integration/tag and independently verify unsigned
candidate provenance, protected-key signing, final assets and upgrade/rollback.
No production key was read or used by this slice. No real release tag, main
merge, publication, deployment or node mutation occurred. Native Iran–Germany
remains owner-DEFERRED; UI/rendered acceptance remains separate and OPEN.

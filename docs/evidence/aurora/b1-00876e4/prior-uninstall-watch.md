# Existing HQ watch item recurred

Fresh historical-board read found the SAME state.db changed-since-plan failure before this B1 change. This supports tracking recurrence independently of Aurora; it does not establish the exact writer or root fix. Keep plan/hash/verified-backup guards and separate Core ownership. Prior later success explicitly did not close this intermittent watch item. No new issue/duplicate was created; limited issue search returned HQ reports, then actual stored board comments below were read.

## #67/comment6183781297

https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6183781297

## HQ EXECUTION — Stage 25: e2e-uninstall CI failure root-cause evidence (2026-10-15)

**PR #202 DRAFT | HEAD `4741a231ca74fb0c3afa3b62d423f74f74b19a09`.** HEAD static-analysis run **38676579817 SUCCESS**; HEAD main CI **38676579782 QUEUED**, R3.1 **38676579767 IN_PROGRESS**.

**New CI failure on earlier SHA `0d3143d`:** `ci` run **38676220427**, job `e2e-uninstall` **116230312656 FAILED**. Direct job log identifies exact failure: `[uninstall-e2e] FAIL: BCC uninstall` after `baft uninstall: /var/lib/baft-bcc/state.db changed since the plan was made; nothing of it was removed; everything was restored` (journal `run-20261015T032908.736337649Z-13122-0`). BCC service was restarted and wrote/modified state while uninstall plan was active, so uninstall's change-detection protection refused deletion and restored state. This is **not** evidence of incidentclass test failure. Other diagnostic lines (IR service errors, expected downgrade refusal) appear but are not the primary test failure.

**Assessment:** Possible concurrent BCC state writer vs uninstall plan race; protection against deleting modified state is correct fail-closed behavior. Need inspect uninstall e2e harness and uninstall stop/quiesce ordering before changing code; do not disable change detection or force delete. This is observed on older SHA; HEAD result unknown. No code commit this stage; avoid CI queue growth.

**Next:** compare failure to other SHA e2e-uninstall results and inspect test setup/service lifecycle. No merge/release/live changes.

## #67/comment6183792979

https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6183792979

## HQ EXECUTION — Stage 26: BCC uninstall race narrowed to e2e scenario (2026-10-15)

**HEAD `4741a231ca74fb0c3afa3b62d423f74f74b19a09`:** static-analysis SUCCESS; ci QUEUED; R3.1 IN_PROGRESS at check.

**Investigated** `tests/e2e/uninstall.sh` and failure logs for prior `e2e-uninstall` job **116230312656**. The test first runs normal uninstall/restore/upgrade/reinstall flows while keeping `baft-bcc.service` active; near the end, it runs `UN --bcc --delete-bcc-state --yes` and expects verified emergency backup + removal. The failing output was specifically `state.db changed since the plan was made; nothing of it was removed; everything was restored`. The harness defines BCC service with `Restart=on-failure`, creates a node and enrolls an agent. Thus the BCC database can be modified during the final uninstall plan; this is a plausible service/write race, not yet proven at instruction-level.

**Safety requirement:** retain fail-closed changed-since-plan protection and verified emergency backup. Next inspect uninstall implementation to establish exact order of service stop, backup, plan fingerprint and delete, then add deterministic regression rather than weakening safeguards. No new commits, no merge/release/production changes.

## #67/comment6183803327

https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6183803327

## HQ EXECUTION — Stage 27: uninstall order verified, failure window identified (2026-10-15)

**Read source** `internal/uninstall/apply.go` and `tests/e2e/uninstall.sh` on PR #202 branch. `apply.go` explicitly orders BCC stop first when `p.Backup != nil`, then `emergencyBackup`, then `resumeForward` stops remaining services and `moveOut` compares each file's current SHA against **plan-time SHA**. `stopService` executes `systemctl stop`, marks stopped, disables if needed. The observed `state.db changed since the plan was made` is thus compatible with BCC state changing **between plan hash collection and stop/backup**, not necessarily a write after service stop. This corrects prior less-specific race hypothesis.

**Safety-critical distinction:** backup may reflect the newer durable DB while `moveOut` still rejects the stale plan-time hash. Never bypass this guard without a deliberate design for revalidation and backup-to-current-file correspondence; must preserve user changes and auditability. No evidence yet that systemctl stop failed.

**Proposed targeted fix approach (not implemented):** after stopping BCC and obtaining verified backup, reconcile the selected BCC state file against the backup manifest and rebase its planned fingerprint only if current file exactly matches the verified backup, with explicit tests for legitimate pre-stop writes vs malicious/concurrent post-backup changes. Review journal/recovery semantics before implementation. No commit, merge, release or live changes.

## #67/comment6183844343

https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6183844343

## HQ EXECUTION — Stage 32: critical HEAD e2e-uninstall PASS (2026-10-15)

PR #202 DRAFT exact HEAD `4741a231ca74fb0c3afa3b62d423f74f74b19a09`, CI run **38676579782**: **`e2e-uninstall` job 116231465349 completed SUCCESS**, so earlier SHA's `state.db changed since the plan was made` failure did **not** reproduce in this HEAD run. **12/15 jobs completed SUCCESS**, three still IN_PROGRESS: `test` 116231465313, `e2e-launch1` 116231465402, `e2e-ssh-bootstrap` 116231465431. Static-analysis SUCCESS, R3.1 IN_PROGRESS.

**Caveat:** single success does not eliminate an intermittent plan-to-stop race; keep fail-closed hash guard, track recurrence, and avoid unnecessary changes without deterministic reproducer. No new commit, merge, release or production changes.

Current source00876e4 and native scripts/backend inputs unchanged. ONE bounded failed-jobs-only retry remains diagnostic; even success will not close the recurring Core watch item or any field gate.

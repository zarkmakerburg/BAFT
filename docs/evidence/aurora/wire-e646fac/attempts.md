# Attempts and provenance

1. Fresh tools/worktree/PR200/203/204/#67/199 and previous claim release were inspected;
   issue199 assignment confirmed before separate branch/code. No overlapping newer claim.
2. No local Go/gofmt or supported browser available. No install/server/control fallback tried.
   Optional guessed monitoring.go/glob reads returned missing-path errors; corrected store.go.
   Later guessed bcc-ui.yml read failed; corrected actual bcc-v3-ui-tests.yml.
3. First development-tree npm ci37 locked dev packages/scripts disabled succeeded.
   npm test and classic Node test run observed30+12+14 PASS; original development stdout
   was not separately archived as a file. It is not presented as exact published-SHA evidence.
4. Eight files staged via patch; git diff --cached --check PASS; local tree dbef5d0 matched
   create_tree result. Remote branch absent. Fresh board/head matched parent de446b5.
   Commit e646fac created with parent de446b5; new branch began at exact parent;
   update_ref expected_sha=de446b5 force=false succeeded, preserving all history.
5. Draft205 opened against Aurora, not main. Fetched detached exact head/tree verified.
   Fresh npm ci then strict/build/size30+12+14 PASS/0 FAIL/0 SKIP retained in exact-local.log.
   Local grouped logger used pipefail but no errexit: conclusions also checked against each
   full test/build output and zero failures, not its final exit code alone. Clean git status
   and diff checks verified separately. No local Go compile/test/gofmt success claimed.
6. UI job116433804135 on Node22.23.3 SUCCESS; complete decoded log retained, including
   npm/VM experimental/action Node deprecation warnings. This is Node DOM execution,
   not a browser rendering/screenshot or visual accessibility/performance result.
7. Actual UI merge checkout b22ed64 fetched and tree/parents independently verified:
   tree equals source dbef5d0; no new source code introduced by its synthetic merge.
8. Source CI main38738495678 still running at this snapshot. Static38738495204 SUCCESS.
   No retry or failed source-test attempt observed so far; producer acceptance remains pending.
   No submitted review/inline threads observed. Later failures must be retained before fixes.
9. No production/servers/DNS/secrets/main merge/tag/release/transport/shaping/pool/exit
   switch/restart-resume/standby change. No cross-lane incident/installer/persistence edit.

This record qualifies only this bounded read-contract test slice, not full #199 convergence.
